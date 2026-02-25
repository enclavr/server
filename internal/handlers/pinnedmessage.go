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

type PinnedMessageHandler struct {
	db  *database.Database
	hub *websocket.Hub
}

func NewPinnedMessageHandler(db *database.Database, hub *websocket.Hub) *PinnedMessageHandler {
	return &PinnedMessageHandler{db: db, hub: hub}
}

type PinMessageRequest struct {
	MessageID uuid.UUID `json:"message_id"`
}

type PinnedMessageResponse struct {
	ID        uuid.UUID `json:"id"`
	RoomID    uuid.UUID `json:"room_id"`
	MessageID uuid.UUID `json:"message_id"`
	PinnedBy  uuid.UUID `json:"pinned_by"`
	Username  string    `json:"username"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

func (h *PinnedMessageHandler) PinMessage(w http.ResponseWriter, r *http.Request) {
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

	var req PinMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var msg models.Message
	if err := h.db.First(&msg, "id = ? AND room_id = ?", req.MessageID, roomID).Error; err != nil {
		http.Error(w, "Message not found", http.StatusNotFound)
		return
	}

	var existingPin models.PinnedMessage
	if err := h.db.First(&existingPin, "room_id = ? AND message_id = ?", roomID, req.MessageID).Error; err == nil {
		http.Error(w, "Message is already pinned", http.StatusConflict)
		return
	}

	pin := &models.PinnedMessage{
		RoomID:    roomID,
		MessageID: req.MessageID,
		PinnedBy:  userID,
	}

	if err := h.db.Create(pin).Error; err != nil {
		http.Error(w, "Failed to pin message", http.StatusInternalServerError)
		return
	}

	var user models.User
	h.db.First(&user, "id = ?", userID)

	response := PinnedMessageResponse{
		ID:        pin.ID,
		RoomID:    pin.RoomID,
		MessageID: pin.MessageID,
		PinnedBy:  pin.PinnedBy,
		Username:  user.Username,
		Content:   msg.Content,
		CreatedAt: pin.CreatedAt,
	}

	wsMsg := &websocket.Message{
		Type:      "message-pinned",
		RoomID:    roomID,
		UserID:    userID,
		Timestamp: time.Now(),
	}
	wsPayload, _ := json.Marshal(response)
	wsMsg.Payload = wsPayload
	h.hub.BroadcastToRoom(roomID, wsMsg, uuid.Nil)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *PinnedMessageHandler) UnpinMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	pinIDStr := r.URL.Query().Get("pin_id")
	if pinIDStr == "" {
		http.Error(w, "pin_id is required", http.StatusBadRequest)
		return
	}

	pinID, err := uuid.Parse(pinIDStr)
	if err != nil {
		http.Error(w, "Invalid pin_id", http.StatusBadRequest)
		return
	}

	var pin models.PinnedMessage
	if err := h.db.First(&pin, "id = ?", pinID).Error; err != nil {
		http.Error(w, "Pinned message not found", http.StatusNotFound)
		return
	}

	roomID := pin.RoomID

	if err := h.db.Delete(&pin).Error; err != nil {
		http.Error(w, "Failed to unpin message", http.StatusInternalServerError)
		return
	}

	wsMsg := &websocket.Message{
		Type:      "message-unpinned",
		RoomID:    roomID,
		UserID:    userID,
		Timestamp: time.Now(),
	}
	wsPayload, _ := json.Marshal(map[string]string{"id": pinID.String()})
	wsMsg.Payload = wsPayload
	h.hub.BroadcastToRoom(roomID, wsMsg, uuid.Nil)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "unpinned"}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *PinnedMessageHandler) GetPinnedMessages(w http.ResponseWriter, r *http.Request) {
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

	var pins []models.PinnedMessage
	if err := h.db.Where("room_id = ?", roomID).Order("created_at DESC").Find(&pins).Error; err != nil {
		http.Error(w, "Failed to fetch pinned messages", http.StatusInternalServerError)
		return
	}

	var response []PinnedMessageResponse
	for _, pin := range pins {
		var msg models.Message
		h.db.First(&msg, "id = ?", pin.MessageID)

		var user models.User
		h.db.First(&user, "id = ?", pin.PinnedBy)

		content := msg.Content
		if msg.IsDeleted {
			content = "[deleted]"
		}

		response = append(response, PinnedMessageResponse{
			ID:        pin.ID,
			RoomID:    pin.RoomID,
			MessageID: pin.MessageID,
			PinnedBy:  pin.PinnedBy,
			Username:  user.Username,
			Content:   content,
			CreatedAt: pin.CreatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}
