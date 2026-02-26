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
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	err = db.AutoMigrate(
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

	testDB := &database.Database{DB: db}
	handler := NewAuthHandler(testDB, authService)
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
				Password: "password123",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "missing username",
			body: RegisterRequest{
				Email:    "test@example.com",
				Password: "password123",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "missing email",
			body: RegisterRequest{
				Username: "testuser",
				Password: "password123",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "missing password",
			body: RegisterRequest{
				Username: "testuser",
				Email:    "test@example.com",
			},
			expectedStatus: http.StatusBadRequest,
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

func TestLogin(t *testing.T) {
	handler := setupTestHandler(t)

	registerBody := RegisterRequest{
		Username: "testuser",
		Email:    "test@example.com",
		Password: "password123",
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
				Password: "password123",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "wrong password",
			body: LoginRequest{
				Username: "testuser",
				Password: "wrongpassword",
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "nonexistent user",
			body: LoginRequest{
				Username: "nonexistent",
				Password: "password123",
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "missing username",
			body: LoginRequest{
				Password: "password123",
			},
			expectedStatus: http.StatusBadRequest,
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
		Password: "password123",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(struct {
				RefreshToken string `json:"refresh_token"`
			}{RefreshToken: tt.refreshToken})
			req := httptest.NewRequest(http.MethodPost, "/refresh", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.RefreshToken(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestGetMe(t *testing.T) {
	handler := setupTestHandler(t)

	registerBody := RegisterRequest{
		Username: "testuser",
		Email:    "test@example.com",
		Password: "password123",
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
			//nolint:staticcheck // Using same key as handler for consistency
			ctx := context.WithValue(req.Context(), "user_id", tt.userID)
			req = req.WithContext(ctx)
			w := httptest.NewRecorder()

			handler.GetMe(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}
