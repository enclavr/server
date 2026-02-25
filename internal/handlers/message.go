package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/internal/websocket"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
)

type MessageHandler struct {
	db  *database.Database
	hub *websocket.Hub
}

func NewMessageHandler(db *database.Database, hub *websocket.Hub) *MessageHandler {
	return &MessageHandler{db: db, hub: hub}
}

type SearchMessagesRequest struct {
	Query string `json:"query"`
}

type SearchResult struct {
	ID        uuid.UUID `json:"id"`
	RoomID    uuid.UUID `json:"room_id"`
	RoomName  string    `json:"room_name"`
	UserID    uuid.UUID `json:"user_id"`
	Username  string    `json:"username"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type SendMessageRequest struct {
	RoomID  uuid.UUID `json:"room_id"`
	Content string    `json:"content"`
	Type    string    `json:"type"`
}

type UpdateMessageRequest struct {
	Content string `json:"content"`
}

type MessageResponse struct {
	ID        uuid.UUID `json:"id"`
	RoomID    uuid.UUID `json:"room_id"`
	UserID    uuid.UUID `json:"user_id"`
	Username  string    `json:"username"`
	Type      string    `json:"type"`
	Content   string    `json:"content"`
	IsEdited  bool      `json:"is_edited"`
	IsDeleted bool      `json:"is_deleted"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (h *MessageHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Content == "" {
		http.Error(w, "Message content cannot be empty", http.StatusBadRequest)
		return
	}

	msgType := models.MessageTypeText
	if req.Type == "system" {
		msgType = models.MessageTypeSystem
	}

	msg := &models.Message{
		RoomID:  req.RoomID,
		UserID:  userID,
		Content: req.Content,
		Type:    msgType,
	}

	if err := h.db.Create(msg).Error; err != nil {
		http.Error(w, "Failed to save message", http.StatusInternalServerError)
		return
	}

	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	response := MessageResponse{
		ID:        msg.ID,
		RoomID:    msg.RoomID,
		UserID:    msg.UserID,
		Username:  user.Username,
		Type:      string(msg.Type),
		Content:   msg.Content,
		IsEdited:  msg.IsEdited,
		IsDeleted: msg.IsDeleted,
		CreatedAt: msg.CreatedAt,
		UpdatedAt: msg.UpdatedAt,
	}

	wsMsg := &websocket.Message{
		Type:      "chat-message",
		RoomID:    msg.RoomID,
		UserID:    msg.UserID,
		Timestamp: time.Now(),
	}
	wsPayload, _ := json.Marshal(response)
	wsMsg.Payload = wsPayload
	h.hub.BroadcastToRoom(msg.RoomID, wsMsg, uuid.Nil)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		return
	}
}

func (h *MessageHandler) GetMessages(w http.ResponseWriter, r *http.Request) {
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

	var messages []models.Message
	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	if err := h.db.Where("room_id = ?", roomID).Order("created_at DESC").Limit(limit).Find(&messages).Error; err != nil {
		http.Error(w, "Failed to fetch messages", http.StatusInternalServerError)
		return
	}

	var response []MessageResponse
	for _, msg := range messages {
		var user models.User
		h.db.First(&user, "id = ?", msg.UserID)

		content := msg.Content
		if msg.IsDeleted {
			content = ""
		}

		response = append(response, MessageResponse{
			ID:        msg.ID,
			RoomID:    msg.RoomID,
			UserID:    msg.UserID,
			Username:  user.Username,
			Type:      string(msg.Type),
			Content:   content,
			IsEdited:  msg.IsEdited,
			IsDeleted: msg.IsDeleted,
			CreatedAt: msg.CreatedAt,
			UpdatedAt: msg.UpdatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		return
	}
}

func (h *MessageHandler) UpdateMessage(w http.ResponseWriter, r *http.Request) {
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

	var req UpdateMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Content == "" {
		http.Error(w, "Message content cannot be empty", http.StatusBadRequest)
		return
	}

	var msg models.Message
	if err := h.db.First(&msg, "id = ?", messageID).Error; err != nil {
		http.Error(w, "Message not found", http.StatusNotFound)
		return
	}

	if msg.UserID != userID {
		http.Error(w, "You can only edit your own messages", http.StatusForbidden)
		return
	}

	msg.Content = req.Content
	msg.IsEdited = true
	msg.UpdatedAt = time.Now()

	if err := h.db.Save(&msg).Error; err != nil {
		http.Error(w, "Failed to update message", http.StatusInternalServerError)
		return
	}

	var user models.User
	h.db.First(&user, "id = ?", msg.UserID)

	response := MessageResponse{
		ID:        msg.ID,
		RoomID:    msg.RoomID,
		UserID:    msg.UserID,
		Username:  user.Username,
		Type:      string(msg.Type),
		Content:   msg.Content,
		IsEdited:  msg.IsEdited,
		IsDeleted: msg.IsDeleted,
		CreatedAt: msg.CreatedAt,
		UpdatedAt: msg.UpdatedAt,
	}

	wsMsg := &websocket.Message{
		Type:      "message-updated",
		RoomID:    msg.RoomID,
		UserID:    msg.UserID,
		Timestamp: time.Now(),
	}
	wsPayload, _ := json.Marshal(response)
	wsMsg.Payload = wsPayload
	h.hub.BroadcastToRoom(msg.RoomID, wsMsg, uuid.Nil)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *MessageHandler) DeleteMessage(w http.ResponseWriter, r *http.Request) {
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

	var msg models.Message
	if err := h.db.First(&msg, "id = ?", messageID).Error; err != nil {
		http.Error(w, "Message not found", http.StatusNotFound)
		return
	}

	if msg.UserID != userID {
		http.Error(w, "You can only delete your own messages", http.StatusForbidden)
		return
	}

	msg.IsDeleted = true
	msg.Content = ""
	msg.UpdatedAt = time.Now()

	if err := h.db.Save(&msg).Error; err != nil {
		http.Error(w, "Failed to delete message", http.StatusInternalServerError)
		return
	}

	wsMsg := &websocket.Message{
		Type:      "message-deleted",
		RoomID:    msg.RoomID,
		UserID:    userID,
		Timestamp: time.Now(),
	}
	wsPayload, _ := json.Marshal(map[string]string{"id": messageID.String()})
	wsMsg.Payload = wsPayload
	h.hub.BroadcastToRoom(msg.RoomID, wsMsg, uuid.Nil)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "deleted"}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *MessageHandler) SearchMessages(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "Search query is required", http.StatusBadRequest)
		return
	}

	if len(query) > 200 {
		http.Error(w, "Search query is too long", http.StatusBadRequest)
		return
	}

	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	type SearchResult struct {
		ID        uuid.UUID `json:"id"`
		RoomID    uuid.UUID `json:"room_id"`
		RoomName  string    `json:"room_name"`
		UserID    uuid.UUID `json:"user_id"`
		Username  string    `json:"username"`
		Content   string    `json:"content"`
		CreatedAt time.Time `json:"created_at"`
	}

	var results []SearchResult

	err := h.db.Table("messages").
		Select("messages.id, messages.room_id, rooms.name as room_name, messages.user_id, users.username, messages.content, messages.created_at").
		Joins("JOIN rooms ON messages.room_id = rooms.id").
		Joins("JOIN users ON messages.user_id = users.id").
		Joins("JOIN user_rooms ON messages.room_id = user_rooms.room_id AND user_rooms.user_id = ?", userID).
		Where("messages.is_deleted = ?", false).
		Where("to_tsvector('english', messages.content) @@ plainto_tsquery('english', ?)", query).
		Order("messages.created_at DESC").
		Limit(limit).
		Scan(&results).Error

	if err != nil {
		log.Printf("Search error: %v", err)
		http.Error(w, "Search failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(results); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}
