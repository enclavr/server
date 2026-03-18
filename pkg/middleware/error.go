package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/enclavr/server/pkg/errors"
	"github.com/enclavr/server/pkg/logger"
	"github.com/getsentry/sentry-go"
	"github.com/google/uuid"
)

type ErrorResponse struct {
	Error     string            `json:"error"`
	Code      string            `json:"code"`
	Message   string            `json:"message,omitempty"`
	Details   []string          `json:"details,omitempty"`
	RequestID string            `json:"request_id"`
	ErrorID   string            `json:"error_id,omitempty"`
	Timestamp string            `json:"timestamp"`
	Extra     map[string]string `json:"extra,omitempty"`
	Stack     string            `json:"stack,omitempty"`
}

func WriteError(w http.ResponseWriter, r *http.Request, code int, message string) {
	errorID := uuid.New().String()
	requestID := GetRequestID(r)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	resp := ErrorResponse{
		Error:     http.StatusText(code),
		Code:      string(errors.ErrCodeBadRequest),
		Message:   message,
		RequestID: requestID,
		ErrorID:   errorID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	jsonErr := json.NewEncoder(w).Encode(resp)
	if jsonErr != nil {
		log.Printf("Error encoding error response: %v", jsonErr)
	}
}

func WriteJSON(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
	}
}

func WriteAPIError(w http.ResponseWriter, r *http.Request, err *errors.Error) {
	errorID := uuid.New().String()
	requestID := GetRequestID(r)
	status := errors.ToHTTPStatus(err.Code)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := ErrorResponse{
		Error:     string(err.Code),
		Code:      string(err.Code),
		Message:   err.Message,
		Details:   err.Details,
		RequestID: requestID,
		ErrorID:   errorID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	jsonErr := json.NewEncoder(w).Encode(resp)
	if jsonErr != nil {
		log.Printf("Error encoding error response: %v", jsonErr)
	}
}

func WriteDetailedError(w http.ResponseWriter, r *http.Request, code int, codeStr errors.Code, message string, details []string, extra map[string]string) {
	errorID := uuid.New().String()
	requestID := GetRequestID(r)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	logger.WithContext(r.Context()).Warn("HTTP error response", map[string]interface{}{
		"status":     code,
		"code":       codeStr,
		"message":    message,
		"request_id": requestID,
		"error_id":   errorID,
		"path":       r.URL.Path,
		"method":     r.Method,
	})

	resp := ErrorResponse{
		Error:     http.StatusText(code),
		Code:      string(codeStr),
		Message:   message,
		Details:   details,
		RequestID: requestID,
		ErrorID:   errorID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Extra:     extra,
	}
	jsonErr := json.NewEncoder(w).Encode(resp)
	if jsonErr != nil {
		log.Printf("Error encoding error response: %v", jsonErr)
	}
}

func WriteErrorWithID(w http.ResponseWriter, r *http.Request, code int, message string, errorID string) {
	requestID := GetRequestID(r)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	resp := ErrorResponse{
		Error:     http.StatusText(code),
		Code:      string(errors.ErrCodeInternal),
		Message:   message,
		RequestID: requestID,
		ErrorID:   errorID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	jsonErr := json.NewEncoder(w).Encode(resp)
	if jsonErr != nil {
		log.Printf("Error encoding error response: %v", jsonErr)
	}
}

func WriteValidationErrors(w http.ResponseWriter, r *http.Request, validationErrs errors.ValidationErrors) {
	errorID := uuid.New().String()
	requestID := GetRequestID(r)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnprocessableEntity)

	details := make([]string, len(validationErrs))
	for i, err := range validationErrs {
		details[i] = fmt.Sprintf("%s: %s", err.Field, err.Message)
	}

	logger.WithContext(r.Context()).Warn("Validation error response", map[string]interface{}{
		"status":     http.StatusUnprocessableEntity,
		"code":       errors.ErrCodeValidation,
		"errors":     details,
		"request_id": requestID,
		"error_id":   errorID,
		"path":       r.URL.Path,
		"method":     r.Method,
	})

	resp := ErrorResponse{
		Error:     string(errors.ErrCodeValidation),
		Code:      string(errors.ErrCodeValidation),
		Message:   "Validation failed",
		Details:   details,
		RequestID: requestID,
		ErrorID:   errorID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	jsonErr := json.NewEncoder(w).Encode(resp)
	if jsonErr != nil {
		log.Printf("Error encoding error response: %v", jsonErr)
	}
}

func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				var errMsg string
				switch e := err.(type) {
				case error:
					errMsg = e.Error()
				case string:
					errMsg = e
				default:
					errMsg = fmt.Sprintf("%v", e)
				}

				errorID := uuid.New().String()
				requestID := GetRequestID(r)

				log.Printf("[PANIC] %s | Path: %s | RequestID: %s | ErrorID: %s | Error: %s\n%s",
					r.Method, r.URL.Path, requestID, errorID, errMsg, debug.Stack())

				logger.WithContext(r.Context()).Error("Panic recovered", map[string]interface{}{
					"method":     r.Method,
					"path":       r.URL.Path,
					"request_id": requestID,
					"error_id":   errorID,
					"error":      errMsg,
				})

				if sentryHub := sentry.GetHubFromContext(r.Context()); sentryHub != nil {
					var errValue error
					switch e := err.(type) {
					case error:
						errValue = e
					default:
						errValue = fmt.Errorf("%v", err)
					}
					sentryHub.WithScope(func(scope *sentry.Scope) {
						scope.SetExtra("request_id", requestID)
						scope.SetExtra("error_id", errorID)
						scope.SetExtra("path", r.URL.Path)
						scope.SetExtra("method", r.Method)
						sentryHub.CaptureException(errValue)
					})
					sentryHub.Flush(2 * time.Second)
				}

				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Error-ID", errorID)
				w.WriteHeader(http.StatusInternalServerError)
				if encErr := json.NewEncoder(w).Encode(ErrorResponse{
					Error:     string(errors.ErrCodeInternal),
					Code:      string(errors.ErrCodeInternal),
					Message:   "An unexpected error occurred",
					RequestID: requestID,
					ErrorID:   errorID,
					Timestamp: time.Now().UTC().Format(time.RFC3339),
				}); encErr != nil {
					log.Printf("Error encoding error response: %v", encErr)
				}
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type correlationIDContextKey string

const correlationIDKey correlationIDContextKey = "correlation_id"

type CorrelationIDOptions struct {
	HeaderName string
	Length     int
}

func CorrelationID(opts *CorrelationIDOptions) func(http.Handler) http.Handler {
	headerName := "X-Correlation-ID"
	length := 32

	if opts != nil {
		if opts.HeaderName != "" {
			headerName = opts.HeaderName
		}
		if opts.Length > 0 {
			length = opts.Length
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			correlationID := r.Header.Get(headerName)
			if correlationID == "" || len(correlationID) > length {
				correlationID = uuid.New().String()
			}

			correlationID = strings.TrimSpace(correlationID)

			w.Header().Set(headerName, correlationID)
			ctx := r.Context()
			ctx = context.WithValue(ctx, correlationIDKey, correlationID)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetCorrelationID(r *http.Request) string {
	if correlationID, ok := r.Context().Value(correlationIDKey).(string); ok {
		return correlationID
	}
	return ""
}
