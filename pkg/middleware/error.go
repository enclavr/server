package middleware

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/getsentry/sentry-go"
)

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
	Code    int    `json:"code,omitempty"`
}

func WriteError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	jsonErr := json.NewEncoder(w).Encode(ErrorResponse{
		Error:   http.StatusText(code),
		Message: message,
		Code:    code,
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
				fmt.Fprintf(w, `{"error": "Internal Server Error", "message": "An unexpected error occurred"}`)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
