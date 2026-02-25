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

type StickerHandler struct {
	db *database.Database
}

func NewStickerHandler(db *database.Database) *StickerHandler {
	return &StickerHandler{db: db}
}

type CreateStickerRequest struct {
	Name     string `json:"name"`
	ImageURL string `json:"image_url"`
}

type StickerResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	ImageURL  string    `json:"image_url"`
	CreatedBy uuid.UUID `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

func (h *StickerHandler) CreateSticker(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req CreateStickerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.ImageURL == "" {
		http.Error(w, "Name and image URL are required", http.StatusBadRequest)
		return
	}

	if len(req.Name) > 50 {
		http.Error(w, "Sticker name too long (max 50 characters)", http.StatusBadRequest)
		return
	}

	sticker := &models.ServerSticker{
		Name:      req.Name,
		ImageURL:  req.ImageURL,
		CreatedBy: userID,
	}

	if err := h.db.Create(sticker).Error; err != nil {
		log.Printf("Error creating sticker: %v", err)
		http.Error(w, "Failed to create sticker", http.StatusInternalServerError)
		return
	}

	response := StickerResponse{
		ID:        sticker.ID,
		Name:      sticker.Name,
		ImageURL:  sticker.ImageURL,
		CreatedBy: sticker.CreatedBy,
		CreatedAt: sticker.CreatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *StickerHandler) GetStickers(w http.ResponseWriter, r *http.Request) {
	var stickers []models.ServerSticker
	if err := h.db.Order("created_at DESC").Find(&stickers).Error; err != nil {
		log.Printf("Error fetching stickers: %v", err)
		http.Error(w, "Failed to fetch stickers", http.StatusInternalServerError)
		return
	}

	var response []StickerResponse
	for _, sticker := range stickers {
		response = append(response, StickerResponse{
			ID:        sticker.ID,
			Name:      sticker.Name,
			ImageURL:  sticker.ImageURL,
			CreatedBy: sticker.CreatedBy,
			CreatedAt: sticker.CreatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *StickerHandler) DeleteSticker(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	stickerIDStr := r.URL.Query().Get("sticker_id")
	if stickerIDStr == "" {
		http.Error(w, "sticker_id is required", http.StatusBadRequest)
		return
	}

	stickerID, err := uuid.Parse(stickerIDStr)
	if err != nil {
		http.Error(w, "Invalid sticker_id", http.StatusBadRequest)
		return
	}

	var sticker models.ServerSticker
	if err := h.db.First(&sticker, "id = ?", stickerID).Error; err != nil {
		http.Error(w, "Sticker not found", http.StatusNotFound)
		return
	}

	if sticker.CreatedBy != userID {
		http.Error(w, "You can only delete your own stickers", http.StatusForbidden)
		return
	}

	if err := h.db.Delete(&sticker).Error; err != nil {
		log.Printf("Error deleting sticker: %v", err)
		http.Error(w, "Failed to delete sticker", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "deleted"}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}
