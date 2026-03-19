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

func setupTestDBForPrivacy(t *testing.T) *gorm.DB {
	db := openTestDB(t)

	err := db.AutoMigrate(&models.UserPrivacySettings{})
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupPrivacyHandler(t *testing.T) (*PrivacyHandler, *database.Database, uuid.UUID) {
	db := setupTestDBForPrivacy(t)
	testDB := &database.Database{DB: db}
	handler := NewPrivacyHandler(testDB)

	user := models.User{
		ID:       uuid.New(),
		Username: "testuser",
		Email:    "test@example.com",
	}
	db.Create(&user)

	return handler, testDB, user.ID
}

func TestPrivacyHandler_GetPrivacySettings(t *testing.T) {
	handler, _, userID := setupPrivacyHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/privacy", nil)
	ctx := req.Context()
	ctx = context.WithValue(ctx, middleware.UserIDKey, userID)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.GetPrivacySettings(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var settings models.UserPrivacySettings
	if err := json.Unmarshal(w.Body.Bytes(), &settings); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if settings.AllowDirectMessages != "everyone" {
		t.Errorf("expected allow_direct_messages 'everyone', got '%s'", settings.AllowDirectMessages)
	}

	if settings.ShowOnlineStatus != true {
		t.Errorf("expected show_online_status true, got %v", settings.ShowOnlineStatus)
	}
}

func TestPrivacyHandler_UpdatePrivacySettings(t *testing.T) {
	handler, _, userID := setupPrivacyHandler(t)

	tests := []struct {
		name           string
		body           UpdatePrivacySettingsRequest
		expectedStatus int
	}{
		{
			name: "valid update - change allow_direct_messages to friends",
			body: UpdatePrivacySettingsRequest{
				AllowDirectMessages: stringPtr("friends"),
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "valid update - disable online status",
			body: UpdatePrivacySettingsRequest{
				ShowOnlineStatus: boolPtr(false),
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "valid update - allow search by email",
			body: UpdatePrivacySettingsRequest{
				AllowSearchByEmail: boolPtr(true),
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "invalid value for allow_direct_messages",
			body: UpdatePrivacySettingsRequest{
				AllowDirectMessages: stringPtr("invalid"),
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "multiple valid updates",
			body: UpdatePrivacySettingsRequest{
				AllowDirectMessages:   stringPtr("none"),
				AllowRoomInvites:      stringPtr("friends"),
				ShowOnlineStatus:      boolPtr(false),
				ShowReadReceipts:      boolPtr(false),
				AllowSearchByUsername: boolPtr(false),
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPut, "/privacy/update", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			ctx := req.Context()
			ctx = context.WithValue(ctx, middleware.UserIDKey, userID)
			req = req.WithContext(ctx)
			w := httptest.NewRecorder()

			handler.UpdatePrivacySettings(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d, body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestPrivacyHandler_GetPrivacySettingsUnauthorized(t *testing.T) {
	handler, _, _ := setupPrivacyHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/privacy", nil)
	w := httptest.NewRecorder()

	handler.GetPrivacySettings(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestPrivacyHandler_UpdatePrivacySettingsUnauthorized(t *testing.T) {
	handler, _, _ := setupPrivacyHandler(t)

	body, _ := json.Marshal(UpdatePrivacySettingsRequest{
		AllowDirectMessages: stringPtr("friends"),
	})
	req := httptest.NewRequest(http.MethodPut, "/privacy/update", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.UpdatePrivacySettings(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestPrivacyHandler_ResetPrivacySettings(t *testing.T) {
	handler, _, userID := setupPrivacyHandler(t)

	existingSettings := models.UserPrivacySettings{
		UserID:                userID,
		AllowDirectMessages:   "none",
		AllowRoomInvites:      "none",
		AllowVoiceCalls:       "none",
		ShowOnlineStatus:      false,
		ShowReadReceipts:      false,
		ShowTypingIndicator:   false,
		AllowSearchByEmail:    true,
		AllowSearchByUsername: false,
	}
	handler.db.Create(&existingSettings)

	req := httptest.NewRequest(http.MethodPost, "/privacy/reset", nil)
	ctx := req.Context()
	ctx = context.WithValue(ctx, middleware.UserIDKey, userID)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ResetPrivacySettings(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var settings models.UserPrivacySettings
	if err := json.Unmarshal(w.Body.Bytes(), &settings); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if settings.AllowDirectMessages != "everyone" {
		t.Errorf("expected allow_direct_messages 'everyone', got '%s'", settings.AllowDirectMessages)
	}

	if settings.ShowOnlineStatus != true {
		t.Errorf("expected show_online_status true, got %v", settings.ShowOnlineStatus)
	}

	if settings.AllowSearchByUsername != true {
		t.Errorf("expected allow_search_by_username true, got %v", settings.AllowSearchByUsername)
	}
}

func TestPrivacyHandler_ExportPrivacySettings(t *testing.T) {
	handler, _, userID := setupPrivacyHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/privacy/export", nil)
	ctx := req.Context()
	ctx = context.WithValue(ctx, middleware.UserIDKey, userID)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ExportPrivacySettings(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected content-type 'application/json', got '%s'", contentType)
	}

	disposition := w.Header().Get("Content-Disposition")
	if disposition != "attachment; filename=enclavr-privacy.json" {
		t.Errorf("expected content-disposition 'attachment; filename=enclavr-privacy.json', got '%s'", disposition)
	}
}

func TestPrivacyHandler_UpdatePrivacySettingsAllFields(t *testing.T) {
	handler, _, userID := setupPrivacyHandler(t)

	body, _ := json.Marshal(UpdatePrivacySettingsRequest{
		AllowDirectMessages:   stringPtr("friends"),
		AllowRoomInvites:      stringPtr("none"),
		AllowVoiceCalls:       stringPtr("everyone"),
		ShowOnlineStatus:      boolPtr(false),
		ShowReadReceipts:      boolPtr(false),
		ShowTypingIndicator:   boolPtr(false),
		AllowSearchByEmail:    boolPtr(true),
		AllowSearchByUsername: boolPtr(false),
	})
	req := httptest.NewRequest(http.MethodPut, "/privacy/update", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := req.Context()
	ctx = context.WithValue(ctx, middleware.UserIDKey, userID)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.UpdatePrivacySettings(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d, body: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var settings models.UserPrivacySettings
	if err := json.Unmarshal(w.Body.Bytes(), &settings); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if settings.AllowDirectMessages != "friends" {
		t.Errorf("expected allow_direct_messages 'friends', got '%s'", settings.AllowDirectMessages)
	}

	if settings.AllowRoomInvites != "none" {
		t.Errorf("expected allow_room_invites 'none', got '%s'", settings.AllowRoomInvites)
	}

	if settings.AllowVoiceCalls != "everyone" {
		t.Errorf("expected allow_voice_calls 'everyone', got '%s'", settings.AllowVoiceCalls)
	}

	if settings.ShowOnlineStatus != false {
		t.Errorf("expected show_online_status false, got %v", settings.ShowOnlineStatus)
	}

	if settings.ShowReadReceipts != false {
		t.Errorf("expected show_read_receipts false, got %v", settings.ShowReadReceipts)
	}

	if settings.ShowTypingIndicator != false {
		t.Errorf("expected show_typing_indicator false, got %v", settings.ShowTypingIndicator)
	}

	if settings.AllowSearchByEmail != true {
		t.Errorf("expected allow_search_by_email true, got %v", settings.AllowSearchByEmail)
	}

	if settings.AllowSearchByUsername != false {
		t.Errorf("expected allow_search_by_username false, got %v", settings.AllowSearchByUsername)
	}
}

func boolPtr(b bool) *bool {
	return &b
}
