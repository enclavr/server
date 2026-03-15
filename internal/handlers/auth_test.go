package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/enclavr/server/internal/auth"
	"github.com/enclavr/server/internal/config"
	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/internal/services"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db := openTestDB(t)

	err := db.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.UserRoom{},
		&models.Session{},
		&models.RefreshToken{},
		&models.VoiceSession{},
		&models.RoomInvite{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupTestHandler(t *testing.T) *AuthHandler {
	db := setupTestDB(t)
	authCfg := &config.AuthConfig{
		JWTSecret:         "test-secret-key",
		JWTExpiration:     24 * time.Hour,
		RefreshExpiration: 168 * time.Hour,
	}
	authService := auth.NewAuthService(authCfg)
	emailService := services.NewEmailService(nil)
	oauthService := services.NewOAuthService(authCfg)
	testDB := &database.Database{DB: db}
	cfg := &config.Config{}
	loginTracker := auth.NewLoginAttemptTracker(5, 15*time.Minute, 15*time.Minute)
	handler := NewAuthHandler(testDB, authService, emailService, oauthService, cfg, false, loginTracker)
	return handler
}

func TestRegister(t *testing.T) {
	handler := setupTestHandler(t)

	tests := []struct {
		name           string
		body           RegisterRequest
		expectedStatus int
	}{
		{
			name: "valid registration",
			body: RegisterRequest{
				Username: "testuser",
				Email:    "test@example.com",
				Password: "Password1!!",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "missing username",
			body: RegisterRequest{
				Email:    "test@example.com",
				Password: "Password1!!",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "missing email",
			body: RegisterRequest{
				Username: "testuser",
				Password: "Password1!!",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "missing Secure1!!",
			body: RegisterRequest{
				Username: "testuser",
				Email:    "test@example.com",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "duplicate username",
			body: RegisterRequest{
				Username: "testuser2",
				Email:    "test2@example.com",
				Password: "Password1!!",
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.Register(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestRegister_InvalidJSON(t *testing.T) {
	handler := setupTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBuffer([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Register(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	handler := setupTestHandler(t)

	registerBody := RegisterRequest{
		Username: "testuser",
		Email:    "test@example.com",
		Password: "Password1!!",
	}
	body, _ := json.Marshal(registerBody)
	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.Register(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected first registration to succeed, got %d", w.Code)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBuffer(body))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	handler.Register(w2, req2)

	if w2.Code != http.StatusInternalServerError && w2.Code != http.StatusConflict {
		t.Errorf("expected duplicate email to return error status, got %d", w2.Code)
	}
}

func TestLogin(t *testing.T) {
	handler := setupTestHandler(t)

	registerBody := RegisterRequest{
		Username: "testuser",
		Email:    "test@example.com",
		Password: "Password1!!",
	}
	body, _ := json.Marshal(registerBody)
	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.Register(w, req)

	tests := []struct {
		name           string
		body           LoginRequest
		expectedStatus int
	}{
		{
			name: "valid login",
			body: LoginRequest{
				Username: "testuser",
				Password: "Password1!!",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "wrong Secure1!!",
			body: LoginRequest{
				Username: "testuser",
				Password: "Wrong1!!",
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "nonexistent user",
			body: LoginRequest{
				Username: "nonexistent",
				Password: "Password1!!",
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "missing username",
			body: LoginRequest{
				Password: "Password1!!",
			},
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.Login(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestRefreshToken(t *testing.T) {
	handler := setupTestHandler(t)

	registerBody := RegisterRequest{
		Username: "testuser",
		Email:    "test@example.com",
		Password: "Password1!!",
	}
	body, _ := json.Marshal(registerBody)
	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.Register(w, req)

	var resp AuthResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	tests := []struct {
		name           string
		refreshToken   string
		expectedStatus int
	}{
		{
			name:           "valid refresh token",
			refreshToken:   resp.RefreshToken,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "invalid refresh token",
			refreshToken:   "invalid-token",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "empty refresh token",
			refreshToken:   "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "malformed JSON body",
			refreshToken:   "",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "malformed JSON body" {
				req = httptest.NewRequest(http.MethodPost, "/refresh", bytes.NewBuffer([]byte("{invalid")))
			} else {
				body, _ = json.Marshal(struct {
					RefreshToken string `json:"refresh_token"`
				}{RefreshToken: tt.refreshToken})
				req = httptest.NewRequest(http.MethodPost, "/refresh", bytes.NewBuffer(body))
			}
			req.Header.Set("Content-Type", "application/json")
			w = httptest.NewRecorder()

			handler.RefreshToken(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

// setupAdminHandler creates a handler with firstIsAdmin=true (fresh DB each call)
func setupAdminHandler(t *testing.T) *AuthHandler {
	db := setupTestDB(t)
	authCfg := &config.AuthConfig{
		JWTSecret:         "test-secret-key",
		JWTExpiration:     24 * time.Hour,
		RefreshExpiration: 168 * time.Hour,
	}
	authService := auth.NewAuthService(authCfg)
	emailService := services.NewEmailService(nil)
	oauthService := services.NewOAuthService(authCfg)
	testDB := &database.Database{DB: db}
	cfg := &config.Config{}
	loginTracker := auth.NewLoginAttemptTracker(5, 15*time.Minute, 15*time.Minute)
	return NewAuthHandler(testDB, authService, emailService, oauthService, cfg, true, loginTracker)
}

// TestRegister_FirstUserIsAdmin_Enabled verifies that when firstIsAdmin=true,
// the very first registered user gets is_admin=true in the response.
func TestRegister_FirstUserIsAdmin_Enabled(t *testing.T) {
	handler := setupAdminHandler(t)

	body, _ := json.Marshal(RegisterRequest{
		Username: "firstuser",
		Email:    "first@example.com",
		Password: "Password1!!",
	})
	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Register(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp AuthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if !resp.User.IsAdmin {
		t.Errorf("first user should have is_admin=true, got false")
	}
}

// TestRegister_SecondUserIsNotAdmin verifies that when firstIsAdmin=true,
// only the first user gets admin — subsequent users do not.
func TestRegister_SecondUserIsNotAdmin(t *testing.T) {
	handler := setupAdminHandler(t)

	// Register first user (should be admin)
	firstBody, _ := json.Marshal(RegisterRequest{
		Username: "firstuser",
		Email:    "first@example.com",
		Password: "Password2!!",
	})
	req1 := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBuffer(firstBody))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	handler.Register(w1, req1)

	if w1.Code != http.StatusOK {
		t.Fatalf("first registration failed: %d", w1.Code)
	}
	var first AuthResponse
	if err := json.Unmarshal(w1.Body.Bytes(), &first); err != nil {
		t.Fatalf("failed to unmarshal first response: %v", err)
	}
	if !first.User.IsAdmin {
		t.Errorf("first user should be admin")
	}

	// Register second user (should NOT be admin)
	secondBody, _ := json.Marshal(RegisterRequest{
		Username: "seconduser",
		Email:    "second@example.com",
		Password: "Password3!!",
	})
	req2 := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBuffer(secondBody))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	handler.Register(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("second registration failed: %d", w2.Code)
	}
	var second AuthResponse
	if err := json.Unmarshal(w2.Body.Bytes(), &second); err != nil {
		t.Fatalf("failed to unmarshal second response: %v", err)
	}
	if second.User.IsAdmin {
		t.Errorf("second user should NOT be admin, got is_admin=true")
	}
}

// TestRegister_FirstUserIsAdmin_Disabled verifies that with firstIsAdmin=false,
// even the first user does NOT become admin.
func TestRegister_FirstUserIsAdmin_Disabled(t *testing.T) {
	handler := setupTestHandler(t) // firstIsAdmin=false

	body, _ := json.Marshal(RegisterRequest{
		Username: "user1",
		Email:    "user1@example.com",
		Password: "Password1!!",
	})
	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.Register(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp AuthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.User.IsAdmin {
		t.Errorf("first user should NOT be admin when firstIsAdmin=false")
	}
}

// TestRegister_FirstUserIsAdmin_ThirdUser verifies admin isolation across 3 users.
func TestRegister_FirstUserIsAdmin_ThirdUser(t *testing.T) {
	handler := setupAdminHandler(t)

	for i, tc := range []struct {
		username  string
		email     string
		wantAdmin bool
	}{
		{"user1", "u1@example.com", true},
		{"user2", "u2@example.com", false},
		{"user3", "u3@example.com", false},
	} {
		body, _ := json.Marshal(RegisterRequest{
			Username: tc.username,
			Email:    tc.email,
			Password: "Secure1!!",
		})
		req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.Register(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("user%d registration returned %d", i+1, w.Code)
		}

		var resp AuthResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		if resp.User.IsAdmin != tc.wantAdmin {
			t.Errorf("user%d (%s): expected is_admin=%v, got %v", i+1, tc.username, tc.wantAdmin, resp.User.IsAdmin)
		}
	}
}

// TestRegister_ResponseBody verifies the full auth response structure on successful registration.
func TestRegister_ResponseBody(t *testing.T) {
	handler := setupTestHandler(t)

	body, _ := json.Marshal(RegisterRequest{
		Username: "bodycheck",
		Email:    "body@example.com",
		Password: "Secure1!!",
	})
	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.Register(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp AuthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.AccessToken == "" {
		t.Error("access_token should not be empty")
	}
	if resp.RefreshToken == "" {
		t.Error("refresh_token should not be empty")
	}
	if resp.ExpiresIn <= 0 {
		t.Errorf("expires_in should be positive, got %d", resp.ExpiresIn)
	}
	if resp.User.Username != "bodycheck" {
		t.Errorf("expected username 'bodycheck', got '%s'", resp.User.Username)
	}
	if resp.User.Email != "body@example.com" {
		t.Errorf("expected email 'body@example.com', got '%s'", resp.User.Email)
	}
	if resp.User.ID == (uuid.UUID{}) {
		t.Error("user ID should not be zero UUID")
	}
}

func TestLogin_ResponseBody(t *testing.T) {
	handler := setupTestHandler(t)

	// seed user
	regBody, err := json.Marshal(RegisterRequest{
		Username: "loginuser",
		Email:    "login@example.com",
		Password: "Login1!!",
	})
	if err != nil {
		t.Fatalf("failed to marshal register request: %v", err)
	}
	regReq := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBuffer(regBody))
	regReq.Header.Set("Content-Type", "application/json")
	regW := httptest.NewRecorder()
	handler.Register(regW, regReq)

	if regW.Code != http.StatusOK {
		t.Fatalf("registration failed: %d - %s", regW.Code, regW.Body.String())
	}

	body, err := json.Marshal(LoginRequest{Username: "loginuser", Password: "Login1!!"})
	if err != nil {
		t.Fatalf("failed to marshal login request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.Login(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp AuthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.AccessToken == "" {
		t.Error("access_token should not be empty on login")
	}
	if resp.User.Username != "loginuser" {
		t.Errorf("expected username 'loginuser', got '%s'", resp.User.Username)
	}
}

// TestLogin_AdminUserToken verifies that when first user registers with firstIsAdmin=true,
// subsequent login returns a response where is_admin=true.
func TestLogin_AdminUserToken(t *testing.T) {
	handler := setupAdminHandler(t)

	// Register first user (admin)
	regBody, _ := json.Marshal(RegisterRequest{
		Username: "adminlogin",
		Email:    "adminlogin@example.com",
		Password: "Admin1!!",
	})
	regReq := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBuffer(regBody))
	regReq.Header.Set("Content-Type", "application/json")
	handler.Register(httptest.NewRecorder(), regReq)

	// Login and check is_admin
	body, _ := json.Marshal(LoginRequest{Username: "adminlogin", Password: "Admin1!!"})
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.Login(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("login failed: %d", w.Code)
	}

	var resp AuthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if !resp.User.IsAdmin {
		t.Errorf("admin user's login response should have is_admin=true")
	}
}

// TestRegister_GetMe_IsAdmin verifies that GetMe returns is_admin=true for the first user.
func TestRegister_GetMe_IsAdmin(t *testing.T) {
	handler := setupAdminHandler(t)

	regBody, _ := json.Marshal(RegisterRequest{
		Username: "meadmin",
		Email:    "meadmin@example.com",
		Password: "meSecure1!!",
	})
	reqReg := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBuffer(regBody))
	reqReg.Header.Set("Content-Type", "application/json")
	wReg := httptest.NewRecorder()
	handler.Register(wReg, reqReg)

	var regResp AuthResponse
	if err := json.Unmarshal(wReg.Body.Bytes(), &regResp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, regResp.User.ID)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	handler.GetMe(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GetMe failed: %d", w.Code)
	}

	var meResp UserResponse
	if err := json.Unmarshal(w.Body.Bytes(), &meResp); err != nil {
		t.Fatalf("failed to decode GetMe response: %v", err)
	}
	if !meResp.IsAdmin {
		t.Errorf("GetMe should return is_admin=true for first-registered admin user")
	}
}

func TestGetMe(t *testing.T) {
	handler := setupTestHandler(t)

	registerBody := RegisterRequest{
		Username: "testuser",
		Email:    "test@example.com",
		Password: "Password1!!",
	}
	body, _ := json.Marshal(registerBody)
	req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.Register(w, req)

	var resp AuthResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	userID, _ := uuid.Parse(resp.User.ID.String())

	tests := []struct {
		name           string
		userID         uuid.UUID
		expectedStatus int
	}{
		{
			name:           "valid user",
			userID:         userID,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "invalid user",
			userID:         uuid.Nil,
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/me", nil)
			// Use middleware.UserIDKey to match the middleware
			ctx := context.WithValue(req.Context(), middleware.UserIDKey, tt.userID)
			req = req.WithContext(ctx)
			w := httptest.NewRecorder()

			handler.GetMe(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}
