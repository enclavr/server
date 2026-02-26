package services

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/enclavr/server/internal/config"
	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/google/uuid"
)

type PushService struct {
	db  *database.Database
	cfg *config.Config
}

type PushPayload struct {
	Title              string      `json:"title"`
	Body               string      `json:"body"`
	Icon               string      `json:"icon"`
	Badge              string      `json:"badge"`
	Tag                string      `json:"tag"`
	Data               interface{} `json:"data,omitempty"`
	RequireInteraction bool        `json:"requireInteraction,omitempty"`
}

type PushNotification struct {
	Notification PushPayload `json:"notification"`
	Data         interface{} `json:"data,omitempty"`
}

func NewPushService(db *database.Database, cfg *config.Config) *PushService {
	return &PushService{db: db, cfg: cfg}
}

func (s *PushService) SendNotification(userID uuid.UUID, payload PushPayload) error {
	var settings models.UserNotificationSettings
	if err := s.db.Where("user_id = ?", userID).First(&settings).Error; err != nil {
		log.Printf("Error fetching notification settings: %v", err)
		return err
	}

	if !settings.EnablePush {
		return nil
	}

	if s.isQuietHours(settings.QuietHoursStart, settings.QuietHoursEnd, settings.QuietHoursEnabled) {
		return nil
	}

	var subs []models.PushSubscription
	if err := s.db.Where("user_id = ? AND is_active = ?", userID, true).Find(&subs).Error; err != nil {
		log.Printf("Error fetching push subscriptions: %v", err)
		return err
	}

	for _, sub := range subs {
		go s.sendPush(sub.Endpoint, payload)
	}

	return nil
}

func (s *PushService) sendPush(endpoint string, payload PushPayload) {
	notification := PushNotification{
		Notification: payload,
	}

	body, err := json.Marshal(notification)
	if err != nil {
		log.Printf("Error marshalling push notification: %v", err)
		return
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(body))
	if err != nil {
		log.Printf("Error creating push request: %v", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("TTL", "86400")
	req.Header.Set("Urgency", "normal")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error sending push notification: %v", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		log.Printf("Push notification failed with status: %d", resp.StatusCode)
	}
}

func (s *PushService) isQuietHours(start, end string, enabled bool) bool {
	if !enabled {
		return false
	}

	now := time.Now()
	currentTime := now.Format("15:04")

	if start > end {
		if currentTime >= start || currentTime < end {
			return true
		}
	} else {
		if currentTime >= start && currentTime < end {
			return true
		}
	}

	return false
}

func (s *PushService) NotifyNewMessage(userID uuid.UUID, roomName, senderName, messagePreview string) error {
	payload := PushPayload{
		Title: senderName,
		Body:  messagePreview,
		Tag:   "message-" + roomName,
		Icon:  "/icon.png",
		Badge: "/badge.png",
	}
	return s.SendNotification(userID, payload)
}

func (s *PushService) NotifyNewDM(userID uuid.UUID, senderName, messagePreview string) error {
	payload := PushPayload{
		Title: "DM from " + senderName,
		Body:  messagePreview,
		Tag:   "dm",
		Icon:  "/icon.png",
		Badge: "/badge.png",
	}
	return s.SendNotification(userID, payload)
}

func (s *PushService) NotifyMention(userID uuid.UUID, roomName, senderName, messagePreview string) error {
	payload := PushPayload{
		Title: senderName + " mentioned you in " + roomName,
		Body:  messagePreview,
		Tag:   "mention",
		Icon:  "/icon.png",
		Badge: "/badge.png",
	}
	return s.SendNotification(userID, payload)
}

func (s *PushService) NotifyVoiceJoin(userID uuid.UUID, roomName, userName string) error {
	payload := PushPayload{
		Title: userName + " joined voice",
		Body:  userName + " joined " + roomName,
		Tag:   "voice-" + roomName,
		Icon:  "/icon.png",
		Badge: "/badge.png",
	}
	return s.SendNotification(userID, payload)
}
