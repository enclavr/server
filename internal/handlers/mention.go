package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/internal/services"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
)

type MentionHandler struct {
	db             *database.Database
	mentionService *services.MentionService
}

func NewMentionHandler(db *database.Database) *MentionHandler {
	return &MentionHandler{
		db:             db,
		mentionService: services.NewMentionService(db),
	}
}

type MentionResponse struct {
	ID            uuid.UUID `json:"id"`
	MessageID     uuid.UUID `json:"message_id"`
	RoomID        uuid.UUID `json:"room_id"`
	UserID        uuid.UUID `json:"user_id"`
	Username      string    `json:"username"`
	MentionedBy   uuid.UUID `json:"mentioned_by"`
	MentionerName string    `json:"mentioner_name"`
	Type          string    `json:"type"`
	CreatedAt     string    `json:"created_at"`
}

func (h *MentionHandler) GetUserMentions(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	mentions, err := h.mentionService.GetMentionsForUser(userID, limit)
	if err != nil {
		http.Error(w, "Failed to fetch mentions", http.StatusInternalServerError)
		return
	}

	responses := make([]MentionResponse, 0, len(mentions))
	for _, m := range mentions {
		var username string
		var user models.User
		if err := h.db.Select("username").First(&user, "id = ?", m.UserID).Error; err == nil {
			username = user.Username
		}

		var mentionerName string
		var mentioner models.User
		if err := h.db.Select("username").First(&mentioner, "id = ?", m.MentionedBy).Error; err == nil {
			mentionerName = mentioner.Username
		}

		responses = append(responses, MentionResponse{
			ID:            m.ID,
			MessageID:     m.MessageID,
			RoomID:        m.RoomID,
			UserID:        m.UserID,
			Username:      username,
			MentionedBy:   m.MentionedBy,
			MentionerName: mentionerName,
			Type:          string(m.Type),
			CreatedAt:     m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(responses)
}

func (h *MentionHandler) GetMessageMentions(w http.ResponseWriter, r *http.Request) {
	_, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
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

	mentions, err := h.mentionService.GetMentionsForMessage(messageID)
	if err != nil {
		http.Error(w, "Failed to fetch mentions", http.StatusInternalServerError)
		return
	}

	responses := make([]MentionResponse, 0, len(mentions))
	for _, m := range mentions {
		var username string
		var user models.User
		if err := h.db.Select("username").First(&user, "id = ?", m.UserID).Error; err == nil {
			username = user.Username
		}

		var mentionerName string
		var mentioner models.User
		if err := h.db.Select("username").First(&mentioner, "id = ?", m.MentionedBy).Error; err == nil {
			mentionerName = mentioner.Username
		}

		responses = append(responses, MentionResponse{
			ID:            m.ID,
			MessageID:     m.MessageID,
			RoomID:        m.RoomID,
			UserID:        m.UserID,
			Username:      username,
			MentionedBy:   m.MentionedBy,
			MentionerName: mentionerName,
			Type:          string(m.Type),
			CreatedAt:     m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(responses)
}
