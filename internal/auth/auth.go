package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/enclavr/server/internal/config"
	"github.com/enclavr/server/internal/models"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrTokenExpired        = errors.New("token expired")
	ErrInvalidToken        = errors.New("invalid token")
	ErrInvalid2FA          = errors.New("invalid 2FA code")
	Err2FARequired         = errors.New("2FA verification required")
	ErrWeakPassword        = errors.New("password does not meet complexity requirements")
	ErrInvalidRecoveryCode = errors.New("invalid recovery code")
	ErrTooManyAttempts     = errors.New("too many login attempts, please try again later")
)

const (
	MinPasswordLength    = 8
	RequireUppercase     = true
	RequireLowercase     = true
	RequireNumber        = true
	RequireSpecial       = true
	PasswordHistoryCount = 5
	PasswordMaxAgeDays   = 90
)

type Claims struct {
	UserID    uuid.UUID `json:"user_id"`
	SessionID uuid.UUID `json:"session_id,omitempty"`
	Username  string    `json:"username"`
	IsAdmin   bool      `json:"is_admin"`
	jwt.RegisteredClaims
}

type RefreshClaims struct {
	UserID      uuid.UUID `json:"user_id"`
	Type        string    `json:"type"`
	TokenFamily string    `json:"token_family,omitempty"`
	jwt.RegisteredClaims
}

type AuthService struct {
	cfg       *config.AuthConfig
	encryptor *Encryptor
}

func NewAuthService(cfg *config.AuthConfig) *AuthService {
	return &AuthService{cfg: cfg}
}

func NewAuthServiceWithEncryption(cfg *config.AuthConfig, encryptor *Encryptor) *AuthService {
	return &AuthService{cfg: cfg, encryptor: encryptor}
}

func (s *AuthService) HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

func (s *AuthService) CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func (s *AuthService) HashToken(token string) string {
	mac := hmac.New(sha256.New, []byte(s.cfg.JWTSecret))
	mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *AuthService) ValidatePasswordStrength(password string) error {
	if len(password) < MinPasswordLength {
		return fmt.Errorf("password must be at least %d characters", MinPasswordLength)
	}

	var hasUpper, hasLower, hasNumber, hasSpecial bool

	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsNumber(char):
			hasNumber = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	if RequireUppercase && !hasUpper {
		return errors.New("password must contain at least one uppercase letter")
	}
	if RequireLowercase && !hasLower {
		return errors.New("password must contain at least one lowercase letter")
	}
	if RequireNumber && !hasNumber {
		return errors.New("password must contain at least one number")
	}
	if RequireSpecial && !hasSpecial {
		return errors.New("password must contain at least one special character")
	}

	return nil
}

func (s *AuthService) CheckPasswordHistory(password string, historyHashes []string) error {
	if len(historyHashes) == 0 {
		return nil
	}

	for _, hash := range historyHashes {
		if s.CheckPassword(password, hash) {
			return fmt.Errorf("password cannot be one of the last %d passwords used", PasswordHistoryCount)
		}
	}
	return nil
}

func (s *AuthService) IsPasswordExpired(passwordChangedAt *time.Time) bool {
	if passwordChangedAt == nil {
		return true
	}
	expiryDate := passwordChangedAt.AddDate(0, 0, PasswordMaxAgeDays)
	return time.Now().After(expiryDate)
}

func (s *AuthService) GenerateToken(user *models.User, sessionID ...uuid.UUID) (string, error) {
	notBefore := jwt.NewNumericDate(time.Now())
	if user.PasswordChangedAt != nil {
		notBefore = jwt.NewNumericDate(*user.PasswordChangedAt)
	}

	claims := Claims{
		UserID:   user.ID,
		Username: user.Username,
		IsAdmin:  user.IsAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.cfg.JWTExpiration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: notBefore,
			Issuer:    "enclavr",
		},
	}

	if len(sessionID) > 0 {
		claims.SessionID = sessionID[0]
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.cfg.JWTSecret))
}

func (s *AuthService) GenerateRefreshToken(user *models.User) (string, error) {
	return s.GenerateRefreshTokenWithFamily(user, "")
}

func (s *AuthService) GenerateRefreshTokenWithFamily(user *models.User, tokenFamily string) (string, error) {
	claims := RefreshClaims{
		UserID:      user.ID,
		Type:        "refresh",
		TokenFamily: tokenFamily,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.cfg.RefreshExpiration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "enclavr",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.cfg.JWTSecret))
}

func (s *AuthService) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.cfg.JWTSecret), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, ErrInvalidToken
}

func (s *AuthService) ValidateRefreshToken(tokenString string) (*RefreshClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &RefreshClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.cfg.JWTSecret), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*RefreshClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, ErrInvalidToken
}

func GenerateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func GenerateTokenFamily() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func GenerateUUID() uuid.UUID {
	return uuid.New()
}

func (s *AuthService) GenerateTwoFactorSecret() (string, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "Enclavr",
		AccountName: "",
		Algorithm:   otp.AlgorithmSHA1,
		Digits:      otp.DigitsSix,
		Period:      30,
	})
	if err != nil {
		return "", err
	}
	return key.Secret(), nil
}

func (s *AuthService) ValidateTwoFactorCode(secret, code string) bool {
	return totp.Validate(code, secret)
}

func (s *AuthService) EncryptSecret(secret string) (string, error) {
	if s.encryptor == nil {
		return secret, nil
	}
	return s.encryptor.Encrypt(secret)
}

func (s *AuthService) DecryptSecret(encryptedSecret string) (string, error) {
	if s.encryptor == nil {
		return encryptedSecret, nil
	}
	return s.encryptor.Decrypt(encryptedSecret)
}

func (s *AuthService) HasEncryptor() bool {
	return s.encryptor != nil
}

func (s *AuthService) GenerateEmailVerificationToken(userID uuid.UUID) (string, error) {
	claims := RefreshClaims{
		UserID: userID,
		Type:   "email_verification",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "enclavr",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.cfg.JWTSecret))
}

func (s *AuthService) ValidateEmailVerificationToken(tokenString string) (*RefreshClaims, error) {
	return s.validateTokenType(tokenString, "email_verification")
}

func (s *AuthService) GeneratePasswordResetToken(userID uuid.UUID) (string, error) {
	claims := RefreshClaims{
		UserID: userID,
		Type:   "password_reset",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "enclavr",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.cfg.JWTSecret))
}

func (s *AuthService) ValidatePasswordResetToken(tokenString string) (*RefreshClaims, error) {
	return s.validateTokenType(tokenString, "password_reset")
}

func (s *AuthService) validateTokenType(tokenString, expectedType string) (*RefreshClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &RefreshClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.cfg.JWTSecret), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*RefreshClaims); ok && token.Valid {
		if claims.Type != expectedType {
			return nil, ErrInvalidToken
		}
		return claims, nil
	}

	return nil, ErrInvalidToken
}

func (s *AuthService) GenerateRecoveryCodes() ([]string, error) {
	codes := make([]string, 10)
	for i := 0; i < 10; i++ {
		b := make([]byte, 4)
		if _, err := rand.Read(b); err != nil {
			return nil, err
		}
		codes[i] = base64.StdEncoding.EncodeToString(b)[:8]
	}
	return codes, nil
}

func (s *AuthService) HashRecoveryCodes(codes []string) (string, error) {
	hashedCodes := make([]string, len(codes))
	for i, code := range codes {
		hash, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
		if err != nil {
			return "", err
		}
		hashedCodes[i] = string(hash)
	}
	return strings.Join(hashedCodes, "|||"), nil
}

func (s *AuthService) ValidateRecoveryCode(hashedCodes, providedCode string) bool {
	hashedList := strings.Split(hashedCodes, "|||")
	for _, hashedCode := range hashedList {
		if err := bcrypt.CompareHashAndPassword([]byte(hashedCode), []byte(providedCode)); err == nil {
			return true
		}
	}
	return false
}

func (s *AuthService) RemoveUsedRecoveryCode(hashedCodes, usedCode string) (string, error) {
	hashedList := strings.Split(hashedCodes, "|||")
	newList := make([]string, 0, len(hashedList))
	for _, hashedCode := range hashedList {
		if err := bcrypt.CompareHashAndPassword([]byte(hashedCode), []byte(usedCode)); err == nil {
			continue
		}
		newList = append(newList, hashedCode)
	}
	if len(newList) == len(hashedList) {
		return "", ErrInvalidRecoveryCode
	}
	return strings.Join(newList, "|||"), nil
}

func ConstantTimeCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

type PasswordResetAttempt struct {
	Count        int
	FirstAttempt time.Time
}

type PasswordResetRateLimiter struct {
	attempts       map[string]*PasswordResetAttempt
	mu             sync.RWMutex
	maxAttempts    int
	windowDuration time.Duration
}

func NewPasswordResetRateLimiter(maxAttempts int, windowDuration time.Duration) *PasswordResetRateLimiter {
	rl := &PasswordResetRateLimiter{
		attempts:       make(map[string]*PasswordResetAttempt),
		maxAttempts:    maxAttempts,
		windowDuration: windowDuration,
	}
	go rl.cleanup()
	return rl
}

func (rl *PasswordResetRateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for key, attempt := range rl.attempts {
			if now.Sub(attempt.FirstAttempt) > rl.windowDuration*2 {
				delete(rl.attempts, key)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *PasswordResetRateLimiter) RecordRequest(email string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	attempt, exists := rl.attempts[email]
	if !exists {
		rl.attempts[email] = &PasswordResetAttempt{
			Count:        1,
			FirstAttempt: now,
		}
		return true
	}

	if now.Sub(attempt.FirstAttempt) > rl.windowDuration {
		rl.attempts[email] = &PasswordResetAttempt{
			Count:        1,
			FirstAttempt: now,
		}
		return true
	}

	attempt.Count++
	return attempt.Count <= rl.maxAttempts
}

type LoginAttempt struct {
	Count        int
	FirstAttempt time.Time
	LockoutUntil time.Time
}

type LoginAttemptTracker struct {
	attempts        map[string]*LoginAttempt
	mu              sync.RWMutex
	maxAttempts     int
	lockoutDuration time.Duration
	windowDuration  time.Duration
}

func NewLoginAttemptTracker(maxAttempts int, lockoutDuration, windowDuration time.Duration) *LoginAttemptTracker {
	t := &LoginAttemptTracker{
		attempts:        make(map[string]*LoginAttempt),
		maxAttempts:     maxAttempts,
		lockoutDuration: lockoutDuration,
		windowDuration:  windowDuration,
	}
	go t.cleanup()
	return t
}

func (t *LoginAttemptTracker) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		t.mu.Lock()
		now := time.Now()
		for key, attempt := range t.attempts {
			if now.Sub(attempt.FirstAttempt) > t.windowDuration*2 {
				delete(t.attempts, key)
			}
		}
		t.mu.Unlock()
	}
}

func (t *LoginAttemptTracker) RecordFailure(identifier string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	attempt, exists := t.attempts[identifier]
	if !exists {
		t.attempts[identifier] = &LoginAttempt{
			Count:        1,
			FirstAttempt: now,
		}
		return
	}

	if now.Sub(attempt.FirstAttempt) > t.windowDuration {
		t.attempts[identifier] = &LoginAttempt{
			Count:        1,
			FirstAttempt: now,
		}
		return
	}

	attempt.Count++
	if attempt.Count >= t.maxAttempts {
		attempt.LockoutUntil = now.Add(t.lockoutDuration)
	}
}

func (t *LoginAttemptTracker) RecordSuccess(identifier string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.attempts, identifier)
}

func (t *LoginAttemptTracker) IsLockedOut(identifier string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	attempt, exists := t.attempts[identifier]
	if !exists {
		return false
	}

	now := time.Now()
	if now.Before(attempt.LockoutUntil) {
		return true
	}

	if now.Sub(attempt.FirstAttempt) > t.windowDuration {
		return false
	}

	return attempt.Count >= t.maxAttempts
}
