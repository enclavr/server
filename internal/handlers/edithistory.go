package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
)

type EditHistoryHandler struct {
	db *database.Database
}

func NewEditHistoryHandler(db *database.Database) *EditHistoryHandler {
	return &EditHistoryHandler{db: db}
}

type EditHistoryResponse struct {
	ID         uuid.UUID `json:"id"`
	MessageID  uuid.UUID `json:"message_id"`
	UserID     uuid.UUID `json:"user_id"`
	OldContent string    `json:"old_content"`
	NewContent string    `json:"new_content"`
	CreatedAt  string    `json:"created_at"`
}

func (h *EditHistoryHandler) GetMessageEditHistory(w http.ResponseWriter, r *http.Request) {
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

	var history []models.MessageEditHistory
	if err := h.db.Where("message_id = ?", messageID).
		Order("created_at ASC").
		Find(&history).Error; err != nil {
		log.Printf("Error fetching edit history: %v", err)
		http.Error(w, "Failed to fetch edit history", http.StatusInternalServerError)
		return
	}

	results := make([]EditHistoryResponse, 0, len(history))
	for _, h_entry := range history {
		results = append(results, EditHistoryResponse{
			ID:         h_entry.ID,
			MessageID:  h_entry.MessageID,
			UserID:     h_entry.UserID,
			OldContent: h_entry.OldContent,
			NewContent: h_entry.NewContent,
			CreatedAt:  h_entry.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(results); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}
