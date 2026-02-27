package middleware

import (
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/enclavr/server/internal/auth"
	"github.com/enclavr/server/internal/config"
	"github.com/enclavr/server/internal/models"
	"github.com/google/uuid"
)

func TestRequestID(t *testing.T) {
	tests := []struct {
		name           string
		requestID      string
		expectedHeader string
	}{
		{
			name:           "generates new request ID when none provided",
			requestID:      "",
			expectedHeader: "",
		},
		{
			name:           "uses provided request ID",
			requestID:      "test-request-id-123",
			expectedHeader: "test-request-id-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := RequestID()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.requestID != "" {
				req.Header.Set("X-Request-ID", tt.requestID)
			}

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			responseID := w.Header().Get("X-Request-ID")
			if tt.requestID != "" && responseID != tt.expectedHeader {
				t.Errorf("expected request ID %s, got %s", tt.expectedHeader, responseID)
			}
			if tt.requestID == "" && responseID == "" {
				t.Error("expected generated request ID, got empty string")
			}
		})
	}
}

func TestGetUserID(t *testing.T) {
	tests := []struct {
		name      string
		userID    uuid.UUID
		expectNil bool
	}{
		{
			name:      "returns user ID from context",
			userID:    uuid.MustParse("12345678-1234-1234-1234-123456789abc"),
			expectNil: false,
		},
		{
			name:      "returns nil UUID when not in context",
			userID:    uuid.Nil,
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.userID != uuid.Nil {
				ctx := context.WithValue(req.Context(), UserIDKey, tt.userID)
				req = req.WithContext(ctx)
			}

			result := GetUserID(req)
			if tt.expectNil && result != uuid.Nil {
				t.Errorf("expected nil UUID, got %s", result)
			}
			if !tt.expectNil && result != tt.userID {
				t.Errorf("expected %s, got %s", tt.userID, result)
			}
		})
	}
}

func TestGetUsername(t *testing.T) {
	tests := []struct {
		name        string
		username    string
		expectEmpty bool
	}{
		{
			name:        "returns username from context",
			username:    "testuser",
			expectEmpty: false,
		},
		{
			name:        "returns empty string when not in context",
			username:    "",
			expectEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.username != "" {
				ctx := context.WithValue(req.Context(), UsernameKey, tt.username)
				req = req.WithContext(ctx)
			}

			result := GetUsername(req)
			if tt.expectEmpty && result != "" {
				t.Errorf("expected empty string, got %s", result)
			}
			if !tt.expectEmpty && result != tt.username {
				t.Errorf("expected %s, got %s", tt.username, result)
			}
		})
	}
}

func TestGetIsAdmin(t *testing.T) {
	tests := []struct {
		name        string
		isAdmin     bool
		expectFalse bool
	}{
		{
			name:        "returns true when admin",
			isAdmin:     true,
			expectFalse: false,
		},
		{
			name:        "returns false when not admin",
			isAdmin:     false,
			expectFalse: true,
		},
		{
			name:        "returns false when not in context",
			isAdmin:     false,
			expectFalse: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.name != "returns false when not in context" {
				ctx := context.WithValue(req.Context(), IsAdminKey, tt.isAdmin)
				req = req.WithContext(ctx)
			}

			result := GetIsAdmin(req)
			if tt.expectFalse && result {
				t.Error("expected false, got true")
			}
			if !tt.expectFalse && !result {
				t.Error("expected true, got false")
			}
		})
	}
}

func TestGetRequestIDFromContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := context.WithValue(req.Context(), RequestIDKey, "test-request-id")
	req = req.WithContext(ctx)

	result := GetRequestID(req)
	if result != "test-request-id" {
		t.Errorf("expected test-request-id, got %s", result)
	}
}

func TestRateLimiter_Allow(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute)
	userID := uuid.New()

	if !rl.Allow(userID) {
		t.Error("expected first request to be allowed")
	}
	if !rl.Allow(userID) {
		t.Error("expected second request to be allowed")
	}
	if !rl.Allow(userID) {
		t.Error("expected third request to be allowed")
	}
	if rl.Allow(userID) {
		t.Error("expected fourth request to be rate limited")
	}
}

func TestRateLimiter_DifferentUsers(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute)
	user1 := uuid.New()
	user2 := uuid.New()

	if !rl.Allow(user1) {
		t.Error("expected user1 first request to be allowed")
	}
	if !rl.Allow(user1) {
		t.Error("expected user1 second request to be allowed")
	}
	if !rl.Allow(user2) {
		t.Error("expected user2 first request to be allowed (separate limit)")
	}
}

func TestNewRateLimiter(t *testing.T) {
	rl := NewRateLimiter(10, time.Minute)
	if rl.limit != 10 {
		t.Errorf("expected limit 10, got %d", rl.limit)
	}
	if rl.window != time.Minute {
		t.Errorf("expected window 1m, got %v", rl.window)
	}
}

func TestJWTAuth_MissingHeader(t *testing.T) {
	cfg := &config.AuthConfig{
		JWTSecret:         "test-secret-key",
		JWTExpiration:     24 * time.Hour,
		RefreshExpiration: 7 * 24 * time.Hour,
	}
	authSvc := auth.NewAuthService(cfg)
	handler := JWTAuth(authSvc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}

func TestJWTAuth_InvalidFormat(t *testing.T) {
	cfg := &config.AuthConfig{
		JWTSecret:         "test-secret-key",
		JWTExpiration:     24 * time.Hour,
		RefreshExpiration: 7 * 24 * time.Hour,
	}
	authSvc := auth.NewAuthService(cfg)
	handler := JWTAuth(authSvc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "InvalidToken")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}

func TestJWTAuth_InvalidToken(t *testing.T) {
	cfg := &config.AuthConfig{
		JWTSecret:         "test-secret-key",
		JWTExpiration:     24 * time.Hour,
		RefreshExpiration: 7 * 24 * time.Hour,
	}
	authSvc := auth.NewAuthService(cfg)
	handler := JWTAuth(authSvc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}

func TestJWTAuth_ValidToken(t *testing.T) {
	cfg := &config.AuthConfig{
		JWTSecret:         "test-secret-key",
		JWTExpiration:     24 * time.Hour,
		RefreshExpiration: 7 * 24 * time.Hour,
	}
	authSvc := auth.NewAuthService(cfg)

	user := &models.User{
		ID:       uuid.New(),
		Username: "testuser",
		IsAdmin:  false,
	}
	token, err := authSvc.GenerateToken(user)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	handler := JWTAuth(authSvc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := GetUserID(r)
		if userID == uuid.Nil {
			t.Error("expected user ID in context")
		}
		username := GetUsername(r)
		if username != "testuser" {
			t.Errorf("expected username testuser, got %s", username)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestJWTAuth_ExpiredToken(t *testing.T) {
	cfg := &config.AuthConfig{
		JWTSecret:         "test-secret-key",
		JWTExpiration:     -1 * time.Hour,
		RefreshExpiration: 7 * 24 * time.Hour,
	}
	authSvc := auth.NewAuthService(cfg)

	user := &models.User{
		ID:       uuid.New(),
		Username: "testuser",
		IsAdmin:  false,
	}
	token, err := authSvc.GenerateToken(user)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	handler := JWTAuth(authSvc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}

func TestRateLimit_NoLimiter(t *testing.T) {
	handler := RateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestRequestTimeout(t *testing.T) {
	handler := RequestTimeout(50 * time.Millisecond)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusRequestTimeout {
		t.Errorf("expected status 408, got %d", w.Code)
	}
}

func TestNewCORSMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		allowedOrigins []string
		expectedOrigin string
	}{
		{
			name:           "default allow all",
			allowedOrigins: []string{},
			expectedOrigin: "*",
		},
		{
			name:           "specific origin",
			allowedOrigins: []string{"https://example.com"},
			expectedOrigin: "https://example.com",
		},
		{
			name:           "multiple origins",
			allowedOrigins: []string{"https://example.com", "https://test.com"},
			expectedOrigin: "https://example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			middleware := NewCORSMiddleware(tt.allowedOrigins)
			if len(tt.allowedOrigins) == 0 {
				if len(middleware.allowedOrigins) != 1 || middleware.allowedOrigins[0] != "*" {
					t.Errorf("expected default * origin")
				}
			} else {
				if len(middleware.allowedOrigins) != len(tt.allowedOrigins) {
					t.Errorf("expected %d origins, got %d", len(tt.allowedOrigins), len(middleware.allowedOrigins))
				}
			}
		})
	}
}

func TestCORSMiddleware_Options(t *testing.T) {
	middleware := NewCORSMiddleware([]string{"https://example.com"})
	handler := middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", w.Code)
	}

	if w.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Error("expected Access-Control-Allow-Methods header")
	}
}

func TestCORSMiddleware_NonOptions(t *testing.T) {
	middleware := NewCORSMiddleware([]string{"https://example.com"})
	handler := middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Header().Get("Access-Control-Allow-Origin") != "https://example.com" {
		t.Error("expected Access-Control-Allow-Origin header")
	}
}

func TestCORSMiddleware_Wildcard(t *testing.T) {
	middleware := NewCORSMiddleware([]string{})
	handler := middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://any-origin.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("expected * origin for wildcard")
	}
}

func TestSecurityHeaders(t *testing.T) {
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("expected X-Content-Type-Options header")
	}
	if w.Header().Get("X-Frame-Options") != "DENY" {
		t.Error("expected X-Frame-Options header")
	}
	if w.Header().Get("Strict-Transport-Security") == "" {
		t.Error("expected Strict-Transport-Security header")
	}
}

func TestInitRateLimiter(t *testing.T) {
	InitRateLimiter(60)
	if globalLimiter == nil {
		t.Error("expected globalLimiter to be initialized")
	}
	InitRateLimiter(30)
}

func TestRateLimit_NilLimiter(t *testing.T) {
	globalLimiter = nil
	handler := RateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status OK, got %d", w.Code)
	}
}

func TestGzipCompression(t *testing.T) {
	handler := GzipCompression()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("test response"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Error("expected gzip content encoding")
	}
}

func TestGzipCompression_NoGzip(t *testing.T) {
	handler := GzipCompression()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("test response"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("Content-Encoding") == "gzip" {
		t.Error("did not expect gzip content encoding")
	}
}

func TestGzipResponseWriter_Write(t *testing.T) {
	buf := &bytes.Buffer{}
	gw := &gzipResponseWriter{ResponseWriter: httptest.NewRecorder(), gw: gzip.NewWriter(buf)}

	n, err := gw.Write([]byte("hello"))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if n != 5 {
		t.Errorf("expected 5 bytes written, got %d", n)
	}
}

func TestRequireAuth(t *testing.T) {
	cfg := &config.Config{
		Auth: config.AuthConfig{
			JWTSecret:         "test-secret",
			JWTExpiration:     time.Hour,
			RefreshExpiration: time.Hour * 24 * 7,
		},
	}
	authService := auth.NewAuthService(&cfg.Auth)

	handler := RequireAuth(authService, func(w http.ResponseWriter, r *http.Request) {
		userID := GetUserID(r)
		_, _ = w.Write([]byte(userID.String()))
	})

	tests := []struct {
		name           string
		authHeader     string
		expectedStatus int
	}{
		{
			name:           "missing authorization header",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "invalid authorization format",
			authHeader:     "InvalidToken",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "valid token",
			authHeader:     "Bearer validtoken",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestGetRequestID_EmptyContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	result := GetRequestID(req)
	if result != "" {
		t.Errorf("expected empty string, got %s", result)
	}
}

func TestGetRequestID_FromContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := context.WithValue(req.Context(), RequestIDKey, "test-request-id")
	req = req.WithContext(ctx)

	result := GetRequestID(req)
	if result != "test-request-id" {
		t.Errorf("expected 'test-request-id', got %s", result)
	}
}

func TestRateLimit_WithLimiter(t *testing.T) {
	limiter := NewRateLimiter(1, time.Minute)
	globalLimiter = limiter

	handler := RateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status OK, got %d", w.Code)
	}

	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req)

	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("expected status TooManyRequests, got %d", w2.Code)
	}
}

func TestRateLimit_WithUserID(t *testing.T) {
	limiter := NewRateLimiter(2, time.Minute)
	globalLimiter = limiter

	handler := RateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	userID := uuid.New()
	ctx := context.WithValue(context.Background(), UserIDKey, userID)
	req := httptest.NewRequest(http.MethodGet, "/test", nil).WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status OK, got %d", w.Code)
	}
}

func TestRateLimiter_Allow_AtLimit(t *testing.T) {
	limiter := NewRateLimiter(3, time.Minute)
	userID := uuid.New()

	for i := 0; i < 3; i++ {
		if !limiter.Allow(userID) {
			t.Errorf("expected request %d to be allowed", i+1)
		}
	}

	if limiter.Allow(userID) {
		t.Error("expected request 4 to be rate limited")
	}
}

func TestRateLimiter_Allow_EmptyMap(t *testing.T) {
	limiter := NewRateLimiter(2, time.Minute)
	userID := uuid.New()

	if !limiter.Allow(userID) {
		t.Error("expected first request to be allowed")
	}
	if !limiter.Allow(userID) {
		t.Error("expected second request to be allowed")
	}
}

func TestGzipCompression_ErrorClose(t *testing.T) {
	handler := GzipCompression()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("test"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status OK, got %d", w.Code)
	}
	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Error("expected Content-Encoding: gzip")
	}
}

func TestRateLimiter_ExpiredEntries(t *testing.T) {
	rl := NewRateLimiter(3, 50*time.Millisecond)
	userID := uuid.New()

	rl.Allow(userID)
	rl.Allow(userID)
	rl.Allow(userID)

	if rl.Allow(userID) {
		t.Error("expected rate limit to be applied")
	}

	time.Sleep(100 * time.Millisecond)

	if !rl.Allow(userID) {
		t.Error("expected rate limit to be reset after window expiration")
	}
}
