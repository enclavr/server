package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type MessageType string

const (
	MessageTypeText   MessageType = "text"
	MessageTypeSystem MessageType = "system"
)

type Message struct {
	ID            uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	RoomID        uuid.UUID      `gorm:"type:uuid;not null;index" json:"room_id"`
	UserID        uuid.UUID      `gorm:"type:uuid;not null" json:"user_id"`
	Type          MessageType    `gorm:"type:varchar(20);default:'text'" json:"type"`
	Content       string         `gorm:"type:text;not null" json:"content"`
	IsEdited      bool           `gorm:"default:false" json:"is_edited"`
	IsDeleted     bool           `gorm:"default:false" json:"is_deleted"`
	ForwardedFrom *uuid.UUID     `gorm:"type:uuid" json:"forwarded_from,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`

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
	ID            uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	SenderID      uuid.UUID      `gorm:"type:uuid;not null" json:"sender_id"`
	ReceiverID    uuid.UUID      `gorm:"type:uuid;not null" json:"receiver_id"`
	Content       string         `gorm:"type:text;not null" json:"content"`
	IsEdited      bool           `gorm:"default:false" json:"is_edited"`
	IsDeleted     bool           `gorm:"default:false" json:"is_deleted"`
	ForwardedFrom *uuid.UUID     `gorm:"type:uuid" json:"forwarded_from,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`

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
	ID        uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	ThreadID  uuid.UUID      `gorm:"type:uuid;not null;index" json:"thread_id"`
	UserID    uuid.UUID      `gorm:"type:uuid;not null" json:"user_id"`
	Content   string         `gorm:"type:text;not null" json:"content"`
	IsEdited  bool           `gorm:"default:false" json:"is_edited"`
	IsDeleted bool           `gorm:"default:false" json:"is_deleted"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

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
	AuditActionMessageDelete      AuditAction = "message_delete"
	AuditActionMessageEdit        AuditAction = "message_edit"
	AuditActionUserKick           AuditAction = "user_kick"
	AuditActionUserBan            AuditAction = "user_ban"
	AuditActionUserUnban          AuditAction = "user_unban"
	AuditActionRoleChange         AuditAction = "role_change"
	AuditActionRoomCreate         AuditAction = "room_create"
	AuditActionRoomDelete         AuditAction = "room_delete"
	AuditActionRoomUpdate         AuditAction = "room_update"
	AuditActionCategoryCreate     AuditAction = "category_create"
	AuditActionCategoryDelete     AuditAction = "category_delete"
	AuditActionCategoryUpdate     AuditAction = "category_update"
	AuditActionInviteCreate       AuditAction = "invite_create"
	AuditActionInviteRevoke       AuditAction = "invite_revoke"
	AuditActionSettingsChange     AuditAction = "settings_change"
	AuditActionWebhookCreate      AuditAction = "webhook_create"
	AuditActionWebhookDelete      AuditAction = "webhook_delete"
	AuditActionUserLogin          AuditAction = "user_login"
	AuditActionUserLogout         AuditAction = "user_logout"
	AuditActionUserRegister       AuditAction = "user_register"
	AuditActionUserPasswordChange AuditAction = "user_password_change"
	AuditActionUserEmailChange    AuditAction = "user_email_change"
	AuditActionFileUpload         AuditAction = "file_upload"
	AuditActionFileDelete         AuditAction = "file_delete"
	AuditActionVoiceJoin          AuditAction = "voice_join"
	AuditActionVoiceLeave         AuditAction = "voice_leave"
	AuditActionPollCreate         AuditAction = "poll_create"
	AuditActionPollVote           AuditAction = "poll_vote"
	AuditActionPollEnd            AuditAction = "poll_end"
	AuditActionThreadCreate       AuditAction = "thread_create"
	AuditActionReactionAdd        AuditAction = "reaction_add"
	AuditActionReactionRemove     AuditAction = "reaction_remove"
)

type AuditLog struct {
	ID           uuid.UUID   `gorm:"type:uuid;primary_key" json:"id"`
	UserID       uuid.UUID   `gorm:"type:uuid;not null;index" json:"user_id"`
	Action       AuditAction `gorm:"type:varchar(50);not null;index" json:"action"`
	TargetType   string      `gorm:"size:50;not null" json:"target_type"`
	TargetID     uuid.UUID   `gorm:"type:uuid;not null" json:"target_id"`
	Details      string      `gorm:"type:text" json:"details"`
	OldValue     string      `gorm:"type:jsonb" json:"old_value"`
	NewValue     string      `gorm:"type:jsonb" json:"new_value"`
	IPAddress    string      `gorm:"size:45" json:"ip_address"`
	UserAgent    string      `gorm:"size:500" json:"user_agent"`
	Success      bool        `gorm:"default:true" json:"success"`
	ErrorMessage string      `gorm:"size:500" json:"error_message"`
	CreatedAt    time.Time   `json:"created_at"`

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
	Banned User `gorm:"foreignKey:BannedBy" json:"-"`
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

type MessageReminder struct {
	ID          uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	UserID      uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	MessageID   uuid.UUID      `gorm:"type:uuid;not null;index" json:"message_id"`
	RemindAt    time.Time      `gorm:"not null;index" json:"remind_at"`
	Note        string         `gorm:"size:255" json:"note"`
	IsTriggered bool           `gorm:"default:false" json:"is_triggered"`
	TriggeredAt *time.Time     `json:"triggered_at"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`

	User    User    `gorm:"foreignKey:UserID" json:"-"`
	Message Message `gorm:"foreignKey:MessageID" json:"-"`
}

func (mr *MessageReminder) BeforeCreate(tx *gorm.DB) error {
	if mr.ID == uuid.Nil {
		mr.ID = uuid.New()
	}
	if mr.CreatedAt.IsZero() {
		mr.CreatedAt = time.Now()
	}
	if mr.UpdatedAt.IsZero() {
		mr.UpdatedAt = time.Now()
	}
	return nil
}

type ScheduledMessage struct {
	ID          uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	UserID      uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	RoomID      uuid.UUID      `gorm:"type:uuid;not null;index" json:"room_id"`
	Content     string         `gorm:"type:text;not null" json:"content"`
	SendAt      time.Time      `gorm:"not null;index" json:"send_at"`
	IsSent      bool           `gorm:"default:false" json:"is_sent"`
	SentAt      *time.Time     `json:"sent_at,omitempty"`
	IsCancelled bool           `gorm:"default:false" json:"is_cancelled"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`

	User User `gorm:"foreignKey:UserID" json:"-"`
	Room Room `gorm:"foreignKey:RoomID" json:"-"`
}

func (sm *ScheduledMessage) BeforeCreate(tx *gorm.DB) error {
	if sm.ID == uuid.Nil {
		sm.ID = uuid.New()
	}
	if sm.CreatedAt.IsZero() {
		sm.CreatedAt = time.Now()
	}
	if sm.UpdatedAt.IsZero() {
		sm.UpdatedAt = time.Now()
	}
	return nil
}

type NotificationType string

const (
	NotificationTypeMention       NotificationType = "mention"
	NotificationTypeReply         NotificationType = "reply"
	NotificationTypeReaction      NotificationType = "reaction"
	NotificationTypeDirectMessage NotificationType = "direct_message"
	NotificationTypeRoomInvite    NotificationType = "room_invite"
	NotificationTypeRoomMention   NotificationType = "room_mention"
	NotificationTypeFriendRequest NotificationType = "friend_request"
	NotificationTypeFriendAccept  NotificationType = "friend_accept"
	NotificationTypePollVote      NotificationType = "poll_vote"
	NotificationTypePollEnd       NotificationType = "poll_end"
	NotificationTypeThreadReply   NotificationType = "thread_reply"
	NotificationTypeSystem        NotificationType = "system"
)

type NotificationStatus string

const (
	NotificationStatusUnread   NotificationStatus = "unread"
	NotificationStatusRead     NotificationStatus = "read"
	NotificationStatusArchived NotificationStatus = "archived"
)

type Notification struct {
	ID        uuid.UUID        `gorm:"type:uuid;primary_key" json:"id"`
	UserID    uuid.UUID        `gorm:"type:uuid;not null;index" json:"user_id"`
	Type      NotificationType `gorm:"type:varchar(30);not null;index" json:"type"`
	Title     string           `gorm:"size:200;not null" json:"title"`
	Body      string           `gorm:"type:text" json:"body"`
	Link      string           `gorm:"size:500" json:"link"`
	ActorID   *uuid.UUID       `gorm:"type:uuid" json:"actor_id"`
	ActorName string           `gorm:"size:100" json:"actor_name"`
	RoomID    *uuid.UUID       `gorm:"type:uuid;index" json:"room_id"`
	MessageID *uuid.UUID       `gorm:"type:uuid" json:"message_id"`
	IsRead    bool             `gorm:"default:false;index" json:"is_read"`
	Archived  bool             `gorm:"default:false;index" json:"archived"`
	Data      string           `gorm:"type:jsonb" json:"data"`
	CreatedAt time.Time        `json:"created_at"`
	ReadAt    *time.Time       `json:"read_at"`

	User User `gorm:"foreignKey:UserID" json:"-"`
	Room Room `gorm:"foreignKey:RoomID" json:"-"`
}

func (n *Notification) BeforeCreate(tx *gorm.DB) error {
	if n.ID == uuid.Nil {
		n.ID = uuid.New()
	}
	if n.CreatedAt.IsZero() {
		n.CreatedAt = time.Now()
	}
	return nil
}

type GroupDM struct {
	ID        uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	Name      string         `gorm:"size:100" json:"name"`
	CreatedBy uuid.UUID      `gorm:"type:uuid;not null" json:"created_by"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	Creator User            `gorm:"foreignKey:CreatedBy" json:"-"`
	Members []GroupDMMember `gorm:"foreignKey:GroupDMID" json:"members,omitempty"`
}

func (g *GroupDM) BeforeCreate(tx *gorm.DB) error {
	if g.ID == uuid.Nil {
		g.ID = uuid.New()
	}
	if g.CreatedAt.IsZero() {
		g.CreatedAt = time.Now()
	}
	return nil
}

type GroupDMMember struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	GroupDMID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_group_dm_member" json:"group_dm_id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_group_dm_member" json:"user_id"`
	Role      string    `gorm:"size:20;default:'member'" json:"role"`
	JoinedAt  time.Time `json:"joined_at"`

	GroupDM GroupDM `gorm:"foreignKey:GroupDMID" json:"-"`
	User    User    `gorm:"foreignKey:UserID" json:"-"`
}

func (gm *GroupDMMember) BeforeCreate(tx *gorm.DB) error {
	if gm.ID == uuid.Nil {
		gm.ID = uuid.New()
	}
	if gm.JoinedAt.IsZero() {
		gm.JoinedAt = time.Now()
	}
	return nil
}

type GroupDMMessage struct {
	ID            uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	GroupDMID     uuid.UUID      `gorm:"type:uuid;not null;index" json:"group_dm_id"`
	SenderID      uuid.UUID      `gorm:"type:uuid;not null" json:"sender_id"`
	Content       string         `gorm:"type:text;not null" json:"content"`
	IsEdited      bool           `gorm:"default:false" json:"is_edited"`
	IsDeleted     bool           `gorm:"default:false" json:"is_deleted"`
	ForwardedFrom *uuid.UUID     `gorm:"type:uuid" json:"forwarded_from,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`

	GroupDM GroupDM `gorm:"foreignKey:GroupDMID" json:"-"`
	Sender  User    `gorm:"foreignKey:SenderID" json:"-"`
}

func (gm *GroupDMMessage) BeforeCreate(tx *gorm.DB) error {
	if gm.ID == uuid.Nil {
		gm.ID = uuid.New()
	}
	if gm.CreatedAt.IsZero() {
		gm.CreatedAt = time.Now()
	}
	return nil
}
