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

type RoomSettingsHandler struct {
	db *database.Database
}

func NewRoomSettingsHandler(db *database.Database) *RoomSettingsHandler {
	return &RoomSettingsHandler{db: db}
}

type UpdateRoomSettingsRequest struct {
	AllowMessageEdits *bool `json:"allow_message_edits"`
	AllowReactions    *bool `json:"allow_reactions"`
	RequireApproval   *bool `json:"require_approval"`
	MaxUsers          *int  `json:"max_users"`
	AutoDeleteDays    *int  `json:"auto_delete_days"`
	SlowModeSeconds   *int  `json:"slow_mode_seconds"`
	AllowVoiceChat    *bool `json:"allow_voice_chat"`
	AllowFileUploads  *bool `json:"allow_file_uploads"`
}

func (h *RoomSettingsHandler) GetRoomSettings(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	roomIDStr := r.URL.Query().Get("room_id")
	if roomIDStr == "" {
		http.Error(w, "Room ID is required", http.StatusBadRequest)
		return
	}

	roomID, err := uuid.Parse(roomIDStr)
	if err != nil {
		http.Error(w, "Invalid room ID", http.StatusBadRequest)
		return
	}

	var userRoom models.UserRoom
	if err := h.db.Where("user_id = ? AND room_id = ?", userID, roomID).First(&userRoom).Error; err != nil {
		http.Error(w, "You must be a member of the room to view settings", http.StatusForbidden)
		return
	}

	var settings models.RoomSettings
	if err := h.db.Where("room_id = ?", roomID).First(&settings).Error; err != nil {
		if err.Error() == "record not found" {
			settings = models.RoomSettings{
				RoomID:            roomID,
				AllowMessageEdits: true,
				AllowReactions:    true,
				RequireApproval:   false,
				MaxUsers:          0,
				AutoDeleteDays:    0,
				SlowModeSeconds:   0,
				AllowVoiceChat:    true,
				AllowFileUploads:  true,
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

func (h *RoomSettingsHandler) UpdateRoomSettings(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	roomIDStr := r.URL.Query().Get("room_id")
	if roomIDStr == "" {
		http.Error(w, "Room ID is required", http.StatusBadRequest)
		return
	}

	roomID, err := uuid.Parse(roomIDStr)
	if err != nil {
		http.Error(w, "Invalid room ID", http.StatusBadRequest)
		return
	}

	var userRoom models.UserRoom
	if err := h.db.Where("user_id = ? AND room_id = ?", userID, roomID).First(&userRoom).Error; err != nil {
		http.Error(w, "You must be a member of the room to update settings", http.StatusForbidden)
		return
	}

	if userRoom.Role != "owner" && userRoom.Role != "admin" {
		http.Error(w, "Only room owners and admins can update settings", http.StatusForbidden)
		return
	}

	var req UpdateRoomSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var settings models.RoomSettings
	if err := h.db.Where("room_id = ?", roomID).First(&settings).Error; err != nil {
		if err.Error() == "record not found" {
			settings = models.RoomSettings{
				RoomID: roomID,
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

	if req.AllowMessageEdits != nil {
		settings.AllowMessageEdits = *req.AllowMessageEdits
	}
	if req.AllowReactions != nil {
		settings.AllowReactions = *req.AllowReactions
	}
	if req.RequireApproval != nil {
		settings.RequireApproval = *req.RequireApproval
	}
	if req.MaxUsers != nil {
		settings.MaxUsers = *req.MaxUsers
	}
	if req.AutoDeleteDays != nil {
		settings.AutoDeleteDays = *req.AutoDeleteDays
	}
	if req.SlowModeSeconds != nil {
		settings.SlowModeSeconds = *req.SlowModeSeconds
	}
	if req.AllowVoiceChat != nil {
		settings.AllowVoiceChat = *req.AllowVoiceChat
	}
	if req.AllowFileUploads != nil {
		settings.AllowFileUploads = *req.AllowFileUploads
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
