package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PushSubscription struct {
	ID        uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	UserID    uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	Endpoint  string         `gorm:"size:500;not null" json:"endpoint"`
	P256DH    string         `gorm:"size:100;not null" json:"p256dh"`
	Auth      string         `gorm:"size:50;not null" json:"auth"`
	DeviceID  string         `gorm:"size:100" json:"device_id"`
	DeviceOS  string         `gorm:"size:20" json:"device_os"`
	IsActive  bool           `gorm:"default:true" json:"is_active"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	User User `gorm:"foreignKey:UserID" json:"-"`
}

func (ps *PushSubscription) BeforeCreate(tx *gorm.DB) error {
	if ps.ID == uuid.Nil {
		ps.ID = uuid.New()
	}
	if ps.CreatedAt.IsZero() {
		ps.CreatedAt = time.Now()
	}
	return nil
}

type UserNotificationSettings struct {
	ID                         uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	UserID                     uuid.UUID `gorm:"type:uuid;not null;uniqueIndex" json:"user_id"`
	EnablePush                 bool      `gorm:"default:true" json:"enable_push"`
	EnableDMNotifications      bool      `gorm:"default:true" json:"enable_dm_notifications"`
	EnableMentionNotifications bool      `gorm:"default:true" json:"enable_mention_notifications"`
	EnableRoomNotifications    bool      `gorm:"default:true" json:"enable_room_notifications"`
	EnableSound                bool      `gorm:"default:true" json:"enable_sound"`
	NotifyOnMobile             bool      `gorm:"default:true" json:"notify_on_mobile"`
	QuietHoursEnabled          bool      `gorm:"default:false" json:"quiet_hours_enabled"`
	QuietHoursStart            string    `gorm:"size:5" json:"quiet_hours_start"`
	QuietHoursEnd              string    `gorm:"size:5" json:"quiet_hours_end"`
	CreatedAt                  time.Time `json:"created_at"`
	UpdatedAt                  time.Time `json:"updated_at"`

	User User `gorm:"foreignKey:UserID" json:"-"`
}

func (uns *UserNotificationSettings) BeforeCreate(tx *gorm.DB) error {
	if uns.ID == uuid.Nil {
		uns.ID = uuid.New()
	}
	if uns.CreatedAt.IsZero() {
		uns.CreatedAt = time.Now()
	}
	return nil
}
