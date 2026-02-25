package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type File struct {
	ID          uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	UserID      uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	RoomID      uuid.UUID      `gorm:"type:uuid;not null;index" json:"room_id"`
	MessageID   *uuid.UUID     `gorm:"type:uuid;index" json:"message_id"`
	FileName    string         `gorm:"size:255;not null" json:"file_name"`
	FileSize    int64          `gorm:"not null" json:"file_size"`
	ContentType string         `gorm:"size:100;not null" json:"content_type"`
	StorageKey  string         `gorm:"size:500;not null" json:"storage_key"`
	URL         string         `gorm:"size:500" json:"url"`
	IsDeleted   bool           `gorm:"default:false" json:"is_deleted"`
	CreatedAt   time.Time      `json:"created_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`

	User User `gorm:"foreignKey:UserID" json:"-"`
	Room Room `gorm:"foreignKey:RoomID" json:"-"`
}

func (f *File) BeforeCreate(tx *gorm.DB) error {
	if f.ID == uuid.Nil {
		f.ID = uuid.New()
	}
	if f.CreatedAt.IsZero() {
		f.CreatedAt = time.Now()
	}
	return nil
}
