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
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDBForExport(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	err = db.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.Category{},
		&models.Message{},
		&models.Invite{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupExportHandler(t *testing.T) (*ExportHandler, *database.Database, uuid.UUID, uuid.UUID) {
	db := setupTestDBForExport(t)
	testDB := &database.Database{DB: db}
	handler := NewExportHandler(testDB)

	adminUser := models.User{
		ID:       uuid.New(),
		Username: "admin",
		Email:    "admin@example.com",
		IsAdmin:  true,
	}
	db.Create(&adminUser)

	regularUser := models.User{
		ID:       uuid.New(),
		Username: "user",
		Email:    "user@example.com",
		IsAdmin:  false,
	}
	db.Create(&regularUser)

	room := models.Room{
		ID:          uuid.New(),
		Name:        "Test Room",
		Description: "A test room",
		CreatedBy:   adminUser.ID,
	}
	db.Create(&room)

	category := models.Category{
		ID:   uuid.New(),
		Name: "Test Category",
	}
	db.Create(&category)

	message := models.Message{
		ID:      uuid.New(),
		RoomID:  room.ID,
		UserID:  adminUser.ID,
		Content: "Test message",
	}
	db.Create(&message)

	invite := models.Invite{
		ID:        uuid.New(),
		Code:      "testcode123",
		RoomID:    room.ID,
		CreatedBy: adminUser.ID,
	}
	db.Create(&invite)

	return handler, testDB, adminUser.ID, regularUser.ID
}

func TestExportHandler_ExportServer(t *testing.T) {
	handler, _, adminUserID, regularUserID := setupExportHandler(t)

	tests := []struct {
		name           string
		userID         uuid.UUID
		expectedStatus int
		checkAdmin     bool
	}{
		{
			name:           "successful export as admin",
			userID:         adminUserID,
			expectedStatus: http.StatusOK,
			checkAdmin:     true,
		},
		{
			name:           "forbidden for non-admin user",
			userID:         regularUserID,
			expectedStatus: http.StatusForbidden,
			checkAdmin:     false,
		},
		{
			name:           "unauthorized when no user in context",
			userID:         uuid.Nil,
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			ctx := context.Background()

			if tt.userID != uuid.Nil {
				ctx = context.WithValue(ctx, middleware.UserIDKey, tt.userID)
			}

			req = httptest.NewRequest(http.MethodGet, "/api/v1/export", nil)
			req = req.WithContext(ctx)

			w := httptest.NewRecorder()
			handler.ExportServer(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedStatus == http.StatusOK {
				var export ServerExport
				if err := json.Unmarshal(w.Body.Bytes(), &export); err != nil {
					t.Fatalf("failed to unmarshal response: %v", err)
				}

				if len(export.Users) != 2 {
					t.Errorf("expected 2 users, got %d", len(export.Users))
				}

				if len(export.Rooms) != 1 {
					t.Errorf("expected 1 room, got %d", len(export.Rooms))
				}

				if len(export.Categories) != 1 {
					t.Errorf("expected 1 category, got %d", len(export.Categories))
				}

				if len(export.Messages) != 1 {
					t.Errorf("expected 1 message, got %d", len(export.Messages))
				}

				if len(export.Invites) != 1 {
					t.Errorf("expected 1 invite, got %d", len(export.Invites))
				}

				if export.Version != "1.0.0" {
					t.Errorf("expected version 1.0.0, got %s", export.Version)
				}

				if w.Header().Get("Content-Disposition") == "" {
					t.Error("expected Content-Disposition header")
				}
			}
		})
	}

	t.Run("export with no data", func(t *testing.T) {
		db := setupTestDBForExport(t)
		testDB := &database.Database{DB: db}
		handler := NewExportHandler(testDB)

		adminUser := models.User{
			ID:       uuid.New(),
			Username: "admin",
			Email:    "admin@example.com",
			IsAdmin:  true,
		}
		db.Create(&adminUser)

		ctx := context.WithValue(context.Background(), middleware.UserIDKey, adminUser.ID)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/export", nil)
		req = req.WithContext(ctx)

		w := httptest.NewRecorder()
		handler.ExportServer(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
		}

		var export ServerExport
		if err := json.Unmarshal(w.Body.Bytes(), &export); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}

		if len(export.Users) != 1 {
			t.Errorf("expected 1 user, got %d", len(export.Users))
		}
	})
}

func TestExportHandler_UserNotFound(t *testing.T) {
	db := setupTestDBForExport(t)
	testDB := &database.Database{DB: db}
	handler := NewExportHandler(testDB)

	nonExistentUserID := uuid.New()
	ctx := context.WithValue(context.Background(), middleware.UserIDKey, nonExistentUserID)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/export", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ExportServer(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestExportHandler_AdminAccessRequired(t *testing.T) {
	_, testDB, _, regularUserID := setupExportHandler(t)
	handler := NewExportHandler(testDB)

	ctx := context.WithValue(context.Background(), middleware.UserIDKey, regularUserID)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/export", nil)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.ExportServer(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}

	expectedBody := "Admin access required"
	if !bytes.Contains(w.Body.Bytes(), []byte(expectedBody)) {
		t.Errorf("expected body to contain %s, got %s", expectedBody, w.Body.String())
	}
}
