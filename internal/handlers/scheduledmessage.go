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

type ScheduledMessageHandler struct {
	db  *database.Database
	hub *websocket.Hub
}

func NewScheduledMessageHandler(db *database.Database, hub *websocket.Hub) *ScheduledMessageHandler {
	return &ScheduledMessageHandler{db: db, hub: hub}
}

type CreateScheduledMessageRequest struct {
	RoomID  uuid.UUID `json:"room_id"`
	Content string    `json:"content"`
	SendAt  time.Time `json:"send_at"`
}

type UpdateScheduledMessageRequest struct {
	Content *string    `json:"content"`
	SendAt  *time.Time `json:"send_at"`
}

type ScheduledMessageResponse struct {
	ID          uuid.UUID `json:"id"`
	UserID      uuid.UUID `json:"user_id"`
	RoomID      uuid.UUID `json:"room_id"`
	Content     string    `json:"content"`
	SendAt      string    `json:"send_at"`
	IsSent      bool      `json:"is_sent"`
	SentAt      *string   `json:"sent_at,omitempty"`
	IsCancelled bool      `json:"is_cancelled"`
	CreatedAt   string    `json:"created_at"`
}

func (h *ScheduledMessageHandler) GetScheduledMessages(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	pending := r.URL.Query().Get("pending") == "true"

	var messages []models.ScheduledMessage
	query := h.db.Where("user_id = ?", userID)

	if pending {
		query = query.Where("is_sent = ? AND is_cancelled = ? AND send_at > ?", false, false, time.Now())
	}

	if err := query.Order("send_at ASC").Find(&messages).Error; err != nil {
		log.Printf("Error fetching scheduled messages: %v", err)
		http.Error(w, "Failed to fetch scheduled messages", http.StatusInternalServerError)
		return
	}

	results := make([]ScheduledMessageResponse, 0, len(messages))
	for _, msg := range messages {
		resp := ScheduledMessageResponse{
			ID:          msg.ID,
			UserID:      msg.UserID,
			RoomID:      msg.RoomID,
			Content:     msg.Content,
			SendAt:      msg.SendAt.Format("2006-01-02T15:04:05Z07:00"),
			IsSent:      msg.IsSent,
			IsCancelled: msg.IsCancelled,
			CreatedAt:   msg.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
		if msg.SentAt != nil {
			sent := msg.SentAt.Format("2006-01-02T15:04:05Z07:00")
			resp.SentAt = &sent
		}
		results = append(results, resp)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(results); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *ScheduledMessageHandler) CreateScheduledMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req CreateScheduledMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.RoomID == uuid.Nil {
		http.Error(w, "room_id is required", http.StatusBadRequest)
		return
	}

	if req.Content == "" {
		http.Error(w, "Message content cannot be empty", http.StatusBadRequest)
		return
	}

	if req.SendAt.IsZero() {
		http.Error(w, "send_at is required", http.StatusBadRequest)
		return
	}

	if req.SendAt.Before(time.Now()) {
		http.Error(w, "send_at must be in the future", http.StatusBadRequest)
		return
	}

	var room models.Room
	if err := h.db.First(&room, "id = ?", req.RoomID).Error; err != nil {
		http.Error(w, "Room not found", http.StatusNotFound)
		return
	}

	var userRoom models.UserRoom
	if err := h.db.First(&userRoom, "user_id = ? AND room_id = ?", userID, req.RoomID).Error; err != nil {
		http.Error(w, "You are not a member of this room", http.StatusForbidden)
		return
	}

	scheduledMessage := &models.ScheduledMessage{
		UserID:  userID,
		RoomID:  req.RoomID,
		Content: req.Content,
		SendAt:  req.SendAt,
	}

	if err := h.db.Create(scheduledMessage).Error; err != nil {
		log.Printf("Error creating scheduled message: %v", err)
		http.Error(w, "Failed to create scheduled message", http.StatusInternalServerError)
		return
	}

	response := ScheduledMessageResponse{
		ID:          scheduledMessage.ID,
		UserID:      scheduledMessage.UserID,
		RoomID:      scheduledMessage.RoomID,
		Content:     scheduledMessage.Content,
		SendAt:      scheduledMessage.SendAt.Format("2006-01-02T15:04:05Z07:00"),
		IsSent:      scheduledMessage.IsSent,
		IsCancelled: scheduledMessage.IsCancelled,
		CreatedAt:   scheduledMessage.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *ScheduledMessageHandler) GetScheduledMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}

	var message models.ScheduledMessage
	if err := h.db.First(&message, "id = ? AND user_id = ?", id, userID).Error; err != nil {
		http.Error(w, "Scheduled message not found", http.StatusNotFound)
		return
	}

	response := ScheduledMessageResponse{
		ID:          message.ID,
		UserID:      message.UserID,
		RoomID:      message.RoomID,
		Content:     message.Content,
		SendAt:      message.SendAt.Format("2006-01-02T15:04:05Z07:00"),
		IsSent:      message.IsSent,
		IsCancelled: message.IsCancelled,
		CreatedAt:   message.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *ScheduledMessageHandler) UpdateScheduledMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}

	var message models.ScheduledMessage
	if err := h.db.First(&message, "id = ? AND user_id = ?", id, userID).Error; err != nil {
		http.Error(w, "Scheduled message not found", http.StatusNotFound)
		return
	}

	if message.IsSent {
		http.Error(w, "Cannot update a sent message", http.StatusBadRequest)
		return
	}

	var req UpdateScheduledMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Content != nil {
		if *req.Content == "" {
			http.Error(w, "Message content cannot be empty", http.StatusBadRequest)
			return
		}
		message.Content = *req.Content
	}

	if req.SendAt != nil {
		if req.SendAt.Before(time.Now()) {
			http.Error(w, "send_at must be in the future", http.StatusBadRequest)
			return
		}
		message.SendAt = *req.SendAt
	}

	if err := h.db.Save(&message).Error; err != nil {
		log.Printf("Error updating scheduled message: %v", err)
		http.Error(w, "Failed to update scheduled message", http.StatusInternalServerError)
		return
	}

	response := ScheduledMessageResponse{
		ID:          message.ID,
		UserID:      message.UserID,
		RoomID:      message.RoomID,
		Content:     message.Content,
		SendAt:      message.SendAt.Format("2006-01-02T15:04:05Z07:00"),
		IsSent:      message.IsSent,
		IsCancelled: message.IsCancelled,
		CreatedAt:   message.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *ScheduledMessageHandler) DeleteScheduledMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}

	result := h.db.Where("id = ? AND user_id = ?", id, userID).Delete(&models.ScheduledMessage{})
	if result.Error != nil {
		log.Printf("Error deleting scheduled message: %v", result.Error)
		http.Error(w, "Failed to delete scheduled message", http.StatusInternalServerError)
		return
	}

	if result.RowsAffected == 0 {
		http.Error(w, "Scheduled message not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "deleted"}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *ScheduledMessageHandler) CancelScheduledMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}

	var message models.ScheduledMessage
	if err := h.db.First(&message, "id = ? AND user_id = ?", id, userID).Error; err != nil {
		http.Error(w, "Scheduled message not found", http.StatusNotFound)
		return
	}

	if message.IsSent {
		http.Error(w, "Cannot cancel a sent message", http.StatusBadRequest)
		return
	}

	message.IsCancelled = true
	if err := h.db.Save(&message).Error; err != nil {
		log.Printf("Error cancelling scheduled message: %v", err)
		http.Error(w, "Failed to cancel scheduled message", http.StatusInternalServerError)
		return
	}

	response := ScheduledMessageResponse{
		ID:          message.ID,
		UserID:      message.UserID,
		RoomID:      message.RoomID,
		Content:     message.Content,
		SendAt:      message.SendAt.Format("2006-01-02T15:04:05Z07:00"),
		IsSent:      message.IsSent,
		IsCancelled: message.IsCancelled,
		CreatedAt:   message.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}
