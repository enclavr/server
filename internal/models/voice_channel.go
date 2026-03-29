package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// VoiceChannel represents a persistent voice channel within a room.
// Users can join/leave voice channels for real-time audio communication.
type VoiceChannel struct {
	ID              uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	RoomID          uuid.UUID      `gorm:"type:uuid;not null;index" json:"room_id"`
	Name            string         `gorm:"size:100;not null" json:"name"`
	Description     string         `gorm:"size:500" json:"description"`
	MaxParticipants int            `gorm:"default:10" json:"max_participants"`
	IsPrivate       bool           `gorm:"default:false" json:"is_private"`
	CreatedBy       uuid.UUID      `gorm:"type:uuid;not null" json:"created_by"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`

	Room         Room                      `gorm:"foreignKey:RoomID" json:"-"`
	Creator      User                      `gorm:"foreignKey:CreatedBy" json:"-"`
	Participants []VoiceChannelParticipant `gorm:"foreignKey:ChannelID" json:"participants,omitempty"`
}

// BeforeCreate sets the ID and timestamps before creating a voice channel.
func (vc *VoiceChannel) BeforeCreate(tx *gorm.DB) error {
	if vc.ID == uuid.Nil {
		vc.ID = uuid.New()
	}
	if vc.CreatedAt.IsZero() {
		vc.CreatedAt = time.Now()
	}
	if vc.UpdatedAt.IsZero() {
		vc.UpdatedAt = time.Now()
	}
	return nil
}

// VoiceChannelParticipant tracks users currently in a voice channel.
type VoiceChannelParticipant struct {
	ID         uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	ChannelID  uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_channel_user" json:"channel_id"`
	UserID     uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_channel_user" json:"user_id"`
	IsMuted    bool      `gorm:"default:false" json:"is_muted"`
	IsDeafened bool      `gorm:"default:false" json:"is_deafened"`
	IsSpeaking bool      `gorm:"default:false" json:"is_speaking"`
	JoinedAt   time.Time `json:"joined_at"`

	Channel VoiceChannel `gorm:"foreignKey:ChannelID" json:"-"`
	User    User         `gorm:"foreignKey:UserID" json:"-"`
}

// BeforeCreate sets the ID and joined timestamp before creating a participant.
func (vcp *VoiceChannelParticipant) BeforeCreate(tx *gorm.DB) error {
	if vcp.ID == uuid.Nil {
		vcp.ID = uuid.New()
	}
	if vcp.JoinedAt.IsZero() {
		vcp.JoinedAt = time.Now()
	}
	return nil
}

// VoiceChannelPermission defines granular access control for a voice channel.
// When a permission entry exists for a user, it overrides default channel behavior.
type VoiceChannelPermission struct {
	ID              uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	ChannelID       uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:idx_channel_user_perm" json:"channel_id"`
	UserID          uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:idx_channel_user_perm" json:"user_id"`
	CanJoin         bool           `gorm:"" json:"can_join"`
	CanSpeak        bool           `gorm:"" json:"can_speak"`
	CanMuteOthers   bool           `gorm:"" json:"can_mute_others"`
	CanDeafenOthers bool           `gorm:"" json:"can_deafen_others"`
	CanMoveUsers    bool           `gorm:"" json:"can_move_users"`
	IsPriority      bool           `gorm:"" json:"is_priority"`
	GrantedBy       uuid.UUID      `gorm:"type:uuid;not null" json:"granted_by"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`

	Channel       VoiceChannel `gorm:"foreignKey:ChannelID" json:"-"`
	User          User         `gorm:"foreignKey:UserID" json:"-"`
	GrantedByUser User         `gorm:"foreignKey:GrantedBy" json:"-"`
}

// BeforeCreate sets the ID and timestamps before creating a permission.
func (vcp *VoiceChannelPermission) BeforeCreate(tx *gorm.DB) error {
	if vcp.ID == uuid.Nil {
		vcp.ID = uuid.New()
	}
	now := time.Now()
	if vcp.CreatedAt.IsZero() {
		vcp.CreatedAt = now
	}
	if vcp.UpdatedAt.IsZero() {
		vcp.UpdatedAt = now
	}
	return nil
}
