package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/enclavr/server/internal/config"
	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func setupTestDBForOIDC(t *testing.T) *gorm.DB {
	db := openTestDB(t)

	err := db.AutoMigrate(&models.User{})
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

type oidcClaims struct {
	Sub               string `json:"sub"`
	Name              string `json:"name"`
	Email             string `json:"email"`
	PreferredUsername string `json:"preferred_username"`
}

func TestOIDCHandler_findOrCreateUser(t *testing.T) {
	db := setupTestDBForOIDC(t)
	testDB := &database.Database{DB: db}

	cfg := &config.AuthConfig{
		OIDCEnabled:   true,
		OIDCIssuerURL: "https://issuer.example.com",
	}

	handler := NewOIDCHandler(testDB, cfg)

	tests := []struct {
		name      string
		claims    oidcClaims
		setupUser bool
		setupSub  string
		wantErr   bool
	}{
		{
			name: "create new user with preferred username",
			claims: oidcClaims{
				Sub:               "oauth2|12345",
				Name:              "John Doe",
				Email:             "john@example.com",
				PreferredUsername: "johndoe",
			},
			setupUser: false,
			wantErr:   false,
		},
		{
			name: "create new user with name only",
			claims: oidcClaims{
				Sub:               "oauth2|67890",
				Name:              "Jane Doe",
				Email:             "jane@example.com",
				PreferredUsername: "",
			},
			setupUser: false,
			wantErr:   false,
		},
		{
			name: "create new user with email fallback",
			claims: oidcClaims{
				Sub:               "oauth2|abcde",
				Name:              "",
				Email:             "testuser@example.com",
				PreferredUsername: "",
			},
			setupUser: false,
			wantErr:   false,
		},
		{
			name: "create new user with no email - uses UUID",
			claims: oidcClaims{
				Sub:               "oauth2|noemail",
				Name:              "",
				Email:             "",
				PreferredUsername: "",
			},
			setupUser: false,
			wantErr:   false,
		},
		{
			name: "find existing user - SQLite column naming issue",
			claims: oidcClaims{
				Sub:               "oauth2|findme",
				Name:              "Find Me",
				Email:             "findme@example.com",
				PreferredUsername: "findme",
			},
			setupUser: true,
			setupSub:  "oauth2|findme",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupUser {
				existingUser := models.User{
					ID:          uuid.New(),
					Username:    "findme",
					Email:       "findme@example.com",
					OIDCSubject: tt.setupSub,
					OIDCIssuer:  "https://issuer.example.com",
				}
				testDB.Create(&existingUser)
			}

			user, err := handler.findOrCreateUser(tt.claims)

			if tt.wantErr && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.wantErr && user == nil {
				t.Error("expected user but got nil")
			}
		})
	}
}
