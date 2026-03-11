package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Attachment struct {
	ID           uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	MessageID    uuid.UUID      `gorm:"type:uuid;not null;index" json:"message_id"`
	FileID       uuid.UUID      `gorm:"type:uuid;not null" json:"file_id"`
	UserID       uuid.UUID      `gorm:"type:uuid;not null" json:"user_id"`
	FileName     string         `gorm:"size:255;not null" json:"file_name"`
	FileSize     int64          `gorm:"not null" json:"file_size"`
	ContentType  string         `gorm:"size:100;not null" json:"content_type"`
	ThumbnailURL string         `gorm:"size:500" json:"thumbnail_url"`
	Width        int            `json:"width"`
	Height       int            `json:"height"`
	Duration     int            `json:"duration"`
	AltText      string         `gorm:"size:500" json:"alt_text"`
	IsVoiceMemo  bool           `gorm:"default:false" json:"is_voice_memo"`
	WaveformData string         `gorm:"type:text" json:"waveform_data"`
	Metadata     string         `gorm:"type:jsonb" json:"metadata"`
	CreatedAt    time.Time      `json:"created_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`

	Message Message `gorm:"foreignKey:MessageID" json:"-"`
	File    File    `gorm:"foreignKey:FileID" json:"-"`
	User    User    `gorm:"foreignKey:UserID" json:"-"`
}

func (a *Attachment) BeforeCreate(tx *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now()
	}
	return nil
}

type CategoryPermission struct {
	ID         uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	CategoryID uuid.UUID      `gorm:"type:uuid;not null;index" json:"category_id"`
	UserID     *uuid.UUID     `gorm:"type:uuid" json:"user_id"`
	RoleID     *uuid.UUID     `gorm:"type:uuid" json:"role_id"`
	Permission string         `gorm:"size:20;not null" json:"permission"`
	CanView    bool           `gorm:"default:true" json:"can_view"`
	CanCreate  bool           `gorm:"default:false" json:"can_create"`
	CanEdit    bool           `gorm:"default:false" json:"can_edit"`
	CanDelete  bool           `gorm:"default:false" json:"can_delete"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`

	Category Category `gorm:"foreignKey:CategoryID" json:"-"`
	User     User     `gorm:"foreignKey:UserID" json:"-"`
}

func (cp *CategoryPermission) BeforeCreate(tx *gorm.DB) error {
	if cp.ID == uuid.Nil {
		cp.ID = uuid.New()
	}
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = time.Now()
	}
	return nil
}

type UserDevice struct {
	ID           uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	UserID       uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	DeviceID     string         `gorm:"size:100;not null" json:"device_id"`
	DeviceName   string         `gorm:"size:100" json:"device_name"`
	DeviceType   string         `gorm:"size:20;not null" json:"device_type"`
	OSVersion    string         `gorm:"size:20" json:"os_version"`
	AppVersion   string         `gorm:"size:20" json:"app_version"`
	PushToken    string         `gorm:"size:500" json:"push_token"`
	FCMToken     string         `gorm:"size:500" json:"fcm_token"`
	APNSToken    string         `gorm:"size:500" json:"apns_token"`
	LastActiveAt time.Time      `json:"last_active_at"`
	IsActive     bool           `gorm:"default:true" json:"is_active"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`

	User User `gorm:"foreignKey:UserID" json:"-"`
}

func (ud *UserDevice) BeforeCreate(tx *gorm.DB) error {
	if ud.ID == uuid.Nil {
		ud.ID = uuid.New()
	}
	if ud.CreatedAt.IsZero() {
		ud.CreatedAt = time.Now()
	}
	if ud.LastActiveAt.IsZero() {
		ud.LastActiveAt = time.Now()
	}
	return nil
}

type AuditLogExclusion struct {
	ID        uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	UserID    uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	Action    string         `gorm:"size:50;not null" json:"action"`
	Reason    string         `gorm:"size:255" json:"reason"`
	CreatedBy uuid.UUID      `gorm:"type:uuid;not null" json:"created_by"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	User User `gorm:"foreignKey:UserID" json:"-"`
}

func (ale *AuditLogExclusion) BeforeCreate(tx *gorm.DB) error {
	if ale.ID == uuid.Nil {
		ale.ID = uuid.New()
	}
	if ale.CreatedAt.IsZero() {
		ale.CreatedAt = time.Now()
	}
	return nil
}
