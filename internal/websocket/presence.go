package websocket

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
)

type PresenceInfo struct {
	UserID        uuid.UUID `json:"user_id"`
	Status        string    `json:"status"` // "online", "idle", "away", "dnd", "offline"
	Since         time.Time `json:"since"`
	Activity      string    `json:"activity,omitempty"`
	CustomStatus  string    `json:"custom_status,omitempty"`
	Device        string    `json:"device,omitempty"`    // "mobile", "desktop", "web"
	LastSeen      time.Time `json:"last_seen,omitempty"` // For offline users
	ClientVersion string    `json:"client_version,omitempty"`
}

type RoomEvent struct {
	Type      string          `json:"type"`
	RoomID    uuid.UUID       `json:"room_id"`
	UserID    uuid.UUID       `json:"user_id,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
}

type RoomNotification struct {
	ID           string          `json:"id"`
	Type         string          `json:"type"` // "user_joined", "user_left", "typing", "mention", "message", "user_kicked", "user_banned", "role_changed", "room_archived"
	RoomID       uuid.UUID       `json:"room_id"`
	UserID       uuid.UUID       `json:"user_id,omitempty"`
	TargetUserID uuid.UUID       `json:"target_user_id,omitempty"`
	Message      string          `json:"message,omitempty"`
	Timestamp    time.Time       `json:"timestamp"`
	Read         bool            `json:"read"`
	Actionable   bool            `json:"actionable"`
	Reason       string          `json:"reason,omitempty"`
	Duration     string          `json:"duration,omitempty"` // For bans: "permanent" or "24h", "7d", etc.
	Payload      json.RawMessage `json:"payload,omitempty"`
}

type RoomNotificationSettings struct {
	Enabled       bool `json:"enabled"`
	Sound         bool `json:"sound"`
	MentionOnly   bool `json:"mention_only"`
	DesktopNotify bool `json:"desktop_notify"`
}

type RoomActivity struct {
	UserID       uuid.UUID `json:"user_id"`
	RoomID       uuid.UUID `json:"room_id"`
	Status       string    `json:"status"` // "active", "idle", "away"
	LastActivity time.Time `json:"last_activity"`
	JoinedAt     time.Time `json:"joined_at"`
}

type RoomStats struct {
	RoomID        uuid.UUID `json:"room_id"`
	TotalUsers    int       `json:"total_users"`
	ActiveUsers   int       `json:"active_users"`
	IdleUsers     int       `json:"idle_users"`
	TypingUsers   int       `json:"typing_users"`
	MutedUsers    int       `json:"muted_users"`
	DeafenedUsers int       `json:"deafened_users"`
	SpeakingUsers int       `json:"speaking_users"`
	ScreenSharing int       `json:"screen_sharing"`
	LastActivity  time.Time `json:"last_activity"`
}

type UserActivityPayload struct {
	Status    string `json:"status,omitempty"` // "active", "idle", "away"
	ChannelID string `json:"channel_id,omitempty"`
	ThreadID  string `json:"thread_id,omitempty"`
}

type RoomNotificationEvent struct {
	Type      string                 `json:"type"` // "user-joined", "user-left", "user-kicked", "user-banned", "room-settings-changed"
	UserID    uuid.UUID              `json:"user_id"`
	RoomID    uuid.UUID              `json:"room_id"`
	Timestamp time.Time              `json:"timestamp"`
	User      *UserInfo              `json:"user,omitempty"`
	ExtraData map[string]interface{} `json:"extra_data,omitempty"`
}

type UserInfo struct {
	ID        uuid.UUID `json:"id"`
	Username  string    `json:"username,omitempty"`
	AvatarURL string    `json:"avatar_url,omitempty"`
}

type ConnectionHealth struct {
	ConnectionID    uuid.UUID `json:"connection_id"`
	UserID          uuid.UUID `json:"user_id"`
	LatencyMs       int64     `json:"latency_ms"`
	State           string    `json:"state"`
	LastPing        time.Time `json:"last_ping"`
	LastPong        time.Time `json:"last_pong"`
	PacketsReceived int64     `json:"packets_received"`
	PacketsSent     int64     `json:"packets_sent"`
	BytesReceived   int64     `json:"bytes_received"`
	BytesSent       int64     `json:"bytes_sent"`
	ErrorCount      int32     `json:"error_count"`
	ConnectedAt     time.Time `json:"connected_at"`
}

type ConnectionQuality struct {
	ConnectionID uuid.UUID `json:"connection_id"`
	UserID       uuid.UUID `json:"user_id"`
	Quality      string    `json:"quality"` // "excellent", "good", "fair", "poor", "disconnected"
	LatencyMs    int64     `json:"latency_ms"`
	PacketLoss   float64   `json:"packet_loss,omitempty"`
}

type TypingIndicator struct {
	UserID      uuid.UUID `json:"user_id"`
	RoomID      uuid.UUID `json:"room_id"`
	Context     string    `json:"context"` // "room", "channel:{id}", "thread:{id}", "dm:{id}"
	ContextID   string    `json:"context_id,omitempty"`
	ContextType string    `json:"context_type,omitempty"` // "room", "channel", "thread", "dm"
	ChannelID   string    `json:"channel_id,omitempty"`
	ThreadID    string    `json:"thread_id,omitempty"`
	IsTyping    bool      `json:"is_typing"`
	Timestamp   time.Time `json:"timestamp"`
}

type OnlineUser struct {
	UserID          uuid.UUID `json:"user_id"`
	ConnectionID    uuid.UUID `json:"connection_id"`
	State           string    `json:"state"`
	ConnectedAt     time.Time `json:"connected_at"`
	LastSeen        time.Time `json:"last_seen"`
	RemoteAddress   string    `json:"remote_address,omitempty"`
	Speaking        bool      `json:"speaking,omitempty"`
	Muted           bool      `json:"muted,omitempty"`
	Deafened        bool      `json:"deafened,omitempty"`
	ScreenSharing   bool      `json:"screen_sharing,omitempty"`
	DeviceInfo      string    `json:"device_info,omitempty"`
	ClientVersion   string    `json:"client_version,omitempty"`
	PresenceStatus  string    `json:"presence_status,omitempty"` // "online", "idle", "away", "dnd"
	CustomStatus    string    `json:"custom_status,omitempty"`
	ActiveChannelID string    `json:"active_channel_id,omitempty"`
	ActiveThreadID  string    `json:"active_thread_id,omitempty"`
}

type PresencePayload struct {
	Status   string `json:"status"` // "online", "idle", "away", "dnd"
	Since    int64  `json:"since"`
	Activity string `json:"activity,omitempty"`
	Custom   string `json:"custom_status,omitempty"`
}

type TypingBulkPayload struct {
	TypingUsers []TypingEvent `json:"typing_users"`
}

type ClientReadyPayload struct {
	ClientVersion string `json:"client_version,omitempty"`
	DeviceInfo    string `json:"device_info,omitempty"`
	Platform      string `json:"platform,omitempty"` // "web", "desktop", "mobile"
	Locale        string `json:"locale,omitempty"`
}

type TypingStatusPayload struct {
	Context     string `json:"context"`      // "room", "channel:123", "thread:456", "dm:789"
	ContextID   string `json:"context_id"`   // ID of channel/thread/dm
	ContextType string `json:"context_type"` // "room", "channel", "thread", "dm"
	IsTyping    bool   `json:"is_typing"`
}

type NotificationPreferencePayload struct {
	RoomID         string `json:"room_id,omitempty"`
	Mentions       *bool  `json:"mentions,omitempty"`
	Messages       *bool  `json:"messages,omitempty"`
	Reactions      *bool  `json:"reactions,omitempty"`
	VoiceActivity  *bool  `json:"voice_activity,omitempty"`
	ThreadReplies  *bool  `json:"thread_replies,omitempty"`
	DirectMessages *bool  `json:"direct_messages,omitempty"`
	CustomKeyword  string `json:"custom_keyword,omitempty"`
	SoundEnabled   *bool  `json:"sound_enabled,omitempty"`
	DesktopEnabled *bool  `json:"desktop_enabled,omitempty"`
	MobileEnabled  *bool  `json:"mobile_enabled,omitempty"`
	MuteDuration   int    `json:"mute_duration,omitempty"` // minutes, 0 = indefinite
}

type RoomNotificationPreferences struct {
	RoomID         string    `json:"room_id"`
	Mentions       bool      `json:"mentions"`
	Messages       bool      `json:"messages"`
	Reactions      bool      `json:"reactions"`
	VoiceActivity  bool      `json:"voice_activity"`
	ThreadReplies  bool      `json:"thread_replies"`
	DirectMessages bool      `json:"direct_messages"`
	CustomKeyword  string    `json:"custom_keyword,omitempty"`
	SoundEnabled   bool      `json:"sound_enabled"`
	DesktopEnabled bool      `json:"desktop_enabled"`
	MobileEnabled  bool      `json:"mobile_enabled"`
	MuteDuration   int       `json:"mute_duration"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type EnhancedTypingPayload struct {
	Context        string      `json:"context"`    // "room", "channel", "thread", "dm"
	ContextID      string      `json:"context_id"` // ID of the context (channel_id, thread_id, dm_id)
	ChannelID      string      `json:"channel_id,omitempty"`
	ThreadID       string      `json:"thread_id,omitempty"`
	DmID           string      `json:"dm_id,omitempty"`
	IsTyping       bool        `json:"is_typing"`
	MessageID      string      `json:"message_id,omitempty"` // For reply typing indicators
	MentionedUsers []uuid.UUID `json:"mentioned_users,omitempty"`
}

type EnhancedTypingEvent struct {
	UserID         uuid.UUID   `json:"user_id"`
	RoomID         uuid.UUID   `json:"room_id"`
	Context        string      `json:"context"`
	ContextID      string      `json:"context_id"`
	IsTyping       bool        `json:"is_typing"`
	MessageID      string      `json:"message_id,omitempty"`
	MentionedUsers []uuid.UUID `json:"mentioned_users,omitempty"`
	Timestamp      time.Time   `json:"timestamp"`
}

type PresenceEventPayload struct {
	UserID       uuid.UUID `json:"user_id"`
	Status       string    `json:"status"` // "online", "idle", "away", "dnd", "offline"
	Activity     string    `json:"activity,omitempty"`
	CustomStatus string    `json:"custom_status,omitempty"`
	Device       string    `json:"device,omitempty"` // "mobile", "desktop", "web"
	Since        int64     `json:"since"`
}

type PresenceEvent struct {
	UserID       uuid.UUID `json:"user_id"`
	Status       string    `json:"status"`
	Activity     string    `json:"activity,omitempty"`
	CustomStatus string    `json:"custom_status,omitempty"`
	Device       string    `json:"device,omitempty"`
	Since        time.Time `json:"since"`
	Timestamp    time.Time `json:"timestamp"`
}

type MessageAckPayload struct {
	MessageID   string `json:"message_id"`
	Delivered   bool   `json:"delivered"`
	ReceivedAt  int64  `json:"received_at"`
	SequenceNum int64  `json:"sequence_num,omitempty"`
}

type MessageDeliveryStatusPayload struct {
	MessageIDs []string `json:"message_ids"`
	Status     string   `json:"status"` // "delivered", "read", "failed"
}

type UserFocusPayload struct {
	FocusContext string `json:"focus_context"` // "room", "channel", "thread", "dm", "settings", "profile"
	ContextID    string `json:"context_id"`
	PanelID      string `json:"panel_id"` // "chat", "voice", "members", "files"
	IsFocused    bool   `json:"is_focused"`
}

type MediaStatePayload struct {
	Muted         bool `json:"muted"`
	Deafened      bool `json:"deafened"`
	ScreenSharing bool `json:"screen_sharing"`
	VideoEnabled  bool `json:"video_enabled"`
	VoiceActive   bool `json:"voice_active"`
}

type TypingPausePayload struct {
	Context     string `json:"context"`
	ChannelID   string `json:"channel_id"`
	ThreadID    string `json:"thread_id"`
	ContextType string `json:"context_type"`
	Paused      bool   `json:"paused"`
}

type UserFocusWindowPayload struct {
	Context string `json:"context"` // "room", "channel", "thread", "dm"
	Focused bool   `json:"focused"`
}

type ReactionPayload struct {
	MessageID string `json:"message_id"`
	Emoji     string `json:"emoji"`
	Action    string `json:"action"` // "add" or "remove"
}

type MessagePinPayload struct {
	MessageID string `json:"message_id"`
	Action    string `json:"action"` // "pin" or "unpin"
	ChannelID string `json:"channel_id,omitempty"`
}

type UserViewingChannelPayload struct {
	ChannelID string `json:"channel_id"`
	Viewing   bool   `json:"viewing"`
}

type RoomConfigPayload struct {
	Key   string      `json:"key"`
	Value interface{} `json:"value"`
}

type PresenceState struct {
	UserID    uuid.UUID
	RoomID    uuid.UUID
	Status    string
	UpdatedAt time.Time
}

type RoomState struct {
	RoomID        uuid.UUID    `json:"room_id"`
	OnlineUsers   []OnlineUser `json:"online_users"`
	TypingUsers   []uuid.UUID  `json:"typing_users"`
	ActiveClients int          `json:"active_clients"`
}

type UserStatusUpdate struct {
	UserID       uuid.UUID `json:"user_id"`
	RoomID       uuid.UUID `json:"room_id"`
	Status       string    `json:"status"` // "online", "idle", "away", "dnd"
	CustomStatus string    `json:"custom_status,omitempty"`
	Device       string    `json:"device,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
}

type RoomStateSubscription struct {
	mu          sync.RWMutex
	subscribers map[uuid.UUID]map[uuid.UUID]bool
}

type RoomNotificationSubscription struct {
	RoomID         uuid.UUID `json:"room_id"`
	Types          []string  `json:"types"` // "user_joined", "user_left", "typing", "message", "mention"
	NotifySelf     bool      `json:"notify_self"`
	IncludeHistory bool      `json:"include_history"`
}

type NotificationStore struct {
	sync.RWMutex
	notifications map[uuid.UUID][]RoomNotification
}

type TypingDebouncer struct {
	timers    map[uuid.UUID]*time.Timer
	mutex     sync.Mutex
	delay     time.Duration
	onTimeout func(uuid.UUID)
}
