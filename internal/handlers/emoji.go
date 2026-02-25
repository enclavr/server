package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
)

type EmojiHandler struct {
	db *database.Database
}

func NewEmojiHandler(db *database.Database) *EmojiHandler {
	return &EmojiHandler{db: db}
}

type CreateEmojiRequest struct {
	Name     string `json:"name"`
	ImageURL string `json:"image_url"`
}

type EmojiResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	ImageURL  string    `json:"image_url"`
	CreatedBy uuid.UUID `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

func (h *EmojiHandler) CreateEmoji(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req CreateEmojiRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.ImageURL == "" {
		http.Error(w, "Name and image URL are required", http.StatusBadRequest)
		return
	}

	if len(req.Name) > 50 {
		http.Error(w, "Emoji name too long (max 50 characters)", http.StatusBadRequest)
		return
	}

	emoji := &models.ServerEmoji{
		Name:      req.Name,
		ImageURL:  req.ImageURL,
		CreatedBy: userID,
	}

	if err := h.db.Create(emoji).Error; err != nil {
		log.Printf("Error creating emoji: %v", err)
		http.Error(w, "Failed to create emoji", http.StatusInternalServerError)
		return
	}

	response := EmojiResponse{
		ID:        emoji.ID,
		Name:      emoji.Name,
		ImageURL:  emoji.ImageURL,
		CreatedBy: emoji.CreatedBy,
		CreatedAt: emoji.CreatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *EmojiHandler) GetEmojis(w http.ResponseWriter, r *http.Request) {
	var emojis []models.ServerEmoji
	if err := h.db.Order("created_at DESC").Find(&emojis).Error; err != nil {
		log.Printf("Error fetching emojis: %v", err)
		http.Error(w, "Failed to fetch emojis", http.StatusInternalServerError)
		return
	}

	var response []EmojiResponse
	for _, emoji := range emojis {
		response = append(response, EmojiResponse{
			ID:        emoji.ID,
			Name:      emoji.Name,
			ImageURL:  emoji.ImageURL,
			CreatedBy: emoji.CreatedBy,
			CreatedAt: emoji.CreatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *EmojiHandler) DeleteEmoji(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	emojiIDStr := r.URL.Query().Get("emoji_id")
	if emojiIDStr == "" {
		http.Error(w, "emoji_id is required", http.StatusBadRequest)
		return
	}

	emojiID, err := uuid.Parse(emojiIDStr)
	if err != nil {
		http.Error(w, "Invalid emoji_id", http.StatusBadRequest)
		return
	}

	var emoji models.ServerEmoji
	if err := h.db.First(&emoji, "id = ?", emojiID).Error; err != nil {
		http.Error(w, "Emoji not found", http.StatusNotFound)
		return
	}

	if emoji.CreatedBy != userID {
		http.Error(w, "You can only delete your own emojis", http.StatusForbidden)
		return
	}

	if err := h.db.Delete(&emoji).Error; err != nil {
		log.Printf("Error deleting emoji: %v", err)
		http.Error(w, "Failed to delete emoji", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "deleted"}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}
