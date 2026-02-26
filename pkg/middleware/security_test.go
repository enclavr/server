package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type mockStore struct {
	data map[string]*RateLimiterData
}

func (m *mockStore) GetLimiter(key string) *RateLimiterData {
	return m.data[key]
}

func (m *mockStore) SetLimiter(key string, limiter *RateLimiterData) {
	m.data[key] = limiter
}

func TestNewDistributedRateLimiter(t *testing.T) {
	store := &mockStore{data: make(map[string]*RateLimiterData)}
	limiter := NewDistributedRateLimiter(store, 10, time.Minute)

	if limiter.limit != 10 {
		t.Errorf("Expected limit 10, got %d", limiter.limit)
	}

	if limiter.window != time.Minute {
		t.Errorf("Expected window 1m, got %v", limiter.window)
	}
}

func TestDistributedRateLimiter_Allow(t *testing.T) {
	store := &mockStore{data: make(map[string]*RateLimiterData)}
	limiter := NewDistributedRateLimiter(store, 3, time.Minute)

	for i := 0; i < 3; i++ {
		if !limiter.Allow("user1") {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	if limiter.Allow("user1") {
		t.Error("4th request should be rate limited")
	}
}

func TestDistributedRateLimiter_DifferentKeys(t *testing.T) {
	store := &mockStore{data: make(map[string]*RateLimiterData)}
	limiter := NewDistributedRateLimiter(store, 2, time.Minute)

	if !limiter.Allow("user1") {
		t.Error("Request for user1 should be allowed")
	}

	if !limiter.Allow("user1") {
		t.Error("2nd request for user1 should be allowed")
	}

	if limiter.Allow("user1") {
		t.Error("3rd request for user1 should be rate limited")
	}

	if !limiter.Allow("user2") {
		t.Error("Request for user2 should be allowed")
	}
}

func TestDistributedRateLimiter_CleanupOldRequests(t *testing.T) {
	store := &mockStore{data: make(map[string]*RateLimiterData)}
	limiter := NewDistributedRateLimiter(store, 3, 50*time.Millisecond)

	limiter.Allow("user1")
	limiter.Allow("user1")
	limiter.Allow("user1")

	if limiter.Allow("user1") {
		t.Error("Should be rate limited initially")
	}

	time.Sleep(100 * time.Millisecond)

	if !limiter.Allow("user1") {
		t.Error("Should be allowed after window expiry")
	}
}

func TestSecurityHeaders_Middleware(t *testing.T) {
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	}))

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("Missing X-Content-Type-Options header")
	}

	if w.Header().Get("X-Frame-Options") != "DENY" {
		t.Error("Missing X-Frame-Options header")
	}

	if w.Header().Get("X-XSS-Protection") != "1; mode=block" {
		t.Error("Missing X-XSS-Protection header")
	}

	if w.Header().Get("Referrer-Policy") != "strict-origin-when-cross-origin" {
		t.Error("Missing Referrer-Policy header")
	}

	if w.Header().Get("Permissions-Policy") == "" {
		t.Error("Missing Permissions-Policy header")
	}

	if w.Header().Get("Content-Security-Policy") == "" {
		t.Error("Missing Content-Security-Policy header")
	}

	if w.Header().Get("Strict-Transport-Security") != "max-age=31536000; includeSubDomains" {
		t.Error("Missing or incorrect Strict-Transport-Security header")
	}
}

func TestCORSMiddleware_WildcardOrigin(t *testing.T) {
	m := NewCORSMiddleware([]string{"*"})

	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "http://example.com")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("Expected wildcard origin")
	}
}

func TestCORSMiddleware_SpecificOrigin(t *testing.T) {
	m := NewCORSMiddleware([]string{"http://localhost:3000"})

	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Error("Expected specific origin")
	}

	if w.Header().Get("Vary") != "Origin" {
		t.Error("Expected Vary: Origin header")
	}
}

func TestCORSMiddleware_DisallowedOrigin(t *testing.T) {
	m := NewCORSMiddleware([]string{"http://localhost:3000"})

	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "http://evil.com")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("Should not allow origin")
	}
}

func TestCORSMiddleware_Preflight(t *testing.T) {
	m := NewCORSMiddleware([]string{"http://localhost:3000"})

	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("OPTIONS", "/", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", w.Code)
	}

	if w.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Error("Missing Allow-Methods header")
	}

	if w.Header().Get("Access-Control-Allow-Headers") == "" {
		t.Error("Missing Allow-Headers header")
	}
}

func TestCORSMiddleware_EmptyOrigins(t *testing.T) {
	m := NewCORSMiddleware([]string{})

	if len(m.allowedOrigins) != 1 || m.allowedOrigins[0] != "*" {
		t.Error("Empty origins should default to wildcard")
	}
}
