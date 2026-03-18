package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/enclavr/server/internal/services"
	"github.com/enclavr/server/pkg/errors"
	"github.com/enclavr/server/pkg/logger"
	"github.com/google/uuid"
)

type RateLimiterMiddleware struct {
	limiter         *services.SlidingRateLimiter
	fallbackLimiter *IPRateLimiter
	identifierFunc  func(*http.Request) string
	limit           int
	window          time.Duration
	prefix          string
}

type RateLimiterOption func(*RateLimiterMiddleware)

func WithRateLimiterIdentifier(fn func(*http.Request) string) RateLimiterOption {
	return func(m *RateLimiterMiddleware) {
		m.identifierFunc = fn
	}
}

func WithRateLimiterPrefix(prefix string) RateLimiterOption {
	return func(m *RateLimiterMiddleware) {
		m.prefix = prefix
	}
}

func NewRateLimiterMiddleware(limiter *services.SlidingRateLimiter, limit int, window time.Duration, opts ...RateLimiterOption) *RateLimiterMiddleware {
	m := &RateLimiterMiddleware{
		limiter:        limiter,
		limit:          limit,
		window:         window,
		identifierFunc: getClientIP,
		prefix:         "global",
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

func (m *RateLimiterMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := m.identifierFunc(r)
		if key == "" {
			key = getClientIP(r)
		}

		if m.prefix != "" {
			key = m.prefix + ":" + key
		}

		ctx := r.Context()
		result, err := m.limiter.Allow(ctx, key)
		if err != nil {
			logger.WithContext(ctx).Warn("Rate limiter error, using fallback", map[string]interface{}{
				"error": err.Error(),
				"key":   key,
			})

			if m.fallbackLimiter != nil && m.fallbackLimiter.Allow(key) {
				next.ServeHTTP(w, r)
				return
			}

			m.writeRateLimitResponse(w, r, 0, 0)
			return
		}

		if !result.Allowed {
			w.Header().Set("X-RateLimit-Limit", string(rune(m.limit)))
			w.Header().Set("X-RateLimit-Remaining", string(rune(result.Remaining)))
			if result.RetryAfter > 0 {
				w.Header().Set("X-RateLimit-Reset", string(rune(int(result.RetryAfter.Seconds()))))
			}
			m.writeRateLimitResponse(w, r, result.Remaining, int(result.RetryAfter.Seconds()))
			return
		}

		w.Header().Set("X-RateLimit-Limit", string(rune(m.limit)))
		w.Header().Set("X-RateLimit-Remaining", string(rune(result.Remaining)))

		next.ServeHTTP(w, r)
	})
}

func (m *RateLimiterMiddleware) writeRateLimitResponse(w http.ResponseWriter, r *http.Request, remaining, retryAfter int) {
	err := errors.RateLimitExceeded("Too many requests")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-RateLimit-Remaining", "0")
	if retryAfter > 0 {
		w.Header().Set("Retry-After", string(rune(retryAfter)))
	}
	WriteAPIError(w, r, err)
}

type CircuitBreakerMiddleware struct {
	breaker *services.CircuitBreaker
	service string
	next    http.Handler
}

func NewCircuitBreakerMiddleware(breaker *services.CircuitBreaker, service string) *CircuitBreakerMiddleware {
	return &CircuitBreakerMiddleware{
		breaker: breaker,
		service: service,
	}
}

func (m *CircuitBreakerMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !m.breaker.IsAvailable() {
		logger.WithContext(r.Context()).Warn("Circuit breaker open, service unavailable", map[string]interface{}{
			"service": m.service,
			"state":   m.breaker.State().String(),
		})

		err := errors.CircuitOpen(m.service)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Circuit-State", m.breaker.State().String())
		WriteAPIError(w, r, err)
		return
	}

	m.next.ServeHTTP(w, r)
}

func WrapWithCircuitBreaker(next http.Handler, breaker *services.CircuitBreaker, service string) http.Handler {
	mb := NewCircuitBreakerMiddleware(breaker, service)
	mb.next = next
	return mb
}

type RequestDeduplicator struct {
	cache     *services.CacheService
	window    time.Duration
	separator string
}

func NewRequestDeduplicator(cache *services.CacheService, window time.Duration) *RequestDeduplicator {
	return &RequestDeduplicator{
		cache:     cache,
		window:    window,
		separator: ":",
	}
}

func (d *RequestDeduplicator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodPut {
			next.ServeHTTP(w, r)
			return
		}

		key := d.generateKey(r)
		if key == "" {
			next.ServeHTTP(w, r)
			return
		}

		ctx := r.Context()
		exists, err := d.cache.Exists(ctx, key)
		if err == nil && exists {
			w.Header().Set("X-Request-Duplicate", "true")
			w.WriteHeader(http.StatusConflict)
			return
		}

		if err == nil {
			_ = d.cache.Set(ctx, key, "1", d.window)
		}

		w.Header().Set("X-Request-Duplicate", "false")
		next.ServeHTTP(w, r)
	})
}

func (d *RequestDeduplicator) generateKey(r *http.Request) string {
	var sb strings.Builder
	sb.WriteString(r.Method)
	sb.WriteString(d.separator)
	sb.WriteString(r.URL.Path)

	if r.URL.RawQuery != "" {
		sb.WriteString(d.separator)
		sb.WriteString(r.URL.RawQuery)
	}

	userID := GetUserID(r)
	if userID != uuid.Nil {
		sb.WriteString(d.separator)
		sb.WriteString(userID.String())
	}

	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		hash := hashStrings(authHeader)
		sb.WriteString(d.separator)
		sb.WriteString(hash)
	}

	return sb.String()
}

func hashStrings(s string) string {
	h := uint32(2166136261)
	for _, c := range s {
		h ^= uint32(c)
		h *= 16777619
	}
	return string(rune(h))
}

type RequestDeduplicatorWithBody struct {
	cache     *services.CacheService
	window    time.Duration
	separator string
}

func NewRequestDeduplicatorWithBody(cache *services.CacheService, window time.Duration) *RequestDeduplicatorWithBody {
	return &RequestDeduplicatorWithBody{
		cache:     cache,
		window:    window,
		separator: ":",
	}
}

func (d *RequestDeduplicatorWithBody) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch {
			next.ServeHTTP(w, r)
			return
		}

		bodyKey := r.Header.Get("X-Request-Body-Hash")
		if bodyKey == "" {
			next.ServeHTTP(w, r)
			return
		}

		key := d.generateKeyWithBody(r, bodyKey)
		ctx := r.Context()

		exists, err := d.cache.Exists(ctx, key)
		if err == nil && exists {
			w.Header().Set("X-Request-Duplicate", "true")
			w.WriteHeader(http.StatusConflict)
			return
		}

		if err == nil {
			d.cache.Set(ctx, key, "1", d.window)
		}

		w.Header().Set("X-Request-Duplicate", "false")
		next.ServeHTTP(w, r)
	})
}

func (d *RequestDeduplicatorWithBody) generateKeyWithBody(r *http.Request, bodyHash string) string {
	var sb strings.Builder
	sb.WriteString(r.Method)
	sb.WriteString(d.separator)
	sb.WriteString(r.URL.Path)
	sb.WriteString(d.separator)
	sb.WriteString(bodyHash)

	userID := GetUserID(r)
	if userID != uuid.Nil {
		sb.WriteString(d.separator)
		sb.WriteString(userID.String())
	}

	return sb.String()
}
