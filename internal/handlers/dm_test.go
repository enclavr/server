package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDBForDM(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(getTestDSN()), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	err = db.AutoMigrate(
		&models.User{},
		&models.DirectMessage{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupDMHandler(t *testing.T) (*DirectMessageHandler, *database.Database, uuid.UUID, uuid.UUID) {
	db := setupTestDBForDM(t)
	testDB := &database.Database{DB: db}
	handler := NewDirectMessageHandler(testDB)

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

	return handler, testDB, sender.ID, receiver.ID
}

func TestDirectMessageHandler_SendDM(t *testing.T) {
	handler, _, senderID, receiverID := setupDMHandler(t)

	tests := []struct {
		name           string
		body           SendDMRequest
		expectedStatus int
		userID         uuid.UUID
	}{
		{
			name: "valid DM send",
			body: SendDMRequest{
				ReceiverID: receiverID,
				Content:    "Hello!",
			},
			expectedStatus: http.StatusOK,
			userID:         senderID,
		},
		{
			name: "empty content",
			body: SendDMRequest{
				ReceiverID: receiverID,
				Content:    "",
			},
			expectedStatus: http.StatusBadRequest,
			userID:         senderID,
		},
		{
			name: "send to yourself",
			body: SendDMRequest{
				ReceiverID: senderID,
				Content:    "Hello myself!",
			},
			expectedStatus: http.StatusBadRequest,
			userID:         senderID,
		},
		{
			name: "receiver not found",
			body: SendDMRequest{
				ReceiverID: uuid.New(),
				Content:    "Hello!",
			},
			expectedStatus: http.StatusNotFound,
			userID:         senderID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/dm/send", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(addUserIDToContext(req.Context(), tt.userID))
			w := httptest.NewRecorder()

			handler.SendDM(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d, body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestDirectMessageHandler_GetMessages(t *testing.T) {
	handler, db, senderID, receiverID := setupDMHandler(t)

	dm := models.DirectMessage{
		SenderID:   senderID,
		ReceiverID: receiverID,
		Content:    "Test message",
	}
	db.Create(&dm)

	req := httptest.NewRequest(http.MethodGet, "/dm/messages?user_id="+receiverID.String(), nil)
	req = req.WithContext(addUserIDToContext(req.Context(), senderID))
	w := httptest.NewRecorder()

	handler.GetMessages(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response []DirectMessageResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if len(response) != 1 || response[0].Content != "Test message" {
		t.Errorf("expected 1 message with content 'Test message', got: %+v", response)
	}
}

func TestDirectMessageHandler_GetMessages_InvalidUserID(t *testing.T) {
	handler, _, senderID, _ := setupDMHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/dm/messages?user_id=invalid", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), senderID))
	w := httptest.NewRecorder()

	handler.GetMessages(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestDirectMessageHandler_DeleteDM(t *testing.T) {
	handler, db, senderID, receiverID := setupDMHandler(t)

	dm := models.DirectMessage{
		SenderID:   senderID,
		ReceiverID: receiverID,
		Content:    "Test message",
	}
	db.Create(&dm)

	type DeleteRequest struct {
		MessageID uuid.UUID `json:"message_id"`
	}

	tests := []struct {
		name           string
		body           DeleteRequest
		expectedStatus int
		userID         uuid.UUID
	}{
		{
			name: "valid delete",
			body: DeleteRequest{
				MessageID: dm.ID,
			},
			expectedStatus: http.StatusOK,
			userID:         senderID,
		},
		{
			name: "message not found",
			body: DeleteRequest{
				MessageID: uuid.New(),
			},
			expectedStatus: http.StatusNotFound,
			userID:         senderID,
		},
		{
			name: "unauthorized - not sender",
			body: DeleteRequest{
				MessageID: dm.ID,
			},
			expectedStatus: http.StatusForbidden,
			userID:         receiverID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodDelete, "/dm/delete", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(addUserIDToContext(req.Context(), tt.userID))
			w := httptest.NewRecorder()

			handler.DeleteDM(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d, body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestDirectMessageHandler_UpdateDM(t *testing.T) {
	handler, db, senderID, receiverID := setupDMHandler(t)

	dm := models.DirectMessage{
		SenderID:   senderID,
		ReceiverID: receiverID,
		Content:    "Original message",
	}
	db.Create(&dm)

	type UpdateRequest struct {
		MessageID uuid.UUID `json:"message_id"`
		Content   string    `json:"content"`
	}

	tests := []struct {
		name           string
		body           UpdateRequest
		expectedStatus int
		userID         uuid.UUID
	}{
		{
			name: "valid update",
			body: UpdateRequest{
				MessageID: dm.ID,
				Content:   "Updated message",
			},
			expectedStatus: http.StatusOK,
			userID:         senderID,
		},
		{
			name: "empty content",
			body: UpdateRequest{
				MessageID: dm.ID,
				Content:   "",
			},
			expectedStatus: http.StatusBadRequest,
			userID:         senderID,
		},
		{
			name: "message not found",
			body: UpdateRequest{
				MessageID: uuid.New(),
				Content:   "Updated message",
			},
			expectedStatus: http.StatusNotFound,
			userID:         senderID,
		},
		{
			name: "unauthorized - not sender",
			body: UpdateRequest{
				MessageID: dm.ID,
				Content:   "Updated message",
			},
			expectedStatus: http.StatusForbidden,
			userID:         receiverID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPut, "/dm/update", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(addUserIDToContext(req.Context(), tt.userID))
			w := httptest.NewRecorder()

			handler.UpdateDM(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d, body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestDirectMessageHandler_GetConversations(t *testing.T) {
	handler, db, senderID, receiverID := setupDMHandler(t)

	dm := models.DirectMessage{
		SenderID:   senderID,
		ReceiverID: receiverID,
		Content:    "Test message",
	}
	db.Create(&dm)

	req := httptest.NewRequest(http.MethodGet, "/dm/conversations", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), senderID))
	w := httptest.NewRecorder()

	handler.GetConversations(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}
