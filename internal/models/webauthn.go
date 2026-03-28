package models

import (
	"time"

	"github.com/google/uuid"
)

// WebAuthnCredential stores a registered WebAuthn/passkey credential for a user.
type WebAuthnCredential struct {
	ID           uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	UserID       uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	Name         string    `gorm:"size:100;not null" json:"name"`
	CredentialID string    `gorm:"size:255;not null;uniqueIndex" json:"credential_id"`
	PublicKey    string    `gorm:"type:text;not null" json:"-"`
	AAGUID       string    `gorm:"size:255" json:"aaguid"`
	SignCount    uint32    `gorm:"default:0" json:"sign_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (WebAuthnCredential) TableName() string {
	return "webauthn_credentials"
}

// WebAuthnSession stores temporary challenge data during WebAuthn registration or authentication.
type WebAuthnSession struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	UserID      uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	Challenge   string    `gorm:"size:255;not null" json:"-"`
	SessionData string    `gorm:"type:text" json:"-"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func (WebAuthnSession) TableName() string {
	return "webauthn_sessions"
}
