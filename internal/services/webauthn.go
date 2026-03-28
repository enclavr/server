package services

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/enclavr/server/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type WebAuthnService struct {
	db   *gorm.DB
	RPID string
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

	session := models.WebAuthnSession{
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

func (s *WebAuthnService) FinishRegistration(ctx context.Context, userID uuid.UUID, name string, credentialData map[string]interface{}) (*models.WebAuthnCredential, error) {
	var session models.WebAuthnSession
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

	clientDataBytes, err := base64.RawURLEncoding.DecodeString(clientDataJSON)
	if err != nil {
		return nil, fmt.Errorf("invalid clientDataJSON: %w", err)
	}

	var clientData struct {
		Type      string `json:"type"`
		Challenge string `json:"challenge"`
		Origin    string `json:"origin"`
	}
	if err := json.Unmarshal(clientDataBytes, &clientData); err != nil {
		return nil, fmt.Errorf("failed to parse clientDataJSON: %w", err)
	}

	if clientData.Challenge != session.Challenge {
		return nil, fmt.Errorf("challenge mismatch")
	}

	if clientData.Type != "webauthn.create" {
		return nil, fmt.Errorf("invalid clientData type: %s", clientData.Type)
	}

	publicKeyBytes, signCount, err := parseAttestationObject(attestationObjBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse attestation object: %w", err)
	}

	webAuthnCred := models.WebAuthnCredential{
		ID:           uuid.New(),
		UserID:       userID,
		Name:         name,
		CredentialID: credID,
		PublicKey:    base64.StdEncoding.EncodeToString(publicKeyBytes),
		SignCount:    signCount,
	}

	if err := s.db.Create(&webAuthnCred).Error; err != nil {
		return nil, fmt.Errorf("failed to save credential: %w", err)
	}

	s.db.Delete(&session)

	return &webAuthnCred, nil
}

func (s *WebAuthnService) BeginLogin(ctx context.Context, userID uuid.UUID) ([]byte, string, error) {
	var credentials []models.WebAuthnCredential
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

	session := models.WebAuthnSession{
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

func (s *WebAuthnService) FinishLogin(ctx context.Context, userID uuid.UUID, credentialID string, assertionData map[string]interface{}) (*models.WebAuthnCredential, error) {
	var session models.WebAuthnSession
	if err := s.db.Where("user_id = ? AND expires_at > ?", userID, time.Now()).First(&session).Error; err != nil {
		return nil, fmt.Errorf("session not found or expired: %w", err)
	}

	var webAuthnCred models.WebAuthnCredential
	if err := s.db.Where("credential_id = ? AND user_id = ?", credentialID, userID).First(&webAuthnCred).Error; err != nil {
		return nil, fmt.Errorf("credential not found: %w", err)
	}

	clientDataJSON, ok := assertionData["clientDataJSON"].(string)
	if !ok {
		return nil, fmt.Errorf("missing clientDataJSON")
	}

	authenticatorData, ok := assertionData["authenticatorData"].(string)
	if !ok {
		return nil, fmt.Errorf("missing authenticatorData")
	}

	signature, ok := assertionData["signature"].(string)
	if !ok {
		return nil, fmt.Errorf("missing signature")
	}

	clientDataBytes, err := base64.RawURLEncoding.DecodeString(clientDataJSON)
	if err != nil {
		return nil, fmt.Errorf("invalid clientDataJSON: %w", err)
	}

	var clientData struct {
		Type      string `json:"type"`
		Challenge string `json:"challenge"`
		Origin    string `json:"origin"`
	}
	if err := json.Unmarshal(clientDataBytes, &clientData); err != nil {
		return nil, fmt.Errorf("failed to parse clientDataJSON: %w", err)
	}

	if clientData.Challenge != session.Challenge {
		return nil, fmt.Errorf("challenge mismatch")
	}

	if clientData.Type != "webauthn.get" {
		return nil, fmt.Errorf("invalid clientData type: %s", clientData.Type)
	}

	authDataBytes, err := base64.RawURLEncoding.DecodeString(authenticatorData)
	if err != nil {
		return nil, fmt.Errorf("invalid authenticatorData: %w", err)
	}

	if len(authDataBytes) < 37 {
		return nil, fmt.Errorf("authenticatorData too short")
	}

	flags := authDataBytes[32]
	if flags&0x01 == 0 {
		return nil, fmt.Errorf("user not present")
	}

	rpIDHash := authDataBytes[:32]
	expectedRPIDHash := sha256.Sum256([]byte(s.RPID))
	if !bytesEqual(rpIDHash, expectedRPIDHash[:]) {
		return nil, fmt.Errorf("rpId hash mismatch")
	}

	sigBytes, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return nil, fmt.Errorf("invalid signature: %w", err)
	}

	clientDataHash := sha256.Sum256(clientDataBytes)
	verificationData := append(authDataBytes, clientDataHash[:]...)

	publicKeyBytes, err := base64.StdEncoding.DecodeString(webAuthnCred.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("invalid stored public key: %w", err)
	}

	if err := verifySignature(publicKeyBytes, verificationData, sigBytes); err != nil {
		return nil, fmt.Errorf("signature verification failed: %w", err)
	}

	storedSignCount := webAuthnCred.SignCount
	authDataSignCount := binary.BigEndian.Uint32(authDataBytes[33:37])
	if authDataSignCount > 0 || storedSignCount > 0 {
		if authDataSignCount <= storedSignCount {
			return nil, fmt.Errorf("sign count did not increase - possible cloned authenticator")
		}
	}
	webAuthnCred.SignCount = authDataSignCount
	s.db.Save(&webAuthnCred)

	s.db.Delete(&session)

	return &webAuthnCred, nil
}

func (s *WebAuthnService) GetCredentials(userID uuid.UUID) ([]models.WebAuthnCredential, error) {
	var credentials []models.WebAuthnCredential
	if err := s.db.Where("user_id = ?", userID).Find(&credentials).Error; err != nil {
		return nil, err
	}
	return credentials, nil
}

func (s *WebAuthnService) DeleteCredential(credentialID string) error {
	return s.db.Where("credential_id = ?", credentialID).Delete(&models.WebAuthnCredential{}).Error
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

func parseAttestationObject(data []byte) (publicKey []byte, signCount uint32, err error) {
	if len(data) < 37 {
		return nil, 0, fmt.Errorf("attestation object too short")
	}

	offset := 0

	if offset+1 > len(data) {
		return nil, 0, fmt.Errorf("truncated attestation object")
	}
	if data[offset] != 0xA3 {
		return nil, 0, fmt.Errorf("expected CBOR map with 3 entries")
	}
	offset++

	for i := 0; i < 3; i++ {
		if offset >= len(data) {
			return nil, 0, fmt.Errorf("truncated attestation object")
		}

		keyLen := 0
		b := data[offset]
		if b >= 0x60 && b <= 0x77 {
			keyLen = int(b - 0x60)
			offset++
		} else if b == 0x78 {
			offset++
			if offset >= len(data) {
				return nil, 0, fmt.Errorf("truncated")
			}
			keyLen = int(data[offset])
			offset++
		} else {
			return nil, 0, fmt.Errorf("unexpected CBOR type for key")
		}

		key := string(data[offset : offset+keyLen])
		offset += keyLen

		if key == "authData" {
			if offset >= len(data) {
				return nil, 0, fmt.Errorf("truncated")
			}
			b := data[offset]
			if b == 0x58 {
				offset++
				if offset >= len(data) {
					return nil, 0, fmt.Errorf("truncated")
				}
				authDataLen := int(data[offset])
				offset++
				if offset+authDataLen > len(data) {
					return nil, 0, fmt.Errorf("truncated authData")
				}
				authData := data[offset : offset+authDataLen]

				if len(authData) >= 37 {
					signCount = binary.BigEndian.Uint32(authData[33:37])

					credentialDataStart := 37
					if len(authData) > credentialDataStart+16 {
						offset2 := credentialDataStart + 16
						if offset2+2 <= len(authData) {
							pubKeyLen := int(binary.BigEndian.Uint16(authData[offset2 : offset2+2]))
							offset2 += 2
							if offset2+pubKeyLen <= len(authData) {
								publicKey = make([]byte, pubKeyLen)
								copy(publicKey, authData[offset2:offset2+pubKeyLen])
								return publicKey, signCount, nil
							}
						}
					}
				}
				return nil, 0, fmt.Errorf("no credential data found in authData")
			}
			return nil, 0, fmt.Errorf("unexpected authData encoding")
		}

		if offset >= len(data) {
			return nil, 0, fmt.Errorf("truncated")
		}
		valByte := data[offset]
		if valByte <= 0x17 {
			offset++
		} else if valByte >= 0x18 && valByte <= 0x1B {
			offset++
			valLen := 1 << (valByte - 0x18)
			offset += valLen
		} else if valByte >= 0x40 && valByte <= 0x57 {
			offset++
			valLen := int(valByte - 0x40)
			offset += valLen
		} else if valByte == 0x58 {
			offset++
			if offset >= len(data) {
				return nil, 0, fmt.Errorf("truncated")
			}
			valLen := int(data[offset])
			offset++
			offset += valLen
		} else if valByte == 0x59 {
			offset++
			if offset+2 > len(data) {
				return nil, 0, fmt.Errorf("truncated")
			}
			valLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
			offset += 2
			offset += valLen
		} else if valByte >= 0x80 && valByte <= 0x97 {
			offset++
		} else if valByte >= 0xA0 && valByte <= 0xB7 {
			offset++
		} else {
			offset++
		}
	}

	return nil, 0, fmt.Errorf("authData not found in attestation object")
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func verifySignature(publicKeyBytes, message, signature []byte) error {
	coseKey, err := parseCOSEKey(publicKeyBytes)
	if err != nil {
		return fmt.Errorf("failed to parse COSE key: %w", err)
	}

	switch coseKey.Alg {
	case -7:
		return verifyES256(coseKey.X, coseKey.Y, message, signature)
	case -257:
		return fmt.Errorf("RS256 verification not yet implemented")
	default:
		return fmt.Errorf("unsupported algorithm: %d", coseKey.Alg)
	}
}

type coseKey struct {
	Kty int
	Alg int
	Crv int
	X   []byte
	Y   []byte
}

func parseCOSEKey(data []byte) (*coseKey, error) {
	if len(data) < 10 {
		return nil, fmt.Errorf("COSE key too short")
	}

	if data[0] != 0xA5 {
		return nil, fmt.Errorf("expected CBOR map with 5 entries, got 0x%02x", data[0])
	}

	key := &coseKey{}
	offset := 1

	for i := 0; i < 5; i++ {
		if offset >= len(data) {
			return nil, fmt.Errorf("truncated COSE key")
		}

		var keyID int
		b := data[offset]
		if b <= 0x17 {
			keyID = int(b)
			offset++
		} else if b == 0x18 {
			offset++
			if offset >= len(data) {
				return nil, fmt.Errorf("truncated")
			}
			keyID = int(data[offset])
			offset++
		} else {
			return nil, fmt.Errorf("unexpected key identifier: 0x%02x", b)
		}

		if offset >= len(data) {
			return nil, fmt.Errorf("truncated")
		}

		valByte := data[offset]
		var valBytes []byte

		if valByte <= 0x17 {
			valBytes = []byte{valByte}
			offset++
		} else if valByte == 0x18 {
			offset++
			if offset >= len(data) {
				return nil, fmt.Errorf("truncated")
			}
			valBytes = []byte{data[offset]}
			offset++
		} else if valByte == 0x19 {
			offset++
			if offset+2 > len(data) {
				return nil, fmt.Errorf("truncated")
			}
			valBytes = data[offset : offset+2]
			offset += 2
		} else if valByte >= 0x40 && valByte <= 0x57 {
			valLen := int(valByte - 0x40)
			offset++
			if offset+valLen > len(data) {
				return nil, fmt.Errorf("truncated")
			}
			valBytes = data[offset : offset+valLen]
			offset += valLen
		} else if valByte == 0x58 {
			offset++
			if offset >= len(data) {
				return nil, fmt.Errorf("truncated")
			}
			valLen := int(data[offset])
			offset++
			if offset+valLen > len(data) {
				return nil, fmt.Errorf("truncated")
			}
			valBytes = data[offset : offset+valLen]
			offset += valLen
		} else if valByte == 0x59 {
			offset++
			if offset+2 > len(data) {
				return nil, fmt.Errorf("truncated")
			}
			valLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
			offset += 2
			if offset+valLen > len(data) {
				return nil, fmt.Errorf("truncated")
			}
			valBytes = data[offset : offset+valLen]
			offset += valLen
		} else if valByte == 0x20 || valByte == 0x21 {
			valBytes = []byte{valByte}
			offset++
		} else {
			return nil, fmt.Errorf("unsupported CBOR value type: 0x%02x", valByte)
		}

		switch keyID {
		case 1:
			if len(valBytes) >= 1 {
				key.Kty = int(valBytes[len(valBytes)-1])
			}
		case 3:
			if len(valBytes) == 1 {
				key.Alg = int(int8(valBytes[0]))
			} else if len(valBytes) == 2 {
				key.Alg = int(int16(binary.BigEndian.Uint16(valBytes)))
			}
		case -1:
			if len(valBytes) >= 1 {
				key.Crv = int(valBytes[len(valBytes)-1])
			}
		case -2:
			key.X = valBytes
		case -3:
			key.Y = valBytes
		}
	}

	if key.X == nil || key.Y == nil {
		return nil, fmt.Errorf("missing X or Y coordinate in EC key")
	}

	return key, nil
}

func verifyES256(xBytes, yBytes, message, signature []byte) error {
	x := new(big.Int).SetBytes(xBytes)
	y := new(big.Int).SetBytes(yBytes)

	curve := elliptic.P256()
	if !curve.IsOnCurve(x, y) {
		return fmt.Errorf("public key point not on curve")
	}

	pubKey := &ecdsa.PublicKey{
		Curve: curve,
		X:     x,
		Y:     y,
	}

	type ecdsaSignature struct {
		R, S *big.Int
	}
	var sig ecdsaSignature
	if _, err := asn1.Unmarshal(signature, &sig); err != nil {
		return fmt.Errorf("failed to parse ECDSA signature: %w", err)
	}

	hash := sha256.Sum256(message)
	if !ecdsa.Verify(pubKey, hash[:], sig.R, sig.S) {
		return fmt.Errorf("ECDSA signature verification failed")
	}

	return nil
}
