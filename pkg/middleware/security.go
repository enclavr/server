package middleware

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CORSMiddleware struct {
	allowedOrigins []string
	allowMethods   []string
	allowHeaders   []string
	exposeHeaders  []string
	maxAge         int
}

var (
	defaultAllowMethods  = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"}
	defaultAllowHeaders  = []string{"Accept", "Authorization", "Content-Type", "X-Requested-With"}
	defaultExposeHeaders = []string{"Content-Length", "Content-Type"}
)

func NewCORSMiddleware(allowedOrigins []string) *CORSMiddleware {
	if len(allowedOrigins) == 0 {
		log.Println("WARNING: No ALLOWED_ORIGINS configured. Defaulting to localhost for development.")
		allowedOrigins = []string{"http://localhost:3000", "http://localhost:8080"}
	}
	return &CORSMiddleware{
		allowedOrigins: allowedOrigins,
		allowMethods:   defaultAllowMethods,
		allowHeaders:   defaultAllowHeaders,
		exposeHeaders:  defaultExposeHeaders,
		maxAge:         86400,
	}
}

func (c *CORSMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		allowed := false
		actualOrigin := origin
		for _, o := range c.allowedOrigins {
			if o == "*" {
				allowed = true
				actualOrigin = "*"
				break
			}
			if strings.EqualFold(o, origin) {
				allowed = true
				break
			}
		}

		if r.Method == "OPTIONS" {
			w.Header().Set("Access-Control-Allow-Methods", strings.Join(c.allowMethods, ", "))
			w.Header().Set("Access-Control-Allow-Headers", strings.Join(c.allowHeaders, ", "))
			w.Header().Set("Access-Control-Max-Age", strconv.Itoa(c.maxAge))
			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", actualOrigin)
				w.Header().Set("Vary", "Origin")
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", actualOrigin)
			if actualOrigin != "*" {
				w.Header().Set("Vary", "Origin")
			}
		}

		w.Header().Set("Access-Control-Expose-Headers", strings.Join(c.exposeHeaders, ", "))

		next.ServeHTTP(w, r)
	})
}

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(self), microphone=(self), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; connect-src 'self' wss: https:; font-src 'self' data:; media-src 'self' blob:; frame-src 'none'")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		next.ServeHTTP(w, r)
	})
}

type RateLimiterStore interface {
	GetLimiter(key string) *RateLimiterData
	SetLimiter(key string, limiter *RateLimiterData)
}

type RateLimiterData struct {
	requests []time.Time
	limit    int
	window   time.Duration
}

type DistributedRateLimiter struct {
	store       RateLimiterStore
	data        map[string]*RateLimiterData
	mu          sync.RWMutex
	limit       int
	window      time.Duration
	stopCleanup chan bool
}

func NewDistributedRateLimiter(store RateLimiterStore, limit int, window time.Duration) *DistributedRateLimiter {
	rl := &DistributedRateLimiter{
		store:       store,
		data:        make(map[string]*RateLimiterData),
		limit:       limit,
		window:      window,
		stopCleanup: make(chan bool),
	}
	go rl.cleanup()
	return rl
}

func (rl *DistributedRateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			rl.data = make(map[string]*RateLimiterData)
			rl.mu.Unlock()
		case <-rl.stopCleanup:
			return
		}
	}
}

func (rl *DistributedRateLimiter) Allow(key string) bool {
	now := time.Now()
	windowStart := now.Add(-rl.window)

	rl.mu.RLock()
	var valid []time.Time
	if data, ok := rl.data[key]; ok {
		for _, t := range data.requests {
			if t.After(windowStart) {
				valid = append(valid, t)
			}
		}
	}
	rl.mu.RUnlock()

	if len(valid) >= rl.limit {
		rl.mu.Lock()
		rl.data[key] = &RateLimiterData{
			requests: valid,
			limit:    rl.limit,
			window:   rl.window,
		}
		rl.mu.Unlock()
		return false
	}

	valid = append(valid, now)
	rl.mu.Lock()
	rl.data[key] = &RateLimiterData{
		requests: valid,
		limit:    rl.limit,
		window:   rl.window,
	}
	rl.mu.Unlock()
	return true
}

func (rl *DistributedRateLimiter) Shutdown() {
	close(rl.stopCleanup)
}
