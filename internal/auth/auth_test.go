package auth

import (
	"testing"
	"time"

	"github.com/enclavr/server/internal/config"
	"github.com/enclavr/server/internal/models"
	"github.com/google/uuid"
)

func TestHashPassword(t *testing.T) {
	cfg := &config.AuthConfig{
		JWTSecret:         "test-secret",
		JWTExpiration:     time.Hour,
		RefreshExpiration: time.Hour * 24 * 7,
	}
	svc := NewAuthService(cfg)

	hash, err := svc.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if hash == "password123" {
		t.Error("Hash should not equal plain password")
	}
}

func TestCheckPassword(t *testing.T) {
	cfg := &config.AuthConfig{
		JWTSecret:         "test-secret",
		JWTExpiration:     time.Hour,
		RefreshExpiration: time.Hour * 24 * 7,
	}
	svc := NewAuthService(cfg)

	hash, _ := svc.HashPassword("password123")

	if !svc.CheckPassword("password123", hash) {
		t.Error("CheckPassword should return true for correct password")
	}

	if svc.CheckPassword("wrongpassword", hash) {
		t.Error("CheckPassword should return false for wrong password")
	}
}

func TestGenerateToken(t *testing.T) {
	cfg := &config.AuthConfig{
		JWTSecret:         "test-secret",
		JWTExpiration:     time.Hour,
		RefreshExpiration: time.Hour * 24 * 7,
	}
	svc := NewAuthService(cfg)

	user := &models.User{
		ID:       uuid.New(),
		Username: "testuser",
		IsAdmin:  true,
	}

	token, err := svc.GenerateToken(user)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	if token == "" {
		t.Error("Token should not be empty")
	}
}

func TestGenerateRefreshToken(t *testing.T) {
	cfg := &config.AuthConfig{
		JWTSecret:         "test-secret",
		JWTExpiration:     time.Hour,
		RefreshExpiration: time.Hour * 24 * 7,
	}
	svc := NewAuthService(cfg)

	user := &models.User{
		ID:       uuid.New(),
		Username: "testuser",
		IsAdmin:  false,
	}

	token, err := svc.GenerateRefreshToken(user)
	if err != nil {
		t.Fatalf("GenerateRefreshToken failed: %v", err)
	}

	if token == "" {
		t.Error("Refresh token should not be empty")
	}
}

func TestValidateToken(t *testing.T) {
	cfg := &config.AuthConfig{
		JWTSecret:         "test-secret",
		JWTExpiration:     time.Hour,
		RefreshExpiration: time.Hour * 24 * 7,
	}
	svc := NewAuthService(cfg)

	user := &models.User{
		ID:       uuid.New(),
		Username: "testuser",
		IsAdmin:  true,
	}

	token, _ := svc.GenerateToken(user)

	claims, err := svc.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}

	if claims.UserID != user.ID {
		t.Errorf("Expected user ID %s, got %s", user.ID, claims.UserID)
	}

	if claims.Username != user.Username {
		t.Errorf("Expected username %s, got %s", user.Username, claims.Username)
	}

	if !claims.IsAdmin {
		t.Error("Expected IsAdmin to be true")
	}
}

func TestValidateToken_Invalid(t *testing.T) {
	cfg := &config.AuthConfig{
		JWTSecret:         "test-secret",
		JWTExpiration:     time.Hour,
		RefreshExpiration: time.Hour * 24 * 7,
	}
	svc := NewAuthService(cfg)

	_, err := svc.ValidateToken("invalid-token")
	if err == nil {
		t.Error("Expected error for invalid token")
	}
}

func TestValidateRefreshToken(t *testing.T) {
	cfg := &config.AuthConfig{
		JWTSecret:         "test-secret",
		JWTExpiration:     time.Hour,
		RefreshExpiration: time.Hour * 24 * 7,
	}
	svc := NewAuthService(cfg)

	user := &models.User{
		ID: uuid.New(),
	}

	token, _ := svc.GenerateRefreshToken(user)

	claims, err := svc.ValidateRefreshToken(token)
	if err != nil {
		t.Fatalf("ValidateRefreshToken failed: %v", err)
	}

	if claims.UserID != user.ID {
		t.Errorf("Expected user ID %s, got %s", user.ID, claims.UserID)
	}

	if claims.Type != "refresh" {
		t.Errorf("Expected type 'refresh', got %s", claims.Type)
	}
}

func TestValidateRefreshToken_Invalid(t *testing.T) {
	cfg := &config.AuthConfig{
		JWTSecret:         "test-secret",
		JWTExpiration:     time.Hour,
		RefreshExpiration: time.Hour * 24 * 7,
	}
	svc := NewAuthService(cfg)

	_, err := svc.ValidateRefreshToken("invalid-token")
	if err == nil {
		t.Error("Expected error for invalid refresh token")
	}
}

func TestGenerateState(t *testing.T) {
	state1, err := GenerateState()
	if err != nil {
		t.Fatalf("GenerateState failed: %v", err)
	}

	if len(state1) == 0 {
		t.Error("State should not be empty")
	}

	state2, _ := GenerateState()
	if state1 == state2 {
		t.Error("Generated states should be unique")
	}
}

func TestGenerateUUID(t *testing.T) {
	uuid1 := GenerateUUID()
	uuid2 := GenerateUUID()

	if uuid1 == uuid2 {
		t.Error("Generated UUIDs should be unique")
	}

	if uuid1.String() == "" {
		t.Error("UUID should not be empty")
	}
}
