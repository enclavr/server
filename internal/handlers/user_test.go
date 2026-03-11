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

func setupTestDBForUserHandler(t *testing.T) *gorm.DB {
	db := openTestDB(t)

	err := db.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.Message{},
		&models.DirectMessage{},
		&models.UserRoom{},
		&models.MessageReaction{},
	)
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

func TestUserHandler_GetProfile(t *testing.T) {
	db := setupTestDBForUserHandler(t)
	testDB := &database.Database{DB: db}
	handler := NewUserHandler(testDB)

	testUser := models.User{
		Username:    "profileuser",
		Email:       "profile@example.com",
		DisplayName: "Profile User",
		AvatarURL:   "https://example.com/avatar.png",
		IsAdmin:     true,
	}
	db.Create(&testUser)

	room := models.Room{
		ID:   uuid.Must(uuid.Parse("11111111-1111-1111-1111-111111111111")),
		Name: "Test Room",
	}
	db.Create(&room)

	userRoom := models.UserRoom{
		UserID: testUser.ID,
		RoomID: room.ID,
	}
	db.Create(&userRoom)

	message := models.Message{
		ID:      uuid.Must(uuid.Parse("22222222-2222-2222-2222-222222222222")),
		UserID:  testUser.ID,
		RoomID:  room.ID,
		Content: "Test message",
	}
	db.Create(&message)

	dm := models.DirectMessage{
		ID:         uuid.Must(uuid.Parse("33333333-3333-3333-3333-333333333333")),
		SenderID:   testUser.ID,
		ReceiverID: testUser.ID,
		Content:    "Test DM",
	}
	db.Create(&dm)

	reaction := models.MessageReaction{
		ID:        uuid.Must(uuid.Parse("44444444-4444-4444-4444-444444444444")),
		UserID:    testUser.ID,
		MessageID: message.ID,
		Emoji:     "👍",
	}
	db.Create(&reaction)

	req := httptest.NewRequest(http.MethodGet, "/user/profile", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, testUser.ID))

	w := httptest.NewRecorder()
	handler.GetProfile(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var profile UserProfileResponse
	if err := json.Unmarshal(w.Body.Bytes(), &profile); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if profile.Username != "profileuser" {
		t.Errorf("expected username 'profileuser', got '%s'", profile.Username)
	}
	if profile.Email != "profile@example.com" {
		t.Errorf("expected email 'profile@example.com', got '%s'", profile.Email)
	}
	if !profile.IsAdmin {
		t.Error("expected IsAdmin to be true")
	}
	if profile.Stats.RoomsJoined != 1 {
		t.Errorf("expected rooms_joined to be 1, got %d", profile.Stats.RoomsJoined)
	}
	if profile.Stats.MessagesSent != 1 {
		t.Errorf("expected messages_sent to be 1, got %d", profile.Stats.MessagesSent)
	}
	if profile.Stats.DMsReceived != 1 {
		t.Errorf("expected dms_received to be 1, got %d", profile.Stats.DMsReceived)
	}
	if profile.Stats.ReactionsGiven != 1 {
		t.Errorf("expected reactions_given to be 1, got %d", profile.Stats.ReactionsGiven)
	}
}

func TestUserHandler_GetProfile_Unauthorized(t *testing.T) {
	handler := setupUserHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/user/profile", nil)
	w := httptest.NewRecorder()

	handler.GetProfile(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}
