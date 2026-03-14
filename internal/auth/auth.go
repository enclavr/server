package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
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
)

const (
	MinPasswordLength = 8
	RequireUppercase  = true
	RequireLowercase  = true
	RequireNumber     = true
	RequireSpecial    = true
)

type Claims struct {
	UserID   uuid.UUID `json:"user_id"`
	Username string    `json:"username"`
	IsAdmin  bool      `json:"is_admin"`
	jwt.RegisteredClaims
}

type RefreshClaims struct {
	UserID uuid.UUID `json:"user_id"`
	Type   string    `json:"type"`
	jwt.RegisteredClaims
}

type AuthService struct {
	cfg *config.AuthConfig
}

func NewAuthService(cfg *config.AuthConfig) *AuthService {
	return &AuthService{cfg: cfg}
}

func (s *AuthService) HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

func (s *AuthService) CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
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

func (s *AuthService) GenerateToken(user *models.User) (string, error) {
	claims := Claims{
		UserID:   user.ID,
		Username: user.Username,
		IsAdmin:  user.IsAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.cfg.JWTExpiration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "enclavr",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.cfg.JWTSecret))
}

func (s *AuthService) GenerateRefreshToken(user *models.User) (string, error) {
	claims := RefreshClaims{
		UserID: user.ID,
		Type:   "refresh",
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

func ConstantTimeCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func (s *AuthService) ValidateRecoveryCode(storedHash, providedCode string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(providedCode))
	return err == nil
}
