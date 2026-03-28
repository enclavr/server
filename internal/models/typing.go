package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TypingIndicator tracks active typing states in rooms and DMs.
// Records auto-expire after a configurable duration.
type TypingIndicator struct {
	ID        uuid.UUID  `gorm:"type:uuid;primary_key" json:"id"`
	UserID    uuid.UUID  `gorm:"type:uuid;not null;index:idx_typing_room" json:"user_id"`
	RoomID    *uuid.UUID `gorm:"type:uuid;index:idx_typing_room" json:"room_id,omitempty"`
	DMUserID  *uuid.UUID `gorm:"type:uuid;index:idx_typing_dm" json:"dm_user_id,omitempty"`
	StartedAt time.Time  `json:"started_at"`
	ExpiresAt time.Time  `gorm:"index" json:"expires_at"`

	User   User `gorm:"foreignKey:UserID" json:"-"`
	Room   Room `gorm:"foreignKey:RoomID" json:"-"`
	DMUser User `gorm:"foreignKey:DMUserID" json:"-"`
}

// BeforeCreate sets the ID and timestamps before creating a typing indicator.
func (ti *TypingIndicator) BeforeCreate(tx *gorm.DB) error {
	if ti.ID == uuid.Nil {
		ti.ID = uuid.New()
	}
	if ti.StartedAt.IsZero() {
		ti.StartedAt = time.Now()
	}
	if ti.ExpiresAt.IsZero() {
		ti.ExpiresAt = time.Now().Add(10 * time.Second)
	}
	return nil
}

// IsExpired returns true if the typing indicator has expired.
func (ti *TypingIndicator) IsExpired() bool {
	return time.Now().After(ti.ExpiresAt)
}
