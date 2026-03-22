package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var messageBufferPool = sync.Pool{
	New: func() interface{} {
		buf := make([]byte, 0, 512)
		return &buf
	},
}

func (m *Message) encode() []byte {
	if m.Timestamp.IsZero() {
		m.Timestamp = time.Now()
	}

	bufPtrVal := messageBufferPool.Get()
	bufPtr := bufPtrVal.(*[]byte) // nolint: errcheck
	defer messageBufferPool.Put(bufPtrVal)
	*bufPtr = (*bufPtr)[:0]

	data, err := json.Marshal(m)
	if err != nil {
		log.Println("Error encoding message:", err)
		return []byte{}
	}
	*bufPtr = append(*bufPtr, data...)

	result := make([]byte, len(*bufPtr))
	copy(result, *bufPtr)
	return result
}

func (m *Message) decode(data []byte) error {
	return json.Unmarshal(data, m)
}

type Logger interface {
	Debug(msg string, fields ...interface{})
	Info(msg string, fields ...interface{})
	Warn(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
}

type StructuredLogger struct{}

func (l StructuredLogger) Debug(msg string, fields ...interface{}) {
	log.Printf("[DEBUG] "+msg, fields...)
}

func (l StructuredLogger) Info(msg string, fields ...interface{}) {
	log.Printf("[INFO] "+msg, fields...)
}

func (l StructuredLogger) Warn(msg string, fields ...interface{}) {
	log.Printf("[WARN] "+msg, fields...)
}

func (l StructuredLogger) Error(msg string, fields ...interface{}) {
	log.Printf("[ERROR] "+msg, fields...)
}

var wsLogger Logger = StructuredLogger{}

func SetLogger(logger Logger) {
	wsLogger = logger
}

var (
	ErrConnectionClosed  = errors.New("connection closed")
	ErrInvalidMessage    = errors.New("invalid message")
	ErrRateLimitExceeded = errors.New("rate limit exceeded")
	ErrInvalidState      = errors.New("invalid connection state")
)

func getCloseCode(err error) int {
	if ce, ok := err.(*websocket.CloseError); ok {
		return ce.Code
	}
	return -1
}

func isTemporaryNetworkError(err error) bool {
	if ce, ok := err.(*websocket.CloseError); ok {
		return ce.Code == websocket.CloseAbnormalClosure
	}
	return false
}

func getErrorCategory(err error) (category, detail string) {
	if err == nil {
		return "unknown", "no error provided"
	}

	var ce *websocket.CloseError
	if errors.As(err, &ce) {
		switch ce.Code {
		case websocket.CloseNormalClosure:
			return "graceful", "client initiated normal closure"
		case websocket.CloseGoingAway:
			return "graceful", "client leaving or navigation"
		case websocket.CloseAbnormalClosure:
			return "network", "abnormal network closure"
		case websocket.CloseNoStatusReceived:
			return "protocol", "no close frame received"
		case websocket.CloseTLSHandshake:
			return "tls", "TLS handshake failure"
		case 4000 - 4999:
			return "custom", "application-specific close"
		default:
			return "unknown", fmt.Sprintf("code %d: %s", ce.Code, ce.Text)
		}
	}

	var netErr *net.OpError
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return "timeout", "connection timeout"
		}
		return "network", "network operation error"
	}

	if errors.Is(err, context.Canceled) {
		return "cancelled", "operation cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout", "deadline exceeded"
	}

	return "unknown", err.Error()
}

type WebSocketError struct {
	Code       string
	Message    string
	Connection uuid.UUID
	UserID     uuid.UUID
	Timestamp  time.Time
	Ctx        context.Context
	Cancel     context.CancelFunc
}

func (e *WebSocketError) Error() string {
	return fmt.Sprintf("[%s] %s (conn=%s, user=%s)", e.Code, e.Message, e.Connection, e.UserID)
}

type TraceContext struct {
	TraceID uuid.UUID
	SpanID  uuid.UUID
}

func NewTraceContext() TraceContext {
	return TraceContext{
		TraceID: uuid.New(),
		SpanID:  uuid.New(),
	}
}

const (
	MaxErrorThreshold      = 10
	IdleTimeout            = 5 * time.Minute
	MaxMessageSize         = 512 * 1024
	ReconnectRetryDelay    = 5 * time.Second
	MaxReconnectAttempts   = 3
	MaxMessageQueueSize    = 256
	PingInterval           = 30 * time.Second
	PongTimeout            = 60 * time.Second
	MaxReconnectDelay      = 30 * time.Second
	MinReconnectDelay      = 1 * time.Second
	MaxReconnectStates     = 100
	TypingThrottleDuration = 2 * time.Second
	MaxPendingMessages     = 50
	ReconnectStateTTL      = 2 * time.Minute
	CompletedReconnectTTL  = 5 * time.Minute
)

type ConnectionState int32

const (
	StateConnecting ConnectionState = iota
	StateConnected
	StateDisconnecting
	StateDisconnected
)

func (s ConnectionState) String() string {
	switch s {
	case StateConnecting:
		return "connecting"
	case StateConnected:
		return "connected"
	case StateDisconnecting:
		return "disconnecting"
	case StateDisconnected:
		return "disconnected"
	default:
		return "unknown"
	}
}

type ReconnectBackoff struct {
	attempt    int
	maxAttempt int
	delay      time.Duration
	minDelay   time.Duration
	maxDelay   time.Duration
}

func NewReconnectBackoff() *ReconnectBackoff {
	return &ReconnectBackoff{
		attempt:    0,
		maxAttempt: MaxReconnectAttempts,
		delay:      MinReconnectDelay,
		minDelay:   MinReconnectDelay,
		maxDelay:   MaxReconnectDelay,
	}
}

func (rb *ReconnectBackoff) Reset() {
	rb.attempt = 0
	rb.delay = rb.minDelay
}

func (rb *ReconnectBackoff) Next() (time.Duration, bool) {
	if rb.attempt >= rb.maxAttempt {
		return 0, false
	}
	rb.attempt++
	delay := rb.delay
	rb.delay = time.Duration(float64(rb.delay) * 2)
	if rb.delay > rb.maxDelay {
		rb.delay = rb.maxDelay
	}
	return delay, true
}

type TypingState struct {
	UserID    uuid.UUID
	RoomID    uuid.UUID
	StartedAt time.Time
	Context   string
}

type TypingPayload struct {
	Context   string `json:"context,omitempty"`
	ChannelID string `json:"channel_id,omitempty"`
	ThreadID  string `json:"thread_id,omitempty"`
}

type TypingEvent struct {
	UserID      uuid.UUID `json:"user_id"`
	RoomID      uuid.UUID `json:"room_id"`
	ChannelID   string    `json:"channel_id,omitempty"`
	ThreadID    string    `json:"thread_id,omitempty"`
	ContextType string    `json:"context_type,omitempty"`
	IsTyping    bool      `json:"is_typing"`
	Timestamp   time.Time `json:"timestamp"`
}

type RateLimiter struct {
	mu         sync.Mutex
	messages   int
	resetTime  time.Time
	limit      int
	windowSecs int
}

func NewRateLimiter(limit int, windowSecs int) *RateLimiter {
	return &RateLimiter{
		limit:      limit,
		windowSecs: windowSecs,
		resetTime:  time.Now().Add(time.Duration(windowSecs) * time.Second),
	}
}

func (r *RateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if time.Now().After(r.resetTime) {
		r.messages = 0
		r.resetTime = time.Now().Add(time.Duration(r.windowSecs) * time.Second)
	}

	if r.messages >= r.limit {
		return false
	}
	r.messages++
	return true
}

type Message struct {
	Type         string          `json:"type"`
	RoomID       uuid.UUID       `json:"room_id,omitempty"`
	UserID       uuid.UUID       `json:"user_id,omitempty"`
	TargetUserID uuid.UUID       `json:"target_user_id,omitempty"`
	Payload      json.RawMessage `json:"payload,omitempty"`
	SDP          string          `json:"sdp,omitempty"`
	Candidate    string          `json:"candidate,omitempty"`
	Timestamp    time.Time       `json:"timestamp"`
}

type PendingMessage struct {
	ID        uuid.UUID       `json:"id"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
	RoomID    uuid.UUID       `json:"room_id"`
}

type DeliveryStatus struct {
	MessageID   string    `json:"message_id"`
	Status      string    `json:"status"`
	SentAt      time.Time `json:"sent_at"`
	DeliveredAt time.Time `json:"delivered_at,omitempty"`
	ReadAt      time.Time `json:"read_at,omitempty"`
	RetryCount  int       `json:"retry_count"`
	LastAttempt time.Time `json:"last_attempt"`
}

type ReconnectState struct {
	UserID        uuid.UUID
	OldConnection uuid.UUID
	NewConnection uuid.UUID
	RoomID        uuid.UUID
	Timestamp     time.Time
	Attempts      int
	Completed     bool
}

type HubMetrics struct {
	ActiveClients int64         `json:"active_clients"`
	TotalMessages int64         `json:"total_messages"`
	Uptime        time.Duration `json:"uptime"`
	RoomCount     int           `json:"room_count"`
	RedisEnabled  bool          `json:"redis_enabled"`
}
