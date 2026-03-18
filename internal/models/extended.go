package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Attachment struct {
	ID            uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	MessageID     uuid.UUID      `gorm:"type:uuid;not null;index" json:"message_id"`
	FileID        uuid.UUID      `gorm:"type:uuid;not null" json:"file_id"`
	UserID        uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	FileName      string         `gorm:"size:255;not null" json:"file_name"`
	FileSize      int64          `gorm:"not null" json:"file_size"`
	ContentType   string         `gorm:"size:100;not null" json:"content_type"`
	ThumbnailURL  string         `gorm:"size:500" json:"thumbnail_url"`
	Width         int            `json:"width"`
	Height        int            `json:"height"`
	Duration      int            `json:"duration"`
	AltText       string         `gorm:"size:500" json:"alt_text"`
	IsVoiceMemo   bool           `gorm:"default:false" json:"is_voice_memo"`
	WaveformData  string         `gorm:"type:text" json:"waveform_data"`
	Metadata      string         `gorm:"type:jsonb" json:"metadata"`
	IsShared      bool           `gorm:"default:false" json:"is_shared"`
	ShareCount    int            `gorm:"default:0" json:"share_count"`
	DownloadCount int            `gorm:"default:0" json:"download_count"`
	ViewCount     int            `gorm:"default:0" json:"view_count"`
	CreatedAt     time.Time      `json:"created_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`

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

type MessageAttachmentMetadata struct {
	ID               uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	AttachmentID     uuid.UUID `gorm:"type:uuid;not null;index" json:"attachment_id"`
	BlurHash         string    `gorm:"size:100" json:"blur_hash"`
	OriginalFilename string    `gorm:"size:500" json:"original_filename"`
	FileExtension    string    `gorm:"size:20" json:"file_extension"`
	Encoding         string    `gorm:"size:50" json:"encoding"`
	BitRate          *int      `json:"bit_rate"`
	SampleRate       *int      `json:"sample_rate"`
	Channels         *int      `json:"channels"`
	DurationMs       *int      `json:"duration_ms"`
	Width            *int      `json:"width"`
	Height           *int      `json:"height"`
	AspectRatio      string    `gorm:"size:20" json:"aspect_ratio"`
	ColorModel       string    `gorm:"size:50" json:"color_model"`
	PaletteColors    string    `gorm:"type:jsonb" json:"palette_colors"`
	Metadata         string    `gorm:"type:jsonb" json:"metadata"`
	CreatedAt        time.Time `json:"created_at"`

	Attachment Attachment `gorm:"foreignKey:AttachmentID" json:"-"`
}

func (mam *MessageAttachmentMetadata) BeforeCreate(tx *gorm.DB) error {
	if mam.ID == uuid.Nil {
		mam.ID = uuid.New()
	}
	if mam.CreatedAt.IsZero() {
		mam.CreatedAt = time.Now()
	}
	return nil
}

type AttachmentShare struct {
	ID            uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	AttachmentID  uuid.UUID      `gorm:"type:uuid;not null;index" json:"attachment_id"`
	SharedBy      uuid.UUID      `gorm:"type:uuid;not null" json:"shared_by"`
	ShareURL      string         `gorm:"size:500;uniqueIndex" json:"share_url"`
	Password      string         `gorm:"size:255" json:"-"`
	ExpiresAt     *time.Time     `json:"expires_at"`
	MaxDownloads  int            `gorm:"default:0" json:"max_downloads"`
	DownloadCount int            `gorm:"default:0" json:"download_count"`
	ViewCount     int            `gorm:"default:0" json:"view_count"`
	IsActive      bool           `gorm:"default:true" json:"is_active"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`

	Attachment Attachment `gorm:"foreignKey:AttachmentID" json:"-"`
	User       User       `gorm:"foreignKey:SharedBy" json:"-"`
}

func (as *AttachmentShare) BeforeCreate(tx *gorm.DB) error {
	if as.ID == uuid.Nil {
		as.ID = uuid.New()
	}
	if as.CreatedAt.IsZero() {
		as.CreatedAt = time.Now()
	}
	return nil
}
