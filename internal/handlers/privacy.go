package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PrivacyHandler struct {
	db *database.Database
}

func NewPrivacyHandler(db *database.Database) *PrivacyHandler {
	return &PrivacyHandler{db: db}
}

type UpdatePrivacySettingsRequest struct {
	AllowDirectMessages   *string `json:"allow_direct_messages"`
	AllowRoomInvites      *string `json:"allow_room_invites"`
	AllowVoiceCalls       *string `json:"allow_voice_calls"`
	ShowOnlineStatus      *bool   `json:"show_online_status"`
	ShowReadReceipts      *bool   `json:"show_read_receipts"`
	ShowTypingIndicator   *bool   `json:"show_typing_indicator"`
	AllowSearchByEmail    *bool   `json:"allow_search_by_email"`
	AllowSearchByUsername *bool   `json:"allow_search_by_username"`
}

func (h *PrivacyHandler) GetPrivacySettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var settings models.UserPrivacySettings
	if err := h.db.Where("user_id = ?", userID).First(&settings).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			settings = models.UserPrivacySettings{
				UserID:                userID,
				AllowDirectMessages:   "everyone",
				AllowRoomInvites:      "everyone",
				AllowVoiceCalls:       "everyone",
				ShowOnlineStatus:      true,
				ShowReadReceipts:      true,
				ShowTypingIndicator:   true,
				AllowSearchByEmail:    false,
				AllowSearchByUsername: true,
			}
			if err := h.db.Create(&settings).Error; err != nil {
				http.Error(w, "Failed to create privacy settings", http.StatusInternalServerError)
				return
			}
		} else {
			http.Error(w, "Failed to fetch privacy settings", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(settings); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *PrivacyHandler) UpdatePrivacySettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req UpdatePrivacySettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	validSettings := map[string]map[string]bool{
		"allow_direct_messages": {"everyone": true, "friends": true, "none": true},
		"allow_room_invites":    {"everyone": true, "friends": true, "none": true},
		"allow_voice_calls":     {"everyone": true, "friends": true, "none": true},
	}

	var settings models.UserPrivacySettings
	if err := h.db.Where("user_id = ?", userID).First(&settings).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			settings = models.UserPrivacySettings{
				UserID:                userID,
				AllowDirectMessages:   "everyone",
				AllowRoomInvites:      "everyone",
				AllowVoiceCalls:       "everyone",
				ShowOnlineStatus:      true,
				ShowReadReceipts:      true,
				ShowTypingIndicator:   true,
				AllowSearchByEmail:    false,
				AllowSearchByUsername: true,
			}
		} else {
			http.Error(w, "Failed to fetch privacy settings", http.StatusInternalServerError)
			return
		}
	}

	if req.AllowDirectMessages != nil {
		if validSettings["allow_direct_messages"][*req.AllowDirectMessages] {
			settings.AllowDirectMessages = *req.AllowDirectMessages
		} else {
			http.Error(w, "Invalid value for allow_direct_messages. Must be: everyone, friends, or none", http.StatusBadRequest)
			return
		}
	}

	if req.AllowRoomInvites != nil {
		if validSettings["allow_room_invites"][*req.AllowRoomInvites] {
			settings.AllowRoomInvites = *req.AllowRoomInvites
		} else {
			http.Error(w, "Invalid value for allow_room_invites. Must be: everyone, friends, or none", http.StatusBadRequest)
			return
		}
	}

	if req.AllowVoiceCalls != nil {
		if validSettings["allow_voice_calls"][*req.AllowVoiceCalls] {
			settings.AllowVoiceCalls = *req.AllowVoiceCalls
		} else {
			http.Error(w, "Invalid value for allow_voice_calls. Must be: everyone, friends, or none", http.StatusBadRequest)
			return
		}
	}

	if req.ShowOnlineStatus != nil {
		settings.ShowOnlineStatus = *req.ShowOnlineStatus
	}

	if req.ShowReadReceipts != nil {
		settings.ShowReadReceipts = *req.ShowReadReceipts
	}

	if req.ShowTypingIndicator != nil {
		settings.ShowTypingIndicator = *req.ShowTypingIndicator
	}

	if req.AllowSearchByEmail != nil {
		settings.AllowSearchByEmail = *req.AllowSearchByEmail
	}

	if req.AllowSearchByUsername != nil {
		settings.AllowSearchByUsername = *req.AllowSearchByUsername
	}

	settings.UpdatedAt = time.Now()

	if settings.ID == uuid.Nil {
		if err := h.db.Session(&gorm.Session{SkipHooks: true}).Create(&settings).Error; err != nil {
			http.Error(w, "Failed to create privacy settings", http.StatusInternalServerError)
			return
		}
	} else {
		if err := h.db.Session(&gorm.Session{SkipHooks: true}).Save(&settings).Error; err != nil {
			http.Error(w, "Failed to update privacy settings", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(settings); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *PrivacyHandler) ExportPrivacySettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var settings models.UserPrivacySettings
	if err := h.db.Where("user_id = ?", userID).First(&settings).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			settings = models.UserPrivacySettings{
				UserID:                userID,
				AllowDirectMessages:   "everyone",
				AllowRoomInvites:      "everyone",
				AllowVoiceCalls:       "everyone",
				ShowOnlineStatus:      true,
				ShowReadReceipts:      true,
				ShowTypingIndicator:   true,
				AllowSearchByEmail:    false,
				AllowSearchByUsername: true,
			}
		} else {
			http.Error(w, "Failed to fetch privacy settings", http.StatusInternalServerError)
			return
		}
	}

	type ExportData struct {
		ExportedAt time.Time                  `json:"exported_at"`
		Version    string                     `json:"version"`
		Privacy    models.UserPrivacySettings `json:"privacy"`
	}

	exportData := ExportData{
		ExportedAt: time.Now(),
		Version:    "1.0",
		Privacy:    settings,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=enclavr-privacy.json")
	if err := json.NewEncoder(w).Encode(exportData); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *PrivacyHandler) ResetPrivacySettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	settings := models.UserPrivacySettings{
		UserID:                userID,
		AllowDirectMessages:   "everyone",
		AllowRoomInvites:      "everyone",
		AllowVoiceCalls:       "everyone",
		ShowOnlineStatus:      true,
		ShowReadReceipts:      true,
		ShowTypingIndicator:   true,
		AllowSearchByEmail:    false,
		AllowSearchByUsername: true,
	}

	var existing models.UserPrivacySettings
	if err := h.db.Where("user_id = ?", userID).First(&existing).Error; err != nil {
		if err.Error() != "record not found" {
			http.Error(w, "Failed to fetch privacy settings", http.StatusInternalServerError)
			return
		}
	}

	if existing.ID != uuid.Nil {
		settings.ID = existing.ID
		if err := h.db.Save(&settings).Error; err != nil {
			http.Error(w, "Failed to reset privacy settings", http.StatusInternalServerError)
			return
		}
	} else {
		if err := h.db.Create(&settings).Error; err != nil {
			http.Error(w, "Failed to create privacy settings", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(settings); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}
