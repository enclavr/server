package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/internal/websocket"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
)

type DMReadReceiptHandler struct {
	db    *database.Database
	dmHub *websocket.DMHub
}

func NewDMReadReceiptHandler(db *database.Database, dmHub *websocket.DMHub) *DMReadReceiptHandler {
	return &DMReadReceiptHandler{db: db, dmHub: dmHub}
}

type DMMarkReadRequest struct {
	MessageID uuid.UUID `json:"message_id"`
}

type DMReadReceiptResponse struct {
	ID              uuid.UUID `json:"id"`
	DirectMessageID uuid.UUID `json:"direct_message_id"`
	UserID          uuid.UUID `json:"user_id"`
	ReadAt          time.Time `json:"read_at"`
}

type DMConversationReadStatus struct {
	UserID        uuid.UUID `json:"user_id"`
	Username      string    `json:"username"`
	LastReadMsgID uuid.UUID `json:"last_read_message_id"`
	ReadAt        time.Time `json:"read_at"`
	UnreadCount   int64     `json:"unread_count"`
}

func (h *DMReadReceiptHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req DMMarkReadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var dm models.DirectMessage
	if err := h.db.First(&dm, "id = ?", req.MessageID).Error; err != nil {
		http.Error(w, "Message not found", http.StatusNotFound)
		return
	}

	if dm.SenderID != userID && dm.ReceiverID != userID {
		http.Error(w, "Not authorized to read this message", http.StatusForbidden)
		return
	}

	var existingReceipt models.DMReadReceipt
	err := h.db.Where("direct_message_id = ? AND user_id = ?",
		req.MessageID, userID).First(&existingReceipt).Error

	if err != nil {
		receipt := &models.DMReadReceipt{
			DirectMessageID: req.MessageID,
			UserID:          userID,
			ReadAt:          time.Now(),
		}

		if err := h.db.Create(receipt).Error; err != nil {
			http.Error(w, "Failed to create read receipt", http.StatusInternalServerError)
			return
		}
		existingReceipt = *receipt
	}

	convID := websocket.GenerateConversationID(dm.SenderID, dm.ReceiverID)
	dmMsg := &websocket.DMMessage{
		Type:           "dm-message-read",
		ConversationID: convID,
		UserID:         userID,
		Timestamp:      time.Now(),
	}
	wsPayload, _ := json.Marshal(map[string]interface{}{
		"message_id": req.MessageID.String(),
		"read_at":    existingReceipt.ReadAt.Format(time.RFC3339),
	})
	dmMsg.Payload = wsPayload
	h.dmHub.Broadcast(dmMsg)

	response := DMReadReceiptResponse{
		ID:              existingReceipt.ID,
		DirectMessageID: existingReceipt.DirectMessageID,
		UserID:          existingReceipt.UserID,
		ReadAt:          existingReceipt.ReadAt,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *DMReadReceiptHandler) GetReadStatus(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	peerIDStr := r.URL.Query().Get("peer_id")
	if peerIDStr == "" {
		http.Error(w, "peer_id is required", http.StatusBadRequest)
		return
	}

	peerID, err := uuid.Parse(peerIDStr)
	if err != nil {
		http.Error(w, "Invalid peer_id", http.StatusBadRequest)
		return
	}

	var peer models.User
	if err := h.db.First(&peer, "id = ?", peerID).Error; err != nil {
		http.Error(w, "Peer user not found", http.StatusNotFound)
		return
	}

	var dms []models.DirectMessage
	if err := h.db.Where(
		"(sender_id = ? AND receiver_id = ?) OR (sender_id = ? AND receiver_id = ?)",
		userID, peerID, peerID, userID,
	).Order("created_at DESC").Limit(100).Find(&dms).Error; err != nil {
		http.Error(w, "Failed to fetch messages", http.StatusInternalServerError)
		return
	}

	if len(dms) == 0 {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode([]DMConversationReadStatus{}); err != nil {
			log.Printf("Error encoding response: %v", err)
		}
		return
	}

	msgIDs := make([]uuid.UUID, 0, len(dms))
	for _, dm := range dms {
		msgIDs = append(msgIDs, dm.ID)
	}

	var receipts []models.DMReadReceipt
	h.db.Where("direct_message_id IN ? AND user_id = ?", msgIDs, peerID).Find(&receipts)

	receiptMap := make(map[uuid.UUID]models.DMReadReceipt)
	var lastReadMsgID uuid.UUID
	var lastReadAt time.Time
	for _, receipt := range receipts {
		receiptMap[receipt.DirectMessageID] = receipt
		if receipt.ReadAt.After(lastReadAt) {
			lastReadAt = receipt.ReadAt
			lastReadMsgID = receipt.DirectMessageID
		}
	}

	var unreadCount int64
	for _, dm := range dms {
		if dm.SenderID == userID {
			if _, read := receiptMap[dm.ID]; !read {
				unreadCount++
			}
		}
	}

	status := DMConversationReadStatus{
		UserID:        peerID,
		Username:      peer.Username,
		LastReadMsgID: lastReadMsgID,
		ReadAt:        lastReadAt,
		UnreadCount:   unreadCount,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(status); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *DMReadReceiptHandler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	peerIDStr := r.URL.Query().Get("peer_id")
	if peerIDStr == "" {
		http.Error(w, "peer_id is required", http.StatusBadRequest)
		return
	}

	peerID, err := uuid.Parse(peerIDStr)
	if err != nil {
		http.Error(w, "Invalid peer_id", http.StatusBadRequest)
		return
	}

	var dms []models.DirectMessage
	if err := h.db.Where(
		"sender_id = ? AND receiver_id = ?",
		peerID, userID,
	).Order("created_at DESC").Limit(100).Find(&dms).Error; err != nil {
		http.Error(w, "Failed to fetch messages", http.StatusInternalServerError)
		return
	}

	now := time.Now()
	for _, dm := range dms {
		var existing models.DMReadReceipt
		err := h.db.Where("direct_message_id = ? AND user_id = ?",
			dm.ID, userID).First(&existing).Error
		if err != nil {
			receipt := &models.DMReadReceipt{
				DirectMessageID: dm.ID,
				UserID:          userID,
				ReadAt:          now,
			}
			h.db.Create(receipt)
		}
	}

	convID := websocket.GenerateConversationID(userID, peerID)
	dmMsg := &websocket.DMMessage{
		Type:           "dm-messages-read-all",
		ConversationID: convID,
		UserID:         userID,
		Timestamp:      now,
	}
	wsPayload, _ := json.Marshal(map[string]interface{}{
		"peer_id": peerID.String(),
		"read_at": now.Format(time.RFC3339),
	})
	dmMsg.Payload = wsPayload
	h.dmHub.Broadcast(dmMsg)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       "ok",
		"marked_count": len(dms),
	}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}
