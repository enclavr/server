package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type User struct {
	ID           uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	Username     string         `gorm:"uniqueIndex;not null;size:50" json:"username"`
	Email        string         `gorm:"uniqueIndex;not null" json:"email,omitempty"`
	PasswordHash string         `gorm:"not null" json:"-"`
	DisplayName  string         `gorm:"size:100" json:"display_name"`
	AvatarURL    string         `gorm:"size:500" json:"avatar_url"`
	IsAdmin      bool           `gorm:"default:false" json:"is_admin"`
	OIDCSubject  string         `gorm:"size:255" json:"-"`
	OIDCIssuer   string         `gorm:"size:255" json:"-"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`

	Rooms         []UserRoom     `gorm:"foreignKey:UserID" json:"-"`
	Sessions      []Session      `gorm:"foreignKey:UserID" json:"-"`
	RefreshTokens []RefreshToken `gorm:"foreignKey:UserID" json:"-"`
}

func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	return nil
}

type Room struct {
	ID          uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	Name        string         `gorm:"uniqueIndex;not null;size:100" json:"name"`
	Description string         `gorm:"size:500" json:"description"`
	Password    string         `gorm:"size:255" json:"-"`
	IsPrivate   bool           `gorm:"default:false" json:"is_private"`
	MaxUsers    int            `gorm:"default:50" json:"max_users"`
	CreatedBy   uuid.UUID      `gorm:"type:uuid;not null" json:"created_by"`
	CategoryID  *uuid.UUID     `gorm:"type:uuid" json:"category_id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`

	Users []UserRoom `gorm:"foreignKey:RoomID" json:"-"`
}

func (r *Room) BeforeCreate(tx *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return nil
}

type Category struct {
	ID        uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	Name      string         `gorm:"uniqueIndex;not null;size:100" json:"name"`
	SortOrder int            `gorm:"default:0" json:"sort_order"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	Rooms []Room `gorm:"foreignKey:CategoryID" json:"-"`
}

func (c *Category) BeforeCreate(tx *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return nil
}

type UserRoom struct {
	ID       uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	UserID   uuid.UUID `gorm:"type:uuid;not null" json:"user_id"`
	RoomID   uuid.UUID `gorm:"type:uuid;not null" json:"room_id"`
	Role     string    `gorm:"size:20;default:'member'" json:"role"`
	JoinedAt time.Time `json:"joined_at"`

	User User `gorm:"foreignKey:UserID" json:"-"`
	Room Room `gorm:"foreignKey:RoomID" json:"-"`
}

func (ur *UserRoom) BeforeCreate(tx *gorm.DB) error {
	if ur.ID == uuid.Nil {
		ur.ID = uuid.New()
	}
	if ur.JoinedAt.IsZero() {
		ur.JoinedAt = time.Now()
	}
	return nil
}

type Session struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null" json:"user_id"`
	Token     string    `gorm:"uniqueIndex;not null" json:"-"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
	IPAddress string    `gorm:"size:45" json:"ip_address"`
	UserAgent string    `gorm:"size:500" json:"user_agent"`

	User User `gorm:"foreignKey:UserID" json:"-"`
}

func (s *Session) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}

type RefreshToken struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null" json:"user_id"`
	Token     string    `gorm:"uniqueIndex;not null" json:"-"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`

	User User `gorm:"foreignKey:UserID" json:"-"`
}

func (rt *RefreshToken) BeforeCreate(tx *gorm.DB) error {
	if rt.ID == uuid.Nil {
		rt.ID = uuid.New()
	}
	return nil
}

type MessageType string

const (
	MessageTypeText   MessageType = "text"
	MessageTypeSystem MessageType = "system"
)

type Message struct {
	ID        uuid.UUID   `gorm:"type:uuid;primary_key" json:"id"`
	RoomID    uuid.UUID   `gorm:"type:uuid;not null;index" json:"room_id"`
	UserID    uuid.UUID   `gorm:"type:uuid;not null" json:"user_id"`
	Type      MessageType `gorm:"type:varchar(20);default:'text'" json:"type"`
	Content   string      `gorm:"type:text;not null" json:"content"`
	IsEdited  bool        `gorm:"default:false" json:"is_edited"`
	IsDeleted bool        `gorm:"default:false" json:"is_deleted"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`

	User User `gorm:"foreignKey:UserID" json:"-"`
	Room Room `gorm:"foreignKey:RoomID" json:"-"`
}

func (m *Message) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	return nil
}

type DirectMessage struct {
	ID         uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	SenderID   uuid.UUID `gorm:"type:uuid;not null" json:"sender_id"`
	ReceiverID uuid.UUID `gorm:"type:uuid;not null" json:"receiver_id"`
	Content    string    `gorm:"type:text;not null" json:"content"`
	IsEdited   bool      `gorm:"default:false" json:"is_edited"`
	IsDeleted  bool      `gorm:"default:false" json:"is_deleted"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`

	Sender   User `gorm:"foreignKey:SenderID" json:"-"`
	Receiver User `gorm:"foreignKey:ReceiverID" json:"-"`
}

func (dm *DirectMessage) BeforeCreate(tx *gorm.DB) error {
	if dm.ID == uuid.Nil {
		dm.ID = uuid.New()
	}
	if dm.CreatedAt.IsZero() {
		dm.CreatedAt = time.Now()
	}
	return nil
}

type PinnedMessage struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	RoomID    uuid.UUID `gorm:"type:uuid;not null;index" json:"room_id"`
	MessageID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex" json:"message_id"`
	PinnedBy  uuid.UUID `gorm:"type:uuid;not null" json:"pinned_by"`
	CreatedAt time.Time `json:"created_at"`

	Room    Room    `gorm:"foreignKey:RoomID" json:"-"`
	Message Message `gorm:"foreignKey:MessageID" json:"-"`
	User    User    `gorm:"foreignKey:PinnedBy" json:"-"`
}

func (pm *PinnedMessage) BeforeCreate(tx *gorm.DB) error {
	if pm.ID == uuid.Nil {
		pm.ID = uuid.New()
	}
	if pm.CreatedAt.IsZero() {
		pm.CreatedAt = time.Now()
	}
	return nil
}

type Invite struct {
	ID        uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	Code      string         `gorm:"uniqueIndex;not null;size:32" json:"code"`
	RoomID    uuid.UUID      `gorm:"type:uuid;not null;index" json:"room_id"`
	CreatedBy uuid.UUID      `gorm:"type:uuid;not null" json:"created_by"`
	ExpiresAt time.Time      `json:"expires_at"`
	MaxUses   int            `gorm:"default:0" json:"max_uses"`
	Uses      int            `gorm:"default:0" json:"uses"`
	IsRevoked bool           `gorm:"default:false" json:"is_revoked"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	Room Room `gorm:"foreignKey:RoomID" json:"-"`
	User User `gorm:"foreignKey:CreatedBy" json:"-"`
}

func (i *Invite) BeforeCreate(tx *gorm.DB) error {
	if i.ID == uuid.Nil {
		i.ID = uuid.New()
	}
	if i.Code == "" {
		i.Code = generateInviteCode()
	}
	if i.CreatedAt.IsZero() {
		i.CreatedAt = time.Now()
	}
	if i.ExpiresAt.IsZero() {
		i.ExpiresAt = time.Now().Add(7 * 24 * time.Hour)
	}
	return nil
}

func generateInviteCode() string {
	return uuid.New().String()[:8]
}

type MessageReaction struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	MessageID uuid.UUID `gorm:"type:uuid;not null;index:idx_message_reaction" json:"message_id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index:idx_message_reaction" json:"user_id"`
	Emoji     string    `gorm:"size:100;not null" json:"emoji"`
	CreatedAt time.Time `json:"created_at"`

	Message Message `gorm:"foreignKey:MessageID" json:"-"`
	User    User    `gorm:"foreignKey:UserID" json:"-"`
}

func (mr *MessageReaction) BeforeCreate(tx *gorm.DB) error {
	if mr.ID == uuid.Nil {
		mr.ID = uuid.New()
	}
	if mr.CreatedAt.IsZero() {
		mr.CreatedAt = time.Now()
	}
	return nil
}

type ServerSettings struct {
	ID                   uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	ServerName           string    `gorm:"size:100;default:'Enclavr Server'" json:"server_name"`
	ServerDescription    string    `gorm:"size:500" json:"server_description"`
	AllowRegistration    bool      `gorm:"default:true" json:"allow_registration"`
	MaxRoomsPerUser      int       `gorm:"default:10" json:"max_rooms_per_user"`
	MaxMembersPerRoom    int       `gorm:"default:50" json:"max_members_per_room"`
	EnableVoiceChat      bool      `gorm:"default:true" json:"enable_voice_chat"`
	EnableDirectMessages bool      `gorm:"default:true" json:"enable_direct_messages"`
	EnableFileUploads    bool      `gorm:"default:false" json:"enable_file_uploads"`
	MaxUploadSizeMB      int       `gorm:"default:10" json:"max_upload_size_mb"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

func (ss *ServerSettings) BeforeCreate(tx *gorm.DB) error {
	if ss.ID == uuid.Nil {
		ss.ID = uuid.New()
	}
	if ss.CreatedAt.IsZero() {
		ss.CreatedAt = time.Now()
	}
	return nil
}

type WebhookEvent string

const (
	EventMessageCreated  WebhookEvent = "message.created"
	EventMessageDeleted  WebhookEvent = "message.deleted"
	EventUserJoinedRoom  WebhookEvent = "user.joined_room"
	EventUserLeftRoom    WebhookEvent = "user.left_room"
	EventUserJoinedVoice WebhookEvent = "user.joined_voice"
	EventUserLeftVoice   WebhookEvent = "user.left_voice"
)

type Webhook struct {
	ID        uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	RoomID    uuid.UUID      `gorm:"type:uuid;not null;index" json:"room_id"`
	URL       string         `gorm:"not null;size:500" json:"url"`
	Secret    string         `gorm:"size:255" json:"-"`
	Events    string         `gorm:"type:text;not null" json:"events"`
	IsActive  bool           `gorm:"default:true" json:"is_active"`
	CreatedBy uuid.UUID      `gorm:"type:uuid;not null" json:"created_by"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	Room Room `gorm:"foreignKey:RoomID" json:"-"`
	User User `gorm:"foreignKey:CreatedBy" json:"-"`
}

func (w *Webhook) BeforeCreate(tx *gorm.DB) error {
	if w.ID == uuid.Nil {
		w.ID = uuid.New()
	}
	if w.CreatedAt.IsZero() {
		w.CreatedAt = time.Now()
	}
	return nil
}

type WebhookLog struct {
	ID           uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	WebhookID    uuid.UUID `gorm:"type:uuid;not null;index" json:"webhook_id"`
	Event        string    `gorm:"size:50;not null" json:"event"`
	Payload      string    `gorm:"type:text;not null" json:"payload"`
	StatusCode   int       `json:"status_code"`
	Success      bool      `gorm:"default:false" json:"success"`
	ErrorMessage string    `gorm:"size:500" json:"error_message"`
	ResponseBody string    `gorm:"type:text" json:"response_body"`
	CreatedAt    time.Time `json:"created_at"`

	Webhook Webhook `gorm:"foreignKey:WebhookID" json:"-"`
}

func (wl *WebhookLog) BeforeCreate(tx *gorm.DB) error {
	if wl.ID == uuid.Nil {
		wl.ID = uuid.New()
	}
	if wl.CreatedAt.IsZero() {
		wl.CreatedAt = time.Now()
	}
	return nil
}
