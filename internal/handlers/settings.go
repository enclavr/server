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

type SettingsHandler struct {
	db *database.Database
}

func NewSettingsHandler(db *database.Database) *SettingsHandler {
	return &SettingsHandler{db: db}
}

type UpdateSettingsRequest struct {
	ServerName           string `json:"server_name"`
	ServerDescription    string `json:"server_description"`
	AllowRegistration    *bool  `json:"allow_registration"`
	MaxRoomsPerUser      *int   `json:"max_rooms_per_user"`
	MaxMembersPerRoom    *int   `json:"max_members_per_room"`
	EnableVoiceChat      *bool  `json:"enable_voice_chat"`
	EnableDirectMessages *bool  `json:"enable_direct_messages"`
	EnableFileUploads    *bool  `json:"enable_file_uploads"`
	MaxUploadSizeMB      *int   `json:"max_upload_size_mb"`
}

func (h *SettingsHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	var settings models.ServerSettings
	if err := h.db.First(&settings).Error; err != nil {
		if err.Error() == "record not found" {
			settings = models.ServerSettings{
				ServerName:           "Enclavr Server",
				ServerDescription:    "",
				AllowRegistration:    true,
				MaxRoomsPerUser:      10,
				MaxMembersPerRoom:    50,
				EnableVoiceChat:      true,
				EnableDirectMessages: true,
				EnableFileUploads:    false,
				MaxUploadSizeMB:      10,
			}
			if err := h.db.Create(&settings).Error; err != nil {
				http.Error(w, "Failed to create settings", http.StatusInternalServerError)
				return
			}
		} else {
			http.Error(w, "Failed to fetch settings", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(settings); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *SettingsHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	_, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	isAdmin, ok := r.Context().Value(middleware.IsAdminKey).(bool)
	if !ok || !isAdmin {
		http.Error(w, "Admin access required", http.StatusForbidden)
		return
	}

	var req UpdateSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var settings models.ServerSettings
	if err := h.db.First(&settings).Error; err != nil {
		http.Error(w, "Settings not found", http.StatusNotFound)
		return
	}

	if req.ServerName != "" {
		settings.ServerName = req.ServerName
	}
	if req.ServerDescription != "" {
		settings.ServerDescription = req.ServerDescription
	}
	if req.AllowRegistration != nil {
		settings.AllowRegistration = *req.AllowRegistration
	}
	if req.MaxRoomsPerUser != nil {
		settings.MaxRoomsPerUser = *req.MaxRoomsPerUser
	}
	if req.MaxMembersPerRoom != nil {
		settings.MaxMembersPerRoom = *req.MaxMembersPerRoom
	}
	if req.EnableVoiceChat != nil {
		settings.EnableVoiceChat = *req.EnableVoiceChat
	}
	if req.EnableDirectMessages != nil {
		settings.EnableDirectMessages = *req.EnableDirectMessages
	}
	if req.EnableFileUploads != nil {
		settings.EnableFileUploads = *req.EnableFileUploads
	}
	if req.MaxUploadSizeMB != nil {
		settings.MaxUploadSizeMB = *req.MaxUploadSizeMB
	}

	settings.UpdatedAt = time.Now()

	if err := h.db.Save(&settings).Error; err != nil {
		http.Error(w, "Failed to update settings", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(settings); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}
