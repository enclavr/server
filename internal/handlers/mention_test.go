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
	"github.com/enclavr/server/internal/services"
	"github.com/enclavr/server/internal/websocket"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func setupMentionTestDB(t *testing.T) *gorm.DB {
	db := openTestDB(t)

	err := db.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.UserRoom{},
		&models.Message{},
		&models.MessageMention{},
		&models.Notification{},
		&models.Presence{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupMentionHandlerTest(t *testing.T) (*MentionHandler, *MessageHandler, *database.Database, uuid.UUID, uuid.UUID, uuid.UUID) {
	db := setupMentionTestDB(t)
	testDB := &database.Database{DB: db}
	hub := websocket.NewHub()

	mentionHandler := NewMentionHandler(testDB)
	messageHandler := NewMessageHandler(testDB, hub)

	user1 := models.User{
		ID:       uuid.New(),
		Username: "alice",
		Email:    "alice@example.com",
	}
	user2 := models.User{
		ID:       uuid.New(),
		Username: "bob",
		Email:    "bob@example.com",
	}
	db.Create(&user1)
	db.Create(&user2)

	room := models.Room{
		ID:   uuid.New(),
		Name: "test-room",
	}
	db.Create(&room)

	db.Create(&models.UserRoom{UserID: user1.ID, RoomID: room.ID, Role: "member"})
	db.Create(&models.UserRoom{UserID: user2.ID, RoomID: room.ID, Role: "member"})

	return mentionHandler, messageHandler, testDB, user1.ID, user2.ID, room.ID
}

func mentionContext(ctx context.Context, userID uuid.UUID) context.Context {
	return context.WithValue(ctx, middleware.UserIDKey, userID)
}

func TestMentionHandler_GetUserMentions(t *testing.T) {
	handler, _, db, user1ID, user2ID, roomID := setupMentionHandlerTest(t)

	msg := models.Message{
		ID:      uuid.New(),
		RoomID:  roomID,
		UserID:  user2ID,
		Content: "Hello @alice",
	}
	db.Create(&msg)

	mention := models.MessageMention{
		ID:          uuid.New(),
		MessageID:   msg.ID,
		RoomID:      roomID,
		UserID:      user1ID,
		MentionedBy: user2ID,
		Type:        models.MentionTypeUser,
	}
	db.Create(&mention)

	tests := []struct {
		name           string
		userID         uuid.UUID
		expectedStatus int
	}{
		{
			name:           "get mentions for user",
			userID:         user1ID,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "no mentions for user",
			userID:         user2ID,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "unauthorized",
			userID:         uuid.Nil,
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/mentions", nil)
			if tt.userID != uuid.Nil {
				req = req.WithContext(mentionContext(req.Context(), tt.userID))
			}
			w := httptest.NewRecorder()

			handler.GetUserMentions(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedStatus == http.StatusOK && tt.userID == user1ID {
				var mentions []MentionResponse
				if err := json.Unmarshal(w.Body.Bytes(), &mentions); err != nil {
					t.Fatalf("failed to unmarshal response: %v", err)
				}
				if len(mentions) != 1 {
					t.Errorf("expected 1 mention, got %d", len(mentions))
				}
			}
		})
	}
}

func TestMentionHandler_GetMessageMentions(t *testing.T) {
	handler, _, db, user1ID, user2ID, roomID := setupMentionHandlerTest(t)

	msg := models.Message{
		ID:      uuid.New(),
		RoomID:  roomID,
		UserID:  user2ID,
		Content: "Hello @alice",
	}
	db.Create(&msg)

	mention := models.MessageMention{
		ID:          uuid.New(),
		MessageID:   msg.ID,
		RoomID:      roomID,
		UserID:      user1ID,
		MentionedBy: user2ID,
		Type:        models.MentionTypeUser,
	}
	db.Create(&mention)

	tests := []struct {
		name           string
		messageID      string
		expectedStatus int
	}{
		{
			name:           "valid message mentions",
			messageID:      msg.ID.String(),
			expectedStatus: http.StatusOK,
		},
		{
			name:           "missing message_id",
			messageID:      "",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "invalid message_id",
			messageID:      "invalid",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/api/mentions/message"
			if tt.messageID != "" {
				url += "?message_id=" + tt.messageID
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			req = req.WithContext(mentionContext(req.Context(), user1ID))
			w := httptest.NewRecorder()

			handler.GetMessageMentions(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestMentionService_ParseMentions(t *testing.T) {
	db := setupMentionTestDB(t)
	testDB := &database.Database{DB: db}
	service := services.NewMentionService(testDB)

	user1 := models.User{
		ID:       uuid.New(),
		Username: "alice",
		Email:    "alice@example.com",
	}
	user2 := models.User{
		ID:       uuid.New(),
		Username: "bob",
		Email:    "bob@example.com",
	}
	db.Create(&user1)
	db.Create(&user2)

	room := models.Room{
		ID:   uuid.New(),
		Name: "test-room",
	}
	db.Create(&room)

	db.Create(&models.UserRoom{UserID: user1.ID, RoomID: room.ID, Role: "member"})
	db.Create(&models.UserRoom{UserID: user2.ID, RoomID: room.ID, Role: "member"})

	tests := []struct {
		name          string
		content       string
		expectedCount int
	}{
		{
			name:          "single mention",
			content:       "Hello @alice",
			expectedCount: 1,
		},
		{
			name:          "multiple mentions",
			content:       "Hello @alice and @bob",
			expectedCount: 1,
		},
		{
			name:          "no mentions",
			content:       "Hello world",
			expectedCount: 0,
		},
		{
			name:          "self mention filtered",
			content:       "Hello @bob",
			expectedCount: 0,
		},
		{
			name:          "nonexistent user mention",
			content:       "Hello @charlie",
			expectedCount: 0,
		},
		{
			name:          "duplicate mentions",
			content:       "Hello @alice @alice",
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mentions, err := service.ParseMentions(tt.content, room.ID, user2.ID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(mentions) != tt.expectedCount {
				t.Errorf("expected %d mentions, got %d", tt.expectedCount, len(mentions))
			}
		})
	}
}

func TestSendMessage_WithMentions(t *testing.T) {
	_, messageHandler, db, user1ID, user2ID, roomID := setupMentionHandlerTest(t)

	msg := SendMessageRequest{
		RoomID:  roomID,
		Content: "Hello @alice, check this out!",
	}

	body, _ := json.Marshal(msg)
	req := httptest.NewRequest(http.MethodPost, "/api/message/send", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(mentionContext(req.Context(), user2ID))
	w := httptest.NewRecorder()

	messageHandler.SendMessage(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d, body: %s", http.StatusOK, w.Code, w.Body.String())
		return
	}

	var messageCount int64
	db.Model(&models.Message{}).Count(&messageCount)
	if messageCount != 1 {
		t.Errorf("expected 1 message, got %d", messageCount)
	}

	var mentionCount int64
	db.Model(&models.MessageMention{}).Count(&mentionCount)
	if mentionCount != 1 {
		t.Errorf("expected 1 mention record, got %d", mentionCount)
		var mentions []models.MessageMention
		db.Find(&mentions)
		for _, m := range mentions {
			t.Logf("mention: userID=%s, mentionedBy=%s, type=%s", m.UserID, m.MentionedBy, m.Type)
		}
		t.Logf("user1ID=%s, user2ID=%s, roomID=%s", user1ID, user2ID, roomID)
		var userRooms []models.UserRoom
		db.Find(&userRooms)
		for _, ur := range userRooms {
			t.Logf("userRoom: userID=%s, roomID=%s, role=%s", ur.UserID, ur.RoomID, ur.Role)
		}
	}
}
