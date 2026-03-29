package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/enclavr/server/pkg/validator"
	"github.com/google/uuid"
)

type DirectMessageHandler struct {
	db *database.Database
}

func NewDirectMessageHandler(db *database.Database) *DirectMessageHandler {
	return &DirectMessageHandler{db: db}
}

type SendDMRequest struct {
	ReceiverID uuid.UUID `json:"receiver_id"`
	Content    string    `json:"content"`
}

type DirectMessageResponse struct {
	ID         uuid.UUID    `json:"id"`
	SenderID   uuid.UUID    `json:"sender_id"`
	ReceiverID uuid.UUID    `json:"receiver_id"`
	Content    string       `json:"content"`
	IsEdited   bool         `json:"is_edited"`
	IsDeleted  bool         `json:"is_deleted"`
	CreatedAt  string       `json:"created_at"`
	UpdatedAt  string       `json:"updated_at"`
	Sender     UserResponse `json:"sender,omitempty"`
	Receiver   UserResponse `json:"receiver,omitempty"`
}

func (h *DirectMessageHandler) SendDM(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req SendDMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Content == "" {
		http.Error(w, "Message content is required", http.StatusBadRequest)
		return
	}

	if req.ReceiverID == userID {
		http.Error(w, "Cannot send message to yourself", http.StatusBadRequest)
		return
	}

	var receiver models.User
	if err := h.db.First(&receiver, req.ReceiverID).Error; err != nil {
		http.Error(w, "Receiver not found", http.StatusNotFound)
		return
	}

	var blockCount int64
	if err := h.db.Model(&models.Block{}).Where("blocker_id = ? AND blocked_id = ?", req.ReceiverID, userID).Count(&blockCount).Error; err != nil {
		http.Error(w, "Failed to check block status", http.StatusInternalServerError)
		return
	}
	if blockCount > 0 {
		http.Error(w, "You are blocked by this user", http.StatusForbidden)
		return
	}

	dm := models.DirectMessage{
		SenderID:   userID,
		ReceiverID: req.ReceiverID,
		Content:    validator.SanitizeMessageContent(req.Content),
	}

	if err := h.db.Create(&dm).Error; err != nil {
		http.Error(w, "Failed to send message", http.StatusInternalServerError)
		return
	}

	h.sendDMResponse(w, &dm, &receiver, nil)
}

func (h *DirectMessageHandler) GetConversations(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	type conversation struct {
		UserID      uuid.UUID `json:"user_id"`
		Username    string    `json:"username"`
		DisplayName string    `json:"display_name"`
		AvatarURL   string    `json:"avatar_url"`
		LastMessage string    `json:"last_message"`
		LastTime    string    `json:"last_time"`
		UnreadCount int64     `json:"unread_count"`
	}

	var conversations []conversation

	subQuery := h.db.Table("direct_messages").
		Select("CASE WHEN sender_id = ? THEN receiver_id ELSE sender_id END AS user_id", userID).
		Where("sender_id = ? OR receiver_id = ?", userID, userID).
		Group("user_id").
		Order("MAX(created_at) DESC")

	if err := h.db.Table("(?) as sub", subQuery).
		Select("sub.user_id, users.username, users.display_name, users.avatar_url, MAX(dm.content) as last_message, MAX(dm.created_at) as last_time, COUNT(CASE WHEN dm.sender_id != ? AND dmr.id IS NULL THEN 1 END) as unread_count", userID).
		Joins("JOIN users ON users.id = sub.user_id").
		Joins("LEFT JOIN direct_messages dm ON (dm.sender_id = ? AND dm.receiver_id = sub.user_id) OR (dm.sender_id = sub.user_id AND dm.receiver_id = ?)", userID, userID).
		Joins("LEFT JOIN dm_read_receipts dmr ON dmr.direct_message_id = dm.id AND dmr.user_id = ?", userID).
		Group("sub.user_id, users.id, users.username, users.display_name, users.avatar_url").
		Order("last_time DESC").
		Scan(&conversations).Error; err != nil {
		log.Printf("Error fetching conversations: %v", err)
		http.Error(w, "Failed to fetch conversations", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(conversations); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *DirectMessageHandler) GetMessages(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	otherUserIDStr := r.URL.Query().Get("user_id")
	otherUserID, err := uuid.Parse(otherUserIDStr)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	var dms []models.DirectMessage
	if err := h.db.Where(
		"(sender_id = ? AND receiver_id = ?) OR (sender_id = ? AND receiver_id = ?)",
		userID, otherUserID, otherUserID, userID,
	).Order("created_at DESC").Limit(100).Find(&dms).Error; err != nil {
		http.Error(w, "Failed to fetch messages", http.StatusInternalServerError)
		return
	}

	userIDs := []uuid.UUID{otherUserID}
	for _, dm := range dms {
		userIDs = append(userIDs, dm.SenderID, dm.ReceiverID)
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

	responses := make([]DirectMessageResponse, 0, len(dms))
	for _, dm := range dms {
		sender := userMap[dm.SenderID]
		receiver := userMap[dm.ReceiverID]
		responses = append(responses, h.dmToResponse(&dm, &sender, &receiver))
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(responses); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *DirectMessageHandler) DeleteDM(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		MessageID uuid.UUID `json:"message_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var dm models.DirectMessage
	if err := h.db.First(&dm, req.MessageID).Error; err != nil {
		http.Error(w, "Message not found", http.StatusNotFound)
		return
	}

	if dm.SenderID != userID {
		http.Error(w, "Cannot delete this message", http.StatusForbidden)
		return
	}

	if err := h.db.Model(&dm).Updates(map[string]interface{}{
		"is_deleted": true,
		"updated_at": time.Now(),
	}).Error; err != nil {
		http.Error(w, "Failed to delete message", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "deleted"}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *DirectMessageHandler) UpdateDM(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		MessageID uuid.UUID `json:"message_id"`
		Content   string    `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Content == "" {
		http.Error(w, "Message content is required", http.StatusBadRequest)
		return
	}

	var dm models.DirectMessage
	if err := h.db.First(&dm, req.MessageID).Error; err != nil {
		http.Error(w, "Message not found", http.StatusNotFound)
		return
	}

	if dm.SenderID != userID {
		http.Error(w, "Cannot edit this message", http.StatusForbidden)
		return
	}

	if err := h.db.Model(&dm).Updates(map[string]interface{}{
		"content":    validator.SanitizeMessageContent(req.Content),
		"is_edited":  true,
		"updated_at": time.Now(),
	}).Error; err != nil {
		http.Error(w, "Failed to update message", http.StatusInternalServerError)
		return
	}

	// Batch fetch sender and receiver to avoid N+1 queries
	var users []models.User
	h.db.Where("id IN ?", []uuid.UUID{dm.SenderID, dm.ReceiverID}).Find(&users)
	userMap := make(map[uuid.UUID]models.User)
	for _, u := range users {
		userMap[u.ID] = u
	}
	sender := userMap[dm.SenderID]
	receiver := userMap[dm.ReceiverID]

	h.sendDMResponse(w, &dm, &sender, &receiver)
}

func (h *DirectMessageHandler) dmToResponse(dm *models.DirectMessage, sender, receiver *models.User) DirectMessageResponse {
	return DirectMessageResponse{
		ID:         dm.ID,
		SenderID:   dm.SenderID,
		ReceiverID: dm.ReceiverID,
		Content:    dm.Content,
		IsEdited:   dm.IsEdited,
		IsDeleted:  dm.IsDeleted,
		CreatedAt:  dm.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:  dm.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		Sender: UserResponse{
			ID:          sender.ID,
			Username:    sender.Username,
			DisplayName: sender.DisplayName,
			AvatarURL:   sender.AvatarURL,
		},
		Receiver: UserResponse{
			ID:          receiver.ID,
			Username:    receiver.Username,
			DisplayName: receiver.DisplayName,
			AvatarURL:   receiver.AvatarURL,
		},
	}
}

func (h *DirectMessageHandler) sendDMResponse(w http.ResponseWriter, dm *models.DirectMessage, receiver *models.User, sender *models.User) {
	var s, r UserResponse
	if sender != nil {
		s = UserResponse{
			ID:          sender.ID,
			Username:    sender.Username,
			DisplayName: sender.DisplayName,
			AvatarURL:   sender.AvatarURL,
		}
	}
	if receiver != nil {
		r = UserResponse{
			ID:          receiver.ID,
			Username:    receiver.Username,
			DisplayName: receiver.DisplayName,
			AvatarURL:   receiver.AvatarURL,
		}
	}

	response := DirectMessageResponse{
		ID:         dm.ID,
		SenderID:   dm.SenderID,
		ReceiverID: dm.ReceiverID,
		Content:    dm.Content,
		IsEdited:   dm.IsEdited,
		IsDeleted:  dm.IsDeleted,
		CreatedAt:  dm.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:  dm.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		Sender:     s,
		Receiver:   r,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}
