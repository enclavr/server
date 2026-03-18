package middleware

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/enclavr/server/pkg/errors"
	"github.com/enclavr/server/pkg/logger"
	"github.com/getsentry/sentry-go"
)

type ErrorResponse struct {
	Error     string            `json:"error"`
	Code      string            `json:"code"`
	Message   string            `json:"message,omitempty"`
	Details   []string          `json:"details,omitempty"`
	RequestID string            `json:"request_id,omitempty"`
	Timestamp string            `json:"timestamp"`
	Extra     map[string]string `json:"extra,omitempty"`
}

func WriteError(w http.ResponseWriter, r *http.Request, code int, message string) {
	requestID := GetRequestID(r)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	resp := ErrorResponse{
		Error:     http.StatusText(code),
		Code:      string(errors.ErrCodeBadRequest),
		Message:   message,
		RequestID: requestID,
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
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	jsonErr := json.NewEncoder(w).Encode(resp)
	if jsonErr != nil {
		log.Printf("Error encoding error response: %v", jsonErr)
	}
}

func WriteDetailedError(w http.ResponseWriter, r *http.Request, code int, codeStr errors.Code, message string, details []string, extra map[string]string) {
	requestID := GetRequestID(r)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	logger.WithContext(r.Context()).Warn("HTTP error response", map[string]interface{}{
		"status":     code,
		"code":       codeStr,
		"message":    message,
		"request_id": requestID,
		"path":       r.URL.Path,
		"method":     r.Method,
	})

	resp := ErrorResponse{
		Error:     http.StatusText(code),
		Code:      string(codeStr),
		Message:   message,
		Details:   details,
		RequestID: requestID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Extra:     extra,
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
					errMsg = fmt.Sprintf("%v", err)
				}

				requestID := GetRequestID(r)
				log.Printf("[PANIC] %s | Path: %s | RequestID: %s | Error: %s\n%s", r.Method, r.URL.Path, requestID, errMsg, debug.Stack())

				logger.WithContext(r.Context()).Error("Panic recovered", map[string]interface{}{
					"method":     r.Method,
					"path":       r.URL.Path,
					"request_id": requestID,
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
					sentryHub.CaptureException(errValue)
					sentryHub.Flush(2 * time.Second)
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				if encErr := json.NewEncoder(w).Encode(ErrorResponse{
					Error:     string(errors.ErrCodeInternal),
					Code:      string(errors.ErrCodeInternal),
					Message:   "An unexpected error occurred",
					RequestID: requestID,
					Timestamp: time.Now().UTC().Format(time.RFC3339),
				}); encErr != nil {
					log.Printf("Error encoding error response: %v", encErr)
				}
			}
		}()
		next.ServeHTTP(w, r)
	})
}
