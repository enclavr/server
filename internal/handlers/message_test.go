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
	"github.com/enclavr/server/internal/websocket"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func setupMessageHandlerDB(t *testing.T) *gorm.DB {
	db := openTestDB(t)

	err := db.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.UserRoom{},
		&models.Category{},
		&models.Message{},
		&models.Presence{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupMessageHandlerTest(t *testing.T) (*MessageHandler, *database.Database, uuid.UUID) {
	db := setupMessageHandlerDB(t)
	testDB := &database.Database{DB: db}
	hub := websocket.NewHub()
	handler := NewMessageHandler(testDB, hub)

	user := models.User{
		ID:       uuid.New(),
		Username: "testuser",
		Email:    "test@example.com",
	}
	db.Create(&user)

	room := models.Room{
		ID:   uuid.New(),
		Name: "test-room",
	}
	db.Create(&room)

	userRoom := models.UserRoom{
		UserID: user.ID,
		RoomID: room.ID,
		Role:   "owner",
	}
	db.Create(&userRoom)

	return handler, testDB, user.ID
}

func messageContextWithUserID(ctx context.Context, userID uuid.UUID) context.Context {
	return context.WithValue(ctx, middleware.UserIDKey, userID)
}

func TestMessageHandler_SendMessage(t *testing.T) {
	handler, db, userID := setupMessageHandlerTest(t)

	var room models.Room
	db.First(&room)

	tests := []struct {
		name           string
		body           SendMessageRequest
		expectedStatus int
		setupCtx       func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name: "valid message",
			body: SendMessageRequest{
				RoomID:  room.ID,
				Content: "Hello, world!",
			},
			expectedStatus: http.StatusOK,
			setupCtx:       messageContextWithUserID,
		},
		{
			name: "empty content",
			body: SendMessageRequest{
				RoomID:  room.ID,
				Content: "",
			},
			expectedStatus: http.StatusBadRequest,
			setupCtx:       messageContextWithUserID,
		},
		{
			name: "missing room_id",
			body: SendMessageRequest{
				Content: "Hello",
			},
			expectedStatus: http.StatusBadRequest,
			setupCtx:       messageContextWithUserID,
		},
		{
			name: "non-existent room",
			body: SendMessageRequest{
				RoomID:  uuid.New(),
				Content: "Hello",
			},
			expectedStatus: http.StatusNotFound,
			setupCtx:       messageContextWithUserID,
		},
		{
			name:           "unauthorized",
			body:           SendMessageRequest{},
			expectedStatus: http.StatusUnauthorized,
			setupCtx:       func(ctx context.Context, _ uuid.UUID) context.Context { return ctx },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/messages", bytes.NewReader(body))
			req = req.WithContext(tt.setupCtx(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.SendMessage(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestMessageHandler_GetMessages(t *testing.T) {
	handler, db, userID := setupMessageHandlerTest(t)

	var room models.Room
	db.First(&room)

	msg := models.Message{
		ID:      uuid.New(),
		RoomID:  room.ID,
		UserID:  userID,
		Content: "Test message",
	}
	db.Create(&msg)

	tests := []struct {
		name           string
		roomID         uuid.UUID
		expectedStatus int
		setupCtx       func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name:           "valid room",
			roomID:         room.ID,
			expectedStatus: http.StatusOK,
			setupCtx:       messageContextWithUserID,
		},
		{
			name:           "invalid room uuid",
			roomID:         uuid.Nil,
			expectedStatus: http.StatusBadRequest,
			setupCtx:       messageContextWithUserID,
		},
		{
			name:           "user not in room",
			roomID:         uuid.New(),
			expectedStatus: http.StatusForbidden,
			setupCtx:       messageContextWithUserID,
		},
		{
			name:           "unauthorized",
			roomID:         room.ID,
			expectedStatus: http.StatusUnauthorized,
			setupCtx:       func(ctx context.Context, _ uuid.UUID) context.Context { return ctx },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/messages?room_id="+tt.roomID.String(), nil)
			req = req.WithContext(tt.setupCtx(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.GetMessages(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestMessageHandler_UpdateMessage(t *testing.T) {
	handler, db, userID := setupMessageHandlerTest(t)

	var room models.Room
	db.First(&room)

	msg := models.Message{
		ID:      uuid.New(),
		RoomID:  room.ID,
		UserID:  userID,
		Content: "Original content",
	}
	db.Create(&msg)

	tests := []struct {
		name           string
		messageID      uuid.UUID
		body           UpdateMessageRequest
		expectedStatus int
		setupCtx       func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name:      "valid update",
			messageID: msg.ID,
			body: UpdateMessageRequest{
				Content: "Updated content",
			},
			expectedStatus: http.StatusOK,
			setupCtx:       messageContextWithUserID,
		},
		{
			name:      "empty content",
			messageID: msg.ID,
			body: UpdateMessageRequest{
				Content: "",
			},
			expectedStatus: http.StatusBadRequest,
			setupCtx:       messageContextWithUserID,
		},
		{
			name:      "invalid message uuid",
			messageID: uuid.Nil,
			body: UpdateMessageRequest{
				Content: "Updated",
			},
			expectedStatus: http.StatusNotFound,
			setupCtx:       messageContextWithUserID,
		},
		{
			name:           "unauthorized",
			messageID:      msg.ID,
			body:           UpdateMessageRequest{Content: "Updated"},
			expectedStatus: http.StatusUnauthorized,
			setupCtx:       func(ctx context.Context, _ uuid.UUID) context.Context { return ctx },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPut, "/api/v1/messages?message_id="+tt.messageID.String(), bytes.NewReader(body))
			req = req.WithContext(tt.setupCtx(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.UpdateMessage(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestMessageHandler_DeleteMessage(t *testing.T) {
	handler, db, userID := setupMessageHandlerTest(t)

	var room models.Room
	db.First(&room)

	msg := models.Message{
		ID:      uuid.New(),
		RoomID:  room.ID,
		UserID:  userID,
		Content: "To be deleted",
	}
	db.Create(&msg)

	tests := []struct {
		name           string
		messageID      uuid.UUID
		expectedStatus int
		setupCtx       func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name:           "valid delete",
			messageID:      msg.ID,
			expectedStatus: http.StatusOK,
			setupCtx:       messageContextWithUserID,
		},
		{
			name:           "invalid message uuid",
			messageID:      uuid.Nil,
			expectedStatus: http.StatusNotFound,
			setupCtx:       messageContextWithUserID,
		},
		{
			name:           "message not found",
			messageID:      uuid.New(),
			expectedStatus: http.StatusNotFound,
			setupCtx:       messageContextWithUserID,
		},
		{
			name:           "unauthorized",
			messageID:      msg.ID,
			expectedStatus: http.StatusUnauthorized,
			setupCtx:       func(ctx context.Context, _ uuid.UUID) context.Context { return ctx },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodDelete, "/api/v1/messages?message_id="+tt.messageID.String(), nil)
			req = req.WithContext(tt.setupCtx(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.DeleteMessage(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestMessageHandler_SearchMessages(t *testing.T) {
	handler, _, userID := setupMessageHandlerTest(t)

	tests := []struct {
		name           string
		query          string
		expectedStatus int
		setupCtx       func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name:           "empty query",
			query:          "",
			expectedStatus: http.StatusBadRequest,
			setupCtx:       messageContextWithUserID,
		},
		{
			name:           "unauthorized",
			query:          "test",
			expectedStatus: http.StatusUnauthorized,
			setupCtx:       func(ctx context.Context, _ uuid.UUID) context.Context { return ctx },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/api/v1/messages/search?q=" + tt.query
			req := httptest.NewRequest(http.MethodGet, url, nil)
			req = req.WithContext(tt.setupCtx(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.SearchMessages(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}
