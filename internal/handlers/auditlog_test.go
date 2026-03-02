package handlers

import (
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

func setupTestDBForAudit(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(getTestDSN()), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	err = db.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.AuditLog{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupAuditHandlerWithUser(t *testing.T, isAdmin bool) (*AuditHandler, *database.Database, uuid.UUID) {
	db := setupTestDBForAudit(t)
	testDB := &database.Database{DB: db}
	handler := NewAuditHandler(testDB)

	user := models.User{
		ID:       uuid.New(),
		Username: "adminuser",
		Email:    "admin@example.com",
		IsAdmin:  isAdmin,
	}
	db.Create(&user)

	return handler, testDB, user.ID
}

func TestAuditHandler_GetAuditLogs(t *testing.T) {
	handler, db, adminID := setupAuditHandlerWithUser(t, true)

	room := models.Room{
		ID:   uuid.New(),
		Name: "Test Room",
	}
	db.Create(&room)

	for i := 0; i < 10; i++ {
		log := models.AuditLog{
			UserID:     adminID,
			Action:     models.AuditActionRoomCreate,
			TargetType: "room",
			TargetID:   room.ID,
			Details:    "Joined room",
		}
		db.Create(&log)
	}

	tests := []struct {
		name           string
		queryParams    string
		expectedStatus int
		userID         uuid.UUID
		expectLogs     bool
	}{
		{
			name:           "get audit logs as admin",
			queryParams:    "",
			expectedStatus: http.StatusOK,
			userID:         adminID,
			expectLogs:     true,
		},
		{
			name:           "get audit logs with pagination",
			queryParams:    "page=1&page_size=5",
			expectedStatus: http.StatusOK,
			userID:         adminID,
			expectLogs:     true,
		},
		{
			name:           "get audit logs with action filter",
			queryParams:    "action=room_create",
			expectedStatus: http.StatusOK,
			userID:         adminID,
			expectLogs:     true,
		},
		{
			name:           "invalid page parameter",
			queryParams:    "page=-1",
			expectedStatus: http.StatusOK,
			userID:         adminID,
			expectLogs:     true,
		},
		{
			name:           "invalid page_size parameter",
			queryParams:    "page_size=200",
			expectedStatus: http.StatusOK,
			userID:         adminID,
			expectLogs:     true,
		},
		{
			name:           "unauthorized - user not in context",
			queryParams:    "",
			expectedStatus: http.StatusUnauthorized,
			userID:         uuid.Nil,
			expectLogs:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/audit?"+tt.queryParams, nil)
			var ctx context.Context
			if tt.userID == uuid.Nil {
				ctx = req.Context()
			} else {
				ctx = context.WithValue(req.Context(), middleware.UserIDKey, tt.userID)
			}
			req = req.WithContext(ctx)
			w := httptest.NewRecorder()

			handler.GetAuditLogs(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectLogs && tt.expectedStatus == http.StatusOK {
				var response AuditLogListResponse
				if err := decodeJSON(w.Body.Bytes(), &response); err != nil {
					t.Fatalf("failed to unmarshal response: %v", err)
				}
				if response.Total == 0 {
					t.Errorf("expected logs, got empty response")
				}
			}
		})
	}
}

func TestAuditHandler_GetAuditLogs_NonAdmin(t *testing.T) {
	handler, db, _ := setupAuditHandlerWithUser(t, false)

	nonAdminUser := models.User{
		ID:       uuid.New(),
		Username: "regularuser",
		Email:    "user@example.com",
		IsAdmin:  false,
	}
	db.Create(&nonAdminUser)

	req := httptest.NewRequest(http.MethodGet, "/api/audit", nil)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, nonAdminUser.ID)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.GetAuditLogs(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}

func TestAuditHandler_GetAuditLogs_UserNotFound(t *testing.T) {
	handler, _, _ := setupAuditHandlerWithUser(t, true)

	nonExistentUser := uuid.New()

	req := httptest.NewRequest(http.MethodGet, "/api/audit", nil)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, nonExistentUser)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.GetAuditLogs(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func decodeJSON(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

func TestAuditHandler_LogAction(t *testing.T) {
	handler, db, adminID := setupAuditHandlerWithUser(t, true)

	room := models.Room{
		ID:   uuid.New(),
		Name: "Test Room",
	}
	db.Create(&room)

	handler.LogAction(adminID, models.AuditActionRoomCreate, "room", room.ID, "Created a test room", "192.168.1.1")

	var logs []models.AuditLog
	db.Find(&logs)

	if len(logs) != 1 {
		t.Errorf("expected 1 audit log, got %d", len(logs))
	}

	if logs[0].UserID != adminID {
		t.Errorf("expected user ID %s, got %s", adminID, logs[0].UserID)
	}

	if logs[0].Action != models.AuditActionRoomCreate {
		t.Errorf("expected action %s, got %s", models.AuditActionRoomCreate, logs[0].Action)
	}

	if logs[0].TargetType != "room" {
		t.Errorf("expected target type 'room', got %s", logs[0].TargetType)
	}

	if logs[0].IPAddress != "192.168.1.1" {
		t.Errorf("expected IP address '192.168.1.1', got %s", logs[0].IPAddress)
	}
}
