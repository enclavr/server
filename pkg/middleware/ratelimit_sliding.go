package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/enclavr/server/internal/services"
	"github.com/google/uuid"
)

type TokenBucketRateLimiter struct {
	cache   *services.CacheService
	rate    int
	burst   int
	window  time.Duration
	enabled bool
}

func NewTokenBucketRateLimiter(cache *services.CacheService, rate, burst int, window time.Duration) *TokenBucketRateLimiter {
	return &TokenBucketRateLimiter{
		cache:   cache,
		rate:    rate,
		burst:   burst,
		window:  window,
		enabled: cache != nil,
	}
}

type tokenBucket struct {
	Tokens    int       `json:"tokens"`
	LastCheck time.Time `json:"last_check"`
}

func (rl *TokenBucketRateLimiter) Allow(key string) bool {
	if !rl.enabled {
		return true
	}

	ctx := context.Background()
	cacheKey := fmt.Sprintf("token_bucket:%s", key)

	var bucket tokenBucket
	err := rl.cache.Get(ctx, cacheKey, &bucket)
	now := time.Now()

	if err != nil || bucket.LastCheck.IsZero() {
		bucket = tokenBucket{
			Tokens:    rl.burst - 1,
			LastCheck: now,
		}
		_ = rl.cache.Set(ctx, cacheKey, bucket, rl.window)
		return true
	}

	elapsed := now.Sub(bucket.LastCheck)
	tokensToAdd := int(elapsed.Seconds()) * rl.rate
	bucket.Tokens = min(bucket.Tokens+tokensToAdd, rl.burst)
	bucket.LastCheck = now

	if bucket.Tokens > 0 {
		bucket.Tokens--
		_ = rl.cache.Set(ctx, cacheKey, bucket, rl.window)
		return true
	}

	_ = rl.cache.Set(ctx, cacheKey, bucket, rl.window)
	return false
}

func (rl *TokenBucketRateLimiter) GetRemaining(key string) int {
	if !rl.enabled {
		return rl.burst
	}

	ctx := context.Background()
	cacheKey := fmt.Sprintf("token_bucket:%s", key)

	var bucket tokenBucket
	err := rl.cache.Get(ctx, cacheKey, &bucket)
	if err != nil {
		return rl.burst
	}

	return bucket.Tokens
}

func (rl *TokenBucketRateLimiter) GetResetTime(key string) time.Time {
	if !rl.enabled {
		return time.Now().Add(time.Duration(float64(rl.burst) / float64(rl.rate) * float64(time.Second)))
	}

	ctx := context.Background()
	cacheKey := fmt.Sprintf("token_bucket:%s", key)

	var bucket tokenBucket
	err := rl.cache.Get(ctx, cacheKey, &bucket)
	if err != nil || bucket.Tokens >= rl.burst {
		return time.Now()
	}

	tokensNeeded := rl.burst - bucket.Tokens
	waitTime := time.Duration(float64(tokensNeeded)/float64(rl.rate)) * time.Second
	return time.Now().Add(waitTime)
}

func (rl *TokenBucketRateLimiter) Reset(key string) {
	if !rl.enabled {
		return
	}

	ctx := context.Background()
	cacheKey := fmt.Sprintf("token_bucket:%s", key)
	_ = rl.cache.Delete(ctx, cacheKey)
}

type TokenBucketMiddleware struct {
	limiter  *TokenBucketRateLimiter
	fallback *RateLimiter
	rate     int
	burst    int
	window   time.Duration
}

func NewTokenBucketMiddleware(cache *services.CacheService, rate, burst int, window time.Duration) *TokenBucketMiddleware {
	var limiter *TokenBucketRateLimiter
	if cache != nil {
		limiter = NewTokenBucketRateLimiter(cache, rate, burst, window)
	}

	return &TokenBucketMiddleware{
		limiter:  limiter,
		fallback: NewRateLimiter(burst, window),
		rate:     rate,
		burst:    burst,
		window:   window,
	}
}

func (m *TokenBucketMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := m.getRateLimitKey(r)

		var allowed bool
		var remaining int
		var resetTime time.Time

		if m.limiter != nil && m.limiter.enabled {
			allowed = m.limiter.Allow(key)
			remaining = m.limiter.GetRemaining(key)
			resetTime = m.limiter.GetResetTime(key)
		} else {
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
			remaining = m.burst - 1
			if !allowed {
				remaining = 0
			}
			resetTime = time.Now().Add(m.window)
		}

		w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", m.burst))
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", resetTime.Unix()))

		if !allowed {
			retryAfter := int(resetTime.Sub(time.Now()).Seconds())
			if retryAfter > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			}
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (m *TokenBucketMiddleware) getRateLimitKey(r *http.Request) string {
	userID := GetUserID(r)
	if userID != uuid.Nil {
		return fmt.Sprintf("user:%s", userID.String())
	}

	apiKey := r.Header.Get("X-API-Key")
	if apiKey != "" {
		hash := sha256.Sum256([]byte(apiKey))
		return fmt.Sprintf("apikey:%s", hex.EncodeToString(hash[:8]))
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	hash := sha256.Sum256([]byte(host))
	return fmt.Sprintf("ip:%s", hex.EncodeToString(hash[:8]))
}

func (m *TokenBucketMiddleware) Reset(key string) {
	if m.limiter != nil && m.limiter.enabled {
		m.limiter.Reset(key)
	}
}
