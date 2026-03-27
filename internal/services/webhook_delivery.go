package services

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/pkg/logger"
	"github.com/google/uuid"
)

type WebhookDeliveryService struct {
	cache           *CacheService
	httpClient      *http.Client
	retryQueue      chan WebhookDelivery
	workers         int
	maxRetries      int
	baseDelay       time.Duration
	maxDelay        time.Duration
	deliveryTimeout time.Duration
	queueSize       int
	wg              sync.WaitGroup
	ctx             context.Context
	cancel          context.CancelFunc
	enabled         bool

	mu    sync.RWMutex
	stats WebhookDeliveryStats
}

type WebhookDelivery struct {
	ID          string                 `json:"id"`
	WebhookID   string                 `json:"webhook_id"`
	URL         string                 `json:"url"`
	Secret      string                 `json:"secret,omitempty"`
	Event       string                 `json:"event"`
	Payload     map[string]interface{} `json:"payload"`
	Attempt     int                    `json:"attempt"`
	ScheduledAt time.Time              `json:"scheduled_at"`
	CreatedAt   time.Time              `json:"created_at"`
}

type WebhookDeliveryResult struct {
	DeliveryID   string        `json:"delivery_id"`
	Success      bool          `json:"success"`
	StatusCode   int           `json:"status_code"`
	Duration     time.Duration `json:"duration"`
	Error        string        `json:"error,omitempty"`
	ResponseBody string        `json:"response_body,omitempty"`
	Retryable    bool          `json:"retryable"`
	Attempt      int           `json:"attempt"`
}

type WebhookDeliveryStats struct {
	TotalDelivered int64   `json:"total_delivered"`
	TotalFailed    int64   `json:"total_failed"`
	TotalRetried   int64   `json:"total_retried"`
	TotalPending   int64   `json:"total_pending"`
	SuccessRate    float64 `json:"success_rate"`
	AverageLatency int64   `json:"average_latency_ms"`
}

type WebhookDeliveryConfig struct {
	Cache           *CacheService
	Workers         int
	MaxRetries      int
	BaseDelay       time.Duration
	MaxDelay        time.Duration
	DeliveryTimeout time.Duration
	QueueSize       int
}

func NewWebhookDeliveryService(config WebhookDeliveryConfig) *WebhookDeliveryService {
	if config.Workers <= 0 {
		config.Workers = 4
	}
	if config.MaxRetries <= 0 {
		config.MaxRetries = 3
	}
	if config.BaseDelay <= 0 {
		config.BaseDelay = time.Second
	}
	if config.MaxDelay <= 0 {
		config.MaxDelay = 5 * time.Minute
	}
	if config.DeliveryTimeout <= 0 {
		config.DeliveryTimeout = 30 * time.Second
	}
	if config.QueueSize <= 0 {
		config.QueueSize = 1000
	}

	ctx, cancel := context.WithCancel(context.Background())

	svc := &WebhookDeliveryService{
		cache:           config.Cache,
		httpClient:      &http.Client{Timeout: config.DeliveryTimeout},
		retryQueue:      make(chan WebhookDelivery, config.QueueSize),
		workers:         config.Workers,
		maxRetries:      config.MaxRetries,
		baseDelay:       config.BaseDelay,
		maxDelay:        config.MaxDelay,
		deliveryTimeout: config.DeliveryTimeout,
		queueSize:       config.QueueSize,
		ctx:             ctx,
		cancel:          cancel,
		enabled:         true,
	}

	for i := 0; i < config.Workers; i++ {
		svc.wg.Add(1)
		go svc.worker(i)
	}

	logger.Info("Webhook delivery service started", map[string]interface{}{
		"workers":     config.Workers,
		"max_retries": config.MaxRetries,
		"queue_size":  config.QueueSize,
	})

	return svc
}

func (s *WebhookDeliveryService) QueueDelivery(delivery WebhookDelivery) error {
	if !s.enabled {
		return fmt.Errorf("webhook delivery service is disabled")
	}

	if delivery.ID == "" {
		delivery.ID = uuid.New().String()
	}
	if delivery.CreatedAt.IsZero() {
		delivery.CreatedAt = time.Now()
	}
	if delivery.ScheduledAt.IsZero() {
		delivery.ScheduledAt = time.Now()
	}

	select {
	case s.retryQueue <- delivery:
		s.mu.Lock()
		s.stats.TotalPending++
		s.mu.Unlock()
		return nil
	case <-s.ctx.Done():
		return fmt.Errorf("webhook delivery service is stopped")
	default:
		return fmt.Errorf("webhook delivery queue is full")
	}
}

func (s *WebhookDeliveryService) QueueBatch(deliveries []WebhookDelivery) []error {
	var errors []error
	for _, delivery := range deliveries {
		if err := s.QueueDelivery(delivery); err != nil {
			errors = append(errors, err)
		}
	}
	return errors
}

func (s *WebhookDeliveryService) ScheduleDelivery(delivery WebhookDelivery, delay time.Duration) {
	delivery.ScheduledAt = time.Now().Add(delay)
	go func() {
		select {
		case s.retryQueue <- delivery:
			s.mu.Lock()
			s.stats.TotalPending++
			s.mu.Unlock()
		case <-s.ctx.Done():
			return
		}
	}()
}

func (s *WebhookDeliveryService) worker(id int) {
	defer s.wg.Done()

	logger.Info("Webhook delivery worker started", map[string]interface{}{
		"worker_id": id,
	})

	for {
		select {
		case delivery, ok := <-s.retryQueue:
			if !ok {
				return
			}
			s.processDelivery(delivery)
		case <-s.ctx.Done():
			logger.Info("Webhook delivery worker stopping", map[string]interface{}{
				"worker_id": id,
			})
			return
		}
	}
}

func (s *WebhookDeliveryService) processDelivery(delivery WebhookDelivery) {
	result := s.deliver(delivery)

	s.mu.Lock()
	if result.Success {
		s.stats.TotalDelivered++
	} else {
		s.stats.TotalFailed++
		if result.Retryable && delivery.Attempt < s.maxRetries {
			s.stats.TotalRetried++
		}
	}

	if s.stats.TotalDelivered+s.stats.TotalFailed > 0 {
		s.stats.SuccessRate = float64(s.stats.TotalDelivered) / float64(s.stats.TotalDelivered+s.stats.TotalFailed) * 100
	}
	s.stats.TotalPending--
	s.mu.Unlock()

	if !result.Success && result.Retryable && delivery.Attempt < s.maxRetries {
		delay := s.calculateBackoff(delivery.Attempt)
		delivery.Attempt++
		delivery.ScheduledAt = time.Now().Add(delay)

		go func() {
			select {
			case s.retryQueue <- delivery:
			case <-s.ctx.Done():
				return
			}
		}()

		logger.Warn("Webhook delivery scheduled for retry", map[string]interface{}{
			"delivery_id": delivery.ID,
			"webhook_id":  delivery.WebhookID,
			"attempt":     delivery.Attempt,
			"delay":       delay.Seconds(),
			"error":       result.Error,
		})
	} else if !result.Success {
		logger.Error("Webhook delivery failed permanently", map[string]interface{}{
			"delivery_id": delivery.ID,
			"webhook_id":  delivery.WebhookID,
			"attempts":    delivery.Attempt + 1,
			"error":       result.Error,
		})
	}
}

func (s *WebhookDeliveryService) deliver(delivery WebhookDelivery) WebhookDeliveryResult {
	result := WebhookDeliveryResult{
		DeliveryID: delivery.ID,
		Attempt:    delivery.Attempt,
	}

	start := time.Now()
	defer func() {
		result.Duration = time.Since(start)
	}()

	payloadBytes, err := json.Marshal(delivery.Payload)
	if err != nil {
		result.Error = fmt.Sprintf("failed to marshal payload: %v", err)
		result.Retryable = false
		return result
	}

	req, err := http.NewRequestWithContext(s.ctx, "POST", delivery.URL, bytes.NewReader(payloadBytes))
	if err != nil {
		result.Error = fmt.Sprintf("failed to create request: %v", err)
		result.Retryable = true
		return result
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Event", delivery.Event)
	req.Header.Set("X-Webhook-Delivery-ID", delivery.ID)
	req.Header.Set("X-Webhook-Attempt", fmt.Sprintf("%d", delivery.Attempt+1))
	req.Header.Set("X-Webhook-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

	if delivery.Secret != "" {
		mac := hmac.New(sha256.New, []byte(delivery.Secret))
		mac.Write(payloadBytes)
		signature := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Webhook-Signature", fmt.Sprintf("sha256=%s", signature))
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		result.Error = fmt.Sprintf("request failed: %v", err)
		result.Retryable = true
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode

	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(io.LimitReader(resp.Body, 1<<20)); err != nil {
		result.Error = fmt.Sprintf("failed to read response: %v", err)
		result.Retryable = true
		return result
	}
	result.ResponseBody = buf.String()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		result.Success = true
		result.Retryable = false
		logger.Info("Webhook delivered successfully", map[string]interface{}{
			"delivery_id": delivery.ID,
			"webhook_id":  delivery.WebhookID,
			"status_code": result.StatusCode,
			"duration_ms": result.Duration.Milliseconds(),
		})
	} else if resp.StatusCode >= 500 {
		result.Error = fmt.Sprintf("server error: %d", resp.StatusCode)
		result.Retryable = true
	} else if resp.StatusCode >= 400 {
		result.Error = fmt.Sprintf("client error: %d", resp.StatusCode)
		result.Retryable = false
	} else {
		result.Error = fmt.Sprintf("unexpected status: %d", resp.StatusCode)
		result.Retryable = true
	}

	return result
}

func (s *WebhookDeliveryService) calculateBackoff(attempt int) time.Duration {
	delay := s.baseDelay * time.Duration(1<<attempt)
	if delay > s.maxDelay {
		return s.maxDelay
	}
	return delay
}

func (s *WebhookDeliveryService) GetStats() WebhookDeliveryStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats
}

func (s *WebhookDeliveryService) Stop() {
	s.cancel()
	close(s.retryQueue)
	s.wg.Wait()

	logger.Info("Webhook delivery service stopped", map[string]interface{}{
		"total_delivered": s.stats.TotalDelivered,
		"total_failed":    s.stats.TotalFailed,
		"total_retried":   s.stats.TotalRetried,
	})
}

func (s *WebhookDeliveryService) IsEnabled() bool {
	return s.enabled
}

func (s *WebhookDeliveryService) SetEnabled(enabled bool) {
	s.enabled = enabled
}

type WebhookBatchDelivery struct {
	BatchID     string            `json:"batch_id"`
	Deliveries  []WebhookDelivery `json:"deliveries"`
	Event       string            `json:"event"`
	ScheduledAt time.Time         `json:"scheduled_at"`
	CreatedAt   time.Time         `json:"created_at"`
}

type WebhookBatchService struct {
	deliveryService *WebhookDeliveryService
	maxBatchSize    int
}

func NewWebhookBatchService(deliveryService *WebhookDeliveryService, maxBatchSize int) *WebhookBatchService {
	if maxBatchSize <= 0 {
		maxBatchSize = 100
	}

	return &WebhookBatchService{
		deliveryService: deliveryService,
		maxBatchSize:    maxBatchSize,
	}
}

func (s *WebhookBatchService) CreateBatch(webhooks []models.Webhook, event string, payload map[string]interface{}) WebhookBatchDelivery {
	batch := WebhookBatchDelivery{
		BatchID:     uuid.New().String(),
		Deliveries:  make([]WebhookDelivery, 0, len(webhooks)),
		Event:       event,
		ScheduledAt: time.Now(),
		CreatedAt:   time.Now(),
	}

	for _, webhook := range webhooks {
		if len(batch.Deliveries) >= s.maxBatchSize {
			break
		}

		delivery := WebhookDelivery{
			ID:        uuid.New().String(),
			WebhookID: webhook.ID.String(),
			URL:       webhook.URL,
			Secret:    webhook.Secret,
			Event:     event,
			Payload:   payload,
			Attempt:   0,
		}
		batch.Deliveries = append(batch.Deliveries, delivery)
	}

	return batch
}

func (s *WebhookBatchService) QueueBatch(batch WebhookBatchDelivery) []error {
	return s.deliveryService.QueueBatch(batch.Deliveries)
}

func (s *WebhookBatchService) ScheduleBatch(batch WebhookBatchDelivery, delay time.Duration) {
	for i := range batch.Deliveries {
		delivery := batch.Deliveries[i]
		individualDelay := delay + time.Duration(i)*100*time.Millisecond
		s.deliveryService.ScheduleDelivery(delivery, individualDelay)
	}
}

type WebhookAnalytics struct {
	mu         sync.RWMutex
	deliveries map[string]time.Time
	successes  map[string]time.Time
	failures   map[string]time.Time
	retries    map[string]int
	latencies  []int64
}

func NewWebhookAnalytics() *WebhookAnalytics {
	return &WebhookAnalytics{
		deliveries: make(map[string]time.Time),
		successes:  make(map[string]time.Time),
		failures:   make(map[string]time.Time),
		retries:    make(map[string]int),
		latencies:  make([]int64, 0),
	}
}

func (a *WebhookAnalytics) RecordDelivery(deliveryID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.deliveries[deliveryID] = time.Now()
}

func (a *WebhookAnalytics) RecordSuccess(deliveryID string, latency time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.successes[deliveryID] = time.Now()
	a.latencies = append(a.latencies, latency.Milliseconds())
	if len(a.latencies) > 1000 {
		a.latencies = a.latencies[len(a.latencies)-1000:]
	}
}

func (a *WebhookAnalytics) RecordFailure(deliveryID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.failures[deliveryID] = time.Now()
}

func (a *WebhookAnalytics) RecordRetry(deliveryID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.retries[deliveryID]++
}

func (a *WebhookAnalytics) GetAverageLatency() time.Duration {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if len(a.latencies) == 0 {
		return 0
	}

	var sum int64
	for _, l := range a.latencies {
		sum += l
	}
	return time.Duration(sum/int64(len(a.latencies))) * time.Millisecond
}

func (a *WebhookAnalytics) GetSuccessRate() float64 {
	a.mu.RLock()
	defer a.mu.RUnlock()

	total := len(a.successes) + len(a.failures)
	if total == 0 {
		return 0
	}
	return float64(len(a.successes)) / float64(total) * 100
}
