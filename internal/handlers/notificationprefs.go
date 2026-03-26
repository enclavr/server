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

type NotificationPreferencesHandler struct {
	db *database.Database
}

func NewNotificationPreferencesHandler(db *database.Database) *NotificationPreferencesHandler {
	return &NotificationPreferencesHandler{db: db}
}

type NotificationPreferencesResponse struct {
	ID                         uuid.UUID `json:"id"`
	UserID                     uuid.UUID `json:"user_id"`
	DMNotifications            string    `json:"dm_notifications"`
	GroupNotifications         string    `json:"group_notifications"`
	MentionNotifications       bool      `json:"mention_notifications"`
	ReplyNotifications         bool      `json:"reply_notifications"`
	ReactionNotifications      bool      `json:"reaction_notifications"`
	DirectMessageNotifications bool      `json:"direct_message_notifications"`
	RoomInviteNotifications    bool      `json:"room_invite_notifications"`
	SoundEnabled               bool      `json:"sound_enabled"`
	DesktopNotifications       bool      `json:"desktop_notifications"`
	MobilePushEnabled          bool      `json:"mobile_push_enabled"`
	QuietHoursEnabled          bool      `json:"quiet_hours_enabled"`
	QuietHoursStart            string    `json:"quiet_hours_start"`
	QuietHoursEnd              string    `json:"quiet_hours_end"`
}

type UpdateNotificationPreferencesRequest struct {
	DMNotifications            *string `json:"dm_notifications"`
	GroupNotifications         *string `json:"group_notifications"`
	MentionNotifications       *bool   `json:"mention_notifications"`
	ReplyNotifications         *bool   `json:"reply_notifications"`
	ReactionNotifications      *bool   `json:"reaction_notifications"`
	DirectMessageNotifications *bool   `json:"direct_message_notifications"`
	RoomInviteNotifications    *bool   `json:"room_invite_notifications"`
	SoundEnabled               *bool   `json:"sound_enabled"`
	DesktopNotifications       *bool   `json:"desktop_notifications"`
	MobilePushEnabled          *bool   `json:"mobile_push_enabled"`
	QuietHoursEnabled          *bool   `json:"quiet_hours_enabled"`
	QuietHoursStart            *string `json:"quiet_hours_start"`
	QuietHoursEnd              *string `json:"quiet_hours_end"`
}

func (h *NotificationPreferencesHandler) GetPreferences(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var prefs models.NotificationPreferences
	if err := h.db.Where("user_id = ?", userID).First(&prefs).Error; err != nil {
		if err.Error() == "record not found" {
			prefs = models.NotificationPreferences{
				UserID:                     userID,
				DMNotifications:            models.NotificationLevelAll,
				GroupNotifications:         models.NotificationLevelAll,
				MentionNotifications:       true,
				ReplyNotifications:         true,
				ReactionNotifications:      true,
				DirectMessageNotifications: true,
				RoomInviteNotifications:    true,
				SoundEnabled:               true,
				DesktopNotifications:       true,
				MobilePushEnabled:          true,
				QuietHoursEnabled:          false,
				QuietHoursStart:            "22:00",
				QuietHoursEnd:              "08:00",
			}
			if err := h.db.Create(&prefs).Error; err != nil {
				log.Printf("Error creating notification preferences: %v", err)
				http.Error(w, "Failed to create notification preferences", http.StatusInternalServerError)
				return
			}
		} else {
			log.Printf("Error fetching notification preferences: %v", err)
			http.Error(w, "Failed to fetch notification preferences", http.StatusInternalServerError)
			return
		}
	}

	response := NotificationPreferencesResponse{
		ID:                         prefs.ID,
		UserID:                     prefs.UserID,
		DMNotifications:            string(prefs.DMNotifications),
		GroupNotifications:         string(prefs.GroupNotifications),
		MentionNotifications:       prefs.MentionNotifications,
		ReplyNotifications:         prefs.ReplyNotifications,
		ReactionNotifications:      prefs.ReactionNotifications,
		DirectMessageNotifications: prefs.DirectMessageNotifications,
		RoomInviteNotifications:    prefs.RoomInviteNotifications,
		SoundEnabled:               prefs.SoundEnabled,
		DesktopNotifications:       prefs.DesktopNotifications,
		MobilePushEnabled:          prefs.MobilePushEnabled,
		QuietHoursEnabled:          prefs.QuietHoursEnabled,
		QuietHoursStart:            prefs.QuietHoursStart,
		QuietHoursEnd:              prefs.QuietHoursEnd,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *NotificationPreferencesHandler) UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var prefs models.NotificationPreferences
	if err := h.db.Where("user_id = ?", userID).First(&prefs).Error; err != nil {
		if err.Error() == "record not found" {
			prefs = models.NotificationPreferences{
				UserID: userID,
			}
			if err := h.db.Create(&prefs).Error; err != nil {
				log.Printf("Error creating notification preferences: %v", err)
				http.Error(w, "Failed to create notification preferences", http.StatusInternalServerError)
				return
			}
		} else {
			log.Printf("Error fetching notification preferences: %v", err)
			http.Error(w, "Failed to fetch notification preferences", http.StatusInternalServerError)
			return
		}
	}

	var req UpdateNotificationPreferencesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.DMNotifications != nil {
		prefs.DMNotifications = models.NotificationLevel(*req.DMNotifications)
	}
	if req.GroupNotifications != nil {
		prefs.GroupNotifications = models.NotificationLevel(*req.GroupNotifications)
	}
	if req.MentionNotifications != nil {
		prefs.MentionNotifications = *req.MentionNotifications
	}
	if req.ReplyNotifications != nil {
		prefs.ReplyNotifications = *req.ReplyNotifications
	}
	if req.ReactionNotifications != nil {
		prefs.ReactionNotifications = *req.ReactionNotifications
	}
	if req.DirectMessageNotifications != nil {
		prefs.DirectMessageNotifications = *req.DirectMessageNotifications
	}
	if req.RoomInviteNotifications != nil {
		prefs.RoomInviteNotifications = *req.RoomInviteNotifications
	}
	if req.SoundEnabled != nil {
		prefs.SoundEnabled = *req.SoundEnabled
	}
	if req.DesktopNotifications != nil {
		prefs.DesktopNotifications = *req.DesktopNotifications
	}
	if req.MobilePushEnabled != nil {
		prefs.MobilePushEnabled = *req.MobilePushEnabled
	}
	if req.QuietHoursEnabled != nil {
		prefs.QuietHoursEnabled = *req.QuietHoursEnabled
	}
	if req.QuietHoursStart != nil {
		prefs.QuietHoursStart = *req.QuietHoursStart
	}
	if req.QuietHoursEnd != nil {
		prefs.QuietHoursEnd = *req.QuietHoursEnd
	}

	if err := h.db.Save(&prefs).Error; err != nil {
		log.Printf("Error updating notification preferences: %v", err)
		http.Error(w, "Failed to update notification preferences", http.StatusInternalServerError)
		return
	}

	response := NotificationPreferencesResponse{
		ID:                         prefs.ID,
		UserID:                     prefs.UserID,
		DMNotifications:            string(prefs.DMNotifications),
		GroupNotifications:         string(prefs.GroupNotifications),
		MentionNotifications:       prefs.MentionNotifications,
		ReplyNotifications:         prefs.ReplyNotifications,
		ReactionNotifications:      prefs.ReactionNotifications,
		DirectMessageNotifications: prefs.DirectMessageNotifications,
		RoomInviteNotifications:    prefs.RoomInviteNotifications,
		SoundEnabled:               prefs.SoundEnabled,
		DesktopNotifications:       prefs.DesktopNotifications,
		MobilePushEnabled:          prefs.MobilePushEnabled,
		QuietHoursEnabled:          prefs.QuietHoursEnabled,
		QuietHoursStart:            prefs.QuietHoursStart,
		QuietHoursEnd:              prefs.QuietHoursEnd,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}
