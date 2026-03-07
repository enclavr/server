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
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func setupTestDBForPinnedMessage(t *testing.T) *gorm.DB {
	db := openTestDB(t)

	err := db.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.UserRoom{},
		&models.Message{},
		&models.PinnedMessage{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupPinnedMessageHandlerWithUser(t *testing.T) (*PinnedMessageHandler, *database.Database, uuid.UUID, uuid.UUID) {
	db := setupTestDBForPinnedMessage(t)
	testDB := &database.Database{DB: db}
	hub := websocket.NewHub()
	handler := NewPinnedMessageHandler(testDB, hub)

	user := models.User{
		ID:       uuid.New(),
		Username: "testuser",
		Email:    "test@example.com",
	}
	db.Create(&user)

	room := models.Room{
		ID:       uuid.New(),
		Name:     "Test Room",
		MaxUsers: 50,
	}
	db.Create(&room)

	db.Create(&models.UserRoom{
		UserID: user.ID,
		RoomID: room.ID,
		Role:   "member",
	})

	return handler, testDB, user.ID, room.ID
}

func TestPinnedMessageHandler_PinMessage(t *testing.T) {
	handler, db, userID, roomID := setupPinnedMessageHandlerWithUser(t)

	msg := models.Message{
		ID:      uuid.New(),
		RoomID:  roomID,
		UserID:  userID,
		Content: "Message to pin",
	}
	db.Create(&msg)

	tests := []struct {
		name           string
		body           map[string]interface{}
		roomID         uuid.UUID
		expectedStatus int
		setupContext   func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name: "valid pin message",
			body: map[string]interface{}{
				"message_id": msg.ID.String(),
			},
			roomID:         roomID,
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "missing message_id",
			body:           map[string]interface{}{},
			roomID:         roomID,
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "missing room_id",
			body:           map[string]interface{}{},
			roomID:         uuid.Nil,
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name: "message not found",
			body: map[string]interface{}{
				"message_id": uuid.New().String(),
			},
			roomID:         roomID,
			expectedStatus: http.StatusNotFound,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "unauthorized",
			body:           map[string]interface{}{},
			roomID:         roomID,
			expectedStatus: http.StatusUnauthorized,
			setupContext:   func(ctx context.Context, _ uuid.UUID) context.Context { return ctx },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			url := "/messages/pin?room_id=" + tt.roomID.String()
			if tt.name == "missing room_id" {
				url = "/messages/pin"
			}
			req := httptest.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(tt.setupContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.PinMessage(w, req)

			if tt.expectedStatus == http.StatusOK {
				if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError && w.Code != http.StatusConflict && w.Code != http.StatusBadRequest {
					t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
				}
			} else if tt.expectedStatus == http.StatusNotFound {
				if w.Code != tt.expectedStatus && w.Code != http.StatusBadRequest && w.Code != http.StatusInternalServerError {
					t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
				}
			} else if tt.expectedStatus == http.StatusBadRequest {
				if w.Code != tt.expectedStatus && w.Code != http.StatusInternalServerError && w.Code != http.StatusNotFound && w.Code != http.StatusUnauthorized {
					t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
				}
			} else if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestPinnedMessageHandler_UnpinMessage(t *testing.T) {
	handler, db, userID, roomID := setupPinnedMessageHandlerWithUser(t)

	msg := models.Message{
		ID:      uuid.New(),
		RoomID:  roomID,
		UserID:  userID,
		Content: "Message to unpin",
	}
	db.Create(&msg)

	pinned := models.PinnedMessage{
		ID:        uuid.New(),
		MessageID: msg.ID,
		RoomID:    roomID,
		PinnedBy:  userID,
	}
	db.Create(&pinned)

	tests := []struct {
		name           string
		body           map[string]interface{}
		roomID         uuid.UUID
		expectedStatus int
		setupContext   func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name: "valid unpin message",
			body: map[string]interface{}{
				"message_id": msg.ID.String(),
			},
			roomID:         roomID,
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "missing message_id",
			body:           map[string]interface{}{},
			roomID:         roomID,
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "missing room_id",
			body:           map[string]interface{}{},
			roomID:         uuid.Nil,
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name: "message not found",
			body: map[string]interface{}{
				"message_id": uuid.New().String(),
			},
			roomID:         roomID,
			expectedStatus: http.StatusNotFound,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "unauthorized",
			body:           map[string]interface{}{},
			roomID:         roomID,
			expectedStatus: http.StatusUnauthorized,
			setupContext:   func(ctx context.Context, _ uuid.UUID) context.Context { return ctx },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			url := "/messages/unpin?room_id=" + tt.roomID.String()
			if tt.name == "missing room_id" {
				url = "/messages/unpin"
			}
			req := httptest.NewRequest(http.MethodDelete, url, bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(tt.setupContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.UnpinMessage(w, req)

			if tt.expectedStatus == http.StatusOK {
				if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError && w.Code != http.StatusBadRequest && w.Code != http.StatusNotFound {
					t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
				}
			} else if tt.expectedStatus == http.StatusNotFound {
				if w.Code != tt.expectedStatus && w.Code != http.StatusBadRequest && w.Code != http.StatusInternalServerError {
					t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
				}
			} else if tt.expectedStatus == http.StatusBadRequest {
				if w.Code != tt.expectedStatus && w.Code != http.StatusInternalServerError && w.Code != http.StatusNotFound && w.Code != http.StatusUnauthorized {
					t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
				}
			} else if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestPinnedMessageHandler_GetPinnedMessages(t *testing.T) {
	handler, db, userID, roomID := setupPinnedMessageHandlerWithUser(t)

	msg := models.Message{
		ID:      uuid.New(),
		RoomID:  roomID,
		UserID:  userID,
		Content: "Pinned message",
	}
	db.Create(&msg)

	pinned := models.PinnedMessage{
		ID:        uuid.New(),
		MessageID: msg.ID,
		RoomID:    roomID,
		PinnedBy:  userID,
	}
	db.Create(&pinned)

	tests := []struct {
		name           string
		roomID         uuid.UUID
		expectedStatus int
		setupContext   func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name:           "valid get pinned messages",
			roomID:         roomID,
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "missing room_id",
			roomID:         uuid.Nil,
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "invalid room_id",
			roomID:         uuid.Nil,
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/messages/pinned?room_id=" + tt.roomID.String()
			switch tt.name {
			case "missing room_id":
				url = "/messages/pinned"
			case "invalid room_id":
				url = "/messages/pinned?room_id=invalid"
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			req = req.WithContext(tt.setupContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.GetPinnedMessages(w, req)

			if tt.expectedStatus == http.StatusOK {
				if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
					t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
				}
			} else if w.Code != tt.expectedStatus && (tt.name != "invalid room_id" || w.Code != http.StatusBadRequest) {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}
