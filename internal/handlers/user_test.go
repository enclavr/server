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
	"gorm.io/gorm"
)

func setupTestDBForUserHandler(t *testing.T) *gorm.DB {
	db := openTestDB(t)

	err := db.AutoMigrate(&models.User{})
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupUserHandler(t *testing.T) *UserHandler {
	db := setupTestDBForUserHandler(t)
	testDB := &database.Database{DB: db}
	handler := NewUserHandler(testDB)
	return handler
}

func TestUserHandler_SearchUsers(t *testing.T) {
	handler := setupUserHandler(t)

	testUser := models.User{
		Username:    "testuser",
		DisplayName: "Test User",
	}
	handler.db.Create(&testUser)

	tests := []struct {
		name           string
		query          string
		expectedStatus int
		expectedCount  int
	}{
		{
			name:           "valid search with results",
			query:          "test",
			expectedStatus: http.StatusOK,
			expectedCount:  1,
		},
		{
			name:           "search with no results",
			query:          "nonexistent",
			expectedStatus: http.StatusOK,
			expectedCount:  0,
		},
		{
			name:           "missing query parameter",
			query:          "",
			expectedStatus: http.StatusBadRequest,
			expectedCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/users/search?username="+tt.query, nil)
			w := httptest.NewRecorder()

			handler.SearchUsers(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedStatus == http.StatusOK {
				var results []UserSearchResult
				if err := json.Unmarshal(w.Body.Bytes(), &results); err != nil {
					t.Fatalf("failed to unmarshal response: %v", err)
				}
				if len(results) != tt.expectedCount {
					t.Errorf("expected %d results, got %d", tt.expectedCount, len(results))
				}
			}
		})
	}
}

func TestUserHandler_UpdateUser(t *testing.T) {
	handler := setupUserHandler(t)

	testUser := models.User{
		Username:    "updateuser",
		DisplayName: "Original Name",
	}
	handler.db.Create(&testUser)

	tests := []struct {
		name           string
		updateReq      UpdateUserRequest
		expectedStatus int
		checkResult    func(t *testing.T, updatedUser models.User)
	}{
		{
			name: "update display name",
			updateReq: UpdateUserRequest{
				DisplayName: "New Name",
			},
			expectedStatus: http.StatusOK,
			checkResult: func(t *testing.T, updatedUser models.User) {
				if updatedUser.DisplayName != "New Name" {
					t.Errorf("expected display name 'New Name', got '%s'", updatedUser.DisplayName)
				}
			},
		},
		{
			name: "update avatar URL",
			updateReq: UpdateUserRequest{
				AvatarURL: "https://example.com/avatar.png",
			},
			expectedStatus: http.StatusOK,
			checkResult: func(t *testing.T, updatedUser models.User) {
				if updatedUser.AvatarURL != "https://example.com/avatar.png" {
					t.Errorf("expected avatar URL 'https://example.com/avatar.png', got '%s'", updatedUser.AvatarURL)
				}
			},
		},
		{
			name: "update both fields",
			updateReq: UpdateUserRequest{
				DisplayName: "Updated Name",
				AvatarURL:   "https://example.com/new-avatar.png",
			},
			expectedStatus: http.StatusOK,
			checkResult: func(t *testing.T, updatedUser models.User) {
				if updatedUser.DisplayName != "Updated Name" {
					t.Errorf("expected display name 'Updated Name', got '%s'", updatedUser.DisplayName)
				}
				if updatedUser.AvatarURL != "https://example.com/new-avatar.png" {
					t.Errorf("expected avatar URL 'https://example.com/new-avatar.png', got '%s'", updatedUser.AvatarURL)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(tt.updateReq)
			if err != nil {
				t.Fatalf("failed to marshal request: %v", err)
			}

			req := httptest.NewRequest(http.MethodPut, "/user/update", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, testUser.ID))

			w := httptest.NewRecorder()
			handler.UpdateUser(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedStatus == http.StatusOK {
				var updatedUser models.User
				if err := json.Unmarshal(w.Body.Bytes(), &updatedUser); err != nil {
					t.Fatalf("failed to unmarshal response: %v", err)
				}
				tt.checkResult(t, updatedUser)
			}
		})
	}
}
