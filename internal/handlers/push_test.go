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

func setupTestDBForPush(t *testing.T) *gorm.DB {
	db := openTestDB(t)

	err := db.AutoMigrate(
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
	handler, testDB, userID := setupPushHandler(t)

	subscription := models.PushSubscription{
		ID:       uuid.New(),
		UserID:   userID,
		Endpoint: "https://example.com/push",
		P256DH:   "test-p256dh",
		Auth:     "test-auth",
		IsActive: true,
	}
	testDB.Create(&subscription)

	tests := []struct {
		name           string
		subscriptionID string
		expectedStatus int
	}{
		{
			name:           "successful unsubscribe",
			subscriptionID: subscription.ID.String(),
			expectedStatus: http.StatusOK,
		},
		{
			name:           "invalid subscription ID",
			subscriptionID: "invalid-id",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "missing subscription ID",
			subscriptionID: "",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "subscription not found",
			subscriptionID: uuid.New().String(),
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/api/push/" + tt.subscriptionID
			if tt.name == "missing subscription ID" {
				url = "/api/push/"
			}
			req := httptest.NewRequest(http.MethodDelete, url, nil)
			req = req.WithContext(context.WithValue(context.Background(), middleware.UserIDKey, userID))

			w := httptest.NewRecorder()
			handler.Unsubscribe(w, req)

			if tt.name == "missing subscription ID" {
				if w.Code != tt.expectedStatus && w.Code != http.StatusBadRequest {
					t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
				}
			} else if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestPushHandler_GetSubscriptions(t *testing.T) {
	handler, testDB, userID := setupPushHandler(t)

	subscription := models.PushSubscription{
		ID:       uuid.New(),
		UserID:   userID,
		Endpoint: "https://example.com/push",
		P256DH:   "test-p256dh",
		Auth:     "test-auth",
		IsActive: true,
	}
	testDB.Create(&subscription)

	req := httptest.NewRequest(http.MethodGet, "/subscriptions", nil)
	req = req.WithContext(context.WithValue(context.Background(), middleware.UserIDKey, userID))

	w := httptest.NewRecorder()
	handler.GetSubscriptions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestPushHandler_TestNotification(t *testing.T) {
	handler, testDB, userID := setupPushHandler(t)

	subscription := models.PushSubscription{
		ID:       uuid.New(),
		UserID:   userID,
		Endpoint: "https://example.com/push",
		P256DH:   "test-p256dh",
		Auth:     "test-auth",
		IsActive: true,
	}
	testDB.Create(&subscription)

	tests := []struct {
		name           string
		expectedStatus int
		setupUser      func() uuid.UUID
	}{
		{
			name:           "successful test notification",
			expectedStatus: http.StatusOK,
			setupUser:      func() uuid.UUID { return userID },
		},
		{
			name:           "no subscriptions",
			expectedStatus: http.StatusBadRequest,
			setupUser:      func() uuid.UUID { return uuid.New() },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/test", nil)
			req = req.WithContext(context.WithValue(context.Background(), middleware.UserIDKey, tt.setupUser()))

			w := httptest.NewRecorder()
			handler.TestNotification(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
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
