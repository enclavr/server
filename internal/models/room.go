package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Room struct {
	ID          uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	Name        string         `gorm:"uniqueIndex;not null;size:100" json:"name"`
	Description string         `gorm:"size:500" json:"description"`
	Password    string         `gorm:"size:255" json:"-"`
	IsPrivate   bool           `gorm:"default:false" json:"is_private"`
	MaxUsers    int            `gorm:"default:50" json:"max_users"`
	CreatedBy   uuid.UUID      `gorm:"type:uuid;not null" json:"created_by"`
	CategoryID  *uuid.UUID     `gorm:"type:uuid" json:"category_id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`

	Users []UserRoom `gorm:"foreignKey:RoomID" json:"-"`
}

func (r *Room) BeforeCreate(tx *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return nil
}

type Category struct {
	ID          uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	Name        string         `gorm:"uniqueIndex;not null;size:100" json:"name"`
	Description string         `gorm:"size:500" json:"description"`
	Icon        string         `gorm:"size:100" json:"icon"`
	Color       string         `gorm:"size:20" json:"color"`
	SortOrder   int            `gorm:"default:0" json:"sort_order"`
	IsPrivate   bool           `gorm:"default:false" json:"is_private"`
	CreatedBy   uuid.UUID      `gorm:"type:uuid;not null" json:"created_by"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`

	Rooms []Room `gorm:"foreignKey:CategoryID" json:"-"`
}

func (c *Category) BeforeCreate(tx *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return nil
}

type UserRoom struct {
	ID       uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	UserID   uuid.UUID `gorm:"type:uuid;not null" json:"user_id"`
	RoomID   uuid.UUID `gorm:"type:uuid;not null" json:"room_id"`
	Role     string    `gorm:"size:20;default:'member'" json:"role"`
	JoinedAt time.Time `json:"joined_at"`

	User User `gorm:"foreignKey:UserID" json:"-"`
	Room Room `gorm:"foreignKey:RoomID" json:"-"`
}

func (ur *UserRoom) BeforeCreate(tx *gorm.DB) error {
	if ur.ID == uuid.Nil {
		ur.ID = uuid.New()
	}
	if ur.JoinedAt.IsZero() {
		ur.JoinedAt = time.Now()
	}
	return nil
}

type ServerSettings struct {
	ID                   uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	ServerName           string    `gorm:"size:100;default:'Enclavr Server'" json:"server_name"`
	ServerDescription    string    `gorm:"size:500" json:"server_description"`
	AllowRegistration    bool      `gorm:"default:true" json:"allow_registration"`
	MaxRoomsPerUser      int       `gorm:"default:10" json:"max_rooms_per_user"`
	MaxMembersPerRoom    int       `gorm:"default:50" json:"max_members_per_room"`
	EnableVoiceChat      bool      `gorm:"default:true" json:"enable_voice_chat"`
	EnableDirectMessages bool      `gorm:"default:true" json:"enable_direct_messages"`
	EnableFileUploads    bool      `gorm:"default:false" json:"enable_file_uploads"`
	MaxUploadSizeMB      int       `gorm:"default:10" json:"max_upload_size_mb"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

func (ss *ServerSettings) BeforeCreate(tx *gorm.DB) error {
	if ss.ID == uuid.Nil {
		ss.ID = uuid.New()
	}
	if ss.CreatedAt.IsZero() {
		ss.CreatedAt = time.Now()
	}
	return nil
}

type RoomSettings struct {
	ID                uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	RoomID            uuid.UUID `gorm:"type:uuid;not null;uniqueIndex" json:"room_id"`
	AllowMessageEdits bool      `gorm:"default:true" json:"allow_message_edits"`
	AllowReactions    bool      `gorm:"default:true" json:"allow_reactions"`
	RequireApproval   bool      `gorm:"default:false" json:"require_approval"`
	MaxUsers          int       `gorm:"default:0" json:"max_users"`
	AutoDeleteDays    int       `gorm:"default:0" json:"auto_delete_days"`
	SlowModeSeconds   int       `gorm:"default:0" json:"slow_mode_seconds"`
	AllowVoiceChat    bool      `gorm:"default:true" json:"allow_voice_chat"`
	AllowFileUploads  bool      `gorm:"default:true" json:"allow_file_uploads"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`

	Room Room `gorm:"foreignKey:RoomID" json:"-"`
}

func (rs *RoomSettings) BeforeCreate(tx *gorm.DB) error {
	if rs.ID == uuid.Nil {
		rs.ID = uuid.New()
	}
	if rs.CreatedAt.IsZero() {
		rs.CreatedAt = time.Now()
	}
	return nil
}

type RoomTemplate struct {
	ID          uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	Name        string         `gorm:"size:100;not null;index" json:"name"`
	Description string         `gorm:"size:500" json:"description"`
	CategoryID  *uuid.UUID     `gorm:"type:uuid" json:"category_id"`
	CreatedBy   uuid.UUID      `gorm:"type:uuid;not null;index" json:"created_by"`
	Settings    string         `gorm:"type:jsonb" json:"settings"`
	IsPublic    bool           `gorm:"default:false" json:"is_public"`
	UseCount    int            `gorm:"default:0" json:"use_count"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`

	Category Category `gorm:"foreignKey:CategoryID" json:"-"`
	User     User     `gorm:"foreignKey:CreatedBy" json:"-"`
}

func (rt *RoomTemplate) BeforeCreate(tx *gorm.DB) error {
	if rt.ID == uuid.Nil {
		rt.ID = uuid.New()
	}
	if rt.CreatedAt.IsZero() {
		rt.CreatedAt = time.Now()
	}
	if rt.UpdatedAt.IsZero() {
		rt.UpdatedAt = time.Now()
	}
	return nil
}
