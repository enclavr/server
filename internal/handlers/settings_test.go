package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func setupTestDBForSettings(t *testing.T) *gorm.DB {
	db := openTestDB(t)

	err := db.AutoMigrate(&models.ServerSettings{})
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupSettingsHandler(t *testing.T) (*SettingsHandler, *database.Database) {
	db := setupTestDBForSettings(t)
	testDB := &database.Database{DB: db}
	handler := NewSettingsHandler(testDB)

	return handler, testDB
}

func TestSettingsHandler_GetSettings(t *testing.T) {
	handler, _ := setupSettingsHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	w := httptest.NewRecorder()

	handler.GetSettings(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var settings models.ServerSettings
	if err := json.Unmarshal(w.Body.Bytes(), &settings); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if settings.ServerName != "Enclavr Server" {
		t.Errorf("expected server name 'Enclavr Server', got '%s'", settings.ServerName)
	}
}

func TestSettingsHandler_UpdateSettings(t *testing.T) {
	handler, db := setupSettingsHandler(t)

	settings := models.ServerSettings{
		ServerName: "Original Name",
	}
	db.Create(&settings)

	adminUserID := uuid.New()

	type UpdateRequest struct {
		ServerName string `json:"server_name"`
	}

	tests := []struct {
		name           string
		body           UpdateRequest
		expectedStatus int
		isAdmin        bool
		userID         uuid.UUID
	}{
		{
			name: "valid update as admin",
			body: UpdateRequest{
				ServerName: "Updated Name",
			},
			expectedStatus: http.StatusOK,
			isAdmin:        true,
			userID:         adminUserID,
		},
		{
			name: "forbidden for non-admin",
			body: UpdateRequest{
				ServerName: "Updated Name",
			},
			expectedStatus: http.StatusForbidden,
			isAdmin:        false,
			userID:         adminUserID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPut, "/settings", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			ctx := req.Context()
			ctx = context.WithValue(ctx, middleware.UserIDKey, tt.userID)
			ctx = context.WithValue(ctx, middleware.IsAdminKey, tt.isAdmin)
			req = req.WithContext(ctx)
			w := httptest.NewRecorder()

			handler.UpdateSettings(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d, body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}
