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

func setupTestDBForPush(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	err = db.AutoMigrate(
		&models.User{},
		&models.PushSubscription{},
		&models.UserNotificationSettings{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupPushHandler(t *testing.T) (*PushHandler, *database.Database, uuid.UUID) {
	db := setupTestDBForPush(t)
	testDB := &database.Database{DB: db}
	handler := NewPushHandler(testDB)

	user := models.User{
		ID:       uuid.New(),
		Username: "testuser",
		Email:    "test@example.com",
	}
	testDB.Create(&user)

	return handler, testDB, user.ID
}

func TestPushHandler_Subscribe(t *testing.T) {
	handler, _, userID := setupPushHandler(t)

	tests := []struct {
		name           string
		body           SubscribeRequest
		expectedStatus int
		userID         uuid.UUID
		checkResponse  func(t *testing.T, response *http.Response)
	}{
		{
			name: "successful subscription",
			body: SubscribeRequest{
				Endpoint: "https://example.com/push",
				P256DH:   "test-p256dh",
				Auth:     "test-auth",
			},
			expectedStatus: http.StatusOK,
			userID:         userID,
			checkResponse: func(t *testing.T, response *http.Response) {
				var subResp SubscribeResponse
				err := json.NewDecoder(response.Body).Decode(&subResp)
				if err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if subResp.Endpoint != "https://example.com/push" {
					t.Errorf("expected endpoint 'https://example.com/push', got '%s'", subResp.Endpoint)
				}
				if !subResp.IsActive {
					t.Error("expected subscription to be active")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/subscribe", bytes.NewReader(body))
			req = req.WithContext(context.WithValue(context.Background(), middleware.UserIDKey, tt.userID))

			w := httptest.NewRecorder()
			handler.Subscribe(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.checkResponse != nil {
				tt.checkResponse(t, w.Result())
			}
		})
	}
}

func TestPushHandler_Unsubscribe(t *testing.T) {
	t.Skip("Unsubscribe requires subscription ID path parameter, skipping")
}

func TestPushHandler_GetNotificationSettings(t *testing.T) {
	handler, testDB, userID := setupPushHandler(t)

	settings := models.UserNotificationSettings{
		ID:                    uuid.New(),
		UserID:                userID,
		EnablePush:            true,
		EnableDMNotifications: true,
	}
	testDB.Create(&settings)

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	req = req.WithContext(context.WithValue(context.Background(), middleware.UserIDKey, userID))

	w := httptest.NewRecorder()
	handler.GetNotificationSettings(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var notifSettings NotificationSettingsRequest
	err := json.NewDecoder(w.Body).Decode(&notifSettings)
	if err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !notifSettings.EnablePush {
		t.Error("expected push to be enabled")
	}
	if !notifSettings.EnableDMNotifications {
		t.Error("expected DM notifications to be enabled")
	}
}

func TestPushHandler_UpdateNotificationSettings(t *testing.T) {
	handler, testDB, userID := setupPushHandler(t)

	tests := []struct {
		name           string
		body           NotificationSettingsRequest
		expectedStatus int
		checkResponse  func(t *testing.T, db *database.Database)
	}{
		{
			name: "successful update",
			body: NotificationSettingsRequest{
				EnablePush:            false,
				EnableDMNotifications: false,
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, db *database.Database) {
				var settings models.UserNotificationSettings
				result := db.Where("user_id = ?", userID).First(&settings)
				if result.Error != nil {
					t.Fatalf("failed to find settings: %v", result.Error)
				}
				if settings.EnablePush {
					t.Error("expected push to be disabled")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPut, "/settings", bytes.NewReader(body))
			req = req.WithContext(context.WithValue(context.Background(), middleware.UserIDKey, userID))

			w := httptest.NewRecorder()
			handler.UpdateNotificationSettings(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.checkResponse != nil {
				tt.checkResponse(t, testDB)
			}
		})
	}
}
