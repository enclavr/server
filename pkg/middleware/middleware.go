package middleware

import (
	"compress/gzip"
	"context"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/enclavr/server/internal/auth"
	"github.com/getsentry/sentry-go"
	"github.com/google/uuid"
)

type ContextKey string

const (
	UserIDKey    ContextKey = "user_id"
	UsernameKey  ContextKey = "username"
	IsAdminKey   ContextKey = "is_admin"
	RequestIDKey ContextKey = "request_id"
)

func JWTAuth(authService *auth.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Authorization header required", http.StatusUnauthorized)
				return
			}

			tokenString := strings.TrimPrefix(authHeader, "Bearer ")
			if tokenString == authHeader {
				http.Error(w, "Invalid authorization format", http.StatusUnauthorized)
				return
			}

			claims, err := authService.ValidateToken(tokenString)
			if err != nil {
				if strings.Contains(err.Error(), "token is expired") {
					http.Error(w, "Token expired", http.StatusUnauthorized)
					return
				}
				http.Error(w, "Invalid token", http.StatusUnauthorized)
				return
			}

			ctx := r.Context()
			ctx = context.WithValue(ctx, UserIDKey, claims.UserID)
			ctx = context.WithValue(ctx, UsernameKey, claims.Username)
			ctx = context.WithValue(ctx, IsAdminKey, claims.IsAdmin)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireAuth(authService *auth.AuthService, fn func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	middleware := JWTAuth(authService)
	return middleware(http.HandlerFunc(fn)).ServeHTTP
}

type headerTracker struct {
	http.ResponseWriter
	written bool
	once    sync.Once
}

func (h *headerTracker) WriteHeader(code int) {
	h.once.Do(func() {
		h.ResponseWriter.WriteHeader(code)
		h.written = true
	})
}

func RequestTimeout(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			ht := &headerTracker{ResponseWriter: w, written: false}
			done := make(chan struct{})

			go func() {
				next.ServeHTTP(ht, r.WithContext(ctx))
				close(done)
			}()

			select {
			case <-done:
				return
			case <-ctx.Done():
				ht.WriteHeader(http.StatusRequestTimeout)
			}
		})
	}
}

func RequestID() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" {
				requestID = uuid.New().String()
			}
			w.Header().Set("X-Request-ID", requestID)
			ctx := context.WithValue(r.Context(), RequestIDKey, requestID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetUserID(r *http.Request) uuid.UUID {
	if userID, ok := r.Context().Value(UserIDKey).(uuid.UUID); ok {
		return userID
	}
	return uuid.Nil
}

func GetUsername(r *http.Request) string {
	if username, ok := r.Context().Value(UsernameKey).(string); ok {
		return username
	}
	return ""
}

func GetIsAdmin(r *http.Request) bool {
	if isAdmin, ok := r.Context().Value(IsAdminKey).(bool); ok {
		return isAdmin
	}
	return false
}

func GetRequestID(r *http.Request) string {
	if requestID, ok := r.Context().Value(RequestIDKey).(string); ok {
		return requestID
	}
	return ""
}

type gzipResponseWriter struct {
	http.ResponseWriter
	gw *gzip.Writer
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	return g.gw.Write(b)
}

func GzipCompression() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
				next.ServeHTTP(w, r)
				return
			}

			gw := gzip.NewWriter(w)
			defer func() {
				if err := gw.Close(); err != nil {
					log.Printf("error closing gzip writer: %v", err)
				}
			}()

			w.Header().Set("Content-Encoding", "gzip")
			next.ServeHTTP(&gzipResponseWriter{ResponseWriter: w, gw: gw}, r)
		})
	}
}

func SentryRecovery() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					sentry.CaptureException(err.(error))
					sentry.Flush(2 * time.Second)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
