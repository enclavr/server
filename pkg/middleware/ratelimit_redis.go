package middleware

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/enclavr/server/internal/services"
	"github.com/google/uuid"
)

type RedisRateLimiter struct {
	cache   *services.CacheService
	limit   int
	window  time.Duration
	enabled bool
}

func NewRedisRateLimiter(cache *services.CacheService, limit int, window time.Duration) *RedisRateLimiter {
	return &RedisRateLimiter{
		cache:   cache,
		limit:   limit,
		window:  window,
		enabled: cache != nil,
	}
}

func (rl *RedisRateLimiter) Allow(key string) bool {
	if !rl.enabled {
		return true
	}

	ctx := context.Background()

	count, err := rl.cache.IncrBy(ctx, fmt.Sprintf("ratelimit:%s", key), 1)
	if err != nil {
		return true
	}

	if count == 1 {
		rl.setExpiration(ctx, key)
	}

	return count <= int64(rl.limit)
}

func (rl *RedisRateLimiter) setExpiration(ctx context.Context, key string) {
	rl.cache.Expire(ctx, fmt.Sprintf("ratelimit:%s", key), rl.window) //nolint:errcheck
}

func (rl *RedisRateLimiter) GetRemaining(key string) int {
	if !rl.enabled {
		return rl.limit
	}

	ttl, err := rl.cache.TTL(context.Background(), fmt.Sprintf("ratelimit:%s", key))
	if err != nil || ttl <= 0 {
		return rl.limit
	}

	count, err := rl.cache.IncrBy(context.Background(), fmt.Sprintf("ratelimit:%s", key), 0)
	if err != nil {
		return rl.limit
	}

	remaining := rl.limit - int(count)
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (rl *RedisRateLimiter) GetResetTime(key string) time.Time {
	if !rl.enabled {
		return time.Now().Add(rl.window)
	}

	ttl, err := rl.cache.TTL(context.Background(), fmt.Sprintf("ratelimit:%s", key))
	if err != nil || ttl <= 0 {
		return time.Now().Add(rl.window)
	}

	return time.Now().Add(ttl)
}

func (rl *RedisRateLimiter) Reset(key string) {
	if !rl.enabled {
		return
	}

	rl.deleteKey(context.Background(), key)
}

func (rl *RedisRateLimiter) deleteKey(ctx context.Context, key string) {
	rl.cache.Delete(ctx, fmt.Sprintf("ratelimit:%s", key))
}

type RedisRateLimiterMiddleware struct {
	limiter  *RedisRateLimiter
	fallback *RateLimiter
}

func NewRedisRateLimiterMiddleware(cache *services.CacheService, limit int, window time.Duration) *RedisRateLimiterMiddleware {
	rl := NewRedisRateLimiter(cache, limit, window)
	fallback := NewRateLimiter(limit, window)

	return &RedisRateLimiterMiddleware{
		limiter:  rl,
		fallback: fallback,
	}
}

func (m *RedisRateLimiterMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := m.getRateLimitKey(r)

		allowed := false
		if m.limiter != nil && m.limiter.enabled {
			allowed = m.limiter.Allow(key)
		} else if m.fallback != nil {
			userID := GetUserID(r)
			if userID == uuid.Nil {
				host, _, err := net.SplitHostPort(r.RemoteAddr)
				if err != nil {
					host = r.RemoteAddr
				}
				hash := sha256.Sum256([]byte(host))
				userID = uuid.UUID(hash[:16])
			}
			allowed = m.fallback.Allow(userID)
		} else {
			allowed = true
		}

		if !allowed {
			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", m.getLimit()))
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", m.getResetTime(key).Unix()))
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		remaining := m.getRemaining(key)
		resetTime := m.getResetTime(key)

		w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", m.getLimit()))
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", resetTime.Unix()))

		next.ServeHTTP(w, r)
	})
}

func (m *RedisRateLimiterMiddleware) getRateLimitKey(r *http.Request) string {
	userID := GetUserID(r)
	if userID != uuid.Nil {
		return fmt.Sprintf("user:%s", userID.String())
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	hash := sha256.Sum256([]byte(host))
	return fmt.Sprintf("ip:%s", hash)
}

func (m *RedisRateLimiterMiddleware) getLimit() int {
	if m.limiter != nil {
		return m.limiter.limit
	}
	return m.fallback.limit
}

func (m *RedisRateLimiterMiddleware) getRemaining(key string) int {
	if m.limiter != nil && m.limiter.enabled {
		return m.limiter.GetRemaining(key)
	}
	return m.getLimit()
}

func (m *RedisRateLimiterMiddleware) getResetTime(key string) time.Time {
	if m.limiter != nil && m.limiter.enabled {
		return m.limiter.GetResetTime(key)
	}
	return time.Now().Add(m.getWindow())
}

func (m *RedisRateLimiterMiddleware) getWindow() time.Duration {
	if m.limiter != nil {
		return m.limiter.window
	}
	return m.fallback.window
}

func (m *RedisRateLimiterMiddleware) Reset(key string) {
	if m.limiter != nil && m.limiter.enabled {
		m.limiter.Reset(key)
	}
}
