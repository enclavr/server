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

func setupTestDBForDMReadReceipt(t *testing.T) *gorm.DB {
	db := openTestDB(t)

	err := db.AutoMigrate(
		&models.User{},
		&models.DirectMessage{},
		&models.DMReadReceipt{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupDMReadReceiptHandler(t *testing.T) (*DMReadReceiptHandler, *database.Database, uuid.UUID, uuid.UUID, uuid.UUID) {
	db := setupTestDBForDMReadReceipt(t)
	testDB := &database.Database{DB: db}
	dmHub := websocket.NewDMHub()
	dmHub.Shutdown()
	handler := NewDMReadReceiptHandler(testDB, dmHub)

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

func TestDMReadReceiptHandler_MarkRead(t *testing.T) {
	handler, _, senderID, receiverID, dmID := setupDMReadReceiptHandler(t)

	tests := []struct {
		name           string
		body           DMMarkReadRequest
		expectedStatus int
		userID         uuid.UUID
	}{
		{
			name: "valid mark read by sender",
			body: DMMarkReadRequest{
				MessageID: dmID,
			},
			expectedStatus: http.StatusOK,
			userID:         senderID,
		},
		{
			name: "valid mark read by receiver",
			body: DMMarkReadRequest{
				MessageID: dmID,
			},
			expectedStatus: http.StatusOK,
			userID:         receiverID,
		},
		{
			name: "message not found",
			body: DMMarkReadRequest{
				MessageID: uuid.New(),
			},
			expectedStatus: http.StatusNotFound,
			userID:         senderID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/dm/read", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(addUserIDToContext(req.Context(), tt.userID))
			w := httptest.NewRecorder()

			handler.MarkRead(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d, body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestDMReadReceiptHandler_MarkRead_Idempotent(t *testing.T) {
	handler, db, senderID, _, dmID := setupDMReadReceiptHandler(t)

	receipt := models.DMReadReceipt{
		DirectMessageID: dmID,
		UserID:          senderID,
	}
	db.Create(&receipt)

	body, _ := json.Marshal(DMMarkReadRequest{MessageID: dmID})
	req := httptest.NewRequest(http.MethodPost, "/dm/read", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(addUserIDToContext(req.Context(), senderID))
	w := httptest.NewRecorder()

	handler.MarkRead(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d, body: %s", http.StatusOK, w.Code, w.Body.String())
	}
}

func TestDMReadReceiptHandler_GetReadStatus(t *testing.T) {
	handler, db, senderID, receiverID, dmID := setupDMReadReceiptHandler(t)

	receipt := models.DMReadReceipt{
		DirectMessageID: dmID,
		UserID:          receiverID,
	}
	db.Create(&receipt)

	tests := []struct {
		name           string
		peerID         string
		expectedStatus int
		userID         uuid.UUID
	}{
		{
			name:           "valid get read status",
			peerID:         receiverID.String(),
			expectedStatus: http.StatusOK,
			userID:         senderID,
		},
		{
			name:           "missing peer_id",
			peerID:         "",
			expectedStatus: http.StatusBadRequest,
			userID:         senderID,
		},
		{
			name:           "invalid peer_id",
			peerID:         "invalid",
			expectedStatus: http.StatusBadRequest,
			userID:         senderID,
		},
		{
			name:           "peer not found",
			peerID:         uuid.New().String(),
			expectedStatus: http.StatusNotFound,
			userID:         senderID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/dm/read/status?"
			if tt.peerID != "" {
				url += "peer_id=" + tt.peerID
			}

			req := httptest.NewRequest(http.MethodGet, url, nil)
			req = req.WithContext(addUserIDToContext(req.Context(), tt.userID))
			w := httptest.NewRecorder()

			handler.GetReadStatus(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d, body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}

			if tt.expectedStatus == http.StatusOK {
				var response DMConversationReadStatus
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Fatalf("failed to unmarshal response: %v", err)
				}
				if response.UserID != receiverID {
					t.Errorf("expected user_id %s, got %s", receiverID, response.UserID)
				}
			}
		})
	}
}

func TestDMReadReceiptHandler_MarkAllRead(t *testing.T) {
	handler, db, senderID, receiverID, _ := setupDMReadReceiptHandler(t)

	dm2 := models.DirectMessage{
		ID:         uuid.New(),
		SenderID:   receiverID,
		ReceiverID: senderID,
		Content:    "Second message",
	}
	db.Create(&dm2)

	tests := []struct {
		name           string
		peerID         string
		expectedStatus int
		userID         uuid.UUID
	}{
		{
			name:           "valid mark all read",
			peerID:         receiverID.String(),
			expectedStatus: http.StatusOK,
			userID:         senderID,
		},
		{
			name:           "missing peer_id",
			peerID:         "",
			expectedStatus: http.StatusBadRequest,
			userID:         senderID,
		},
		{
			name:           "invalid peer_id",
			peerID:         "invalid",
			expectedStatus: http.StatusBadRequest,
			userID:         senderID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/dm/read/all?"
			if tt.peerID != "" {
				url += "peer_id=" + tt.peerID
			}

			req := httptest.NewRequest(http.MethodPost, url, nil)
			req = req.WithContext(addUserIDToContext(req.Context(), tt.userID))
			w := httptest.NewRecorder()

			handler.MarkAllRead(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d, body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}
