package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type User struct {
	ID               uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	Username         string         `gorm:"uniqueIndex;not null;size:50" json:"username"`
	Email            string         `gorm:"uniqueIndex;not null" json:"email,omitempty"`
	PasswordHash     string         `gorm:"not null" json:"-"`
	DisplayName      string         `gorm:"size:100" json:"display_name"`
	AvatarURL        string         `gorm:"size:500" json:"avatar_url"`
	IsAdmin          bool           `gorm:"default:false" json:"is_admin"`
	OIDCSubject      string         `gorm:"size:255" json:"-"`
	OIDCIssuer       string         `gorm:"size:255" json:"-"`
	TwoFactorEnabled bool           `gorm:"default:false" json:"two_factor_enabled"`
	TwoFactorSecret  string         `gorm:"size:255" json:"-"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`

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

type InviteLink struct {
	ID          uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	Code        string         `gorm:"uniqueIndex;not null;size:64" json:"code"`
	RoomID      uuid.UUID      `gorm:"type:uuid;not null;index" json:"room_id"`
	CreatedBy   uuid.UUID      `gorm:"type:uuid;not null" json:"created_by"`
	Title       string         `gorm:"size:100" json:"title"`
	Description string         `gorm:"size:255" json:"description"`
	MaxUses     int            `gorm:"default:0" json:"max_uses"`
	Uses        int            `gorm:"default:0" json:"uses"`
	IsPermanent bool           `gorm:"default:true" json:"is_permanent"`
	IsEnabled   bool           `gorm:"default:true" json:"is_enabled"`
	ExpiresAt   *time.Time     `json:"expires_at"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`

	Room Room `gorm:"foreignKey:RoomID" json:"-"`
	User User `gorm:"foreignKey:CreatedBy" json:"-"`
}

func (il *InviteLink) BeforeCreate(tx *gorm.DB) error {
	if il.ID == uuid.Nil {
		il.ID = uuid.New()
	}
	if il.Code == "" {
		il.Code = generateInviteLinkCode()
	}
	if il.CreatedAt.IsZero() {
		il.CreatedAt = time.Now()
	}
	if il.UpdatedAt.IsZero() {
		il.UpdatedAt = time.Now()
	}
	return nil
}

func generateInviteLinkCode() string {
	return uuid.New().String()[:12]
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

type RoomSettings struct {
	ID                uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	RoomID            uuid.UUID `gorm:"type:uuid;not null;uniqueIndex" json:"room_id"`
	AllowMessageEdits bool      `gorm:"default:true" json:"allow_message_edits"`
	AllowReactions    bool      `gorm:"default:true" json:"allow_reactions"`
	RequireApproval   bool      `gorm:"default:false" json:"require_approval"`
	MaxUsers          int       `gorm:"default:0" json:"max_users"`
	AutoDeleteDays    int       `gorm:"default:0" json:"auto_delete_days"`
	SlowModeSeconds   int       `gorm:"default:0" json:"slow_mode_seconds"`
	AllowVoiceChat    bool      `gorm:"default:true" json:"allow_voice_chat"`
	AllowFileUploads  bool      `gorm:"default:true" json:"allow_file_uploads"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`

	Room Room `gorm:"foreignKey:RoomID" json:"-"`
}

func (rs *RoomSettings) BeforeCreate(tx *gorm.DB) error {
	if rs.ID == uuid.Nil {
		rs.ID = uuid.New()
	}
	if rs.CreatedAt.IsZero() {
		rs.CreatedAt = time.Now()
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

type Thread struct {
	ID        uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	RoomID    uuid.UUID      `gorm:"type:uuid;not null;index" json:"room_id"`
	ParentID  uuid.UUID      `gorm:"type:uuid;not null" json:"parent_id"`
	CreatedBy uuid.UUID      `gorm:"type:uuid;not null" json:"created_by"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	Room   Room    `gorm:"foreignKey:RoomID" json:"-"`
	Parent Message `gorm:"foreignKey:ParentID" json:"-"`
	User   User    `gorm:"foreignKey:CreatedBy" json:"-"`
}

func (t *Thread) BeforeCreate(tx *gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}
	return nil
}

type ThreadMessage struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	ThreadID  uuid.UUID `gorm:"type:uuid;not null;index" json:"thread_id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null" json:"user_id"`
	Content   string    `gorm:"type:text;not null" json:"content"`
	IsEdited  bool      `gorm:"default:false" json:"is_edited"`
	IsDeleted bool      `gorm:"default:false" json:"is_deleted"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Thread Thread `gorm:"foreignKey:ThreadID" json:"-"`
	User   User   `gorm:"foreignKey:UserID" json:"-"`
}

func (tm *ThreadMessage) BeforeCreate(tx *gorm.DB) error {
	if tm.ID == uuid.Nil {
		tm.ID = uuid.New()
	}
	if tm.CreatedAt.IsZero() {
		tm.CreatedAt = time.Now()
	}
	return nil
}

type Poll struct {
	ID         uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	RoomID     uuid.UUID      `gorm:"type:uuid;not null;index" json:"room_id"`
	Question   string         `gorm:"size:500;not null" json:"question"`
	CreatedBy  uuid.UUID      `gorm:"type:uuid;not null" json:"created_by"`
	IsMultiple bool           `gorm:"default:false" json:"is_multiple"`
	ExpiresAt  *time.Time     `json:"expires_at"`
	CreatedAt  time.Time      `json:"created_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`

	Room Room `gorm:"foreignKey:RoomID" json:"-"`
	User User `gorm:"foreignKey:CreatedBy" json:"-"`
}

func (p *Poll) BeforeCreate(tx *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}
	return nil
}

type PollOption struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	PollID    uuid.UUID `gorm:"type:uuid;not null;index" json:"poll_id"`
	Text      string    `gorm:"size:500;not null" json:"text"`
	Position  int       `gorm:"default:0" json:"position"`
	CreatedAt time.Time `json:"created_at"`

	Poll Poll `gorm:"foreignKey:PollID" json:"-"`
}

func (po *PollOption) BeforeCreate(tx *gorm.DB) error {
	if po.ID == uuid.Nil {
		po.ID = uuid.New()
	}
	if po.CreatedAt.IsZero() {
		po.CreatedAt = time.Now()
	}
	return nil
}

type PollVote struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	PollID    uuid.UUID `gorm:"type:uuid;not null;index:idx_poll_vote" json:"poll_id"`
	OptionID  uuid.UUID `gorm:"type:uuid;not null;index:idx_poll_vote" json:"option_id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index:idx_poll_vote" json:"user_id"`
	CreatedAt time.Time `json:"created_at"`

	Poll   Poll       `gorm:"foreignKey:PollID" json:"-"`
	Option PollOption `gorm:"foreignKey:OptionID" json:"-"`
	User   User       `gorm:"foreignKey:UserID" json:"-"`
}

func (pv *PollVote) BeforeCreate(tx *gorm.DB) error {
	if pv.ID == uuid.Nil {
		pv.ID = uuid.New()
	}
	if pv.CreatedAt.IsZero() {
		pv.CreatedAt = time.Now()
	}
	return nil
}

type ServerEmoji struct {
	ID        uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	Name      string         `gorm:"size:50;not null" json:"name"`
	ImageURL  string         `gorm:"size:500;not null" json:"image_url"`
	CreatedBy uuid.UUID      `gorm:"type:uuid;not null" json:"created_by"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	User User `gorm:"foreignKey:CreatedBy" json:"-"`
}

func (se *ServerEmoji) BeforeCreate(tx *gorm.DB) error {
	if se.ID == uuid.Nil {
		se.ID = uuid.New()
	}
	if se.CreatedAt.IsZero() {
		se.CreatedAt = time.Now()
	}
	return nil
}

type ServerSticker struct {
	ID        uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	Name      string         `gorm:"size:50;not null" json:"name"`
	ImageURL  string         `gorm:"size:500;not null" json:"image_url"`
	CreatedBy uuid.UUID      `gorm:"type:uuid;not null" json:"created_by"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	User User `gorm:"foreignKey:CreatedBy" json:"-"`
}

func (ss *ServerSticker) BeforeCreate(tx *gorm.DB) error {
	if ss.ID == uuid.Nil {
		ss.ID = uuid.New()
	}
	if ss.CreatedAt.IsZero() {
		ss.CreatedAt = time.Now()
	}
	return nil
}

type SoundboardSound struct {
	ID        uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	Name      string         `gorm:"size:50;not null" json:"name"`
	AudioURL  string         `gorm:"size:500;not null" json:"audio_url"`
	Hotkey    string         `gorm:"size:10" json:"hotkey"`
	Volume    float64        `gorm:"default:1.0" json:"volume"`
	CreatedBy uuid.UUID      `gorm:"type:uuid;not null" json:"created_by"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	User User `gorm:"foreignKey:CreatedBy" json:"-"`
}

func (sbs *SoundboardSound) BeforeCreate(tx *gorm.DB) error {
	if sbs.ID == uuid.Nil {
		sbs.ID = uuid.New()
	}
	if sbs.CreatedAt.IsZero() {
		sbs.CreatedAt = time.Now()
	}
	return nil
}

type DailyAnalytics struct {
	ID            uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	Date          time.Time `gorm:"uniqueIndex;not null" json:"date"`
	TotalMessages int       `gorm:"default:0" json:"total_messages"`
	TotalUsers    int       `gorm:"default:0" json:"total_users"`
	ActiveUsers   int       `gorm:"default:0" json:"active_users"`
	NewUsers      int       `gorm:"default:0" json:"new_users"`
	VoiceMinutes  int       `gorm:"default:0" json:"voice_minutes"`
	CreatedAt     time.Time `json:"created_at"`
}

func (da *DailyAnalytics) BeforeCreate(tx *gorm.DB) error {
	if da.ID == uuid.Nil {
		da.ID = uuid.New()
	}
	if da.CreatedAt.IsZero() {
		da.CreatedAt = time.Now()
	}
	return nil
}

type HourlyActivity struct {
	ID           uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	Date         time.Time `gorm:"uniqueIndex:idx_date_hour;not null" json:"date"`
	Hour         int       `gorm:"uniqueIndex:idx_date_hour;not null" json:"hour"`
	MessageCount int       `gorm:"default:0" json:"message_count"`
	UserCount    int       `gorm:"default:0" json:"user_count"`
	CreatedAt    time.Time `json:"created_at"`
}

func (ha *HourlyActivity) BeforeCreate(tx *gorm.DB) error {
	if ha.ID == uuid.Nil {
		ha.ID = uuid.New()
	}
	if ha.CreatedAt.IsZero() {
		ha.CreatedAt = time.Now()
	}
	return nil
}

type ChannelActivity struct {
	ID           uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	RoomID       uuid.UUID `gorm:"type:uuid;not null;index" json:"room_id"`
	Date         time.Time `gorm:"index;not null" json:"date"`
	MessageCount int       `gorm:"default:0" json:"message_count"`
	UserCount    int       `gorm:"default:0" json:"user_count"`
	CreatedAt    time.Time `json:"created_at"`

	Room Room `gorm:"foreignKey:RoomID" json:"-"`
}

func (ca *ChannelActivity) BeforeCreate(tx *gorm.DB) error {
	if ca.ID == uuid.Nil {
		ca.ID = uuid.New()
	}
	if ca.CreatedAt.IsZero() {
		ca.CreatedAt = time.Now()
	}
	return nil
}

type AuditAction string

const (
	AuditActionMessageDelete  AuditAction = "message_delete"
	AuditActionMessageEdit    AuditAction = "message_edit"
	AuditActionUserKick       AuditAction = "user_kick"
	AuditActionUserBan        AuditAction = "user_ban"
	AuditActionUserUnban      AuditAction = "user_unban"
	AuditActionRoleChange     AuditAction = "role_change"
	AuditActionRoomCreate     AuditAction = "room_create"
	AuditActionRoomDelete     AuditAction = "room_delete"
	AuditActionRoomUpdate     AuditAction = "room_update"
	AuditActionCategoryCreate AuditAction = "category_create"
	AuditActionCategoryDelete AuditAction = "category_delete"
	AuditActionInviteCreate   AuditAction = "invite_create"
	AuditActionInviteRevoke   AuditAction = "invite_revoke"
	AuditActionSettingsChange AuditAction = "settings_change"
	AuditActionWebhookCreate  AuditAction = "webhook_create"
	AuditActionWebhookDelete  AuditAction = "webhook_delete"
)

type AuditLog struct {
	ID         uuid.UUID   `gorm:"type:uuid;primary_key" json:"id"`
	UserID     uuid.UUID   `gorm:"type:uuid;not null;index" json:"user_id"`
	Action     AuditAction `gorm:"type:varchar(50);not null;index" json:"action"`
	TargetType string      `gorm:"size:50;not null" json:"target_type"`
	TargetID   uuid.UUID   `gorm:"type:uuid;not null" json:"target_id"`
	Details    string      `gorm:"type:text" json:"details"`
	IPAddress  string      `gorm:"size:45" json:"ip_address"`
	CreatedAt  time.Time   `json:"created_at"`

	User User `gorm:"foreignKey:UserID" json:"-"`
}

func (al *AuditLog) BeforeCreate(tx *gorm.DB) error {
	if al.ID == uuid.Nil {
		al.ID = uuid.New()
	}
	if al.CreatedAt.IsZero() {
		al.CreatedAt = time.Now()
	}
	return nil
}

type Ban struct {
	ID        uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	UserID    uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	RoomID    uuid.UUID      `gorm:"type:uuid;not null;index" json:"room_id"`
	BannedBy  uuid.UUID      `gorm:"type:uuid;not null" json:"banned_by"`
	Reason    string         `gorm:"size:500" json:"reason"`
	ExpiresAt *time.Time     `json:"expires_at"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	User   User `gorm:"foreignKey:UserID" json:"-"`
	Room   Room `gorm:"foreignKey:RoomID" json:"-"`
	Banned User `gorm:"foreignKey:UserID" json:"-"`
}

func (b *Ban) BeforeCreate(tx *gorm.DB) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	if b.CreatedAt.IsZero() {
		b.CreatedAt = time.Now()
	}
	return nil
}

type ReportReason string

const (
	ReportReasonSpam           ReportReason = "spam"
	ReportReasonHarassment     ReportReason = "harassment"
	ReportReasonInappropriate  ReportReason = "inappropriate_content"
	ReportReasonViolence       ReportReason = "violence"
	ReportReasonMisinformation ReportReason = "misinformation"
	ReportReasonOther          ReportReason = "other"
)

type ReportStatus string

const (
	ReportStatusPending   ReportStatus = "pending"
	ReportStatusReviewed  ReportStatus = "reviewed"
	ReportStatusResolved  ReportStatus = "resolved"
	ReportStatusDismissed ReportStatus = "dismissed"
)

type Report struct {
	ID          uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	ReporterID  uuid.UUID      `gorm:"type:uuid;not null;index" json:"reporter_id"`
	ReportedID  uuid.UUID      `gorm:"type:uuid;not null;index" json:"reported_id"`
	RoomID      uuid.UUID      `gorm:"type:uuid;not null;index" json:"room_id"`
	MessageID   *uuid.UUID     `gorm:"type:uuid" json:"message_id"`
	Reason      ReportReason   `gorm:"type:varchar(50);not null" json:"reason"`
	Description string         `gorm:"type:text" json:"description"`
	Status      ReportStatus   `gorm:"type:varchar(20);default:'pending'" json:"status"`
	ReviewedBy  *uuid.UUID     `gorm:"type:uuid" json:"reviewed_by"`
	ReviewNotes string         `gorm:"type:text" json:"review_notes"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`

	Reporter User `gorm:"foreignKey:ReporterID" json:"-"`
	Reported User `gorm:"foreignKey:ReportedID" json:"-"`
	Room     Room `gorm:"foreignKey:RoomID" json:"-"`
}

func (r *Report) BeforeCreate(tx *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now()
	}
	return nil
}

type Bookmark struct {
	ID        uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	UserID    uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	MessageID uuid.UUID      `gorm:"type:uuid;not null;index" json:"message_id"`
	Note      string         `gorm:"size:500" json:"note"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	User    User    `gorm:"foreignKey:UserID" json:"-"`
	Message Message `gorm:"foreignKey:MessageID" json:"-"`
}

func (b *Bookmark) BeforeCreate(tx *gorm.DB) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	if b.CreatedAt.IsZero() {
		b.CreatedAt = time.Now()
	}
	return nil
}

type UserPreferences struct {
	ID               uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	UserID           uuid.UUID `gorm:"type:uuid;not null;uniqueIndex" json:"user_id"`
	Theme            string    `gorm:"size:20;default:'dark'" json:"theme"`
	Language         string    `gorm:"size:10;default:'en'" json:"language"`
	Timezone         string    `gorm:"size:50;default:'UTC'" json:"timezone"`
	MessagePreview   bool      `gorm:"default:true" json:"message_preview"`
	CompactMode      bool      `gorm:"default:false" json:"compact_mode"`
	ShowOnlineStatus bool      `gorm:"default:true" json:"show_online_status"`
	AnimatedEmoji    bool      `gorm:"default:true" json:"animated_emoji"`
	AutoPlayGifs     bool      `gorm:"default:true" json:"auto_play_gifs"`
	ReducedMotion    bool      `gorm:"default:false" json:"reduced_motion"`
	HighContrastMode bool      `gorm:"default:false" json:"high_contrast_mode"`
	TextSize         string    `gorm:"size:10;default:'medium'" json:"text_size"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`

	User User `gorm:"foreignKey:UserID" json:"-"`
}

func (up *UserPreferences) BeforeCreate(tx *gorm.DB) error {
	if up.ID == uuid.Nil {
		up.ID = uuid.New()
	}
	if up.CreatedAt.IsZero() {
		up.CreatedAt = time.Now()
	}
	return nil
}

type PasswordReset struct {
	ID        uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	UserID    uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	Token     string         `gorm:"uniqueIndex;not null" json:"-"`
	ExpiresAt time.Time      `json:"expires_at"`
	Used      bool           `gorm:"default:false" json:"used"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	User User `gorm:"foreignKey:UserID" json:"-"`
}

func (pr *PasswordReset) BeforeCreate(tx *gorm.DB) error {
	if pr.ID == uuid.Nil {
		pr.ID = uuid.New()
	}
	if pr.CreatedAt.IsZero() {
		pr.CreatedAt = time.Now()
	}
	if pr.ExpiresAt.IsZero() {
		pr.ExpiresAt = time.Now().Add(1 * time.Hour)
	}
	return nil
}

type EmailVerification struct {
	ID        uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	UserID    uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	Token     string         `gorm:"uniqueIndex;not null" json:"-"`
	ExpiresAt time.Time      `json:"expires_at"`
	Used      bool           `gorm:"default:false" json:"used"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	User User `gorm:"foreignKey:UserID" json:"-"`
}

func (ev *EmailVerification) BeforeCreate(tx *gorm.DB) error {
	if ev.ID == uuid.Nil {
		ev.ID = uuid.New()
	}
	if ev.CreatedAt.IsZero() {
		ev.CreatedAt = time.Now()
	}
	if ev.ExpiresAt.IsZero() {
		ev.ExpiresAt = time.Now().Add(24 * time.Hour)
	}
	return nil
}

type TwoFactorRecovery struct {
	ID        uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	UserID    uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	Codes     string         `gorm:"type:text" json:"-"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	User User `gorm:"foreignKey:UserID" json:"-"`
}

func (tfr *TwoFactorRecovery) BeforeCreate(tx *gorm.DB) error {
	if tfr.ID == uuid.Nil {
		tfr.ID = uuid.New()
	}
	if tfr.CreatedAt.IsZero() {
		tfr.CreatedAt = time.Now()
	}
	return nil
}

type Block struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	BlockerID uuid.UUID `gorm:"type:uuid;not null;index" json:"blocker_id"`
	BlockedID uuid.UUID `gorm:"type:uuid;not null;index" json:"blocked_id"`
	Reason    string    `gorm:"size:255" json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

func (b *Block) BeforeCreate(tx *gorm.DB) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	if b.CreatedAt.IsZero() {
		b.CreatedAt = time.Now()
	}
	return nil
}

type MessageRead struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	RoomID    uuid.UUID `gorm:"type:uuid;not null;index" json:"room_id"`
	MessageID uuid.UUID `gorm:"type:uuid;not null" json:"message_id"`
	ReadAt    time.Time `json:"read_at"`
}

func (mr *MessageRead) BeforeCreate(tx *gorm.DB) error {
	if mr.ID == uuid.Nil {
		mr.ID = uuid.New()
	}
	if mr.ReadAt.IsZero() {
		mr.ReadAt = time.Now()
	}
	return nil
}

type UserStatus string

const (
	UserStatusOnline    UserStatus = "online"
	UserStatusAway      UserStatus = "away"
	UserStatusDND       UserStatus = "dnd"
	UserStatusInvisible UserStatus = "invisible"
	UserStatusOffline   UserStatus = "offline"
)

type UserStatusModel struct {
	ID          uuid.UUID  `gorm:"type:uuid;primary_key" json:"id"`
	UserID      uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex" json:"user_id"`
	Status      UserStatus `gorm:"type:varchar(20);default:'offline'" json:"status"`
	StatusText  string     `gorm:"size:150" json:"status_text"`
	StatusEmoji string     `gorm:"size:10" json:"status_emoji"`
	ExpiresAt   *time.Time `json:"expires_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (us *UserStatusModel) BeforeCreate(tx *gorm.DB) error {
	if us.ID == uuid.Nil {
		us.ID = uuid.New()
	}
	if us.Status == "" {
		us.Status = UserStatusOffline
	}
	return nil
}
