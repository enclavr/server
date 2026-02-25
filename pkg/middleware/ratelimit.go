package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

type RateLimiter struct {
	requests map[uuid.UUID][]time.Time
	mu       sync.RWMutex
	limit    int
	window   time.Duration
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		requests: make(map[uuid.UUID][]time.Time),
		limit:    limit,
		window:   window,
	}
	go rl.cleanup()
	return rl
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for userID, times := range rl.requests {
			var valid []time.Time
			for _, t := range times {
				if now.Sub(t) < rl.window {
					valid = append(valid, t)
				}
			}
			if len(valid) == 0 {
				delete(rl.requests, userID)
			} else {
				rl.requests[userID] = valid
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *RateLimiter) Allow(userID uuid.UUID) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-rl.window)

	var valid []time.Time
	if times, ok := rl.requests[userID]; ok {
		for _, t := range times {
			if t.After(windowStart) {
				valid = append(valid, t)
			}
		}
	}

	if len(valid) >= rl.limit {
		rl.requests[userID] = valid
		return false
	}

	rl.requests[userID] = append(valid, now)
	return true
}

var (
	globalLimiter *RateLimiter
	once          sync.Once
)

func InitRateLimiter(requestsPerMinute int) {
	once.Do(func() {
		globalLimiter = NewRateLimiter(requestsPerMinute, time.Minute)
	})
}

func RateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if globalLimiter == nil {
			next.ServeHTTP(w, r)
			return
		}

		userID := GetUserID(r)
		if userID == uuid.Nil {
			ip := r.RemoteAddr
			ipStr := []byte(ip[len(ip)-15:])
			newUserID, _ := uuid.FromBytes(ipStr)
			userID = newUserID
		}

		if !globalLimiter.Allow(userID) {
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}
