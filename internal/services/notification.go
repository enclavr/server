package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/enclavr/server/pkg/logger"
	"github.com/google/uuid"
)

type NotificationType string

const (
	NotificationTypeEmail   NotificationType = "email"
	NotificationTypePush    NotificationType = "push"
	NotificationTypeWebhook NotificationType = "webhook"
	NotificationTypeSMS     NotificationType = "sms"
)

type NotificationPriority string

const (
	PriorityLow    NotificationPriority = "low"
	PriorityNormal NotificationPriority = "normal"
	PriorityHigh   NotificationPriority = "high"
	PriorityUrgent NotificationPriority = "urgent"
)

type Notification struct {
	ID          string
	Type        NotificationType
	Priority    NotificationPriority
	Recipients  []string
	Subject     string
	Body        string
	HTMLBody    string
	Data        map[string]interface{}
	Metadata    map[string]interface{}
	ScheduledAt *time.Time
	CreatedAt   time.Time
}

type NotificationResult struct {
	NotificationID string
	Success        bool
	Error          error
	SentAt         time.Time
	Provider       string
}

type NotificationService struct {
	emailService   *EmailService
	pushService    *PushService
	webhookService *WebhookNotificationService
	queue          chan Notification
	workers        int
	wg             sync.WaitGroup
	ctx            context.Context
	cancel         context.CancelFunc
}

func NewNotificationService(config NotificationConfig) *NotificationService {
	ctx, cancel := context.WithCancel(context.Background())

	ns := &NotificationService{
		queue:   make(chan Notification, config.QueueSize),
		workers: config.Workers,
		ctx:     ctx,
	}

	if config.EmailConfig.SMTPHost != "" || config.EmailConfig.SendGridAPIKey != "" || config.EmailConfig.MailgunAPIKey != "" {
		ns.emailService = NewEmailService(&config.EmailConfig)
	}

	if config.PushConfig.APIKey != "" || config.PushConfig.ServiceAccount != "" {
		ns.pushService = NewPushService(nil, nil)
	}

	if config.WebhookConfig.Secret != "" {
		ns.webhookService = NewWebhookNotificationService(config.WebhookConfig)
	}

	_ = cancel

	for i := 0; i < config.Workers; i++ {
		ns.wg.Add(1)
		go ns.worker(i)
	}

	return ns
}

type NotificationConfig struct {
	QueueSize     int
	Workers       int
	EmailConfig   EmailConfig
	PushConfig    PushNotificationConfig
	WebhookConfig WebhookNotificationConfig
}

func (ns *NotificationService) Send(ctx context.Context, notification Notification) error {
	notification.ID = fmt.Sprintf("notif_%d_%d", time.Now().UnixNano(), time.Now().Nanosecond())
	notification.CreatedAt = time.Now()

	select {
	case ns.queue <- notification:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-ns.ctx.Done():
		return fmt.Errorf("notification service is stopped")
	}
}

func (ns *NotificationService) SendSync(ctx context.Context, notification Notification) []NotificationResult {
	var results []NotificationResult

	notification.ID = fmt.Sprintf("notif_%d_%d", time.Now().UnixNano(), time.Now().Nanosecond())
	notification.CreatedAt = time.Now()

	if ns.emailService != nil && len(notification.Recipients) > 0 {
		result := ns.sendEmail(ctx, notification)
		results = append(results, result)
	}

	if ns.pushService != nil && len(notification.Data) > 0 {
		result := ns.sendPush(ctx, notification)
		results = append(results, result)
	}

	if ns.webhookService != nil && len(notification.Data) > 0 {
		result := ns.sendWebhook(ctx, notification)
		results = append(results, result)
	}

	return results
}

func (ns *NotificationService) sendEmail(ctx context.Context, notification Notification) NotificationResult {
	result := NotificationResult{
		NotificationID: notification.ID,
		Provider:       "email",
		SentAt:         time.Now(),
	}

	if ns.emailService == nil {
		result.Error = fmt.Errorf("email service not configured")
		result.Success = false
		return result
	}

	if len(notification.Recipients) == 0 {
		result.Error = fmt.Errorf("no recipients specified")
		result.Success = false
		return result
	}

	for _, to := range notification.Recipients {
		recipient := EmailRecipient{To: to}
		if notification.Data != nil {
			if name, ok := notification.Data["name"].(string); ok {
				recipient.Name = name
			}
		}
		err := ns.emailService.Send(ctx, recipient, notification.Subject, notification.HTMLBody, notification.Body)
		if err != nil {
			logger.WithContext(ctx).Error("Failed to send email", map[string]interface{}{
				"to":      to,
				"subject": notification.Subject,
				"error":   err.Error(),
			})
			result.Error = err
			result.Success = false
			return result
		}
	}

	result.Success = true
	return result
}

func (ns *NotificationService) sendPush(ctx context.Context, notification Notification) NotificationResult {
	result := NotificationResult{
		NotificationID: notification.ID,
		Provider:       "push",
		SentAt:         time.Now(),
	}

	if ns.pushService == nil {
		result.Error = fmt.Errorf("push service not configured")
		result.Success = false
		return result
	}

	pushPayload := PushPayload{
		Title: notification.Subject,
		Body:  notification.Body,
	}

	if notification.Data != nil {
		if data, ok := notification.Data["data"]; ok {
			pushPayload.Data = data
		}
		if icon, ok := notification.Data["icon"].(string); ok {
			pushPayload.Icon = icon
		}
		if tag, ok := notification.Data["tag"].(string); ok {
			pushPayload.Tag = tag
		}
	}

	if userIDStr, ok := notification.Data["user_id"].(string); ok {
		if userID, err := uuid.Parse(userIDStr); err == nil {
			err := ns.pushService.SendNotification(userID, pushPayload)
			if err != nil {
				result.Error = err
				result.Success = false
				return result
			}
		}
	}

	result.Success = true
	return result
}

func (ns *NotificationService) sendWebhook(ctx context.Context, notification Notification) NotificationResult {
	result := NotificationResult{
		NotificationID: notification.ID,
		Provider:       "webhook",
		SentAt:         time.Now(),
	}

	if ns.webhookService == nil {
		result.Error = fmt.Errorf("webhook service not configured")
		result.Success = false
		return result
	}

	webhookURL, ok := notification.Data["webhook_url"].(string)
	if !ok {
		result.Error = fmt.Errorf("webhook URL not found in notification data")
		result.Success = false
		return result
	}

	event, ok := notification.Data["event"].(string)
	if !ok {
		event = "notification"
	}

	payload := map[string]interface{}{
		"id":        notification.ID,
		"type":      notification.Type,
		"event":     event,
		"subject":   notification.Subject,
		"body":      notification.Body,
		"data":      notification.Data,
		"metadata":  notification.Metadata,
		"timestamp": notification.CreatedAt,
	}

	err := ns.webhookService.Send(ctx, webhookURL, event, payload)
	if err != nil {
		result.Error = err
		result.Success = false
		return result
	}

	result.Success = true
	return result
}

func (ns *NotificationService) worker(id int) {
	defer ns.wg.Done()

	logger.Info("Notification worker started", map[string]interface{}{
		"worker_id": id,
	})

	for {
		select {
		case notification, ok := <-ns.queue:
			if !ok {
				return
			}
			ns.processNotification(notification)
		case <-ns.ctx.Done():
			logger.Info("Notification worker stopping", map[string]interface{}{
				"worker_id": id,
			})
			return
		}
	}
}

func (ns *NotificationService) processNotification(notification Notification) {
	ctx, cancel := context.WithTimeout(ns.ctx, 30*time.Second)
	defer cancel()

	ns.SendSync(ctx, notification)
}

func (ns *NotificationService) Stop() {
	ns.cancel()
	close(ns.queue)
	ns.wg.Wait()
}

type PushNotificationConfig struct {
	APIKey         string
	ProjectID      string
	ServiceAccount string
	FCMEndpoint    string
}

type PushNotificationService struct {
	config PushNotificationConfig
	client *http.Client
}

func NewPushNotificationService(config *PushNotificationConfig) *PushNotificationService {
	return &PushNotificationService{
		config: *config,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (p *PushNotificationService) Send(ctx context.Context, token, title, body string, data map[string]interface{}) error {
	if p.config.APIKey == "" && p.config.ServiceAccount == "" {
		logger.WithContext(ctx).Warn("Push API key not configured, skipping push notification", nil)
		return nil
	}

	payload := map[string]interface{}{
		"to": token,
		"notification": map[string]interface{}{
			"title": title,
			"body":  body,
		},
		"data": data,
	}

	bodyBytes, _ := json.Marshal(payload)
	endpoint := p.config.FCMEndpoint
	if endpoint == "" {
		endpoint = "https://fcm.googleapis.com/fcm/send"
	}

	req, _ := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	if p.config.APIKey != "" {
		req.Header.Set("Authorization", "key="+p.config.APIKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("push API returned status %d", resp.StatusCode)
	}

	return nil
}

func (p *PushNotificationService) SendBatch(ctx context.Context, tokens []string, title, body string, data map[string]interface{}) error {
	for _, token := range tokens {
		if err := p.Send(ctx, token, title, body, data); err != nil {
			return err
		}
	}
	return nil
}

type WebhookNotificationConfig struct {
	Secret     string
	RetryCount int
	RetryDelay time.Duration
	Timeout    time.Duration
}

type WebhookNotificationService struct {
	config     WebhookNotificationConfig
	signer     *WebhookSigner
	client     *http.Client
	retryDelay time.Duration
}

func NewWebhookNotificationService(config WebhookNotificationConfig) *WebhookNotificationService {
	var signer *WebhookSigner
	if config.Secret != "" {
		signer = NewWebhookSigner(config.Secret)
	}

	retryDelay := config.RetryDelay
	if retryDelay == 0 {
		retryDelay = time.Second
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &WebhookNotificationService{
		config: config,
		signer: signer,
		client: &http.Client{
			Timeout: timeout,
		},
		retryDelay: retryDelay,
	}
}

func (w *WebhookNotificationService) Send(ctx context.Context, url, event string, payload interface{}) error {
	return w.SendWithRetry(ctx, url, event, payload, w.config.RetryCount)
}

func (w *WebhookNotificationService) SendWithRetry(ctx context.Context, url, event string, payload interface{}, retries int) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	var signedPayload []byte
	if w.signer != nil {
		signature := w.signer.Sign(payloadBytes)
		signedPayload, _ = json.Marshal(map[string]interface{}{
			"payload":   payload,
			"signature": signature,
			"timestamp": time.Now().Unix(),
		})
	} else {
		signedPayload = payloadBytes
	}

	var lastErr error
	for i := 0; i <= retries; i++ {
		req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(signedPayload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Webhook-Event", event)
		req.Header.Set("X-Webhook-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

		resp, err := w.client.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(w.retryDelay)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}

		lastErr = fmt.Errorf("webhook returned status %d", resp.StatusCode)
		time.Sleep(w.config.RetryDelay)
	}

	return lastErr
}

type WebhookSigner struct {
	secret []byte
}

func NewWebhookSigner(secret string) *WebhookSigner {
	return &WebhookSigner{
		secret: []byte(secret),
	}
}

func (s *WebhookSigner) Sign(payload []byte) string {
	return fmt.Sprintf("sha256=%x", payload)
}

type WebhookEvent struct {
	Event     string                 `json:"event"`
	Timestamp int64                  `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

func ParseWebhookEvent(payload []byte) (*WebhookEvent, error) {
	var event WebhookEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, err
	}
	return &event, nil
}
