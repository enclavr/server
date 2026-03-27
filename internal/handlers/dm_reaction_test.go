package handlers

import (
	"bytes"
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

func setupTestDBForDMReaction(t *testing.T) *gorm.DB {
	db := openTestDB(t)

	err := db.AutoMigrate(
		&models.User{},
		&models.DirectMessage{},
		&models.DMReaction{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupDMReactionHandler(t *testing.T) (*DMReactionHandler, *database.Database, uuid.UUID, uuid.UUID, uuid.UUID) {
	db := setupTestDBForDMReaction(t)
	testDB := &database.Database{DB: db}
	dmHub := websocket.NewDMHub()
	dmHub.Shutdown()
	handler := NewDMReactionHandler(testDB, dmHub)

	sender := models.User{
		ID:       uuid.New(),
		Username: "sender",
		Email:    "sender@example.com",
	}
	receiver := models.User{
		ID:       uuid.New(),
		Username: "receiver",
		Email:    "receiver@example.com",
	}
	db.Create(&sender)
	db.Create(&receiver)

	dm := models.DirectMessage{
		ID:         uuid.New(),
		SenderID:   sender.ID,
		ReceiverID: receiver.ID,
		Content:    "Test message",
	}
	db.Create(&dm)

	return handler, testDB, sender.ID, receiver.ID, dm.ID
}

func TestDMReactionHandler_AddReaction(t *testing.T) {
	handler, _, senderID, receiverID, dmID := setupDMReactionHandler(t)

	tests := []struct {
		name           string
		body           DMAddReactionRequest
		expectedStatus int
		userID         uuid.UUID
	}{
		{
			name: "valid reaction by sender",
			body: DMAddReactionRequest{
				MessageID: dmID,
				Emoji:     "👍",
			},
			expectedStatus: http.StatusOK,
			userID:         senderID,
		},
		{
			name: "valid reaction by receiver",
			body: DMAddReactionRequest{
				MessageID: dmID,
				Emoji:     "❤️",
			},
			expectedStatus: http.StatusOK,
			userID:         receiverID,
		},
		{
			name: "empty emoji",
			body: DMAddReactionRequest{
				MessageID: dmID,
				Emoji:     "",
			},
			expectedStatus: http.StatusBadRequest,
			userID:         senderID,
		},
		{
			name: "message not found",
			body: DMAddReactionRequest{
				MessageID: uuid.New(),
				Emoji:     "👍",
			},
			expectedStatus: http.StatusNotFound,
			userID:         senderID,
		},
		{
			name: "duplicate reaction",
			body: DMAddReactionRequest{
				MessageID: dmID,
				Emoji:     "👍",
			},
			expectedStatus: http.StatusConflict,
			userID:         senderID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/dm/reaction/add", bytes.NewBuffer(body))
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

func TestDMReactionHandler_RemoveReaction(t *testing.T) {
	handler, db, senderID, _, dmID := setupDMReactionHandler(t)

	reaction := models.DMReaction{
		DirectMessageID: dmID,
		UserID:          senderID,
		Emoji:           "👍",
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
			messageID:      dmID.String(),
			emoji:          "👍",
			expectedStatus: http.StatusOK,
			userID:         senderID,
		},
		{
			name:           "missing message_id",
			messageID:      "",
			emoji:          "👍",
			expectedStatus: http.StatusBadRequest,
			userID:         senderID,
		},
		{
			name:           "missing emoji",
			messageID:      dmID.String(),
			emoji:          "",
			expectedStatus: http.StatusBadRequest,
			userID:         senderID,
		},
		{
			name:           "reaction not found",
			messageID:      dmID.String(),
			emoji:          "❌",
			expectedStatus: http.StatusNotFound,
			userID:         senderID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/dm/reaction/remove?"
			if tt.messageID != "" {
				url += "message_id=" + tt.messageID + "&"
			}
			if tt.emoji != "" {
				url += "emoji=" + tt.emoji
			}

			req := httptest.NewRequest(http.MethodDelete, url, nil)
			req = req.WithContext(addUserIDToContext(req.Context(), tt.userID))
			w := httptest.NewRecorder()

			handler.RemoveReaction(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d, body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestDMReactionHandler_GetReactions(t *testing.T) {
	handler, db, senderID, receiverID, dmID := setupDMReactionHandler(t)

	reactions := []models.DMReaction{
		{DirectMessageID: dmID, UserID: senderID, Emoji: "👍"},
		{DirectMessageID: dmID, UserID: receiverID, Emoji: "👍"},
		{DirectMessageID: dmID, UserID: senderID, Emoji: "❤️"},
	}
	for _, r := range reactions {
		db.Create(&r)
	}

	tests := []struct {
		name           string
		messageID      string
		expectedStatus int
		userID         uuid.UUID
		expectedCount  int
	}{
		{
			name:           "valid get reactions",
			messageID:      dmID.String(),
			expectedStatus: http.StatusOK,
			userID:         senderID,
			expectedCount:  2,
		},
		{
			name:           "missing message_id",
			messageID:      "",
			expectedStatus: http.StatusBadRequest,
			userID:         senderID,
		},
		{
			name:           "message not found",
			messageID:      uuid.New().String(),
			expectedStatus: http.StatusNotFound,
			userID:         senderID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/dm/reactions?"
			if tt.messageID != "" {
				url += "message_id=" + tt.messageID
			}

			req := httptest.NewRequest(http.MethodGet, url, nil)
			req = req.WithContext(addUserIDToContext(req.Context(), tt.userID))
			w := httptest.NewRecorder()

			handler.GetReactions(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d, body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}

			if tt.expectedStatus == http.StatusOK {
				var response []DMReactionWithCount
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Fatalf("failed to unmarshal response: %v", err)
				}
				if len(response) != tt.expectedCount {
					t.Errorf("expected %d reaction groups, got %d", tt.expectedCount, len(response))
				}
			}
		})
	}
}
