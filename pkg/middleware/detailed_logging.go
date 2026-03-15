package middleware

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/enclavr/server/pkg/logger"
	"github.com/google/uuid"
)

type DetailedLoggingConfig struct {
	LogRequestHeaders  bool
	LogRequestBody     bool
	LogResponseBody    bool
	LogResponseHeaders bool
	MaxBodyLogSize     int64
	ExcludePaths       []string
	ExcludeMethods     []string
}

var defaultDetailedLoggingConfig = DetailedLoggingConfig{
	LogRequestHeaders:  true,
	LogRequestBody:     false,
	LogResponseBody:    false,
	LogResponseHeaders: false,
	MaxBodyLogSize:     4096,
	ExcludePaths:       []string{"/health", "/ready", "/metrics"},
	ExcludeMethods:     []string{"OPTIONS", "HEAD"},
}

func DetailedRequestLogging(config *DetailedLoggingConfig) func(http.Handler) http.Handler {
	if config == nil {
		config = &defaultDetailedLoggingConfig
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
			for _, method := range config.ExcludeMethods {
				if r.Method == method {
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

			var requestBody []byte
			if config.LogRequestBody && r.Body != nil {
				requestBody, _ = io.ReadAll(r.Body)
				r.Body = io.NopCloser(bytes.NewBuffer(requestBody))
			}

			wrap := &detailedStatusWriter{ResponseWriter: w, statusCode: http.StatusOK}
			var responseBody *bytes.Buffer
			if config.LogResponseBody {
				responseBody = &bytes.Buffer{}
				wrap.bodyWriter = responseBody
			}

			next.ServeHTTP(wrap, r.WithContext(ctx))

			duration := time.Since(start)

			if !skipLog {
				clientIP := getClientIP(r)
				userAgent := r.Header.Get("User-Agent")
				referer := r.Header.Get("Referer")

				fields := map[string]interface{}{
					"method":      r.Method,
					"path":        r.URL.Path,
					"query":       r.URL.RawQuery,
					"ip":          clientIP,
					"status":      wrap.statusCode,
					"duration_ms": duration.Milliseconds(),
					"user_agent":  userAgent,
					"referer":     referer,
					"request_id":  requestID,
				}

				if userID != uuid.Nil {
					fields["user_id"] = userID.String()
				}

				if config.LogRequestHeaders && len(r.Header) > 0 {
					headers := make(map[string]string)
					for k, v := range r.Header {
						if isSensitiveHeader(k) {
							headers[k] = "***"
						} else {
							headers[k] = strings.Join(v, ", ")
						}
					}
					fields["request_headers"] = headers
				}

				if config.LogRequestBody && len(requestBody) > 0 {
					bodyStr := string(requestBody)
					if int64(len(bodyStr)) > config.MaxBodyLogSize {
						bodyStr = bodyStr[:config.MaxBodyLogSize] + "..."
					}
					fields["request_body"] = bodyStr
				}

				if config.LogResponseHeaders {
					headers := make(map[string]string)
					for k, v := range w.Header() {
						headers[k] = strings.Join(v, ", ")
					}
					fields["response_headers"] = headers
				}

				if config.LogResponseBody && responseBody != nil && responseBody.Len() > 0 {
					bodyStr := responseBody.String()
					if int64(len(bodyStr)) > config.MaxBodyLogSize {
						bodyStr = bodyStr[:config.MaxBodyLogSize] + "..."
					}
					fields["response_body"] = bodyStr
				}

				if wrap.statusCode >= 500 || (duration.Seconds() > 2) {
					if wrap.statusCode >= 500 {
						fields["error"] = true
					}
					if duration.Seconds() > 2 {
						fields["slow_request"] = true
					}
					logger.WithContext(ctx).Warn("HTTP Request", fields)
				} else {
					logger.WithContext(ctx).Info("HTTP Request", fields)
				}
			}
		})
	}
}

type detailedStatusWriter struct {
	http.ResponseWriter
	statusCode int
	bodyWriter *bytes.Buffer
}

func (w *detailedStatusWriter) WriteHeader(sc int) {
	w.statusCode = sc
	w.ResponseWriter.WriteHeader(sc)
}

func (w *detailedStatusWriter) Write(b []byte) (int, error) {
	if w.bodyWriter != nil {
		w.bodyWriter.Write(b)
	}
	return w.ResponseWriter.Write(b)
}

func (w *detailedStatusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

func isSensitiveHeader(key string) bool {
	lower := strings.ToLower(key)
	sensitive := []string{
		"authorization",
		"cookie",
		"x-api-key",
		"x-auth-token",
		"x-csrf-token",
		"proxy-authorization",
	}
	for _, s := range sensitive {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}

type APIMetrics struct {
	TotalRequests  int64
	TotalErrors    int64
	ActiveRequests int64
	SlowRequests   int64
}

var (
	activeCount int64
	totalCount  int64
	errorCount  int64
	slowCount   int64
)

func GetAPIMetrics() APIMetrics {
	return APIMetrics{
		TotalRequests:  atomic.LoadInt64(&totalCount),
		TotalErrors:    atomic.LoadInt64(&errorCount),
		ActiveRequests: atomic.LoadInt64(&activeCount),
		SlowRequests:   atomic.LoadInt64(&slowCount),
	}
}

func MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&activeCount, 1)
		defer atomic.AddInt64(&activeCount, -1)

		start := time.Now()

		wrap := &statusWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(wrap, r)

		duration := time.Since(start)
		atomic.AddInt64(&totalCount, 1)

		if wrap.statusCode >= 500 {
			atomic.AddInt64(&errorCount, 1)
		}

		if duration.Seconds() > 2 {
			atomic.AddInt64(&slowCount, 1)
		}
	})
}
