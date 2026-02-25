package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
)

type PushHandler struct {
	db *database.Database
}

func NewPushHandler(db *database.Database) *PushHandler {
	return &PushHandler{db: db}
}

type SubscribeRequest struct {
	Endpoint string `json:"endpoint" binding:"required"`
	P256DH   string `json:"p256dh" binding:"required"`
	Auth     string `json:"auth" binding:"required"`
}

type SubscribeResponse struct {
	ID        string `json:"id"`
	Endpoint  string `json:"endpoint"`
	IsActive  bool   `json:"is_active"`
	DeviceID  string `json:"device_id"`
	DeviceOS  string `json:"device_os"`
	CreatedAt string `json:"created_at"`
}

type NotificationSettingsRequest struct {
	EnablePush                 bool   `json:"enable_push"`
	EnableDMNotifications      bool   `json:"enable_dm_notifications"`
	EnableMentionNotifications bool   `json:"enable_mention_notifications"`
	EnableRoomNotifications    bool   `json:"enable_room_notifications"`
	EnableSound                bool   `json:"enable_sound"`
	NotifyOnMobile             bool   `json:"notify_on_mobile"`
	QuietHoursEnabled          bool   `json:"quiet_hours_enabled"`
	QuietHoursStart            string `json:"quiet_hours_start"`
	QuietHoursEnd              string `json:"quiet_hours_end"`
}

func (h *PushHandler) Subscribe(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req SubscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	deviceID := r.Header.Get("X-Device-ID")
	deviceOS := r.Header.Get("X-Device-OS")
	if deviceOS == "" {
		deviceOS = "web"
	}

	var existing models.PushSubscription
	result := h.db.Where("endpoint = ? AND user_id = ?", req.Endpoint, userID).First(&existing)
	if result.Error == nil {
		existing.IsActive = true
		existing.DeviceID = deviceID
		existing.DeviceOS = deviceOS
		if err := h.db.Save(&existing).Error; err != nil {
			log.Printf("Error updating push subscription: %v", err)
			http.Error(w, "Failed to update subscription", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(SubscribeResponse{
			ID:        existing.ID.String(),
			Endpoint:  existing.Endpoint,
			IsActive:  existing.IsActive,
			DeviceID:  existing.DeviceID,
			DeviceOS:  existing.DeviceOS,
			CreatedAt: existing.CreatedAt.Format(time.RFC3339),
		}); err != nil {
			log.Printf("Error encoding subscribe response: %v", err)
		}
		return
	}

	sub := models.PushSubscription{
		UserID:   userID,
		Endpoint: req.Endpoint,
		P256DH:   req.P256DH,
		Auth:     req.Auth,
		DeviceID: deviceID,
		DeviceOS: deviceOS,
		IsActive: true,
	}

	if err := h.db.Create(&sub).Error; err != nil {
		log.Printf("Error creating push subscription: %v", err)
		http.Error(w, "Failed to create subscription", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(SubscribeResponse{
		ID:        sub.ID.String(),
		Endpoint:  sub.Endpoint,
		IsActive:  sub.IsActive,
		DeviceID:  sub.DeviceID,
		DeviceOS:  sub.DeviceOS,
		CreatedAt: sub.CreatedAt.Format(time.RFC3339),
	}); err != nil {
		log.Printf("Error encoding subscribe response: %v", err)
	}
}

func (h *PushHandler) Unsubscribe(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	subscriptionID := strings.TrimPrefix(r.URL.Path, "/api/push/")
	if subscriptionID == "" {
		http.Error(w, "Subscription ID is required", http.StatusBadRequest)
		return
	}

	subID, err := uuid.Parse(subscriptionID)
	if err != nil {
		http.Error(w, "Invalid subscription ID", http.StatusBadRequest)
		return
	}

	var sub models.PushSubscription
	if err := h.db.First(&sub, subID).Error; err != nil {
		http.Error(w, "Subscription not found", http.StatusNotFound)
		return
	}

	if sub.UserID != userID {
		http.Error(w, "Access denied", http.StatusForbidden)
		return
	}

	sub.IsActive = false
	if err := h.db.Save(&sub).Error; err != nil {
		log.Printf("Error deactivating push subscription: %v", err)
		http.Error(w, "Failed to unsubscribe", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"message": "Unsubscribed successfully"}); err != nil {
		log.Printf("Error encoding unsubscribe response: %v", err)
	}
}

func (h *PushHandler) GetSubscriptions(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var subs []models.PushSubscription
	if err := h.db.Where("user_id = ? AND is_active = ?", userID, true).Find(&subs).Error; err != nil {
		log.Printf("Error fetching push subscriptions: %v", err)
		http.Error(w, "Failed to fetch subscriptions", http.StatusInternalServerError)
		return
	}

	response := make([]SubscribeResponse, len(subs))
	for i, sub := range subs {
		response[i] = SubscribeResponse{
			ID:        sub.ID.String(),
			Endpoint:  sub.Endpoint,
			IsActive:  sub.IsActive,
			DeviceID:  sub.DeviceID,
			DeviceOS:  sub.DeviceOS,
			CreatedAt: sub.CreatedAt.Format(time.RFC3339),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding subscriptions response: %v", err)
	}
}

func (h *PushHandler) GetNotificationSettings(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var settings models.UserNotificationSettings
	result := h.db.Where("user_id = ?", userID).First(&settings)
	if result.Error != nil {
		settings = models.UserNotificationSettings{
			UserID:                     userID,
			EnablePush:                 true,
			EnableDMNotifications:      true,
			EnableMentionNotifications: true,
			EnableRoomNotifications:    true,
			EnableSound:                true,
			NotifyOnMobile:             true,
			QuietHoursEnabled:          false,
			QuietHoursStart:            "22:00",
			QuietHoursEnd:              "08:00",
		}
		if err := h.db.Create(&settings).Error; err != nil {
			log.Printf("Error creating notification settings: %v", err)
			http.Error(w, "Failed to get settings", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(settings); err != nil {
		log.Printf("Error encoding settings response: %v", err)
	}
}

func (h *PushHandler) UpdateNotificationSettings(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req NotificationSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var settings models.UserNotificationSettings
	result := h.db.Where("user_id = ?", userID).First(&settings)
	if result.Error != nil {
		settings = models.UserNotificationSettings{
			UserID: userID,
		}
		if err := h.db.Create(&settings).Error; err != nil {
			log.Printf("Error creating notification settings: %v", err)
			http.Error(w, "Failed to create settings", http.StatusInternalServerError)
			return
		}
	}

	settings.EnablePush = req.EnablePush
	settings.EnableDMNotifications = req.EnableDMNotifications
	settings.EnableMentionNotifications = req.EnableMentionNotifications
	settings.EnableRoomNotifications = req.EnableRoomNotifications
	settings.EnableSound = req.EnableSound
	settings.NotifyOnMobile = req.NotifyOnMobile
	settings.QuietHoursEnabled = req.QuietHoursEnabled
	settings.QuietHoursStart = req.QuietHoursStart
	settings.QuietHoursEnd = req.QuietHoursEnd

	if err := h.db.Save(&settings).Error; err != nil {
		log.Printf("Error updating notification settings: %v", err)
		http.Error(w, "Failed to update settings", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(settings); err != nil {
		log.Printf("Error encoding settings update response: %v", err)
	}
}

func (h *PushHandler) TestNotification(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var subs []models.PushSubscription
	if err := h.db.Where("user_id = ? AND is_active = ?", userID, true).Find(&subs).Error; err != nil {
		log.Printf("Error fetching push subscriptions: %v", err)
		http.Error(w, "Failed to fetch subscriptions", http.StatusInternalServerError)
		return
	}

	if len(subs) == 0 {
		http.Error(w, "No active push subscriptions", http.StatusBadRequest)
		return
	}

	for _, sub := range subs {
		log.Printf("Sending test push to endpoint: %s", sub.Endpoint)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]int{"sent": len(subs)}); err != nil {
		log.Printf("Error encoding test notification response: %v", err)
	}
}
