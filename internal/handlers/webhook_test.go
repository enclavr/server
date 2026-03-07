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

func setupTestDBForWebhook(t *testing.T) *gorm.DB {
	db := openTestDB(t)

	err := db.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.Webhook{},
		&models.WebhookLog{},
		&models.UserRoom{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupWebhookHandler(t *testing.T) (*WebhookHandler, *database.Database, uuid.UUID, uuid.UUID) {
	db := setupTestDBForWebhook(t)
	testDB := &database.Database{DB: db}
	handler := NewWebhookHandler(testDB)

	user := models.User{
		ID:       uuid.New(),
		Username: "adminuser",
		Email:    "admin@example.com",
	}
	db.Create(&user)

	room := models.Room{
		ID:   uuid.New(),
		Name: "Test Room",
	}
	db.Create(&room)

	userRoom := models.UserRoom{
		UserID: user.ID,
		RoomID: room.ID,
		Role:   "owner",
	}
	db.Create(&userRoom)

	return handler, testDB, user.ID, room.ID
}

func TestWebhookHandler_CreateWebhook(t *testing.T) {
	handler, _, userID, roomID := setupWebhookHandler(t)

	tests := []struct {
		name           string
		path           string
		body           CreateWebhookRequest
		expectedStatus int
		userID         uuid.UUID
	}{
		{
			name: "valid webhook creation",
			path: "/api/webhook/create/" + roomID.String(),
			body: CreateWebhookRequest{
				URL:    "https://example.com/webhook",
				Events: []string{"message_create", "user_join"},
			},
			expectedStatus: http.StatusOK,
			userID:         userID,
		},
		{
			name:           "missing room id",
			path:           "/api/webhook/create/",
			body:           CreateWebhookRequest{},
			expectedStatus: http.StatusBadRequest,
			userID:         userID,
		},
		{
			name: "invalid room id",
			path: "/api/webhook/create/invalid-uuid",
			body: CreateWebhookRequest{
				URL:    "https://example.com/webhook",
				Events: []string{"message_create"},
			},
			expectedStatus: http.StatusBadRequest,
			userID:         userID,
		},
		{
			name: "forbidden - not owner or admin",
			path: "/api/webhook/create/" + roomID.String(),
			body: CreateWebhookRequest{
				URL:    "https://example.com/webhook",
				Events: []string{"message_create"},
			},
			expectedStatus: http.StatusForbidden,
			userID:         uuid.New(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, tt.path, bytes.NewReader(body))
			req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, tt.userID))
			w := httptest.NewRecorder()

			handler.CreateWebhook(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestWebhookHandler_GetWebhooks(t *testing.T) {
	handler, db, userID, roomID := setupWebhookHandler(t)

	webhook := models.Webhook{
		ID:       uuid.New(),
		RoomID:   roomID,
		URL:      "https://example.com/webhook",
		Secret:   "secret",
		Events:   "message_create",
		IsActive: true,
	}
	db.Create(&webhook)

	tests := []struct {
		name           string
		path           string
		expectedStatus int
		userID         uuid.UUID
	}{
		{
			name:           "get webhooks for room",
			path:           "/api/webhook/room/" + roomID.String(),
			expectedStatus: http.StatusOK,
			userID:         userID,
		},
		{
			name:           "missing room id",
			path:           "/api/webhook/room/",
			expectedStatus: http.StatusBadRequest,
			userID:         userID,
		},
		{
			name:           "invalid room id",
			path:           "/api/webhook/room/invalid-uuid",
			expectedStatus: http.StatusBadRequest,
			userID:         userID,
		},
		{
			name:           "forbidden - no access",
			path:           "/api/webhook/room/" + roomID.String(),
			expectedStatus: http.StatusForbidden,
			userID:         uuid.New(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, tt.userID))
			w := httptest.NewRecorder()

			handler.GetWebhooks(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestWebhookHandler_DeleteWebhook(t *testing.T) {
	handler, db, userID, roomID := setupWebhookHandler(t)

	regularUser := models.User{
		ID:       uuid.New(),
		Username: "regularuser",
		Email:    "regular@example.com",
	}
	db.Create(&regularUser)

	regularUserRoom := models.UserRoom{
		UserID: regularUser.ID,
		RoomID: roomID,
		Role:   "member",
	}
	db.Create(&regularUserRoom)

	webhook := models.Webhook{
		ID:       uuid.New(),
		RoomID:   roomID,
		URL:      "https://example.com/webhook",
		Secret:   "secret",
		Events:   "message_create",
		IsActive: true,
	}
	db.Create(&webhook)

	webhookForPermissionTest := models.Webhook{
		ID:       uuid.New(),
		RoomID:   roomID,
		URL:      "https://example.com/webhook2",
		Secret:   "secret2",
		Events:   "message_create",
		IsActive: true,
	}
	db.Create(&webhookForPermissionTest)

	webhookID := webhook.ID.String()
	webhookForPermissionID := webhookForPermissionTest.ID.String()

	tests := []struct {
		name           string
		webhookID      string
		expectedStatus int
		userID         uuid.UUID
		setupWebhook   func() string
	}{
		{
			name:           "delete webhook",
			webhookID:      webhookID,
			expectedStatus: http.StatusOK,
			userID:         userID,
			setupWebhook:   func() string { return webhookID },
		},
		{
			name:           "webhook not found",
			webhookID:      uuid.New().String(),
			expectedStatus: http.StatusNotFound,
			userID:         userID,
			setupWebhook:   func() string { return uuid.New().String() },
		},
		{
			name:           "invalid webhook id",
			webhookID:      "invalid-uuid",
			expectedStatus: http.StatusBadRequest,
			userID:         userID,
			setupWebhook:   func() string { return "invalid-uuid" },
		},
		{
			name:           "forbidden - not owner or admin",
			webhookID:      webhookForPermissionID,
			expectedStatus: http.StatusForbidden,
			userID:         regularUser.ID,
			setupWebhook:   func() string { return webhookForPermissionID },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodDelete, "/api/webhook/"+tt.webhookID, nil)
			req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, tt.userID))
			w := httptest.NewRecorder()

			handler.DeleteWebhook(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestWebhookHandler_ToggleWebhook(t *testing.T) {
	handler, db, userID, roomID := setupWebhookHandler(t)

	webhook := models.Webhook{
		ID:       uuid.New(),
		RoomID:   roomID,
		URL:      "https://example.com/webhook",
		Secret:   "secret",
		Events:   "message_create",
		IsActive: true,
	}
	db.Create(&webhook)

	tests := []struct {
		name           string
		path           string
		expectedStatus int
		userID         uuid.UUID
		expectedActive bool
	}{
		{
			name:           "toggle webhook off",
			path:           "/api/webhook/toggle/" + webhook.ID.String(),
			expectedStatus: http.StatusOK,
			userID:         userID,
			expectedActive: false,
		},
		{
			name:           "webhook not found",
			path:           "/api/webhook/toggle/" + uuid.New().String(),
			expectedStatus: http.StatusNotFound,
			userID:         userID,
			expectedActive: false,
		},
		{
			name:           "invalid webhook id",
			path:           "/api/webhook/toggle/invalid-uuid",
			expectedStatus: http.StatusBadRequest,
			userID:         userID,
			expectedActive: false,
		},
		{
			name:           "forbidden - not owner or admin",
			path:           "/api/webhook/toggle/" + webhook.ID.String(),
			expectedStatus: http.StatusForbidden,
			userID:         uuid.New(),
			expectedActive: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, tt.path, nil)
			req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, tt.userID))
			w := httptest.NewRecorder()

			handler.ToggleWebhook(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedStatus == http.StatusOK && tt.expectedActive {
				var response map[string]bool
				if err := json.Unmarshal(w.Body.Bytes(), &response); err == nil {
					if response["is_active"] != tt.expectedActive {
						t.Errorf("expected is_active=%v, got %v", tt.expectedActive, response["is_active"])
					}
				}
			}
		})
	}
}

func TestWebhookHandler_GetWebhookLogs(t *testing.T) {
	handler, db, userID, roomID := setupWebhookHandler(t)

	webhook := models.Webhook{
		ID:       uuid.New(),
		RoomID:   roomID,
		URL:      "https://example.com/webhook",
		Secret:   "secret",
		Events:   "message_create",
		IsActive: true,
	}
	db.Create(&webhook)

	webhookLog := models.WebhookLog{
		WebhookID:  webhook.ID,
		Event:      "message_create",
		Payload:    `{"test": "data"}`,
		StatusCode: 200,
		Success:    true,
	}
	db.Create(&webhookLog)

	tests := []struct {
		name           string
		path           string
		expectedStatus int
		userID         uuid.UUID
	}{
		{
			name:           "get webhook logs",
			path:           "/api/webhook/logs/" + webhook.ID.String(),
			expectedStatus: http.StatusOK,
			userID:         userID,
		},
		{
			name:           "missing webhook id",
			path:           "/api/webhook/logs/",
			expectedStatus: http.StatusBadRequest,
			userID:         userID,
		},
		{
			name:           "invalid webhook id",
			path:           "/api/webhook/logs/invalid-uuid",
			expectedStatus: http.StatusBadRequest,
			userID:         userID,
		},
		{
			name:           "webhook not found",
			path:           "/api/webhook/logs/" + uuid.New().String(),
			expectedStatus: http.StatusNotFound,
			userID:         userID,
		},
		{
			name:           "forbidden - no access",
			path:           "/api/webhook/logs/" + webhook.ID.String(),
			expectedStatus: http.StatusForbidden,
			userID:         uuid.New(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, tt.userID))
			w := httptest.NewRecorder()

			handler.GetWebhookLogs(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestWebhookHandler_TriggerEvent_NoWebhooks(t *testing.T) {
	handler, _, _, _ := setupWebhookHandler(t)

	roomID := uuid.New()
	handler.TriggerEvent(roomID, "message_create", map[string]string{"test": "data"})
}

func TestWebhookHandler_TriggerEvent_WithWebhooks(t *testing.T) {
	_, db, _, roomID := setupWebhookHandler(t)

	webhook := models.Webhook{
		ID:       uuid.New(),
		RoomID:   roomID,
		URL:      "https://example.com/webhook",
		Secret:   "secret",
		Events:   "message_create",
		IsActive: true,
	}
	db.Create(&webhook)

	handler := NewWebhookHandler(&database.Database{DB: db.DB})
	handler.TriggerEvent(roomID, "message_create", map[string]string{"test": "data"})
}

func TestWebhookHandler_TriggerEvent_WrongEvent(t *testing.T) {
	_, db, _, roomID := setupWebhookHandler(t)

	webhook := models.Webhook{
		ID:       uuid.New(),
		RoomID:   roomID,
		URL:      "https://example.com/webhook",
		Secret:   "secret",
		Events:   "user_join",
		IsActive: true,
	}
	db.Create(&webhook)

	handler := NewWebhookHandler(&database.Database{DB: db.DB})
	handler.TriggerEvent(roomID, "message_create", map[string]string{"test": "data"})
}

func TestGenerateSecret(t *testing.T) {
	secret1 := generateSecret()
	secret2 := generateSecret()

	if secret1 == "" {
		t.Error("expected non-empty secret")
	}

	if secret1 == secret2 {
		t.Error("expected different secrets")
	}
}

func TestWebhookPayload_Structure(t *testing.T) {
	payload := WebhookPayload{
		Event:     "message_create",
		RoomID:    uuid.New().String(),
		Timestamp: "2024-01-01T00:00:00Z",
		Data:      map[string]string{"key": "value"},
	}

	if payload.Event != "message_create" {
		t.Errorf("expected Event message_create, got %s", payload.Event)
	}

	if payload.Data == nil {
		t.Error("expected non-nil Data")
	}
}
