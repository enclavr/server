package middleware

import (
	"net/http"
	"regexp"
	"sync"
	"time"
)

type EndpointRateLimitConfig struct {
	Path       string
	Methods    []string
	Limit      int
	Window     time.Duration
	PathRegexp *regexp.Regexp
}

type EndpointRateLimiter struct {
	configs      []EndpointRateLimitConfig
	limiters     map[string]*EndpointLimiter
	mu           sync.RWMutex
	window       time.Duration
	defaultLimit int
}

type EndpointLimiter struct {
	requests []time.Time
	mu       sync.Mutex
	limit    int
	window   time.Duration
}

func NewEndpointRateLimiter(configs []EndpointRateLimitConfig, defaultLimit int, defaultWindow time.Duration) *EndpointRateLimiter {
	erl := &EndpointRateLimiter{
		configs:      configs,
		limiters:     make(map[string]*EndpointLimiter),
		window:       defaultWindow,
		defaultLimit: defaultLimit,
	}

	for i := range configs {
		if configs[i].PathRegexp == nil && configs[i].Path != "" {
			pattern := configs[i].Path
			pattern = regexp.QuoteMeta(pattern)
			pattern = regexp.MustCompile(`\\\*`).ReplaceAllString(pattern, ".*")
			pattern = "^" + pattern + "$"
			re, err := regexp.Compile(pattern)
			if err == nil {
				configs[i].PathRegexp = re
			}
		}
	}

	go erl.cleanup()
	return erl
}

func (erl *EndpointRateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		erl.mu.Lock()
		now := time.Now()
		for key, limiter := range erl.limiters {
			limiter.mu.Lock()
			var valid []time.Time
			for _, t := range limiter.requests {
				if now.Sub(t) < limiter.window {
					valid = append(valid, t)
				}
			}
			if len(valid) == 0 {
				delete(erl.limiters, key)
			} else {
				limiter.requests = valid
			}
			limiter.mu.Unlock()
		}
		erl.mu.Unlock()
	}
}

func (erl *EndpointRateLimiter) getLimiter(key string, limit int, window time.Duration) *EndpointLimiter {
	erl.mu.RLock()
	limiter, exists := erl.limiters[key]
	erl.mu.RUnlock()

	if exists {
		return limiter
	}

	erl.mu.Lock()
	defer erl.mu.Unlock()

	if limiter, exists := erl.limiters[key]; exists {
		return limiter
	}

	limiter = &EndpointLimiter{
		limit:    limit,
		window:   window,
		requests: make([]time.Time, 0),
	}
	erl.limiters[key] = limiter
	return limiter
}

func (erl *EndpointRateLimiter) getConfig(path string, method string) *EndpointRateLimitConfig {
	for _, config := range erl.configs {
		methodMatch := false
		if len(config.Methods) == 0 {
			methodMatch = true
		} else {
			for _, m := range config.Methods {
				if m == method {
					methodMatch = true
					break
				}
			}
		}

		if !methodMatch {
			continue
		}

		if config.PathRegexp != nil {
			if config.PathRegexp.MatchString(path) {
				return &config
			}
		} else if config.Path == path {
			return &config
		}
	}

	return nil
}

func (erl *EndpointRateLimiter) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		method := r.Method

		config := erl.getConfig(path, method)

		limit := erl.defaultLimit
		window := erl.window

		if config != nil {
			limit = config.Limit
			window = config.Window
		}

		key := getClientIP(r) + ":" + path + ":" + method
		limiter := erl.getLimiter(key, limit, window)

		limiter.mu.Lock()
		now := time.Now()
		windowStart := now.Add(-limiter.window)

		var valid []time.Time
		for _, t := range limiter.requests {
			if t.After(windowStart) {
				valid = append(valid, t)
			}
		}

		if len(valid) >= limiter.limit {
			limiter.requests = valid
			limiter.mu.Unlock()

			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-RateLimit-Limit", string(rune(limit)))
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", string(rune(now.Add(window).Unix())))
			http.Error(w, `{"error":"Too Many Requests","code":"rate_limit_exceeded","message":"Rate limit exceeded for this endpoint"}`, http.StatusTooManyRequests)
			return
		}

		valid = append(valid, now)
		limiter.requests = valid
		limiter.mu.Unlock()

		remaining := limit - len(valid)
		w.Header().Set("X-RateLimit-Limit", string(rune(limit)))
		w.Header().Set("X-RateLimit-Remaining", string(rune(remaining)))
		w.Header().Set("X-RateLimit-Reset", string(rune(now.Add(window).Unix())))

		next.ServeHTTP(w, r)
	})
}

type EndpointRateLimitOption func(*EndpointRateLimiterBuilder)

type EndpointRateLimiterBuilder struct {
	configs       []EndpointRateLimitConfig
	defaultLimit  int
	defaultWindow time.Duration
}

func NewEndpointRateLimiterBuilder() *EndpointRateLimiterBuilder {
	return &EndpointRateLimiterBuilder{
		defaultLimit:  100,
		defaultWindow: time.Minute,
	}
}

func (b *EndpointRateLimiterBuilder) WithDefaultLimit(limit int) *EndpointRateLimiterBuilder {
	b.defaultLimit = limit
	return b
}

func (b *EndpointRateLimiterBuilder) WithDefaultWindow(window time.Duration) *EndpointRateLimiterBuilder {
	b.defaultWindow = window
	return b
}

func (b *EndpointRateLimiterBuilder) AddEndpoint(path string, limit int, window time.Duration, methods ...string) *EndpointRateLimiterBuilder {
	b.configs = append(b.configs, EndpointRateLimitConfig{
		Path:    path,
		Methods: methods,
		Limit:   limit,
		Window:  window,
	})
	return b
}

func (b *EndpointRateLimiterBuilder) AddEndpointRegex(pathRegex string, limit int, window time.Duration, methods ...string) *EndpointRateLimiterBuilder {
	re, err := regexp.Compile(pathRegex)
	if err == nil {
		b.configs = append(b.configs, EndpointRateLimitConfig{
			PathRegexp: re,
			Methods:    methods,
			Limit:      limit,
			Window:     window,
		})
	}
	return b
}

func (b *EndpointRateLimiterBuilder) Build() *EndpointRateLimiter {
	return NewEndpointRateLimiter(b.configs, b.defaultLimit, b.defaultWindow)
}

var DefaultEndpointRateLimiter = NewEndpointRateLimiterBuilder().
	AddEndpoint("/api/auth/login", 5, time.Minute).
	AddEndpoint("/api/auth/register", 3, time.Minute).
	AddEndpoint("/api/auth/reset-password", 3, time.Minute).
	AddEndpoint("/api/messages", 60, time.Minute).
	AddEndpoint("/api/rooms", 30, time.Minute).
	AddEndpoint("/api/webhooks", 20, time.Minute).
	Build()
