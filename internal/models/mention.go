package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type MentionType string

const (
	MentionTypeUser MentionType = "user"
	MentionTypeAll  MentionType = "all"
	MentionTypeHere MentionType = "here"
)

type MessageMention struct {
	ID          uuid.UUID   `gorm:"type:uuid;primary_key" json:"id"`
	MessageID   uuid.UUID   `gorm:"type:uuid;not null;index" json:"message_id"`
	RoomID      uuid.UUID   `gorm:"type:uuid;not null;index" json:"room_id"`
	UserID      uuid.UUID   `gorm:"type:uuid;not null;index" json:"user_id"`
	MentionedBy uuid.UUID   `gorm:"type:uuid;not null" json:"mentioned_by"`
	Type        MentionType `gorm:"type:varchar(20);default:'user'" json:"type"`
	CreatedAt   time.Time   `json:"created_at"`

	Message   Message `gorm:"foreignKey:MessageID" json:"-"`
	Room      Room    `gorm:"foreignKey:RoomID" json:"-"`
	User      User    `gorm:"foreignKey:UserID" json:"-"`
	Mentioner User    `gorm:"foreignKey:MentionedBy" json:"-"`
}

func (mm *MessageMention) BeforeCreate(tx *gorm.DB) error {
	if mm.ID == uuid.Nil {
		mm.ID = uuid.New()
	}
	if mm.CreatedAt.IsZero() {
		mm.CreatedAt = time.Now()
	}
	return nil
}
