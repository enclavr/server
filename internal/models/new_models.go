package models

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type APIKey struct {
	ID          uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	Name        string         `gorm:"size:100;not null" json:"name"`
	KeyHash     string         `gorm:"size:64;not null;uniqueIndex" json:"-"`
	KeyPrefix   string         `gorm:"size:8;not null" json:"key_prefix"`
	UserID      uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	Permissions string         `gorm:"type:text" json:"permissions"`
	ExpiresAt   *time.Time     `json:"expires_at"`
	LastUsedAt  *time.Time     `json:"last_used_at"`
	IPWhitelist string         `gorm:"size:500" json:"ip_whitelist"`
	IsActive    bool           `gorm:"default:true" json:"is_active"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`

	User User `gorm:"foreignKey:UserID" json:"-"`
}

func (a *APIKey) BeforeCreate(tx *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now()
	}
	key, err := generateSecureKey(32)
	if err != nil {
		return err
	}
	a.KeyHash = hashKey(key)
	a.KeyPrefix = key[:8]
	return nil
}

func generateSecureKey(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func hashKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

type Role struct {
	ID          uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	Name        string         `gorm:"size:50;not null;uniqueIndex" json:"name"`
	DisplayName string         `gorm:"size:100;not null" json:"display_name"`
	Description string         `gorm:"size:255" json:"description"`
	Permissions string         `gorm:"type:text;not null" json:"permissions"`
	IsDefault   bool           `gorm:"default:false" json:"is_default"`
	IsAdmin     bool           `gorm:"default:false" json:"is_admin"`
	RoomID      *uuid.UUID     `gorm:"type:uuid;index" json:"room_id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`

	Room Room `gorm:"foreignKey:RoomID" json:"-"`
}

func (r *Role) BeforeCreate(tx *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now()
	}
	return nil
}

type RolePermission struct {
	ID         uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	RoleID     uuid.UUID      `gorm:"type:uuid;not null;index" json:"role_id"`
	Permission string         `gorm:"size:50;not null" json:"permission"`
	Resource   string         `gorm:"size:50;not null" json:"resource"`
	Action     string         `gorm:"size:20;not null" json:"action"`
	CreatedAt  time.Time      `json:"created_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`

	Role Role `gorm:"foreignKey:RoleID" json:"-"`
}

func (rp *RolePermission) BeforeCreate(tx *gorm.DB) error {
	if rp.ID == uuid.Nil {
		rp.ID = uuid.New()
	}
	if rp.CreatedAt.IsZero() {
		rp.CreatedAt = time.Now()
	}
	return nil
}

type UserRole struct {
	ID        uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	UserID    uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	RoleID    uuid.UUID      `gorm:"type:uuid;not null;index" json:"role_id"`
	RoomID    *uuid.UUID     `gorm:"type:uuid;index" json:"room_id"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	User User `gorm:"foreignKey:UserID" json:"-"`
	Role Role `gorm:"foreignKey:RoleID" json:"-"`
	Room Room `gorm:"foreignKey:RoomID" json:"-"`
}

func (ur *UserRole) BeforeCreate(tx *gorm.DB) error {
	if ur.ID == uuid.Nil {
		ur.ID = uuid.New()
	}
	if ur.CreatedAt.IsZero() {
		ur.CreatedAt = time.Now()
	}
	return nil
}

type UserNotification struct {
	ID        uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	UserID    uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	Type      string         `gorm:"size:50;not null" json:"type"`
	Title     string         `gorm:"size:200;not null" json:"title"`
	Body      string         `gorm:"type:text" json:"body"`
	Data      string         `gorm:"type:jsonb" json:"data"`
	IsRead    bool           `gorm:"default:false;index" json:"is_read"`
	ReadAt    *time.Time     `json:"read_at"`
	ExpiresAt *time.Time     `json:"expires_at"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	User User `gorm:"foreignKey:UserID" json:"-"`
}

func (un *UserNotification) BeforeCreate(tx *gorm.DB) error {
	if un.ID == uuid.Nil {
		un.ID = uuid.New()
	}
	if un.CreatedAt.IsZero() {
		un.CreatedAt = time.Now()
	}
	return nil
}

type OAuthProvider string

const (
	OAuthProviderGoogle    OAuthProvider = "google"
	OAuthProviderGitHub    OAuthProvider = "github"
	OAuthProviderDiscord   OAuthProvider = "discord"
	OAuthProviderTwitter   OAuthProvider = "twitter"
	OAuthProviderSlack     OAuthProvider = "slack"
	OAuthProviderMicrosoft OAuthProvider = "microsoft"
)

type OAuthAccount struct {
	ID           uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	UserID       uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	Provider     OAuthProvider  `gorm:"type:varchar(20);not null" json:"provider"`
	ProviderID   string         `gorm:"size:255;not null" json:"provider_id"`
	AccessToken  string         `gorm:"type:text" json:"-"`
	RefreshToken string         `gorm:"type:text" json:"-"`
	ExpiresAt    *time.Time     `json:"expires_at"`
	Scope        string         `gorm:"size:500" json:"scope"`
	AvatarURL    string         `gorm:"size:500" json:"avatar_url"`
	ProfileData  string         `gorm:"type:jsonb" json:"profile_data"`
	IsActive     bool           `gorm:"default:true" json:"is_active"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`

	User User `gorm:"foreignKey:UserID" json:"-"`
}

func (oa *OAuthAccount) BeforeCreate(tx *gorm.DB) error {
	if oa.ID == uuid.Nil {
		oa.ID = uuid.New()
	}
	if oa.CreatedAt.IsZero() {
		oa.CreatedAt = time.Now()
	}
	return nil
}

type RoomMute struct {
	ID          uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	UserID      uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	RoomID      uuid.UUID      `gorm:"type:uuid;not null;index" json:"room_id"`
	MutedBy     uuid.UUID      `gorm:"type:uuid;not null" json:"muted_by"`
	Reason      string         `gorm:"size:500" json:"reason"`
	ExpiresAt   *time.Time     `json:"expires_at"`
	IsPermanent bool           `gorm:"default:false" json:"is_permanent"`
	CreatedAt   time.Time      `json:"created_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`

	User        User `gorm:"foreignKey:UserID" json:"-"`
	Room        Room `gorm:"foreignKey:RoomID" json:"-"`
	MutedByUser User `gorm:"foreignKey:MutedBy" json:"-"`
}

func (rm *RoomMute) BeforeCreate(tx *gorm.DB) error {
	if rm.ID == uuid.Nil {
		rm.ID = uuid.New()
	}
	if rm.CreatedAt.IsZero() {
		rm.CreatedAt = time.Now()
	}
	return nil
}

type StickerPack struct {
	ID          uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	Name        string         `gorm:"size:100;not null" json:"name"`
	Description string         `gorm:"size:500" json:"description"`
	CoverURL    string         `gorm:"size:500" json:"cover_url"`
	IsPremium   bool           `gorm:"default:false" json:"is_premium"`
	Price       int            `gorm:"default:0" json:"price"`
	CreatedBy   *uuid.UUID     `gorm:"type:uuid" json:"created_by"`
	IsGlobal    bool           `gorm:"default:false" json:"is_global"`
	UseCount    int            `gorm:"default:0" json:"use_count"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`

	User User `gorm:"foreignKey:CreatedBy" json:"-"`
}

func (sp *StickerPack) BeforeCreate(tx *gorm.DB) error {
	if sp.ID == uuid.Nil {
		sp.ID = uuid.New()
	}
	return nil
}

type RoomRating struct {
	ID        uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	RoomID    uuid.UUID      `gorm:"type:uuid;not null" json:"room_id"`
	UserID    uuid.UUID      `gorm:"type:uuid;not null" json:"user_id"`
	Rating    int            `gorm:"not null" json:"rating"`
	Comment   string         `gorm:"type:text" json:"comment"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	Room Room `gorm:"foreignKey:RoomID" json:"-"`
	User User `gorm:"foreignKey:UserID" json:"-"`
}

func (rr *RoomRating) BeforeCreate(tx *gorm.DB) error {
	if rr.ID == uuid.Nil {
		rr.ID = uuid.New()
	}
	return nil
}

type UserActivityLog struct {
	ID           uuid.UUID  `gorm:"type:uuid;primary_key" json:"id"`
	UserID       uuid.UUID  `gorm:"type:uuid;not null" json:"user_id"`
	ActivityType string     `gorm:"size:50;not null" json:"activity_type"`
	RoomID       *uuid.UUID `gorm:"type:uuid" json:"room_id"`
	TargetType   string     `gorm:"size:50" json:"target_type"`
	TargetID     *uuid.UUID `gorm:"type:uuid" json:"target_id"`
	Metadata     string     `gorm:"type:jsonb" json:"metadata"`
	IPAddress    string     `gorm:"size:45" json:"ip_address"`
	UserAgent    string     `gorm:"size:500" json:"user_agent"`
	SessionID    *uuid.UUID `gorm:"type:uuid" json:"session_id"`
	CreatedAt    time.Time  `json:"created_at"`

	User User `gorm:"foreignKey:UserID" json:"-"`
	Room Room `gorm:"foreignKey:RoomID" json:"-"`
}

func (ual *UserActivityLog) BeforeCreate(tx *gorm.DB) error {
	if ual.ID == uuid.Nil {
		ual.ID = uuid.New()
	}
	if ual.CreatedAt.IsZero() {
		ual.CreatedAt = time.Now()
	}
	return nil
}

type RoomMetric struct {
	ID                uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	RoomID            uuid.UUID `gorm:"type:uuid;not null" json:"room_id"`
	Date              time.Time `gorm:"type:date;not null" json:"date"`
	MessageCount      int       `gorm:"default:0" json:"message_count"`
	UniqueUsers       int       `gorm:"default:0" json:"unique_users"`
	VoiceMinutes      int       `gorm:"default:0" json:"voice_minutes"`
	FileUploads       int       `gorm:"default:0" json:"file_uploads"`
	AvgResponseTimeMs int       `json:"avg_response_time_ms"`
	PeakUsers         int       `gorm:"default:0" json:"peak_users"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`

	Room Room `gorm:"foreignKey:RoomID" json:"-"`
}

func (rm *RoomMetric) BeforeCreate(tx *gorm.DB) error {
	if rm.ID == uuid.Nil {
		rm.ID = uuid.New()
	}
	return nil
}

type ConnectionStatus string

const (
	ConnectionStatusPending  ConnectionStatus = "pending"
	ConnectionStatusAccepted ConnectionStatus = "accepted"
	ConnectionStatusRejected ConnectionStatus = "rejected"
	ConnectionStatusBlocked  ConnectionStatus = "blocked"
)

type ConnectionDirection string

const (
	ConnectionDirectionOneway ConnectionDirection = "oneway"
	ConnectionDirectionMutual ConnectionDirection = "mutual"
)

type MessageDraft struct {
	ID        uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	UserID    uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	RoomID    *uuid.UUID     `gorm:"type:uuid;index" json:"room_id"`
	Content   string         `gorm:"type:text;not null" json:"content"`
	IsDraft   bool           `gorm:"default:true" json:"is_draft"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	User User `gorm:"foreignKey:UserID" json:"-"`
	Room Room `gorm:"foreignKey:RoomID" json:"-"`
}

func (md *MessageDraft) BeforeCreate(tx *gorm.DB) error {
	if md.ID == uuid.Nil {
		md.ID = uuid.New()
	}
	if md.CreatedAt.IsZero() {
		md.CreatedAt = time.Now()
	}
	return nil
}

type BlockedUser struct {
	ID        uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	BlockerID uuid.UUID      `gorm:"type:uuid;not null;index" json:"blocker_id"`
	BlockedID uuid.UUID      `gorm:"type:uuid;not null;index" json:"blocked_id"`
	Reason    string         `gorm:"size:500" json:"reason"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	BlockerUser User `gorm:"foreignKey:BlockerID" json:"-"`
	BlockedUser User `gorm:"foreignKey:BlockedID" json:"-"`
}

func (bu *BlockedUser) BeforeCreate(tx *gorm.DB) error {
	if bu.ID == uuid.Nil {
		bu.ID = uuid.New()
	}
	if bu.CreatedAt.IsZero() {
		bu.CreatedAt = time.Now()
	}
	return nil
}

type UserConnection struct {
	ID              uuid.UUID           `gorm:"type:uuid;primary_key" json:"id"`
	UserID          uuid.UUID           `gorm:"type:uuid;not null;index" json:"user_id"`
	ConnectedUserID uuid.UUID           `gorm:"type:uuid;not null;index" json:"connected_user_id"`
	Status          ConnectionStatus    `gorm:"size:20;not null;default:'pending'" json:"status"`
	Direction       ConnectionDirection `gorm:"size:20;not null;default:'oneway'" json:"direction"`
	CreatedAt       time.Time           `json:"created_at"`
	UpdatedAt       time.Time           `json:"updated_at"`
	DeletedAt       gorm.DeletedAt      `gorm:"index" json:"-"`

	User          User `gorm:"foreignKey:UserID" json:"-"`
	ConnectedUser User `gorm:"foreignKey:ConnectedUserID" json:"-"`
}

func (uc *UserConnection) BeforeCreate(tx *gorm.DB) error {
	if uc.ID == uuid.Nil {
		uc.ID = uuid.New()
	}
	if uc.CreatedAt.IsZero() {
		uc.CreatedAt = time.Now()
	}
	return nil
}

type RoomFeatured struct {
	ID         uuid.UUID  `gorm:"type:uuid;primary_key" json:"id"`
	RoomID     uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex" json:"room_id"`
	FeaturedBy uuid.UUID  `gorm:"type:uuid;not null" json:"featured_by"`
	Reason     string     `gorm:"size:500" json:"reason"`
	Position   int        `gorm:"default:0" json:"position"`
	StartsAt   *time.Time `json:"starts_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
	CreatedAt  time.Time  `json:"created_at"`

	Room Room `gorm:"foreignKey:RoomID" json:"-"`
	User User `gorm:"foreignKey:FeaturedBy" json:"-"`
}

func (rf *RoomFeatured) TableName() string {
	return "room_featured"
}

func (rf *RoomFeatured) BeforeCreate(tx *gorm.DB) error {
	if rf.ID == uuid.Nil {
		rf.ID = uuid.New()
	}
	if rf.CreatedAt.IsZero() {
		rf.CreatedAt = time.Now()
	}
	return nil
}

type SessionActivity struct {
	ID                uuid.UUID  `gorm:"type:uuid;primary_key" json:"id"`
	SessionID         uuid.UUID  `gorm:"type:uuid;not null;index" json:"session_id"`
	UserID            uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	RoomID            *uuid.UUID `gorm:"type:uuid;index" json:"room_id"`
	ActivityType      string     `gorm:"size:30;not null" json:"activity_type"`
	Duration          int        `gorm:"default:0" json:"duration"`
	Metadata          string     `gorm:"type:jsonb" json:"metadata"`
	IPAddress         string     `gorm:"size:45" json:"ip_address"`
	Country           string     `gorm:"size:2" json:"country"`
	City              string     `gorm:"size:100" json:"city"`
	DeviceType        string     `gorm:"size:20" json:"device_type"`
	Browser           string     `gorm:"size:50" json:"browser"`
	OS                string     `gorm:"size:30" json:"os"`
	NetworkType       string     `gorm:"size:20" json:"network_type"`
	PageViews         int        `gorm:"default:0" json:"page_views"`
	MessagesSent      int        `gorm:"default:0" json:"messages_sent"`
	CommandsRun       int        `gorm:"default:0" json:"commands_run"`
	ErrorsEncountered int        `gorm:"default:0" json:"errors_encountered"`
	StartedAt         time.Time  `json:"started_at"`
	EndedAt           *time.Time `json:"ended_at"`
	CreatedAt         time.Time  `json:"created_at"`

	User    User    `gorm:"foreignKey:UserID" json:"-"`
	Room    Room    `gorm:"foreignKey:RoomID" json:"-"`
	Session Session `gorm:"foreignKey:SessionID" json:"-"`
}

func (sa *SessionActivity) BeforeCreate(tx *gorm.DB) error {
	if sa.ID == uuid.Nil {
		sa.ID = uuid.New()
	}
	if sa.CreatedAt.IsZero() {
		sa.CreatedAt = time.Now()
	}
	if sa.StartedAt.IsZero() {
		sa.StartedAt = time.Now()
	}
	return nil
}
