package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type VoiceSession struct {
	ID        uuid.UUID  `gorm:"type:uuid;primary_key" json:"id"`
	RoomID    uuid.UUID  `gorm:"type:uuid;not null;index" json:"room_id"`
	UserID    uuid.UUID  `gorm:"type:uuid;not null" json:"user_id"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`

	Room Room `gorm:"foreignKey:RoomID" json:"-"`
	User User `gorm:"foreignKey:UserID" json:"-"`
}

func (vs *VoiceSession) BeforeCreate(tx *gorm.DB) error {
	if vs.ID == uuid.Nil {
		vs.ID = uuid.New()
	}
	if vs.StartedAt.IsZero() {
		vs.StartedAt = time.Now()
	}
	return nil
}

type RoomInvite struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	RoomID    uuid.UUID `gorm:"type:uuid;not null" json:"room_id"`
	Code      string    `gorm:"uniqueIndex;not null;size:32" json:"code"`
	CreatedBy uuid.UUID `gorm:"type:uuid;not null" json:"created_by"`
	ExpiresAt time.Time `json:"expires_at"`
	MaxUses   *int      `json:"max_uses,omitempty"`
	UsedCount int       `gorm:"default:0" json:"used_count"`
	CreatedAt time.Time `json:"created_at"`

	Room Room `gorm:"foreignKey:RoomID" json:"-"`
}

func (ri *RoomInvite) BeforeCreate(tx *gorm.DB) error {
	if ri.ID == uuid.Nil {
		ri.ID = uuid.New()
	}
	return nil
}

type PresenceStatus string

const (
	PresenceOnline  PresenceStatus = "online"
	PresenceAway    PresenceStatus = "away"
	PresenceBusy    PresenceStatus = "busy"
	PresenceOffline PresenceStatus = "offline"
)

type Presence struct {
	ID       uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	UserID   uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex" json:"user_id"`
	Status   PresenceStatus `gorm:"type:varchar(20);default:'offline'" json:"status"`
	RoomID   *uuid.UUID     `gorm:"type:uuid" json:"room_id,omitempty"`
	LastSeen time.Time      `json:"last_seen"`

	User User `gorm:"foreignKey:UserID" json:"-"`
	Room Room `gorm:"foreignKey:RoomID" json:"-"`
}

func (p *Presence) BeforeCreate(tx *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	if p.LastSeen.IsZero() {
		p.LastSeen = time.Now()
	}
	return nil
}
