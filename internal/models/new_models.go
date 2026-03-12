package models

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type APIKey struct {
	ID          uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	Name        string         `gorm:"size:100;not null" json:"name"`
	KeyHash     string         `gorm:"size:64;not null;uniqueIndex" json:"-"`
	KeyPrefix   string         `gorm:"size:8;not null" json:"key_prefix"`
	UserID      uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	Permissions string         `gorm:"type:text" json:"permissions"`
	ExpiresAt   *time.Time     `json:"expires_at"`
	LastUsedAt  *time.Time     `json:"last_used_at"`
	IPWhitelist string         `gorm:"size:500" json:"ip_whitelist"`
	IsActive    bool           `gorm:"default:true" json:"is_active"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`

	User User `gorm:"foreignKey:UserID" json:"-"`
}

func (a *APIKey) BeforeCreate(tx *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now()
	}
	key, err := generateSecureKey(32)
	if err != nil {
		return err
	}
	a.KeyHash = hashKey(key)
	a.KeyPrefix = key[:8]
	return nil
}

func generateSecureKey(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func hashKey(key string) string {
	return key
}

type Role struct {
	ID          uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	Name        string         `gorm:"size:50;not null;uniqueIndex" json:"name"`
	DisplayName string         `gorm:"size:100;not null" json:"display_name"`
	Description string         `gorm:"size:255" json:"description"`
	Permissions string         `gorm:"type:text;not null" json:"permissions"`
	IsDefault   bool           `gorm:"default:false" json:"is_default"`
	IsAdmin     bool           `gorm:"default:false" json:"is_admin"`
	RoomID      *uuid.UUID     `gorm:"type:uuid;index" json:"room_id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`

	Room Room `gorm:"foreignKey:RoomID" json:"-"`
}

func (r *Role) BeforeCreate(tx *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now()
	}
	return nil
}

type RolePermission struct {
	ID         uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	RoleID     uuid.UUID      `gorm:"type:uuid;not null;index" json:"role_id"`
	Permission string         `gorm:"size:50;not null" json:"permission"`
	Resource   string         `gorm:"size:50;not null" json:"resource"`
	Action     string         `gorm:"size:20;not null" json:"action"`
	CreatedAt  time.Time      `json:"created_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`

	Role Role `gorm:"foreignKey:RoleID" json:"-"`
}

func (rp *RolePermission) BeforeCreate(tx *gorm.DB) error {
	if rp.ID == uuid.Nil {
		rp.ID = uuid.New()
	}
	if rp.CreatedAt.IsZero() {
		rp.CreatedAt = time.Now()
	}
	return nil
}

type UserRole struct {
	ID        uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	UserID    uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	RoleID    uuid.UUID      `gorm:"type:uuid;not null;index" json:"role_id"`
	RoomID    *uuid.UUID     `gorm:"type:uuid;index" json:"room_id"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	User User `gorm:"foreignKey:UserID" json:"-"`
	Role Role `gorm:"foreignKey:RoleID" json:"-"`
	Room Room `gorm:"foreignKey:RoomID" json:"-"`
}

func (ur *UserRole) BeforeCreate(tx *gorm.DB) error {
	if ur.ID == uuid.Nil {
		ur.ID = uuid.New()
	}
	if ur.CreatedAt.IsZero() {
		ur.CreatedAt = time.Now()
	}
	return nil
}

type UserNotification struct {
	ID        uuid.UUID      `gorm:"type:uuid;primary_key" json:"id"`
	UserID    uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	Type      string         `gorm:"size:50;not null" json:"type"`
	Title     string         `gorm:"size:200;not null" json:"title"`
	Body      string         `gorm:"type:text" json:"body"`
	Data      string         `gorm:"type:jsonb" json:"data"`
	IsRead    bool           `gorm:"default:false;index" json:"is_read"`
	ReadAt    *time.Time     `json:"read_at"`
	ExpiresAt *time.Time     `json:"expires_at"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	User User `gorm:"foreignKey:UserID" json:"-"`
}

func (un *UserNotification) BeforeCreate(tx *gorm.DB) error {
	if un.ID == uuid.Nil {
		un.ID = uuid.New()
	}
	if un.CreatedAt.IsZero() {
		un.CreatedAt = time.Now()
	}
	return nil
}
