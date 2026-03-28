package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AnnouncementPriority string

const (
	AnnouncementPriorityNormal AnnouncementPriority = "normal"
	AnnouncementPriorityHigh   AnnouncementPriority = "high"
	AnnouncementPriorityUrgent AnnouncementPriority = "urgent"
)

type Announcement struct {
	ID        uuid.UUID            `gorm:"type:uuid;primary_key" json:"id"`
	Title     string               `gorm:"size:200;not null" json:"title"`
	Content   string               `gorm:"type:text;not null" json:"content"`
	Priority  AnnouncementPriority `gorm:"type:varchar(20);default:'normal'" json:"priority"`
	CreatedBy uuid.UUID            `gorm:"type:uuid;not null;index" json:"created_by"`
	IsActive  bool                 `gorm:"default:true;index" json:"is_active"`
	ExpiresAt *time.Time           `json:"expires_at"`
	CreatedAt time.Time            `json:"created_at"`
	UpdatedAt time.Time            `json:"updated_at"`
	DeletedAt gorm.DeletedAt       `gorm:"index" json:"-"`

	User User `gorm:"foreignKey:CreatedBy" json:"-"`
}

func (a *Announcement) BeforeCreate(tx *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now()
	}
	if a.UpdatedAt.IsZero() {
		a.UpdatedAt = time.Now()
	}
	return nil
}
