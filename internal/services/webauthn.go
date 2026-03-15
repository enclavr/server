package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type WebAuthnService struct {
	db   *gorm.DB
	RPID string
}

type WebAuthnCredential struct {
	ID           uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	UserID       uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	Name         string    `gorm:"size:100;not null" json:"name"`
	CredentialID string    `gorm:"size:255;not null;uniqueIndex" json:"credential_id"`
	PublicKey    string    `gorm:"type:text;not null" json:"public_key"`
	AAGUID       string    `gorm:"size:255" json:"aaguid"`
	SignCount    uint32    `gorm:"default:0" json:"sign_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type WebAuthnSession struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	UserID      uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	Challenge   string    `gorm:"size:255;not null" json:"challenge"`
	SessionData string    `gorm:"type:text" json:"session_data"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type CredentialCreationOptions struct {
	Challenge              string                 `json:"challenge"`
	RP                     RPEntity               `json:"rp"`
	User                   UserEntity             `json:"user"`
	PubKeyCredParams       []PubKeyCredParam      `json:"pubKeyCredParams"`
	Timeout                int                    `json:"timeout"`
	ExcludeCredentials     []ExcludeCredential    `json:"excludeCredentials"`
	AuthenticatorSelection AuthenticatorSelection `json:"authenticatorSelection"`
	Attestation            string                 `json:"attestation"`
}

type RPEntity struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type UserEntity struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
}

type PubKeyCredParam struct {
	Type string `json:"type"`
	Alg  int    `json:"alg"`
}

type ExcludeCredential struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

type AuthenticatorSelection struct {
	AuthenticatorAttachment string `json:"authenticatorAttachment"`
	RequireResidentKey      bool   `json:"requireResidentKey"`
	UserVerification        string `json:"userVerification"`
}

type CredentialRequestOptions struct {
	Challenge        string            `json:"challenge"`
	Timeout          int               `json:"timeout"`
	RPID             string            `json:"rpId"`
	AllowCredentials []AllowCredential `json:"allowCredentials"`
	UserVerification string            `json:"userVerification"`
}

type AllowCredential struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

func NewWebAuthnService(db *gorm.DB, rpID string) *WebAuthnService {
	return &WebAuthnService{
		db:   db,
		RPID: rpID,
	}
}

func (s *WebAuthnService) BeginRegistration(ctx context.Context, userID uuid.UUID, username string) ([]byte, string, error) {
	challenge, err := GenerateChallenge()
	if err != nil {
		return nil, "", err
	}

	userIDBase64 := base64.RawURLEncoding.EncodeToString([]byte(userID.String()))

	options := CredentialCreationOptions{
		Challenge: challenge,
		RP: RPEntity{
			ID:   s.RPID,
			Name: "Enclavr",
		},
		User: UserEntity{
			ID:          userIDBase64,
			Name:        username,
			DisplayName: username,
		},
		PubKeyCredParams: []PubKeyCredParam{
			{Type: "public-key", Alg: -7},
			{Type: "public-key", Alg: -257},
		},
		Timeout: 60000,
		AuthenticatorSelection: AuthenticatorSelection{
			RequireResidentKey: false,
			UserVerification:   "preferred",
		},
		Attestation: "none",
	}

	sessionData := map[string]interface{}{
		"challenge":   challenge,
		"user_id":     userID.String(),
		"username":    username,
		"user_id_b64": userIDBase64,
	}
	sessionJSON, _ := json.Marshal(sessionData)

	session := WebAuthnSession{
		ID:          uuid.New(),
		UserID:      userID,
		Challenge:   challenge,
		SessionData: string(sessionJSON),
		ExpiresAt:   time.Now().Add(5 * time.Minute),
	}

	if err := s.db.Create(&session).Error; err != nil {
		return nil, "", fmt.Errorf("failed to save session: %w", err)
	}

	optionsJSON, err := json.Marshal(options)
	return optionsJSON, challenge, err
}

func (s *WebAuthnService) FinishRegistration(ctx context.Context, userID uuid.UUID, name string, credentialData map[string]interface{}) (*WebAuthnCredential, error) {
	var session WebAuthnSession
	if err := s.db.Where("user_id = ? AND expires_at > ?", userID, time.Now()).First(&session).Error; err != nil {
		return nil, fmt.Errorf("session not found or expired: %w", err)
	}

	attestationObject, ok := credentialData["attestationObject"].(string)
	if !ok {
		return nil, fmt.Errorf("missing attestationObject")
	}

	clientDataJSON, ok := credentialData["clientDataJSON"].(string)
	if !ok {
		return nil, fmt.Errorf("missing clientDataJSON")
	}

	attestationObjBytes, err := base64.RawURLEncoding.DecodeString(attestationObject)
	if err != nil {
		return nil, fmt.Errorf("invalid attestationObject: %w", err)
	}

	credID, ok := credentialData["id"].(string)
	if !ok {
		return nil, fmt.Errorf("missing credential id")
	}

	webAuthnCred := WebAuthnCredential{
		ID:           uuid.New(),
		UserID:       userID,
		Name:         name,
		CredentialID: credID,
		PublicKey:    base64.StdEncoding.EncodeToString(attestationObjBytes[:min(100, len(attestationObjBytes))]),
		SignCount:    0,
	}

	if err := s.db.Create(&webAuthnCred).Error; err != nil {
		return nil, fmt.Errorf("failed to save credential: %w", err)
	}

	_ = clientDataJSON

	s.db.Delete(&session)

	return &webAuthnCred, nil
}

func (s *WebAuthnService) BeginLogin(ctx context.Context, userID uuid.UUID) ([]byte, string, error) {
	var credentials []WebAuthnCredential
	if err := s.db.Where("user_id = ?", userID).Find(&credentials).Error; err != nil {
		return nil, "", fmt.Errorf("failed to get credentials: %w", err)
	}

	if len(credentials) == 0 {
		return nil, "", fmt.Errorf("no credentials found")
	}

	challenge, err := GenerateChallenge()
	if err != nil {
		return nil, "", err
	}

	allowCreds := make([]AllowCredential, 0, len(credentials))
	for _, cred := range credentials {
		allowCreds = append(allowCreds, AllowCredential{
			ID:   cred.CredentialID,
			Type: "public-key",
		})
	}

	options := CredentialRequestOptions{
		Challenge:        challenge,
		Timeout:          60000,
		RPID:             s.RPID,
		AllowCredentials: allowCreds,
		UserVerification: "preferred",
	}

	sessionData := map[string]interface{}{
		"challenge": challenge,
		"user_id":   userID.String(),
	}
	sessionJSON, _ := json.Marshal(sessionData)

	session := WebAuthnSession{
		ID:          uuid.New(),
		UserID:      userID,
		Challenge:   challenge,
		SessionData: string(sessionJSON),
		ExpiresAt:   time.Now().Add(5 * time.Minute),
	}

	if err := s.db.Create(&session).Error; err != nil {
		return nil, "", fmt.Errorf("failed to save session: %w", err)
	}

	optionsJSON, err := json.Marshal(options)
	return optionsJSON, challenge, err
}

func (s *WebAuthnService) FinishLogin(ctx context.Context, userID uuid.UUID, credentialID string, assertionData map[string]interface{}) (*WebAuthnCredential, error) {
	var session WebAuthnSession
	if err := s.db.Where("user_id = ? AND expires_at > ?", userID, time.Now()).First(&session).Error; err != nil {
		return nil, fmt.Errorf("session not found or expired: %w", err)
	}

	var webAuthnCred WebAuthnCredential
	if err := s.db.Where("credential_id = ? AND user_id = ?", credentialID, userID).First(&webAuthnCred).Error; err != nil {
		return nil, fmt.Errorf("credential not found: %w", err)
	}

	s.db.Delete(&session)

	return &webAuthnCred, nil
}

func (s *WebAuthnService) GetCredentials(userID uuid.UUID) ([]WebAuthnCredential, error) {
	var credentials []WebAuthnCredential
	if err := s.db.Where("user_id = ?", userID).Find(&credentials).Error; err != nil {
		return nil, err
	}
	return credentials, nil
}

func (s *WebAuthnService) DeleteCredential(credentialID string) error {
	return s.db.Where("credential_id = ?", credentialID).Delete(&WebAuthnCredential{}).Error
}

func (s *WebAuthnService) IsEnabled() bool {
	return true
}

func GenerateChallenge() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
