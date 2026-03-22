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
	Theme               *string `json:"theme"`
	Language            *string `json:"language"`
	Timezone            *string `json:"timezone"`
	MessagePreview      *bool   `json:"message_preview"`
	CompactMode         *bool   `json:"compact_mode"`
	ShowOnlineStatus    *bool   `json:"show_online_status"`
	AnimatedEmoji       *bool   `json:"animated_emoji"`
	AutoPlayGifs        *bool   `json:"auto_play_gifs"`
	ReducedMotion       *bool   `json:"reduced_motion"`
	HighContrastMode    *bool   `json:"high_contrast_mode"`
	TextSize            *string `json:"text_size"`
	NotificationSound   *string `json:"notification_sound"`
	DesktopNotification *bool   `json:"desktop_notification"`
	MobileNotification  *bool   `json:"mobile_notification"`
	MentionNotification *bool   `json:"mention_notification"`
	DmNotification      *bool   `json:"dm_notification"`
	ShowTypingIndicator *bool   `json:"show_typing_indicator"`
	ShowReadReceipts    *bool   `json:"show_read_receipts"`
	AutoScrollMessages  *bool   `json:"auto_scroll_messages"`
	Use24HourFormat     *bool   `json:"use_24_hour_format"`
	DisplayMode         *string `json:"display_mode"`
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
				UserID:              userID,
				Theme:               "dark",
				Language:            "en",
				Timezone:            "UTC",
				MessagePreview:      true,
				CompactMode:         false,
				ShowOnlineStatus:    true,
				AnimatedEmoji:       true,
				AutoPlayGifs:        true,
				ReducedMotion:       false,
				HighContrastMode:    false,
				TextSize:            "medium",
				NotificationSound:   "default",
				DesktopNotification: true,
				MobileNotification:  true,
				MentionNotification: true,
				DmNotification:      true,
				ShowTypingIndicator: true,
				ShowReadReceipts:    true,
				AutoScrollMessages:  true,
				Use24HourFormat:     false,
				DisplayMode:         "card",
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
	if req.NotificationSound != nil {
		preferences.NotificationSound = *req.NotificationSound
	}
	if req.DesktopNotification != nil {
		preferences.DesktopNotification = *req.DesktopNotification
	}
	if req.MobileNotification != nil {
		preferences.MobileNotification = *req.MobileNotification
	}
	if req.MentionNotification != nil {
		preferences.MentionNotification = *req.MentionNotification
	}
	if req.DmNotification != nil {
		preferences.DmNotification = *req.DmNotification
	}
	if req.ShowTypingIndicator != nil {
		preferences.ShowTypingIndicator = *req.ShowTypingIndicator
	}
	if req.ShowReadReceipts != nil {
		preferences.ShowReadReceipts = *req.ShowReadReceipts
	}
	if req.AutoScrollMessages != nil {
		preferences.AutoScrollMessages = *req.AutoScrollMessages
	}
	if req.Use24HourFormat != nil {
		preferences.Use24HourFormat = *req.Use24HourFormat
	}
	if req.DisplayMode != nil {
		preferences.DisplayMode = *req.DisplayMode
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

func (h *PreferencesHandler) ExportPreferences(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var preferences models.UserPreferences
	if err := h.db.Where("user_id = ?", userID).First(&preferences).Error; err != nil {
		if err.Error() == "record not found" {
			preferences = models.UserPreferences{
				UserID:              userID,
				Theme:               "dark",
				Language:            "en",
				Timezone:            "UTC",
				MessagePreview:      true,
				CompactMode:         false,
				ShowOnlineStatus:    true,
				AnimatedEmoji:       true,
				AutoPlayGifs:        true,
				ReducedMotion:       false,
				HighContrastMode:    false,
				TextSize:            "medium",
				NotificationSound:   "default",
				DesktopNotification: true,
				MobileNotification:  true,
				MentionNotification: true,
				DmNotification:      true,
				ShowTypingIndicator: true,
				ShowReadReceipts:    true,
				AutoScrollMessages:  true,
				Use24HourFormat:     false,
				DisplayMode:         "card",
			}
		} else {
			http.Error(w, "Failed to fetch preferences", http.StatusInternalServerError)
			return
		}
	}

	type ExportData struct {
		ExportedAt  time.Time              `json:"exported_at"`
		Version     string                 `json:"version"`
		Preferences models.UserPreferences `json:"preferences"`
	}

	exportData := ExportData{
		ExportedAt:  time.Now(),
		Version:     "1.0",
		Preferences: preferences,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=enclavr-preferences.json")
	if err := json.NewEncoder(w).Encode(exportData); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *PreferencesHandler) ImportPreferences(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	type ImportData struct {
		Theme               *string `json:"theme"`
		Language            *string `json:"language"`
		Timezone            *string `json:"timezone"`
		MessagePreview      *bool   `json:"message_preview"`
		CompactMode         *bool   `json:"compact_mode"`
		ShowOnlineStatus    *bool   `json:"show_online_status"`
		AnimatedEmoji       *bool   `json:"animated_emoji"`
		AutoPlayGifs        *bool   `json:"auto_play_gifs"`
		ReducedMotion       *bool   `json:"reduced_motion"`
		HighContrastMode    *bool   `json:"high_contrast_mode"`
		TextSize            *string `json:"text_size"`
		NotificationSound   *string `json:"notification_sound"`
		DesktopNotification *bool   `json:"desktop_notification"`
		MobileNotification  *bool   `json:"mobile_notification"`
		MentionNotification *bool   `json:"mention_notification"`
		DmNotification      *bool   `json:"dm_notification"`
		ShowTypingIndicator *bool   `json:"show_typing_indicator"`
		ShowReadReceipts    *bool   `json:"show_read_receipts"`
		AutoScrollMessages  *bool   `json:"auto_scroll_messages"`
		Use24HourFormat     *bool   `json:"use_24_hour_format"`
		DisplayMode         *string `json:"display_mode"`
	}

	var importData ImportData
	if err := json.NewDecoder(r.Body).Decode(&importData); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	validThemes := map[string]bool{"dark": true, "light": true, "system": true}
	validLanguages := map[string]bool{"en": true, "es": true, "fr": true, "de": true, "ja": true, "zh": true}
	validTextSizes := map[string]bool{"small": true, "medium": true, "large": true}
	validDisplayModes := map[string]bool{"card": true, "compact": true, "list": true}

	var preferences models.UserPreferences
	if err := h.db.Where("user_id = ?", userID).First(&preferences).Error; err != nil {
		if err.Error() == "record not found" {
			preferences = models.UserPreferences{UserID: userID}
		} else {
			http.Error(w, "Failed to fetch preferences", http.StatusInternalServerError)
			return
		}
	}

	if importData.Theme != nil && validThemes[*importData.Theme] {
		preferences.Theme = *importData.Theme
	}
	if importData.Language != nil && validLanguages[*importData.Language] {
		preferences.Language = *importData.Language
	}
	if importData.Timezone != nil {
		preferences.Timezone = *importData.Timezone
	}
	if importData.MessagePreview != nil {
		preferences.MessagePreview = *importData.MessagePreview
	}
	if importData.CompactMode != nil {
		preferences.CompactMode = *importData.CompactMode
	}
	if importData.ShowOnlineStatus != nil {
		preferences.ShowOnlineStatus = *importData.ShowOnlineStatus
	}
	if importData.AnimatedEmoji != nil {
		preferences.AnimatedEmoji = *importData.AnimatedEmoji
	}
	if importData.AutoPlayGifs != nil {
		preferences.AutoPlayGifs = *importData.AutoPlayGifs
	}
	if importData.ReducedMotion != nil {
		preferences.ReducedMotion = *importData.ReducedMotion
	}
	if importData.HighContrastMode != nil {
		preferences.HighContrastMode = *importData.HighContrastMode
	}
	if importData.TextSize != nil && validTextSizes[*importData.TextSize] {
		preferences.TextSize = *importData.TextSize
	}
	if importData.NotificationSound != nil {
		preferences.NotificationSound = *importData.NotificationSound
	}
	if importData.DesktopNotification != nil {
		preferences.DesktopNotification = *importData.DesktopNotification
	}
	if importData.MobileNotification != nil {
		preferences.MobileNotification = *importData.MobileNotification
	}
	if importData.MentionNotification != nil {
		preferences.MentionNotification = *importData.MentionNotification
	}
	if importData.DmNotification != nil {
		preferences.DmNotification = *importData.DmNotification
	}
	if importData.ShowTypingIndicator != nil {
		preferences.ShowTypingIndicator = *importData.ShowTypingIndicator
	}
	if importData.ShowReadReceipts != nil {
		preferences.ShowReadReceipts = *importData.ShowReadReceipts
	}
	if importData.AutoScrollMessages != nil {
		preferences.AutoScrollMessages = *importData.AutoScrollMessages
	}
	if importData.Use24HourFormat != nil {
		preferences.Use24HourFormat = *importData.Use24HourFormat
	}
	if importData.DisplayMode != nil && validDisplayModes[*importData.DisplayMode] {
		preferences.DisplayMode = *importData.DisplayMode
	}

	preferences.UpdatedAt = time.Now()

	if preferences.ID == uuid.Nil {
		if err := h.db.Create(&preferences).Error; err != nil {
			http.Error(w, "Failed to create preferences", http.StatusInternalServerError)
			return
		}
	} else {
		if err := h.db.Save(&preferences).Error; err != nil {
			http.Error(w, "Failed to update preferences", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(preferences); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *PreferencesHandler) ResetPreferences(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	preferences := models.UserPreferences{
		UserID:              userID,
		Theme:               "dark",
		Language:            "en",
		Timezone:            "UTC",
		MessagePreview:      true,
		CompactMode:         false,
		ShowOnlineStatus:    true,
		AnimatedEmoji:       true,
		AutoPlayGifs:        true,
		ReducedMotion:       false,
		HighContrastMode:    false,
		TextSize:            "medium",
		NotificationSound:   "default",
		DesktopNotification: true,
		MobileNotification:  true,
		MentionNotification: true,
		DmNotification:      true,
		ShowTypingIndicator: true,
		ShowReadReceipts:    true,
		AutoScrollMessages:  true,
		Use24HourFormat:     false,
		DisplayMode:         "card",
	}

	var existing models.UserPreferences
	if err := h.db.Where("user_id = ?", userID).First(&existing).Error; err != nil {
		if err.Error() != "record not found" {
			http.Error(w, "Failed to fetch preferences", http.StatusInternalServerError)
			return
		}
	}

	if existing.ID != uuid.Nil {
		preferences.ID = existing.ID
		if err := h.db.Save(&preferences).Error; err != nil {
			http.Error(w, "Failed to reset preferences", http.StatusInternalServerError)
			return
		}
	} else {
		if err := h.db.Create(&preferences).Error; err != nil {
			http.Error(w, "Failed to create preferences", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(preferences); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}
