package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/enclavr/server/internal/config"
	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDBForOIDC(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	err = db.AutoMigrate(&models.User{})
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func TestOIDCHandler_Disabled(t *testing.T) {
	db := setupTestDBForOIDC(t)
	testDB := &database.Database{DB: db}

	cfg := &config.AuthConfig{
		OIDCEnabled: false,
	}

	handler := NewOIDCHandler(testDB, cfg)

	if handler.IsEnabled() {
		t.Error("handler should not be enabled when OIDC is disabled")
	}
}

func TestOIDCHandler_Login_NotEnabled(t *testing.T) {
	db := setupTestDBForOIDC(t)
	testDB := &database.Database{DB: db}

	cfg := &config.AuthConfig{
		OIDCEnabled: false,
	}

	handler := NewOIDCHandler(testDB, cfg)

	req := httptest.NewRequest(http.MethodGet, "/auth/oidc/login", nil)
	w := httptest.NewRecorder()

	handler.Login(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestOIDCHandler_Callback_NotEnabled(t *testing.T) {
	db := setupTestDBForOIDC(t)
	testDB := &database.Database{DB: db}

	cfg := &config.AuthConfig{
		OIDCEnabled: false,
	}

	handler := NewOIDCHandler(testDB, cfg)

	req := httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?code=test&state=test", nil)
	w := httptest.NewRecorder()

	handler.Callback(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestOIDCHandler_GetConfig(t *testing.T) {
	db := setupTestDBForOIDC(t)
	testDB := &database.Database{DB: db}

	cfg := &config.AuthConfig{
		OIDCEnabled: false,
	}

	handler := NewOIDCHandler(testDB, cfg)

	req := httptest.NewRequest(http.MethodGet, "/auth/oidc/config", nil)
	w := httptest.NewRecorder()

	handler.GetConfig(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
		t.Errorf("expected status %d or %d, got %d", http.StatusOK, http.StatusNotFound, w.Code)
	}
}
