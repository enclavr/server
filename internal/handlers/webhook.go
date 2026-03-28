package handlers

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
)

type WebhookHandler struct {
	db *database.Database
}

func NewWebhookHandler(db *database.Database) *WebhookHandler {
	return &WebhookHandler{db: db}
}

type CreateWebhookRequest struct {
	URL    string   `json:"url" binding:"required,url"`
	Events []string `json:"events" binding:"required,min=1"`
}

type WebhookResponse struct {
	ID        string   `json:"id"`
	RoomID    string   `json:"room_id"`
	URL       string   `json:"url"`
	Events    []string `json:"events"`
	IsActive  bool     `json:"is_active"`
	CreatedAt string   `json:"created_at"`
}

func (h *WebhookHandler) CreateWebhook(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	roomIDStr := strings.TrimPrefix(r.URL.Path, "/api/webhook/create/")
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
	result := h.db.Where("user_id = ? AND room_id = ? AND role IN ?", userID, roomID, []string{"owner", "admin"}).First(&userRoom)
	if result.Error != nil {
		http.Error(w, "Only room owner or admin can create webhooks", http.StatusForbidden)
		return
	}

	var req CreateWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := validateWebhookURL(req.URL); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	secret := generateSecret()
	events := strings.Join(req.Events, ",")

	webhook := models.Webhook{
		RoomID:    roomID,
		URL:       req.URL,
		Secret:    secret,
		Events:    events,
		IsActive:  true,
		CreatedBy: userID,
	}

	if err := h.db.Create(&webhook).Error; err != nil {
		log.Printf("Error creating webhook: %v", err)
		http.Error(w, "Failed to create webhook", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(WebhookResponse{
		ID:        webhook.ID.String(),
		RoomID:    webhook.RoomID.String(),
		URL:       webhook.URL,
		Events:    strings.Split(webhook.Events, ","),
		IsActive:  webhook.IsActive,
		CreatedAt: webhook.CreatedAt.Format(time.RFC3339),
	}); err != nil {
		log.Printf("Error encoding webhook response: %v", err)
	}
}

func (h *WebhookHandler) GetWebhooks(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	roomIDStr := strings.TrimPrefix(r.URL.Path, "/api/webhook/room/")
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
	result := h.db.Where("user_id = ? AND room_id = ? AND role IN ?", userID, roomID, []string{"owner", "admin", "moderator"}).First(&userRoom)
	if result.Error != nil {
		http.Error(w, "Access denied", http.StatusForbidden)
		return
	}

	var webhooks []models.Webhook
	if err := h.db.Where("room_id = ?", roomID).Find(&webhooks).Error; err != nil {
		http.Error(w, "Failed to fetch webhooks", http.StatusInternalServerError)
		return
	}

	response := make([]WebhookResponse, len(webhooks))
	for i, w := range webhooks {
		response[i] = WebhookResponse{
			ID:        w.ID.String(),
			RoomID:    w.RoomID.String(),
			URL:       w.URL,
			Events:    strings.Split(w.Events, ","),
			IsActive:  w.IsActive,
			CreatedAt: w.CreatedAt.Format(time.RFC3339),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding webhooks response: %v", err)
	}
}

func (h *WebhookHandler) DeleteWebhook(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	webhookIDStr := strings.TrimPrefix(r.URL.Path, "/api/webhook/")
	if webhookIDStr == "" || strings.HasPrefix(webhookIDStr, "room/") || strings.HasPrefix(webhookIDStr, "create/") || strings.HasPrefix(webhookIDStr, "toggle/") {
		http.Error(w, "Invalid webhook ID", http.StatusBadRequest)
		return
	}

	webhookIDStr = strings.TrimSuffix(webhookIDStr, "/toggle")
	webhookID, err := uuid.Parse(webhookIDStr)
	if err != nil {
		http.Error(w, "Invalid webhook ID", http.StatusBadRequest)
		return
	}

	var webhook models.Webhook
	if err := h.db.First(&webhook, webhookID).Error; err != nil {
		http.Error(w, "Webhook not found", http.StatusNotFound)
		return
	}

	var userRoom models.UserRoom
	result := h.db.Where("user_id = ? AND room_id = ? AND role IN ?", userID, webhook.RoomID, []string{"owner", "admin"}).First(&userRoom)
	if result.Error != nil {
		http.Error(w, "Only room owner or admin can delete webhooks", http.StatusForbidden)
		return
	}

	if err := h.db.Delete(&webhook).Error; err != nil {
		http.Error(w, "Failed to delete webhook", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"message": "Webhook deleted successfully"}); err != nil {
		log.Printf("Error encoding delete response: %v", err)
	}
}

func (h *WebhookHandler) ToggleWebhook(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	webhookIDStr := strings.TrimPrefix(r.URL.Path, "/api/webhook/toggle/")
	if webhookIDStr == "" {
		http.Error(w, "Invalid webhook ID", http.StatusBadRequest)
		return
	}

	webhookID, err := uuid.Parse(webhookIDStr)
	if err != nil {
		http.Error(w, "Invalid webhook ID", http.StatusBadRequest)
		return
	}

	var webhook models.Webhook
	if err := h.db.First(&webhook, webhookID).Error; err != nil {
		http.Error(w, "Webhook not found", http.StatusNotFound)
		return
	}

	var userRoom models.UserRoom
	result := h.db.Where("user_id = ? AND room_id = ? AND role IN ?", userID, webhook.RoomID, []string{"owner", "admin"}).First(&userRoom)
	if result.Error != nil {
		http.Error(w, "Only room owner or admin can toggle webhooks", http.StatusForbidden)
		return
	}

	webhook.IsActive = !webhook.IsActive
	if err := h.db.Save(&webhook).Error; err != nil {
		http.Error(w, "Failed to update webhook", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]bool{"is_active": webhook.IsActive}); err != nil {
		log.Printf("Error encoding toggle response: %v", err)
	}
}

type WebhookLogResponse struct {
	ID           string `json:"id"`
	WebhookID    string `json:"webhook_id"`
	Event        string `json:"event"`
	Payload      string `json:"payload"`
	StatusCode   int    `json:"status_code"`
	Success      bool   `json:"success"`
	ErrorMessage string `json:"error_message"`
	ResponseBody string `json:"response_body"`
	CreatedAt    string `json:"created_at"`
}

func (h *WebhookHandler) GetWebhookLogs(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	webhookIDStr := strings.TrimPrefix(r.URL.Path, "/api/webhook/logs/")
	if webhookIDStr == "" {
		http.Error(w, "Webhook ID is required", http.StatusBadRequest)
		return
	}

	webhookID, err := uuid.Parse(webhookIDStr)
	if err != nil {
		http.Error(w, "Invalid webhook ID", http.StatusBadRequest)
		return
	}

	var webhook models.Webhook
	if err := h.db.First(&webhook, webhookID).Error; err != nil {
		http.Error(w, "Webhook not found", http.StatusNotFound)
		return
	}

	var userRoom models.UserRoom
	result := h.db.Where("user_id = ? AND room_id = ? AND role IN ?", userID, webhook.RoomID, []string{"owner", "admin", "moderator"}).First(&userRoom)
	if result.Error != nil {
		http.Error(w, "Access denied", http.StatusForbidden)
		return
	}

	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	var webhookLogs []models.WebhookLog
	if err := h.db.Where("webhook_id = ?", webhookID).Order("created_at DESC").Limit(limit).Find(&webhookLogs).Error; err != nil {
		http.Error(w, "Failed to fetch webhook logs", http.StatusInternalServerError)
		return
	}

	response := make([]WebhookLogResponse, len(webhookLogs))
	for i, log := range webhookLogs {
		response[i] = WebhookLogResponse{
			ID:           log.ID.String(),
			WebhookID:    log.WebhookID.String(),
			Event:        log.Event,
			Payload:      log.Payload,
			StatusCode:   log.StatusCode,
			Success:      log.Success,
			ErrorMessage: log.ErrorMessage,
			ResponseBody: log.ResponseBody,
			CreatedAt:    log.CreatedAt.Format(time.RFC3339),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding webhook logs response: %v", err)
	}
}

func generateSecret() string {
	return uuid.New().String()
}

func validateWebhookURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return fmt.Errorf("only http and https schemes are allowed")
	}

	hostname := parsed.Hostname()
	if hostname == "" {
		return fmt.Errorf("URL must have a hostname")
	}

	if hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1" || hostname == "0.0.0.0" {
		return fmt.Errorf("URL must not point to localhost")
	}

	ip := net.ParseIP(hostname)
	if ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("URL must not point to a private or reserved IP address")
		}
		if ip.String() == "169.254.169.254" || ip.String() == "fd00::ec2::254" {
			return fmt.Errorf("URL must not point to cloud metadata endpoints")
		}
		return nil
	}

	ips, err := net.LookupIP(hostname)
	if err != nil {
		return fmt.Errorf("failed to resolve hostname: %w", err)
	}
	for _, resolvedIP := range ips {
		if resolvedIP.IsLoopback() || resolvedIP.IsPrivate() || resolvedIP.IsLinkLocalUnicast() || resolvedIP.IsLinkLocalMulticast() {
			return fmt.Errorf("URL resolves to a private or reserved IP address")
		}
	}

	return nil
}

type WebhookPayload struct {
	Event     string      `json:"event"`
	RoomID    string      `json:"room_id"`
	Timestamp string      `json:"timestamp"`
	Data      interface{} `json:"data"`
}

func (h *WebhookHandler) TriggerEvent(roomID uuid.UUID, event string, data interface{}) {
	var webhooks []models.Webhook
	if err := h.db.Where("room_id = ? AND is_active = ?", roomID, true).Find(&webhooks).Error; err != nil {
		log.Printf("Error fetching webhooks: %v", err)
		return
	}

	for _, webhook := range webhooks {
		events := strings.Split(webhook.Events, ",")
		hasEvent := false
		for _, e := range events {
			if strings.TrimSpace(e) == event {
				hasEvent = true
				break
			}
		}

		if !hasEvent {
			continue
		}

		go h.sendWebhook(webhook, event, data)
	}
}

func (h *WebhookHandler) sendWebhook(webhook models.Webhook, event string, data interface{}) {
	payload := WebhookPayload{
		Event:     event,
		RoomID:    webhook.RoomID.String(),
		Timestamp: time.Now().Format(time.RFC3339),
		Data:      data,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshalling webhook payload: %v", err)
		return
	}

	req, err := http.NewRequest("POST", webhook.URL, bytes.NewBuffer(body))
	if err != nil {
		log.Printf("Error creating webhook request: %v", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Event", event)
	req.Header.Set("X-Webhook-Timestamp", payload.Timestamp)

	if webhook.Secret != "" {
		mac := hmac.New(sha256.New, []byte(webhook.Secret))
		mac.Write(body)
		signature := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Webhook-Signature", signature)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)

	webhookLog := models.WebhookLog{
		WebhookID: webhook.ID,
		Event:     event,
		Payload:   string(body),
		Success:   false,
	}

	if err != nil {
		webhookLog.ErrorMessage = err.Error()
		h.db.Create(&webhookLog)
		return
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("Error closing webhook response body: %v", closeErr)
		}
	}()

	webhookLog.StatusCode = resp.StatusCode
	webhookLog.Success = resp.StatusCode >= 200 && resp.StatusCode < 300

	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		log.Printf("Error reading webhook response: %v", err)
	}
	webhookLog.ResponseBody = buf.String()

	h.db.Create(&webhookLog)
}
