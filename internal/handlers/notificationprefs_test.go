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

func setupNotificationPrefsDB(t *testing.T) *gorm.DB {
	db := openTestDB(t)

	err := db.AutoMigrate(
		&models.User{},
		&models.NotificationPreferences{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupNotificationPrefsTest(t *testing.T) (*NotificationPreferencesHandler, *database.Database, uuid.UUID) {
	db := setupNotificationPrefsDB(t)
	testDB := &database.Database{DB: db}
	handler := NewNotificationPreferencesHandler(testDB)

	user := models.User{
		ID:       uuid.New(),
		Username: "testuser",
		Email:    "test@example.com",
	}
	db.Create(&user)

	return handler, testDB, user.ID
}

func TestNotificationPreferencesHandler_GetPreferences(t *testing.T) {
	handler, _, userID := setupNotificationPrefsTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/notification-preferences", nil)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.GetPreferences(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var result NotificationPreferencesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if result.UserID != userID {
		t.Errorf("expected user_id %s, got %s", userID, result.UserID)
	}

	if !result.MentionNotifications {
		t.Error("expected mention_notifications to be true by default")
	}

	if !result.SoundEnabled {
		t.Error("expected sound_enabled to be true by default")
	}

	if result.QuietHoursEnabled {
		t.Error("expected quiet_hours_enabled to be false by default")
	}
}

func TestNotificationPreferencesHandler_UpdatePreferences(t *testing.T) {
	handler, _, userID := setupNotificationPrefsTest(t)

	soundEnabled := false
	dmNotifications := "mentions"
	quietHoursEnabled := true
	quietHoursStart := "23:00"

	reqBody := UpdateNotificationPreferencesRequest{
		SoundEnabled:      &soundEnabled,
		DMNotifications:   &dmNotifications,
		QuietHoursEnabled: &quietHoursEnabled,
		QuietHoursStart:   &quietHoursStart,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPut, "/api/notification-preferences/update", bytes.NewReader(body))
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler.UpdatePreferences(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var result NotificationPreferencesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if result.SoundEnabled {
		t.Error("expected sound_enabled to be false after update")
	}

	if result.DMNotifications != "mentions" {
		t.Errorf("expected dm_notifications 'mentions', got '%s'", result.DMNotifications)
	}

	if !result.QuietHoursEnabled {
		t.Error("expected quiet_hours_enabled to be true after update")
	}

	if result.QuietHoursStart != "23:00" {
		t.Errorf("expected quiet_hours_start '23:00', got '%s'", result.QuietHoursStart)
	}
}

func TestNotificationPreferencesHandler_GetPreferences_Unauthorized(t *testing.T) {
	handler, _, _ := setupNotificationPrefsTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/notification-preferences", nil)

	w := httptest.NewRecorder()
	handler.GetPreferences(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}
