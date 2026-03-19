package middleware

import (
	"compress/gzip"
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/enclavr/server/internal/auth"
	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/getsentry/sentry-go"
	"github.com/google/uuid"
)

type ContextKey string

const (
	UserIDKey     ContextKey = "user_id"
	UsernameKey   ContextKey = "username"
	IsAdminKey    ContextKey = "is_admin"
	RequestIDKey  ContextKey = "request_id"
	BookmarkIDKey ContextKey = "bookmark_id"
	ReminderIDKey ContextKey = "reminder_id"
	SessionIDKey  ContextKey = "session_id"
)

type IPRateLimiter struct {
	requests map[string][]time.Time
	mu       sync.Mutex
	limit    int
	window   time.Duration
}

func NewIPRateLimiter(limit int, window time.Duration) *IPRateLimiter {
	rl := &IPRateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
	go rl.cleanup()
	return rl
}

func (rl *IPRateLimiter) cleanup() {
	ticker := time.NewTicker(rl.window)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for key, times := range rl.requests {
			var valid []time.Time
			for _, t := range times {
				if now.Sub(t) < rl.window {
					valid = append(valid, t)
				}
			}
			if len(valid) == 0 {
				delete(rl.requests, key)
			} else {
				rl.requests[key] = valid
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *IPRateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	times := rl.requests[key]

	var valid []time.Time
	for _, t := range times {
		if now.Sub(t) < rl.window {
			valid = append(valid, t)
		}
	}

	if len(valid) >= rl.limit {
		rl.requests[key] = valid
		return false
	}

	rl.requests[key] = append(valid, now)
	return true
}

var globalIPLimiter = NewIPRateLimiter(100, time.Minute)
var authIPLimiter = NewIPRateLimiter(5, time.Minute)

func IPRateLimit(rl *IPRateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := getClientIP(r)
			if !rl.Allow(ip) {
				http.Error(w, "Too many requests", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func GlobalIPRateLimit() func(http.Handler) http.Handler {
	return IPRateLimit(globalIPLimiter)
}

func AuthIPRateLimit() func(http.Handler) http.Handler {
	return IPRateLimit(authIPLimiter)
}

func getClientIP(r *http.Request) string {
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		ips := strings.Split(forwarded, ",")
		return strings.TrimSpace(ips[0])
	}
	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" {
		return realIP
	}
	return r.RemoteAddr
}

func JWTAuth(authService *auth.AuthService) func(http.Handler) http.Handler {
	return JWTAuthWithSession(authService, nil)
}

func JWTAuthWithSession(authService *auth.AuthService, db *database.Database) func(http.Handler) http.Handler {
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

			if db != nil && claims.SessionID != uuid.Nil {
				var session models.Session
				err := db.Where("id = ? AND user_id = ? AND expires_at > ?", claims.SessionID, claims.UserID, time.Now()).First(&session).Error
				if err != nil {
					http.Error(w, "Session expired or invalid", http.StatusUnauthorized)
					return
				}
			}

			ctx := r.Context()
			ctx = context.WithValue(ctx, UserIDKey, claims.UserID)
			ctx = context.WithValue(ctx, UsernameKey, claims.Username)
			ctx = context.WithValue(ctx, IsAdminKey, claims.IsAdmin)
			if claims.SessionID != uuid.Nil {
				ctx = context.WithValue(ctx, SessionIDKey, claims.SessionID)
			}

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

func GetSessionID(r *http.Request) uuid.UUID {
	if sessionID, ok := r.Context().Value(SessionIDKey).(uuid.UUID); ok {
		return sessionID
	}
	return uuid.Nil
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
					var errValue error
					switch e := err.(type) {
					case error:
						errValue = e
					case string:
						errValue = fmt.Errorf("%s", e)
					default:
						errValue = fmt.Errorf("%v", e)
					}
					sentry.CaptureException(errValue)
					sentry.Flush(2 * time.Second)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
