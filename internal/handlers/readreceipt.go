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

type ReadReceiptHandler struct {
	db  *database.Database
	hub *websocket.Hub
}

func NewReadReceiptHandler(db *database.Database, hub *websocket.Hub) *ReadReceiptHandler {
	return &ReadReceiptHandler{db: db, hub: hub}
}

type MarkReadRequest struct {
	MessageID uuid.UUID `json:"message_id"`
	RoomID    uuid.UUID `json:"room_id"`
}

func (h *ReadReceiptHandler) MarkMessageRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req MarkReadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.MessageID == uuid.Nil {
		http.Error(w, "message_id is required", http.StatusBadRequest)
		return
	}

	if req.RoomID == uuid.Nil {
		http.Error(w, "room_id is required", http.StatusBadRequest)
		return
	}

	var userRoom models.UserRoom
	if err := h.db.First(&userRoom, "user_id = ? AND room_id = ?", userID, req.RoomID).Error; err != nil {
		http.Error(w, "You are not a member of this room", http.StatusForbidden)
		return
	}

	messageRead := &models.MessageRead{
		UserID:    userID,
		RoomID:    req.RoomID,
		MessageID: req.MessageID,
		ReadAt:    time.Now(),
	}

	if err := h.db.Create(messageRead).Error; err != nil {
		log.Printf("Error marking message as read: %v", err)
		http.Error(w, "Failed to mark message as read", http.StatusInternalServerError)
		return
	}

	wsMsg := &websocket.Message{
		Type:      "message-read",
		RoomID:    req.RoomID,
		UserID:    userID,
		Timestamp: time.Now(),
	}
	wsPayload, _ := json.Marshal(map[string]interface{}{
		"message_id": req.MessageID.String(),
		"user_id":    userID.String(),
		"read_at":    time.Now().Format(time.RFC3339),
	})
	wsMsg.Payload = wsPayload
	h.hub.BroadcastToRoom(req.RoomID, wsMsg, userID)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "read"}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *ReadReceiptHandler) GetReadReceipts(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	messageIDStr := r.URL.Query().Get("message_id")
	if messageIDStr == "" {
		http.Error(w, "message_id is required", http.StatusBadRequest)
		return
	}

	messageID, err := uuid.Parse(messageIDStr)
	if err != nil {
		http.Error(w, "Invalid message_id", http.StatusBadRequest)
		return
	}

	var message models.Message
	if err := h.db.First(&message, "id = ?", messageID).Error; err != nil {
		http.Error(w, "Message not found", http.StatusNotFound)
		return
	}

	var userRoom models.UserRoom
	if err := h.db.First(&userRoom, "user_id = ? AND room_id = ?", userID, message.RoomID).Error; err != nil {
		http.Error(w, "You are not a member of this room", http.StatusForbidden)
		return
	}

	var reads []models.MessageRead
	if err := h.db.Where("message_id = ?", messageID).Find(&reads).Error; err != nil {
		log.Printf("Error fetching read receipts: %v", err)
		http.Error(w, "Failed to fetch read receipts", http.StatusInternalServerError)
		return
	}

	type ReadReceiptResponse struct {
		UserID   uuid.UUID `json:"user_id"`
		Username string    `json:"username"`
		ReadAt   time.Time `json:"read_at"`
	}

	var response []ReadReceiptResponse
	for _, read := range reads {
		var user models.User
		h.db.First(&user, "id = ?", read.UserID)

		response = append(response, ReadReceiptResponse{
			UserID:   read.UserID,
			Username: user.Username,
			ReadAt:   read.ReadAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *ReadReceiptHandler) GetLastReadMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	roomIDStr := r.URL.Query().Get("room_id")
	if roomIDStr == "" {
		http.Error(w, "room_id is required", http.StatusBadRequest)
		return
	}

	roomID, err := uuid.Parse(roomIDStr)
	if err != nil {
		http.Error(w, "Invalid room_id", http.StatusBadRequest)
		return
	}

	var userRoom models.UserRoom
	if err := h.db.First(&userRoom, "user_id = ? AND room_id = ?", userID, roomID).Error; err != nil {
		http.Error(w, "You are not a member of this room", http.StatusForbidden)
		return
	}

	var messageRead models.MessageRead
	err = h.db.Where("user_id = ? AND room_id = ?", userID, roomID).
		Order("read_at DESC").
		First(&messageRead).Error

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message_id": nil,
			"read_at":    nil,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"message_id": messageRead.MessageID,
		"read_at":    messageRead.ReadAt,
	}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}
