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

func setupTestDBForReaction(t *testing.T) *gorm.DB {
	db := openTestDB(t)

	err := db.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.UserRoom{},
		&models.Message{},
		&models.MessageReaction{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupReactionHandler(t *testing.T) (*ReactionHandler, *database.Database, uuid.UUID, uuid.UUID, uuid.UUID) {
	db := setupTestDBForReaction(t)
	testDB := &database.Database{DB: db}
	hub := websocket.NewHub()
	handler := NewReactionHandler(testDB, hub)

	user := models.User{
		ID:       uuid.New(),
		Username: "testuser",
		Email:    "test@example.com",
	}
	room := models.Room{
		ID:       uuid.New(),
		Name:     "Test Room",
		MaxUsers: 50,
	}
	message := models.Message{
		ID:      uuid.New(),
		RoomID:  room.ID,
		UserID:  user.ID,
		Content: "Test message",
	}
	db.Create(&user)
	db.Create(&room)
	db.Create(&message)

	return handler, testDB, user.ID, room.ID, message.ID
}

func TestReactionHandler_AddReaction(t *testing.T) {
	handler, _, userID, _, messageID := setupReactionHandler(t)

	tests := []struct {
		name           string
		body           AddReactionRequest
		expectedStatus int
		userID         uuid.UUID
	}{
		{
			name: "valid reaction",
			body: AddReactionRequest{
				MessageID: messageID,
				Emoji:     "👍",
			},
			expectedStatus: http.StatusOK,
			userID:         userID,
		},
		{
			name: "missing emoji",
			body: AddReactionRequest{
				MessageID: messageID,
				Emoji:     "",
			},
			expectedStatus: http.StatusBadRequest,
			userID:         userID,
		},
		{
			name: "message not found",
			body: AddReactionRequest{
				MessageID: uuid.New(),
				Emoji:     "👍",
			},
			expectedStatus: http.StatusNotFound,
			userID:         userID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/reaction/add", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(addUserIDToContext(req.Context(), tt.userID))
			w := httptest.NewRecorder()

			handler.AddReaction(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d, body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestReactionHandler_AddReaction_Duplicate(t *testing.T) {
	handler, db, userID, _, messageID := setupReactionHandler(t)

	reaction := models.MessageReaction{
		ID:        uuid.New(),
		MessageID: messageID,
		UserID:    userID,
		Emoji:     "👍",
	}
	db.Create(&reaction)

	body, _ := json.Marshal(AddReactionRequest{
		MessageID: messageID,
		Emoji:     "👍",
	})
	req := httptest.NewRequest(http.MethodPost, "/reaction/add", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.AddReaction(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected status %d, got %d", http.StatusConflict, w.Code)
	}
}

func TestReactionHandler_RemoveReaction(t *testing.T) {
	handler, db, userID, _, messageID := setupReactionHandler(t)

	reaction := models.MessageReaction{
		ID:        uuid.New(),
		MessageID: messageID,
		UserID:    userID,
		Emoji:     "👍",
	}
	db.Create(&reaction)

	tests := []struct {
		name           string
		messageID      string
		emoji          string
		expectedStatus int
		userID         uuid.UUID
	}{
		{
			name:           "valid remove",
			messageID:      messageID.String(),
			emoji:          "👍",
			expectedStatus: http.StatusOK,
			userID:         userID,
		},
		{
			name:           "missing message_id",
			messageID:      "",
			emoji:          "👍",
			expectedStatus: http.StatusBadRequest,
			userID:         userID,
		},
		{
			name:           "missing emoji",
			messageID:      messageID.String(),
			emoji:          "",
			expectedStatus: http.StatusBadRequest,
			userID:         userID,
		},
		{
			name:           "message not found",
			messageID:      uuid.New().String(),
			emoji:          "👍",
			expectedStatus: http.StatusNotFound,
			userID:         userID,
		},
		{
			name:           "reaction not found",
			messageID:      messageID.String(),
			emoji:          "😄",
			expectedStatus: http.StatusNotFound,
			userID:         userID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodDelete, "/reaction/remove?message_id="+tt.messageID+"&emoji="+tt.emoji, nil)
			req = req.WithContext(addUserIDToContext(req.Context(), tt.userID))
			w := httptest.NewRecorder()

			handler.RemoveReaction(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d, body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestReactionHandler_GetReactions(t *testing.T) {
	handler, db, userID, _, messageID := setupReactionHandler(t)

	reaction := models.MessageReaction{
		ID:        uuid.New(),
		MessageID: messageID,
		UserID:    userID,
		Emoji:     "👍",
	}
	db.Create(&reaction)

	tests := []struct {
		name           string
		messageID      string
		expectedStatus int
		userID         uuid.UUID
		setupContext   func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name:           "valid get reactions",
			messageID:      messageID.String(),
			expectedStatus: http.StatusOK,
			userID:         userID,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "missing message_id",
			messageID:      "",
			expectedStatus: http.StatusBadRequest,
			userID:         userID,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "invalid message_id",
			messageID:      "invalid",
			expectedStatus: http.StatusBadRequest,
			userID:         userID,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "unauthorized",
			messageID:      messageID.String(),
			expectedStatus: http.StatusUnauthorized,
			userID:         userID,
			setupContext:   func(ctx context.Context, _ uuid.UUID) context.Context { return ctx },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/reactions?message_id="+tt.messageID, nil)
			req = req.WithContext(tt.setupContext(req.Context(), tt.userID))
			w := httptest.NewRecorder()

			handler.GetReactions(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d, body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestReactionHandler_GetReactions_MultipleUsers(t *testing.T) {
	handler, db, userID, _, messageID := setupReactionHandler(t)

	otherUser := models.User{
		ID:       uuid.New(),
		Username: "otheruser",
		Email:    "other@example.com",
	}
	db.Create(&otherUser)

	reaction1 := models.MessageReaction{
		ID:        uuid.New(),
		MessageID: messageID,
		UserID:    userID,
		Emoji:     "👍",
	}
	db.Create(&reaction1)

	reaction2 := models.MessageReaction{
		ID:        uuid.New(),
		MessageID: messageID,
		UserID:    otherUser.ID,
		Emoji:     "👍",
	}
	db.Create(&reaction2)

	reaction3 := models.MessageReaction{
		ID:        uuid.New(),
		MessageID: messageID,
		UserID:    userID,
		Emoji:     "❤️",
	}
	db.Create(&reaction3)

	req := httptest.NewRequest(http.MethodGet, "/reactions?message_id="+messageID.String(), nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetReactions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var reactions []ReactionWithCount
	if err := json.Unmarshal(w.Body.Bytes(), &reactions); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(reactions) != 2 {
		t.Errorf("expected 2 reaction groups, got %d", len(reactions))
	}
}
