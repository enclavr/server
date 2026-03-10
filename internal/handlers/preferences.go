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

type PreferencesHandler struct {
	db *database.Database
}

func NewPreferencesHandler(db *database.Database) *PreferencesHandler {
	return &PreferencesHandler{db: db}
}

type UpdatePreferencesRequest struct {
	Theme            *string `json:"theme"`
	Language         *string `json:"language"`
	Timezone         *string `json:"timezone"`
	MessagePreview   *bool   `json:"message_preview"`
	CompactMode      *bool   `json:"compact_mode"`
	ShowOnlineStatus *bool   `json:"show_online_status"`
	AnimatedEmoji    *bool   `json:"animated_emoji"`
	AutoPlayGifs     *bool   `json:"auto_play_gifs"`
	ReducedMotion    *bool   `json:"reduced_motion"`
	HighContrastMode *bool   `json:"high_contrast_mode"`
	TextSize         *string `json:"text_size"`
}

func (h *PreferencesHandler) GetPreferences(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var preferences models.UserPreferences
	if err := h.db.Where("user_id = ?", userID).First(&preferences).Error; err != nil {
		if err.Error() == "record not found" {
			preferences = models.UserPreferences{
				UserID:           userID,
				Theme:            "dark",
				Language:         "en",
				Timezone:         "UTC",
				MessagePreview:   true,
				CompactMode:      false,
				ShowOnlineStatus: true,
				AnimatedEmoji:    true,
				AutoPlayGifs:     true,
				ReducedMotion:    false,
				HighContrastMode: false,
				TextSize:         "medium",
			}
			if err := h.db.Create(&preferences).Error; err != nil {
				http.Error(w, "Failed to create preferences", http.StatusInternalServerError)
				return
			}
		} else {
			http.Error(w, "Failed to fetch preferences", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(preferences); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *PreferencesHandler) UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req UpdatePreferencesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var preferences models.UserPreferences
	if err := h.db.Where("user_id = ?", userID).First(&preferences).Error; err != nil {
		if err.Error() == "record not found" {
			preferences = models.UserPreferences{
				UserID: userID,
				Theme:  "dark",
			}
			if err := h.db.Create(&preferences).Error; err != nil {
				http.Error(w, "Failed to create preferences", http.StatusInternalServerError)
				return
			}
		} else {
			http.Error(w, "Failed to fetch preferences", http.StatusInternalServerError)
			return
		}
	}

	if req.Theme != nil {
		preferences.Theme = *req.Theme
	}
	if req.Language != nil {
		preferences.Language = *req.Language
	}
	if req.Timezone != nil {
		preferences.Timezone = *req.Timezone
	}
	if req.MessagePreview != nil {
		preferences.MessagePreview = *req.MessagePreview
	}
	if req.CompactMode != nil {
		preferences.CompactMode = *req.CompactMode
	}
	if req.ShowOnlineStatus != nil {
		preferences.ShowOnlineStatus = *req.ShowOnlineStatus
	}
	if req.AnimatedEmoji != nil {
		preferences.AnimatedEmoji = *req.AnimatedEmoji
	}
	if req.AutoPlayGifs != nil {
		preferences.AutoPlayGifs = *req.AutoPlayGifs
	}
	if req.ReducedMotion != nil {
		preferences.ReducedMotion = *req.ReducedMotion
	}
	if req.HighContrastMode != nil {
		preferences.HighContrastMode = *req.HighContrastMode
	}
	if req.TextSize != nil {
		preferences.TextSize = *req.TextSize
	}

	preferences.UpdatedAt = time.Now()

	if err := h.db.Save(&preferences).Error; err != nil {
		http.Error(w, "Failed to update preferences", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(preferences); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}
