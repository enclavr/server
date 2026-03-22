package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type User struct {
	ID                uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	Username          string         `gorm:"uniqueIndex;not null;size:50" json:"username"`
	Email             string         `gorm:"uniqueIndex;not null" json:"email,omitempty"`
	PasswordHash      string         `gorm:"not null" json:"-"`
	DisplayName       string         `gorm:"size:100" json:"display_name"`
	AvatarURL         string         `gorm:"size:500" json:"avatar_url"`
	IsAdmin           bool           `gorm:"default:false" json:"is_admin"`
	OIDCSubject       string         `gorm:"size:255" json:"-"`
	OIDCIssuer        string         `gorm:"size:255" json:"-"`
	TwoFactorEnabled  bool           `gorm:"default:false" json:"two_factor_enabled"`
	TwoFactorSecret   string         `gorm:"size:255" json:"-"`
	FailedLoginCount  int            `gorm:"default:0" json:"-"`
	LockedUntil       *time.Time     `gorm:"index" json:"locked_until"`
	PasswordChangedAt *time.Time     `gorm:"index" json:"password_changed_at"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`

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
	ID          uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	UserID      uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	Token       string    `gorm:"uniqueIndex;not null" json:"-"`
	TokenFamily string    `gorm:"size:255;index" json:"-"`
	ExpiresAt   time.Time `json:"expires_at"`
	CreatedAt   time.Time `json:"created_at"`

	User User `gorm:"foreignKey:UserID" json:"-"`
}

func (rt *RefreshToken) BeforeCreate(tx *gorm.DB) error {
	if rt.ID == uuid.Nil {
		rt.ID = uuid.New()
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

type PasswordHistory struct {
	ID           uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	UserID       uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	PasswordHash string    `gorm:"not null" json:"-"`
	CreatedAt    time.Time `json:"created_at"`
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

type UserPreferences struct {
	ID                  uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	UserID              uuid.UUID `gorm:"type:uuid;not null;uniqueIndex" json:"user_id"`
	Theme               string    `gorm:"size:20;default:'dark'" json:"theme"`
	Language            string    `gorm:"size:10;default:'en'" json:"language"`
	Timezone            string    `gorm:"size:50;default:'UTC'" json:"timezone"`
	MessagePreview      bool      `gorm:"default:true" json:"message_preview"`
	CompactMode         bool      `gorm:"default:false" json:"compact_mode"`
	ShowOnlineStatus    bool      `gorm:"default:true" json:"show_online_status"`
	AnimatedEmoji       bool      `gorm:"default:true" json:"animated_emoji"`
	AutoPlayGifs        bool      `gorm:"default:true" json:"auto_play_gifs"`
	ReducedMotion       bool      `gorm:"default:false" json:"reduced_motion"`
	HighContrastMode    bool      `gorm:"default:false" json:"high_contrast_mode"`
	TextSize            string    `gorm:"size:10;default:'medium'" json:"text_size"`
	NotificationSound   string    `gorm:"size:50;default:'default'" json:"notification_sound"`
	DesktopNotification bool      `gorm:"default:true" json:"desktop_notification"`
	MobileNotification  bool      `gorm:"default:true" json:"mobile_notification"`
	MentionNotification bool      `gorm:"default:true" json:"mention_notification"`
	DmNotification      bool      `gorm:"default:true" json:"dm_notification"`
	ShowTypingIndicator bool      `gorm:"default:true" json:"show_typing_indicator"`
	ShowReadReceipts    bool      `gorm:"default:true" json:"show_read_receipts"`
	AutoScrollMessages  bool      `gorm:"default:true" json:"auto_scroll_messages"`
	Use24HourFormat     bool      `gorm:"default:false" json:"use_24_hour_format"`
	DisplayMode         string    `gorm:"size:20;default:'card'" json:"display_mode"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`

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

type UserPrivacySettings struct {
	ID                    uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	UserID                uuid.UUID `gorm:"type:uuid;not null;uniqueIndex" json:"user_id"`
	AllowDirectMessages   string    `gorm:"size:20;default:'everyone'" json:"allow_direct_messages"`
	AllowRoomInvites      string    `gorm:"size:20;default:'everyone'" json:"allow_room_invites"`
	AllowVoiceCalls       string    `gorm:"size:20;default:'everyone'" json:"allow_voice_calls"`
	ShowOnlineStatus      bool      `json:"show_online_status"`
	ShowReadReceipts      bool      `json:"show_read_receipts"`
	ShowTypingIndicator   bool      `json:"show_typing_indicator"`
	AllowSearchByEmail    bool      `json:"allow_search_by_email"`
	AllowSearchByUsername bool      `json:"allow_search_by_username"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`

	User User `gorm:"foreignKey:UserID" json:"-"`
}

func (ups *UserPrivacySettings) BeforeCreate(tx *gorm.DB) error {
	if ups.ID == uuid.Nil {
		ups.ID = uuid.New()
	}
	if ups.CreatedAt.IsZero() {
		ups.CreatedAt = time.Now()
	}
	if ups.UpdatedAt.IsZero() {
		ups.UpdatedAt = time.Now()
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

func (us *UserStatusModel) TableName() string {
	return "user_statuses"
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
