package middleware

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/enclavr/server/pkg/errors"
	"github.com/getsentry/sentry-go"
)

type ErrorResponse struct {
	Error   string   `json:"error"`
	Code    string   `json:"code"`
	Message string   `json:"message,omitempty"`
	Details []string `json:"details,omitempty"`
}

func WriteError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	jsonErr := json.NewEncoder(w).Encode(ErrorResponse{
		Error:   http.StatusText(code),
		Code:    string(errors.ErrCodeBadRequest),
		Message: message,
	})
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

func WriteAPIError(w http.ResponseWriter, err *errors.Error) {
	status := errors.ToHTTPStatus(err.Code)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	jsonErr := json.NewEncoder(w).Encode(ErrorResponse{
		Error:   string(err.Code),
		Code:    string(err.Code),
		Message: err.Message,
		Details: err.Details,
	})
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

				log.Printf("[PANIC] %s | Path: %s | Error: %s\n%s", r.Method, r.URL.Path, errMsg, debug.Stack())

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
					Error:   string(errors.ErrCodeInternal),
					Code:    string(errors.ErrCodeInternal),
					Message: "An unexpected error occurred",
				}); encErr != nil {
					log.Printf("Error encoding error response: %v", encErr)
				}
			}
		}()
		next.ServeHTTP(w, r)
	})
}
