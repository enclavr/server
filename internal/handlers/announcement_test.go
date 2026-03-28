package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func setupAnnouncementTestDB(t *testing.T) *gorm.DB {
	db := openTestDB(t)

	err := db.AutoMigrate(
		&models.User{},
		&models.Announcement{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupAnnouncementTest(t *testing.T) (*AnnouncementHandler, *database.Database, uuid.UUID, uuid.UUID) {
	db := setupAnnouncementTestDB(t)
	testDB := &database.Database{DB: db}
	handler := NewAnnouncementHandler(testDB)

	admin := models.User{
		ID:       uuid.New(),
		Username: "admin",
		Email:    "admin@example.com",
		IsAdmin:  true,
	}
	user := models.User{
		ID:       uuid.New(),
		Username: "user",
		Email:    "user@example.com",
	}
	db.Create(&admin)
	db.Create(&user)

	return handler, testDB, admin.ID, user.ID
}

func TestAnnouncementHandler_CreateAnnouncement(t *testing.T) {
	handler, _, adminID, userID := setupAnnouncementTest(t)

	tests := []struct {
		name           string
		body           CreateAnnouncementRequest
		userID         uuid.UUID
		expectedStatus int
	}{
		{
			name: "admin creates announcement",
			body: CreateAnnouncementRequest{
				Title:    "Test Announcement",
				Content:  "This is a test announcement",
				Priority: models.AnnouncementPriorityNormal,
			},
			userID:         adminID,
			expectedStatus: http.StatusCreated,
		},
		{
			name: "non-admin cannot create",
			body: CreateAnnouncementRequest{
				Title:   "Test",
				Content: "Content",
			},
			userID:         userID,
			expectedStatus: http.StatusForbidden,
		},
		{
			name: "missing title",
			body: CreateAnnouncementRequest{
				Content: "Content",
			},
			userID:         adminID,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "missing content",
			body: CreateAnnouncementRequest{
				Title: "Title",
			},
			userID:         adminID,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "empty body",
			body:           CreateAnnouncementRequest{},
			userID:         adminID,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/announcement/create", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(addUserIDToContext(req.Context(), tt.userID))
			w := httptest.NewRecorder()

			handler.CreateAnnouncement(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d, body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}

			if tt.expectedStatus == http.StatusCreated {
				var resp AnnouncementResponse
				if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
					t.Fatalf("failed to unmarshal response: %v", err)
				}
				if resp.Title != tt.body.Title {
					t.Errorf("expected title '%s', got '%s'", tt.body.Title, resp.Title)
				}
				if !resp.IsActive {
					t.Error("expected announcement to be active")
				}
			}
		})
	}
}

func TestAnnouncementHandler_GetAnnouncements(t *testing.T) {
	handler, db, adminID, userID := setupAnnouncementTest(t)

	announcement := models.Announcement{
		ID:        uuid.New(),
		Title:     "Test Announcement",
		Content:   "Test content",
		Priority:  models.AnnouncementPriorityNormal,
		CreatedBy: adminID,
		IsActive:  true,
	}
	db.Create(&announcement)

	tests := []struct {
		name           string
		userID         uuid.UUID
		expectedStatus int
	}{
		{
			name:           "user can list announcements",
			userID:         userID,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "admin can list announcements",
			userID:         adminID,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "unauthorized",
			userID:         uuid.Nil,
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/announcements", nil)
			if tt.userID != uuid.Nil {
				req = req.WithContext(addUserIDToContext(req.Context(), tt.userID))
			}
			w := httptest.NewRecorder()

			handler.GetAnnouncements(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedStatus == http.StatusOK {
				var announcements []AnnouncementResponse
				if err := json.Unmarshal(w.Body.Bytes(), &announcements); err != nil {
					t.Fatalf("failed to unmarshal response: %v", err)
				}
				if len(announcements) != 1 {
					t.Errorf("expected 1 announcement, got %d", len(announcements))
				}
			}
		})
	}
}

func TestAnnouncementHandler_DeleteAnnouncement(t *testing.T) {
	handler, db, adminID, userID := setupAnnouncementTest(t)

	announcement := models.Announcement{
		ID:        uuid.New(),
		Title:     "To Delete",
		Content:   "Content",
		Priority:  models.AnnouncementPriorityNormal,
		CreatedBy: adminID,
		IsActive:  true,
	}
	db.Create(&announcement)

	tests := []struct {
		name           string
		announcementID string
		userID         uuid.UUID
		expectedStatus int
	}{
		{
			name:           "admin deletes announcement",
			announcementID: announcement.ID.String(),
			userID:         adminID,
			expectedStatus: http.StatusNoContent,
		},
		{
			name:           "non-admin cannot delete",
			announcementID: announcement.ID.String(),
			userID:         userID,
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "missing id",
			announcementID: "",
			userID:         adminID,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/api/announcement/delete"
			if tt.announcementID != "" {
				url += "?id=" + tt.announcementID
			}
			req := httptest.NewRequest(http.MethodDelete, url, nil)
			req = req.WithContext(addUserIDToContext(req.Context(), tt.userID))
			w := httptest.NewRecorder()

			handler.DeleteAnnouncement(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestAnnouncementHandler_DeactivateAnnouncement(t *testing.T) {
	handler, db, adminID, userID := setupAnnouncementTest(t)

	announcement := models.Announcement{
		ID:        uuid.New(),
		Title:     "To Deactivate",
		Content:   "Content",
		Priority:  models.AnnouncementPriorityNormal,
		CreatedBy: adminID,
		IsActive:  true,
	}
	db.Create(&announcement)

	tests := []struct {
		name           string
		announcementID string
		userID         uuid.UUID
		expectedStatus int
	}{
		{
			name:           "admin deactivates",
			announcementID: announcement.ID.String(),
			userID:         adminID,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "non-admin cannot deactivate",
			announcementID: announcement.ID.String(),
			userID:         userID,
			expectedStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/api/announcement/deactivate?id=" + tt.announcementID
			req := httptest.NewRequest(http.MethodPost, url, nil)
			req = req.WithContext(addUserIDToContext(req.Context(), tt.userID))
			w := httptest.NewRecorder()

			handler.DeactivateAnnouncement(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedStatus == http.StatusOK {
				var resp AnnouncementResponse
				if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
					t.Fatalf("failed to unmarshal response: %v", err)
				}
				if resp.IsActive {
					t.Error("expected announcement to be deactivated")
				}
			}
		})
	}
}
