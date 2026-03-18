package middleware

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/enclavr/server/pkg/logger"
	"github.com/google/uuid"
)

type LoggingConfig struct {
	LogRequests  bool
	LogHeaders   bool
	ExcludePaths []string
	LogResponse  bool
	LogBodySize  int
}

var defaultLoggingConfig = LoggingConfig{
	LogRequests:  true,
	LogHeaders:   false,
	ExcludePaths: []string{"/health", "/ready"},
	LogResponse:  false,
	LogBodySize:  1024,
}

func RequestLogging(config *LoggingConfig) func(http.Handler) http.Handler {
	if config == nil {
		config = &defaultLoggingConfig
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			skipLog := false
			for _, path := range config.ExcludePaths {
				if r.URL.Path == path {
					skipLog = true
					break
				}
			}

			ctx := r.Context()

			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" {
				requestID = uuid.New().String()
			}
			ctx = context.WithValue(ctx, logger.RequestIDKey, requestID)

			userID := GetUserID(r)
			if userID != uuid.Nil {
				ctx = context.WithValue(ctx, logger.UserIDKey, userID)
			}

			w.Header().Set("X-Request-ID", requestID)

			wrap := &statusWriter{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(wrap, r.WithContext(ctx))

			duration := time.Since(start)

			if !skipLog && config.LogRequests {
				clientIP := r.RemoteAddr
				if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
					clientIP = forwarded
				}

				logger.RequestLog(
					ctx,
					r.Method,
					r.URL.Path,
					clientIP,
					wrap.statusCode,
					duration,
					&userID,
				)
			}
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	statusCode int
	bytes      int
}

func (w *statusWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

func (w *statusWriter) Hijacked() bool {
	if hijacker, ok := w.ResponseWriter.(http.Hijacker); ok {
		_, _, _ = hijacker.Hijack()
		return true
	}
	return false
}

func EnhancedRequestLogging(config *LoggingConfig) func(http.Handler) http.Handler {
	if config == nil {
		config = &defaultLoggingConfig
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			skipLog := false
			for _, path := range config.ExcludePaths {
				if r.URL.Path == path {
					skipLog = true
					break
				}
			}

			ctx := r.Context()

			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" {
				requestID = uuid.New().String()
			}
			ctx = context.WithValue(ctx, logger.RequestIDKey, requestID)

			userID := GetUserID(r)
			if userID != uuid.Nil {
				ctx = context.WithValue(ctx, logger.UserIDKey, userID)
			}

			w.Header().Set("X-Request-ID", requestID)

			wrap := &statusWriter{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(wrap, r.WithContext(ctx))

			duration := time.Since(start)

			if !skipLog && config.LogRequests {
				clientIP := r.RemoteAddr
				if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
					clientIP = forwarded
				}

				referer := r.Header.Get("Referer")
				userAgent := r.Header.Get("User-Agent")
				contentLength := r.Header.Get("Content-Length")

				fields := map[string]interface{}{
					"method":        r.Method,
					"path":          r.URL.Path,
					"query":         r.URL.RawQuery,
					"ip":            clientIP,
					"status":        wrap.statusCode,
					"duration_ms":   duration.Milliseconds(),
					"request_id":    requestID,
					"response_size": wrap.bytes,
				}

				if userID != uuid.Nil {
					fields["user_id"] = userID.String()
				}
				if referer != "" {
					fields["referer"] = referer
				}
				if userAgent != "" {
					fields["user_agent"] = userAgent
				}
				if contentLength != "" {
					if cl, err := strconv.Atoi(contentLength); err == nil {
						fields["request_size"] = cl
					}
				}

				logger.RequestLog(
					ctx,
					r.Method,
					r.URL.Path,
					clientIP,
					wrap.statusCode,
					duration,
					&userID,
				)
			}
		})
	}
}

func LogError(ctx context.Context, err error, msg string) {
	log.Printf("%s | Error: %v", msg, err)
}

func LogPanic(ctx context.Context, recovered interface{}, stack []byte) {
	var errMsg string
	switch e := recovered.(type) {
	case error:
		errMsg = e.Error()
	case string:
		errMsg = e
	default:
		errMsg = "unknown panic"
	}

	log.Printf("[PANIC] %s | Stack: %s", errMsg, stack)
}
