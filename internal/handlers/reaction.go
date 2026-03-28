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

type ReactionHandler struct {
	db  *database.Database
	hub *websocket.Hub
}

func NewReactionHandler(db *database.Database, hub *websocket.Hub) *ReactionHandler {
	return &ReactionHandler{db: db, hub: hub}
}

type AddReactionRequest struct {
	MessageID uuid.UUID `json:"message_id"`
	Emoji     string    `json:"emoji"`
}

type ReactionResponse struct {
	ID        uuid.UUID `json:"id"`
	MessageID uuid.UUID `json:"message_id"`
	UserID    uuid.UUID `json:"user_id"`
	Username  string    `json:"username"`
	Emoji     string    `json:"emoji"`
	CreatedAt time.Time `json:"created_at"`
}

type ReactionWithCount struct {
	Emoji      string   `json:"emoji"`
	Count      int      `json:"count"`
	Users      []string `json:"users"`
	HasReacted bool     `json:"has_reacted"`
}

func (h *ReactionHandler) AddReaction(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req AddReactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Emoji == "" {
		http.Error(w, "Emoji is required", http.StatusBadRequest)
		return
	}

	var msg models.Message
	if err := h.db.First(&msg, "id = ?", req.MessageID).Error; err != nil {
		http.Error(w, "Message not found", http.StatusNotFound)
		return
	}

	var userRoom models.UserRoom
	if err := h.db.Where("user_id = ? AND room_id = ?", userID, msg.RoomID).First(&userRoom).Error; err != nil {
		http.Error(w, "You must be a member of this room to react", http.StatusForbidden)
		return
	}

	var existingReaction models.MessageReaction
	if err := h.db.Where("message_id = ? AND user_id = ? AND emoji = ?", req.MessageID, userID, req.Emoji).First(&existingReaction).Error; err == nil {
		http.Error(w, "You have already reacted with this emoji", http.StatusConflict)
		return
	}

	reaction := &models.MessageReaction{
		MessageID: req.MessageID,
		UserID:    userID,
		Emoji:     req.Emoji,
	}

	if err := h.db.Create(reaction).Error; err != nil {
		http.Error(w, "Failed to add reaction", http.StatusInternalServerError)
		return
	}

	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	response := ReactionResponse{
		ID:        reaction.ID,
		MessageID: reaction.MessageID,
		UserID:    reaction.UserID,
		Username:  user.Username,
		Emoji:     reaction.Emoji,
		CreatedAt: reaction.CreatedAt,
	}

	wsMsg := &websocket.Message{
		Type:      "reaction-added",
		RoomID:    msg.RoomID,
		UserID:    userID,
		Timestamp: time.Now(),
	}
	wsPayload, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error marshaling WebSocket payload for AddReaction: %v", err)
	} else {
		wsMsg.Payload = wsPayload
		h.hub.BroadcastToRoom(msg.RoomID, wsMsg, uuid.Nil)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *ReactionHandler) RemoveReaction(w http.ResponseWriter, r *http.Request) {
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

	emoji := r.URL.Query().Get("emoji")
	if emoji == "" {
		http.Error(w, "emoji is required", http.StatusBadRequest)
		return
	}

	var msg models.Message
	if err := h.db.First(&msg, "id = ?", messageID).Error; err != nil {
		http.Error(w, "Message not found", http.StatusNotFound)
		return
	}

	var reaction models.MessageReaction
	if err := h.db.Where("message_id = ? AND user_id = ? AND emoji = ?", messageID, userID, emoji).First(&reaction).Error; err != nil {
		http.Error(w, "Reaction not found", http.StatusNotFound)
		return
	}

	if err := h.db.Delete(&reaction).Error; err != nil {
		http.Error(w, "Failed to remove reaction", http.StatusInternalServerError)
		return
	}

	wsMsg := &websocket.Message{
		Type:      "reaction-removed",
		RoomID:    msg.RoomID,
		UserID:    userID,
		Timestamp: time.Now(),
	}
	wsPayload, _ := json.Marshal(map[string]interface{}{
		"message_id": messageID.String(),
		"emoji":      emoji,
		"user_id":    userID.String(),
	})
	wsMsg.Payload = wsPayload
	h.hub.BroadcastToRoom(msg.RoomID, wsMsg, uuid.Nil)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "removed"}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *ReactionHandler) GetReactions(w http.ResponseWriter, r *http.Request) {
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

	var reactions []models.MessageReaction
	if err := h.db.Where("message_id = ?", messageID).Find(&reactions).Error; err != nil {
		http.Error(w, "Failed to fetch reactions", http.StatusInternalServerError)
		return
	}

	userIDs := make([]uuid.UUID, 0, len(reactions))
	for _, r := range reactions {
		userIDs = append(userIDs, r.UserID)
	}
	userMap := make(map[uuid.UUID]models.User)
	if len(userIDs) > 0 {
		var users []models.User
		if err := h.db.Where("id IN ?", userIDs).Find(&users).Error; err == nil {
			for _, u := range users {
				userMap[u.ID] = u
			}
		}
	}

	reactionMap := make(map[string]*ReactionWithCount)
	for _, r := range reactions {
		if _, exists := reactionMap[r.Emoji]; !exists {
			reactionMap[r.Emoji] = &ReactionWithCount{
				Emoji:      r.Emoji,
				Users:      []string{},
				HasReacted: false,
			}
		}
		reactionMap[r.Emoji].Count++

		if user, ok := userMap[r.UserID]; ok {
			reactionMap[r.Emoji].Users = append(reactionMap[r.Emoji].Users, user.Username)
			if r.UserID == userID {
				reactionMap[r.Emoji].HasReacted = true
			}
		}
	}

	var response []ReactionWithCount
	for _, rc := range reactionMap {
		response = append(response, *rc)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}
