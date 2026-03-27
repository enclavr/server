package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type DMReaction struct {
	ID              uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	DirectMessageID uuid.UUID `gorm:"type:uuid;not null;index:idx_dm_reaction" json:"direct_message_id"`
	UserID          uuid.UUID `gorm:"type:uuid;not null;index:idx_dm_reaction" json:"user_id"`
	Emoji           string    `gorm:"size:100;not null" json:"emoji"`
	CreatedAt       time.Time `json:"created_at"`

	DirectMessage DirectMessage `gorm:"foreignKey:DirectMessageID" json:"-"`
	User          User          `gorm:"foreignKey:UserID" json:"-"`
}

func (dr *DMReaction) BeforeCreate(tx *gorm.DB) error {
	if dr.ID == uuid.Nil {
		dr.ID = uuid.New()
	}
	if dr.CreatedAt.IsZero() {
		dr.CreatedAt = time.Now()
	}
	return nil
}

type DMReadReceipt struct {
	ID              uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	DirectMessageID uuid.UUID `gorm:"type:uuid;not null;index:idx_dm_read_receipt" json:"direct_message_id"`
	UserID          uuid.UUID `gorm:"type:uuid;not null;index:idx_dm_read_receipt" json:"user_id"`
	ReadAt          time.Time `json:"read_at"`

	DirectMessage DirectMessage `gorm:"foreignKey:DirectMessageID" json:"-"`
	User          User          `gorm:"foreignKey:UserID" json:"-"`
}

func (drr *DMReadReceipt) BeforeCreate(tx *gorm.DB) error {
	if drr.ID == uuid.Nil {
		drr.ID = uuid.New()
	}
	if drr.ReadAt.IsZero() {
		drr.ReadAt = time.Now()
	}
	return nil
}
