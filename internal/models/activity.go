package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type NotificationLevel string

const (
	NotificationLevelAll      NotificationLevel = "all"
	NotificationLevelMentions NotificationLevel = "mentions"
	NotificationLevelNone     NotificationLevel = "none"
)

type NotificationPreferences struct {
	ID                         uuid.UUID                       `gorm:"type:uuid;primary_key" json:"id"`
	UserID                     uuid.UUID                       `gorm:"type:uuid;not null;uniqueIndex" json:"user_id"`
	RoomNotifications          map[uuid.UUID]NotificationLevel `gorm:"-" json:"room_notifications"`
	RoomNotificationsJSON      string                          `gorm:"type:jsonb" json:"-"`
	DMNotifications            NotificationLevel               `gorm:"type:varchar(20);default:'all'" json:"dm_notifications"`
	GroupNotifications         NotificationLevel               `gorm:"type:varchar(20);default:'all'" json:"group_notifications"`
	MentionNotifications       bool                            `gorm:"default:true" json:"mention_notifications"`
	ReplyNotifications         bool                            `gorm:"default:true" json:"reply_notifications"`
	ReactionNotifications      bool                            `gorm:"default:true" json:"reaction_notifications"`
	DirectMessageNotifications bool                            `gorm:"default:true" json:"direct_message_notifications"`
	RoomInviteNotifications    bool                            `gorm:"default:true" json:"room_invite_notifications"`
	SoundEnabled               bool                            `gorm:"default:true" json:"sound_enabled"`
	DesktopNotifications       bool                            `gorm:"default:true" json:"desktop_notifications"`
	MobilePushEnabled          bool                            `gorm:"default:true" json:"mobile_push_enabled"`
	QuietHoursEnabled          bool                            `gorm:"default:false" json:"quiet_hours_enabled"`
	QuietHoursStart            string                          `gorm:"size:5;default:'22:00'" json:"quiet_hours_start"`
	QuietHoursEnd              string                          `gorm:"size:5;default:'08:00'" json:"quiet_hours_end"`
	CreatedAt                  time.Time                       `json:"created_at"`
	UpdatedAt                  time.Time                       `json:"updated_at"`

	User User `gorm:"foreignKey:UserID" json:"-"`
}

func (np *NotificationPreferences) BeforeCreate(tx *gorm.DB) error {
	if np.ID == uuid.Nil {
		np.ID = uuid.New()
	}
	if np.CreatedAt.IsZero() {
		np.CreatedAt = time.Now()
	}
	return nil
}

type RoomBookmark struct {
	ID        uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	UserID    uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	RoomID    uuid.UUID      `gorm:"type:uuid;not null;index" json:"room_id"`
	Note      string         `gorm:"size:500" json:"note"`
	Position  int            `gorm:"default:0" json:"position"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	User User `gorm:"foreignKey:UserID" json:"-"`
	Room Room `gorm:"foreignKey:RoomID" json:"-"`
}

func (rb *RoomBookmark) BeforeCreate(tx *gorm.DB) error {
	if rb.ID == uuid.Nil {
		rb.ID = uuid.New()
	}
	if rb.CreatedAt.IsZero() {
		rb.CreatedAt = time.Now()
	}
	if rb.UpdatedAt.IsZero() {
		rb.UpdatedAt = time.Now()
	}
	return nil
}

type MessageEditHistory struct {
	ID         uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	MessageID  uuid.UUID `gorm:"type:uuid;not null;index" json:"message_id"`
	UserID     uuid.UUID `gorm:"type:uuid;not null" json:"user_id"`
	OldContent string    `gorm:"type:text;not null" json:"old_content"`
	NewContent string    `gorm:"type:text;not null" json:"new_content"`
	CreatedAt  time.Time `json:"created_at"`

	Message Message `gorm:"foreignKey:MessageID" json:"-"`
	User    User    `gorm:"foreignKey:UserID" json:"-"`
}

func (meh *MessageEditHistory) BeforeCreate(tx *gorm.DB) error {
	if meh.ID == uuid.Nil {
		meh.ID = uuid.New()
	}
	if meh.CreatedAt.IsZero() {
		meh.CreatedAt = time.Now()
	}
	return nil
}

type ActivityType string

const (
	ActivityTypeLogin         ActivityType = "login"
	ActivityTypeLogout        ActivityType = "logout"
	ActivityTypeMessage       ActivityType = "message"
	ActivityTypeRoomJoin      ActivityType = "room_join"
	ActivityTypeRoomLeave     ActivityType = "room_leave"
	ActivityTypeVoiceJoin     ActivityType = "voice_join"
	ActivityTypeVoiceLeave    ActivityType = "voice_leave"
	ActivityTypeFileUpload    ActivityType = "file_upload"
	ActivityTypeReaction      ActivityType = "reaction"
	ActivityTypeProfileUpdate ActivityType = "profile_update"
)

type UserActivity struct {
	ID        uuid.UUID    `gorm:"type:uuid;primary_key" json:"id"`
	UserID    uuid.UUID    `gorm:"type:uuid;not null;index" json:"user_id"`
	Activity  ActivityType `gorm:"type:varchar(30);not null;index" json:"activity"`
	RoomID    *uuid.UUID   `gorm:"type:uuid;index" json:"room_id"`
	TargetID  *uuid.UUID   `gorm:"type:uuid" json:"target_id"`
	Metadata  string       `gorm:"type:jsonb" json:"metadata"`
	IPAddress string       `gorm:"size:45" json:"ip_address"`
	UserAgent string       `gorm:"size:500" json:"user_agent"`
	CreatedAt time.Time    `json:"created_at"`

	User User `gorm:"foreignKey:UserID" json:"-"`
	Room Room `gorm:"foreignKey:RoomID" json:"-"`
}

func (ua *UserActivity) BeforeCreate(tx *gorm.DB) error {
	if ua.ID == uuid.Nil {
		ua.ID = uuid.New()
	}
	if ua.CreatedAt.IsZero() {
		ua.CreatedAt = time.Now()
	}
	return nil
}

type RoomParticipant struct {
	ID         uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	RoomID     uuid.UUID      `gorm:"type:uuid;not null;index" json:"room_id"`
	UserID     uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	JoinedAt   time.Time      `json:"joined_at"`
	LastReadAt time.Time      `json:"last_read_at"`
	CreatedAt  time.Time      `json:"created_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`

	Room Room `gorm:"foreignKey:RoomID" json:"-"`
	User User `gorm:"foreignKey:UserID" json:"-"`
}

func (rp *RoomParticipant) BeforeCreate(tx *gorm.DB) error {
	if rp.ID == uuid.Nil {
		rp.ID = uuid.New()
	}
	if rp.JoinedAt.IsZero() {
		rp.JoinedAt = time.Now()
	}
	if rp.LastReadAt.IsZero() {
		rp.LastReadAt = time.Now()
	}
	if rp.CreatedAt.IsZero() {
		rp.CreatedAt = time.Now()
	}
	return nil
}
