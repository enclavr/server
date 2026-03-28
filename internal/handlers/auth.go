package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/enclavr/server/internal/auth"
	"github.com/enclavr/server/internal/config"
	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/internal/services"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AuthHandler struct {
	db                   *database.Database
	authService          *auth.AuthService
	emailService         *services.EmailService
	oauthService         *services.OAuthService
	cfg                  *config.Config
	firstIsAdmin         bool
	loginTracker         *auth.LoginAttemptTracker
	passwordResetLimiter *auth.PasswordResetRateLimiter
}

func NewAuthHandler(db *database.Database, authService *auth.AuthService, emailService *services.EmailService, oauthService *services.OAuthService, cfg *config.Config, firstIsAdmin bool, loginTracker *auth.LoginAttemptTracker) *AuthHandler {
	return &AuthHandler{
		db:                   db,
		authService:          authService,
		emailService:         emailService,
		oauthService:         oauthService,
		cfg:                  cfg,
		firstIsAdmin:         firstIsAdmin,
		loginTracker:         loginTracker,
		passwordResetLimiter: auth.NewPasswordResetRateLimiter(3, 1*time.Hour),
	}
}

type RegisterRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

const (
	MaxFailedLoginAttempts = 5
	AccountLockoutDuration = 15 * time.Minute
)

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type AuthResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	ExpiresIn    int64        `json:"expires_in"`
	User         UserResponse `json:"user"`
}

type UserResponse struct {
	ID               uuid.UUID `json:"id"`
	Username         string    `json:"username"`
	Email            string    `json:"email"`
	DisplayName      string    `json:"display_name"`
	AvatarURL        string    `json:"avatar_url"`
	IsAdmin          bool      `json:"is_admin"`
	EmailVerified    bool      `json:"email_verified"`
	TwoFactorEnabled bool      `json:"two_factor_enabled"`
	PasswordExpired  bool      `json:"password_expired"`
}

type OAuthBeginRequest struct {
	Provider string `json:"provider"`
}

type OAuthCallbackRequest struct {
	Provider    string `json:"provider"`
	Code        string `json:"code"`
	State       string `json:"state"`
	RedirectURI string `json:"redirect_uri"`
}

type TwoFASetupResponse struct {
	Secret    string `json:"secret"`
	QRCodeURL string `json:"qr_code_url"`
}

type TwoFAVerifyRequest struct {
	Code string `json:"code"`
}

type PasswordResetRequest struct {
	Email string `json:"email"`
}

type PasswordResetCompleteRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

type EmailVerificationRequest struct {
	Token string `json:"token"`
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Username == "" || req.Password == "" || req.Email == "" {
		http.Error(w, "Username, email and password are required", http.StatusBadRequest)
		return
	}

	if err := h.authService.ValidatePasswordStrength(req.Password); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	hashedPassword, err := h.authService.HashPassword(req.Password)
	if err != nil {
		http.Error(w, "Failed to hash password", http.StatusInternalServerError)
		return
	}

	user := models.User{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: hashedPassword,
		DisplayName:  req.Username,
	}

	if h.firstIsAdmin {
		var userCount int64
		h.db.Model(&models.User{}).Count(&userCount)
		if userCount == 0 {
			user.IsAdmin = true
		}
	}

	if err := h.db.Create(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			http.Error(w, "Username or email already exists", http.StatusConflict)
			return
		}
		http.Error(w, "Failed to create user", http.StatusInternalServerError)
		return
	}

	if h.emailService != nil && h.emailService.IsEnabled() {
		token, err := h.authService.GenerateEmailVerificationToken(user.ID)
		if err == nil {
			verifyURL := h.cfg.Server.GetBaseURL() + "/verify-email?token=" + token
			_ = h.emailService.SendEmailVerificationEmail(r.Context(), services.EmailRecipient{
				To:   user.Email,
				Name: user.DisplayName,
			}, verifyURL)
		}
	}

	h.sendAuthResponse(w, &user, r)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Username == "" || req.Password == "" {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	identifier := req.Username
	if h.loginTracker != nil && h.loginTracker.IsLockedOut(identifier) {
		http.Error(w, "Too many login attempts, please try again later", http.StatusTooManyRequests)
		return
	}

	var user models.User
	if err := h.db.Where("username = ? OR email = ?", req.Username, req.Username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			h.authService.CheckPassword(req.Password, "")
			if h.loginTracker != nil {
				h.loginTracker.RecordFailure(identifier)
			}
			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
			return
		}
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	if user.LockedUntil != nil && time.Now().Before(*user.LockedUntil) {
		http.Error(w, "Account is temporarily locked due to too many failed login attempts", http.StatusTooManyRequests)
		return
	}

	if !h.authService.CheckPassword(req.Password, user.PasswordHash) {
		failedCount := user.FailedLoginCount + 1
		updates := map[string]interface{}{
			"failed_login_count": failedCount,
		}

		if failedCount >= MaxFailedLoginAttempts {
			lockedUntil := time.Now().Add(AccountLockoutDuration)
			updates["locked_until"] = lockedUntil
		}

		h.db.Model(&user).Updates(updates)

		if h.loginTracker != nil {
			h.loginTracker.RecordFailure(identifier)
		}
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	if user.FailedLoginCount > 0 || user.LockedUntil != nil {
		h.db.Model(&user).Updates(map[string]interface{}{
			"failed_login_count": 0,
			"locked_until":       nil,
		})
	}

	if h.loginTracker != nil {
		h.loginTracker.RecordSuccess(identifier)
	}

	if user.TwoFactorEnabled {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"require_2fa": "true", "user_id": user.ID.String()})
		return
	}

	if user.PasswordHash != "" && h.authService.IsPasswordExpired(user.PasswordChangedAt) {
		h.sendAuthResponseWithPasswordExpiry(w, &user, r)
		return
	}

	h.sendAuthResponse(w, &user, r)
}

func (h *AuthHandler) LoginWith2FA(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID     string `json:"user_id"`
		Code       string `json:"code"`
		IsRecovery bool   `json:"is_recovery"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		http.Error(w, "Invalid user id", http.StatusBadRequest)
		return
	}

	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	if req.IsRecovery {
		var recovery models.TwoFactorRecovery
		if err := h.db.Where("user_id = ?", userID).First(&recovery).Error; err != nil {
			http.Error(w, "Invalid recovery code", http.StatusUnauthorized)
			return
		}

		if !h.authService.ValidateRecoveryCode(recovery.Codes, req.Code) {
			http.Error(w, "Invalid recovery code", http.StatusUnauthorized)
			return
		}

		newCodes, err := h.authService.RemoveUsedRecoveryCode(recovery.Codes, req.Code)
		if err != nil {
			http.Error(w, "Failed to update recovery codes", http.StatusInternalServerError)
			return
		}

		h.db.Model(&recovery).Update("codes", newCodes)

		h.sendAuthResponse(w, &user, r)
		return
	}

	secret := user.TwoFactorSecret
	if h.authService.HasEncryptor() {
		decrypted, err := h.authService.DecryptSecret(user.TwoFactorSecret)
		if err != nil {
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
		secret = decrypted
	}

	if !h.authService.ValidateTwoFactorCode(secret, req.Code) {
		http.Error(w, "Invalid 2FA code", http.StatusUnauthorized)
		return
	}

	h.sendAuthResponse(w, &user, r)
}

func (h *AuthHandler) OAuthBegin(w http.ResponseWriter, r *http.Request) {
	if h.oauthService == nil || !h.oauthService.IsEnabled() {
		http.Error(w, "OAuth not enabled", http.StatusNotFound)
		return
	}

	var req OAuthBeginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	provider := services.OAuthProvider(req.Provider)
	if !h.oauthService.IsProviderEnabled(provider) {
		http.Error(w, "Provider not enabled", http.StatusBadRequest)
		return
	}

	state, err := auth.GenerateState()
	if err != nil {
		http.Error(w, "Failed to generate state", http.StatusInternalServerError)
		return
	}

	redirectURI := h.cfg.Server.GetBaseURL() + "/api/v1/auth/oauth/callback"
	authURL, err := h.oauthService.GetAuthURL(provider, state, redirectURI)
	if err != nil {
		http.Error(w, "Failed to get auth URL", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"auth_url": authURL,
		"state":    state,
	})
}

func (h *AuthHandler) OAuthCallback(w http.ResponseWriter, r *http.Request) {
	if h.oauthService == nil || !h.oauthService.IsEnabled() {
		http.Error(w, "OAuth not enabled", http.StatusNotFound)
		return
	}

	var req OAuthCallbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	provider := services.OAuthProvider(req.Provider)
	redirectURI := h.cfg.Server.GetBaseURL() + "/api/v1/auth/oauth/callback"
	token, err := h.oauthService.ExchangeCode(r.Context(), provider, req.Code, redirectURI)
	if err != nil {
		http.Error(w, "Failed to exchange code: "+err.Error(), http.StatusBadRequest)
		return
	}

	userInfo, err := h.oauthService.GetUserInfo(r.Context(), provider, token)
	if err != nil {
		http.Error(w, "Failed to get user info: "+err.Error(), http.StatusBadRequest)
		return
	}

	user, err := h.oauthService.FindOrCreateUser(h.db.DB, userInfo)
	if err != nil {
		http.Error(w, "Failed to find or create user", http.StatusInternalServerError)
		return
	}

	h.sendAuthResponse(w, user, r)
}

func (h *AuthHandler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	var req RefreshTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	claims, err := h.authService.ValidateRefreshToken(req.RefreshToken)
	if err != nil {
		http.Error(w, "Invalid refresh token", http.StatusUnauthorized)
		return
	}

	var user models.User
	if err := h.db.First(&user, claims.UserID).Error; err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	var existingToken models.RefreshToken
	if err := h.db.Where("user_id = ? AND token = ?", user.ID, req.RefreshToken).First(&existingToken).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Token reuse detected - revoke all tokens for this user since
			// we cannot determine the token family of the reused token
			h.db.Where("user_id = ?", user.ID).Delete(&models.RefreshToken{})
			http.Error(w, "Token reuse detected, sessions revoked", http.StatusUnauthorized)
			return
		}
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	h.db.Where("user_id = ? AND token = ?", user.ID, req.RefreshToken).Delete(&models.RefreshToken{})
	if existingToken.TokenFamily != "" {
		h.db.Where("user_id = ? AND token_family = ?", user.ID, existingToken.TokenFamily).Delete(&models.RefreshToken{})
	}

	h.sendAuthResponseWithRotation(w, &user, r)
}

func (h *AuthHandler) sendAuthResponse(w http.ResponseWriter, user *models.User, r *http.Request) {
	h.sendAuthResponseWithRotation(w, user, r)
}

func (h *AuthHandler) sendAuthResponseWithRotation(w http.ResponseWriter, user *models.User, r *http.Request) {
	sessionID := uuid.New()
	accessToken, err := h.authService.GenerateToken(user, sessionID)
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	tokenFamily, err := auth.GenerateTokenFamily()
	if err != nil {
		http.Error(w, "Failed to generate token family", http.StatusInternalServerError)
		return
	}

	refreshToken, err := h.authService.GenerateRefreshTokenWithFamily(user, tokenFamily)
	if err != nil {
		http.Error(w, "Failed to generate refresh token", http.StatusInternalServerError)
		return
	}

	var existingSessionCount int64
	h.db.Model(&models.Session{}).Where("user_id = ? AND user_agent = ? AND expires_at > ?", user.ID, r.UserAgent(), time.Now()).Count(&existingSessionCount)

	session := models.Session{
		ID:        sessionID,
		UserID:    user.ID,
		Token:     accessToken,
		ExpiresAt: time.Now().Add(h.cfg.Auth.JWTExpiration),
		CreatedAt: time.Now(),
		IPAddress: r.RemoteAddr,
		UserAgent: r.UserAgent(),
	}
	if err := h.db.Create(&session).Error; err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	dbRefreshToken := models.RefreshToken{
		ID:          uuid.New(),
		UserID:      user.ID,
		Token:       refreshToken,
		TokenFamily: tokenFamily,
		ExpiresAt:   time.Now().Add(h.cfg.Auth.RefreshExpiration),
		CreatedAt:   time.Now(),
	}
	if err := h.db.Create(&dbRefreshToken).Error; err != nil {
		http.Error(w, "Failed to create refresh token", http.StatusInternalServerError)
		return
	}

	if existingSessionCount == 0 && h.emailService != nil && h.emailService.IsEnabled() {
		go func() {
			_ = h.emailService.SendNewDeviceLoginEmail(
				context.Background(),
				services.EmailRecipient{To: user.Email, Name: user.DisplayName},
				extractDeviceName(r.UserAgent()),
				r.RemoteAddr,
				"Unknown",
				time.Now().Format("January 2, 2006 at 3:04 PM"),
			)
		}()
	}

	response := AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(24 * time.Hour.Seconds()),
		User: UserResponse{
			ID:               user.ID,
			Username:         user.Username,
			Email:            user.Email,
			DisplayName:      user.DisplayName,
			AvatarURL:        user.AvatarURL,
			IsAdmin:          user.IsAdmin,
			EmailVerified:    user.EmailVerified,
			TwoFactorEnabled: user.TwoFactorEnabled,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *AuthHandler) sendAuthResponseWithPasswordExpiry(w http.ResponseWriter, user *models.User, r *http.Request) {
	sessionID := uuid.New()
	accessToken, err := h.authService.GenerateToken(user, sessionID)
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	tokenFamily, err := auth.GenerateTokenFamily()
	if err != nil {
		http.Error(w, "Failed to generate token family", http.StatusInternalServerError)
		return
	}

	refreshToken, err := h.authService.GenerateRefreshTokenWithFamily(user, tokenFamily)
	if err != nil {
		http.Error(w, "Failed to generate refresh token", http.StatusInternalServerError)
		return
	}

	session := models.Session{
		ID:        sessionID,
		UserID:    user.ID,
		Token:     accessToken,
		ExpiresAt: time.Now().Add(h.cfg.Auth.JWTExpiration),
		CreatedAt: time.Now(),
		IPAddress: r.RemoteAddr,
		UserAgent: r.UserAgent(),
	}
	if err := h.db.Create(&session).Error; err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	dbRefreshToken := models.RefreshToken{
		ID:          uuid.New(),
		UserID:      user.ID,
		Token:       refreshToken,
		TokenFamily: tokenFamily,
		ExpiresAt:   time.Now().Add(h.cfg.Auth.RefreshExpiration),
		CreatedAt:   time.Now(),
	}
	if err := h.db.Create(&dbRefreshToken).Error; err != nil {
		http.Error(w, "Failed to create refresh token", http.StatusInternalServerError)
		return
	}

	response := AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(24 * time.Hour.Seconds()),
		User: UserResponse{
			ID:               user.ID,
			Username:         user.Username,
			Email:            user.Email,
			DisplayName:      user.DisplayName,
			AvatarURL:        user.AvatarURL,
			IsAdmin:          user.IsAdmin,
			EmailVerified:    user.EmailVerified,
			TwoFactorEnabled: user.TwoFactorEnabled,
			PasswordExpired:  true,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func extractDeviceName(userAgent string) string {
	if userAgent == "" {
		return "Unknown Device"
	}
	switch {
	case contains(userAgent, "Chrome"):
		return "Chrome Browser"
	case contains(userAgent, "Firefox"):
		return "Firefox Browser"
	case contains(userAgent, "Safari"):
		return "Safari Browser"
	case contains(userAgent, "Edge"):
		return "Edge Browser"
	case contains(userAgent, "Mobile"):
		return "Mobile Device"
	default:
		return "Unknown Device"
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

func (h *AuthHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	response := UserResponse{
		ID:               user.ID,
		Username:         user.Username,
		Email:            user.Email,
		DisplayName:      user.DisplayName,
		AvatarURL:        user.AvatarURL,
		IsAdmin:          user.IsAdmin,
		EmailVerified:    user.EmailVerified,
		TwoFactorEnabled: user.TwoFactorEnabled,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *AuthHandler) EnableTwoFactor(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	if user.TwoFactorEnabled {
		http.Error(w, "2FA already enabled", http.StatusBadRequest)
		return
	}

	secret, err := h.authService.GenerateTwoFactorSecret()
	if err != nil {
		http.Error(w, "Failed to generate secret", http.StatusInternalServerError)
		return
	}

	qrCodeURL := "otpauth://totp/Enclavr:" + user.Email + "?secret=" + secret + "&issuer=Enclavr"

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(TwoFASetupResponse{
		Secret:    secret,
		QRCodeURL: qrCodeURL,
	})
}

func (h *AuthHandler) ConfirmTwoFactor(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Secret string `json:"secret"`
		Code   string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	if !h.authService.ValidateTwoFactorCode(req.Secret, req.Code) {
		http.Error(w, "Invalid 2FA code", http.StatusBadRequest)
		return
	}

	recoveryCodes, err := h.authService.GenerateRecoveryCodes()
	if err != nil {
		http.Error(w, "Failed to generate recovery codes", http.StatusInternalServerError)
		return
	}

	hashedCodes, err := h.authService.HashRecoveryCodes(recoveryCodes)
	if err != nil {
		http.Error(w, "Failed to hash recovery codes", http.StatusInternalServerError)
		return
	}

	secretToStore := req.Secret
	if h.authService.HasEncryptor() {
		encrypted, err := h.authService.EncryptSecret(req.Secret)
		if err != nil {
			http.Error(w, "Failed to encrypt secret", http.StatusInternalServerError)
			return
		}
		secretToStore = encrypted
	}

	h.db.Model(&user).Updates(map[string]interface{}{
		"two_factor_enabled": true,
		"two_factor_secret":  secretToStore,
	})

	if err := h.db.Create(&models.TwoFactorRecovery{
		ID:        uuid.New(),
		UserID:    user.ID,
		Codes:     hashedCodes,
		CreatedAt: time.Now(),
	}).Error; err != nil {
		http.Error(w, "Failed to store recovery codes", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":        true,
		"recovery_codes": recoveryCodes,
	})
}

func (h *AuthHandler) DisableTwoFactor(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	if !h.authService.CheckPassword(req.Password, user.PasswordHash) {
		http.Error(w, "Invalid password", http.StatusUnauthorized)
		return
	}

	h.db.Model(&user).Updates(map[string]interface{}{
		"two_factor_enabled": false,
		"two_factor_secret":  "",
	})

	h.db.Where("user_id = ?", user.ID).Delete(&models.TwoFactorRecovery{})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})
}

func (h *AuthHandler) RequestPasswordReset(w http.ResponseWriter, r *http.Request) {
	var req PasswordResetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if h.passwordResetLimiter != nil && !h.passwordResetLimiter.RecordRequest(req.Email) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"message": "Too many reset requests, please try again later",
		})
		return
	}

	var user models.User
	if err := h.db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"message": "If the email exists, a reset link will be sent",
			})
			return
		}
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	token, err := h.authService.GeneratePasswordResetToken(user.ID)
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	resetToken := models.PasswordReset{
		ID:        uuid.New(),
		UserID:    user.ID,
		Token:     token,
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	h.db.Create(&resetToken)

	if h.emailService != nil && h.emailService.IsEnabled() {
		resetURL := h.cfg.Server.GetBaseURL() + "/reset-password?token=" + token
		_ = h.emailService.SendPasswordResetEmail(r.Context(), services.EmailRecipient{
			To:   user.Email,
			Name: user.DisplayName,
		}, resetURL)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "If the email exists, a reset link will be sent",
	})
}

func (h *AuthHandler) CompletePasswordReset(w http.ResponseWriter, r *http.Request) {
	var req PasswordResetCompleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	claims, err := h.authService.ValidatePasswordResetToken(req.Token)
	if err != nil {
		http.Error(w, "Invalid or expired token", http.StatusBadRequest)
		return
	}

	var resetToken models.PasswordReset
	if err := h.db.Where("user_id = ? AND token = ? AND used = ?", claims.UserID, req.Token, false).First(&resetToken).Error; err != nil {
		http.Error(w, "Invalid token", http.StatusBadRequest)
		return
	}

	if time.Now().After(resetToken.ExpiresAt) {
		http.Error(w, "Token expired", http.StatusBadRequest)
		return
	}

	if err := h.authService.ValidatePasswordStrength(req.NewPassword); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var user models.User
	if err := h.db.First(&user, claims.UserID).Error; err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	var passwordHistory []models.PasswordHistory
	h.db.Where("user_id = ?", user.ID).Order("created_at DESC").Limit(5).Find(&passwordHistory)
	historyHashes := make([]string, len(passwordHistory))
	for i, ph := range passwordHistory {
		historyHashes[i] = ph.PasswordHash
	}

	if err := h.authService.CheckPasswordHistory(req.NewPassword, historyHashes); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	oldPasswordHash := user.PasswordHash

	hashedPassword, err := h.authService.HashPassword(req.NewPassword)
	if err != nil {
		http.Error(w, "Failed to hash password", http.StatusInternalServerError)
		return
	}

	if oldPasswordHash != "" {
		historyEntry := models.PasswordHistory{
			ID:           uuid.New(),
			UserID:       user.ID,
			PasswordHash: oldPasswordHash,
		}
		if err := h.db.Create(&historyEntry).Error; err != nil {
			log.Printf("Error saving password history: %v", err)
		}
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&user).Update("password_hash", hashedPassword).Error; err != nil {
			return err
		}
		if err := tx.Model(&user).Update("password_changed_at", time.Now()).Error; err != nil {
			return err
		}
		if err := tx.Model(&resetToken).Update("used", true).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", user.ID).Delete(&models.TwoFactorRecovery{}).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		http.Error(w, "Failed to complete password reset", http.StatusInternalServerError)
		return
	}

	if h.emailService != nil && h.emailService.IsEnabled() {
		_ = h.emailService.SendPasswordChangedEmail(r.Context(), services.EmailRecipient{
			To:   user.Email,
			Name: user.DisplayName,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Password reset successful",
	})
}

func (h *AuthHandler) RequestEmailVerification(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	if user.OIDCIssuer != "" {
		http.Error(w, "Email already verified", http.StatusBadRequest)
		return
	}

	token, err := h.authService.GenerateEmailVerificationToken(user.ID)
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	verifyURL := h.cfg.Server.GetBaseURL() + "/verify-email?token=" + token

	if h.emailService != nil && h.emailService.IsEnabled() {
		_ = h.emailService.SendEmailVerificationEmail(r.Context(), services.EmailRecipient{
			To:   user.Email,
			Name: user.DisplayName,
		}, verifyURL)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Verification email sent",
	})
}

func (h *AuthHandler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	var req EmailVerificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	claims, err := h.authService.ValidateEmailVerificationToken(req.Token)
	if err != nil {
		http.Error(w, "Invalid or expired token", http.StatusBadRequest)
		return
	}

	var user models.User
	if err := h.db.First(&user, claims.UserID).Error; err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	if user.EmailVerified {
		http.Error(w, "Email already verified", http.StatusBadRequest)
		return
	}

	user.EmailVerified = true
	if err := h.db.Save(&user).Error; err != nil {
		http.Error(w, "Failed to verify email", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Email verified successfully",
	})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err == nil && req.RefreshToken != "" {
		h.db.Where("user_id = ? AND token = ?", userID, req.RefreshToken).Delete(&models.RefreshToken{})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})
}

func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.authService.ValidatePasswordStrength(req.NewPassword); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	if !h.authService.CheckPassword(req.CurrentPassword, user.PasswordHash) {
		http.Error(w, "Current password is incorrect", http.StatusUnauthorized)
		return
	}

	var passwordHistory []models.PasswordHistory
	h.db.Where("user_id = ?", user.ID).Order("created_at DESC").Limit(5).Find(&passwordHistory)
	historyHashes := make([]string, len(passwordHistory))
	for i, ph := range passwordHistory {
		historyHashes[i] = ph.PasswordHash
	}

	if err := h.authService.CheckPasswordHistory(req.NewPassword, historyHashes); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	oldPasswordHash := user.PasswordHash

	hashedPassword, err := h.authService.HashPassword(req.NewPassword)
	if err != nil {
		http.Error(w, "Failed to hash password", http.StatusInternalServerError)
		return
	}

	if oldPasswordHash != "" {
		historyEntry := models.PasswordHistory{
			ID:           uuid.New(),
			UserID:       user.ID,
			PasswordHash: oldPasswordHash,
		}
		h.db.Create(&historyEntry)

		var count int64
		h.db.Model(&models.PasswordHistory{}).Where("user_id = ?", user.ID).Count(&count)
		if count > 5 {
			h.db.Model(&models.PasswordHistory{}).
				Where("user_id = ? AND id NOT IN (SELECT id FROM password_history WHERE user_id = ? ORDER BY created_at DESC LIMIT 5)", user.ID, user.ID).
				Delete(&models.PasswordHistory{})
		}
	}

	h.db.Model(&user).Update("password_hash", hashedPassword)
	h.db.Model(&user).Update("password_changed_at", time.Now())

	h.db.Where("user_id = ?", user.ID).Delete(&models.RefreshToken{})
	h.db.Where("user_id = ?", user.ID).Delete(&models.PasswordReset{})

	if h.emailService != nil && h.emailService.IsEnabled() {
		_ = h.emailService.SendPasswordChangedEmail(r.Context(), services.EmailRecipient{
			To:   user.Email,
			Name: user.DisplayName,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Password changed successfully, all sessions have been invalidated",
	})
}

func (h *AuthHandler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Password == "" {
		http.Error(w, "Password is required to delete account", http.StatusBadRequest)
		return
	}

	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	if user.OIDCSubject == "" {
		if !h.authService.CheckPassword(req.Password, user.PasswordHash) {
			http.Error(w, "Invalid password", http.StatusUnauthorized)
			return
		}
	}

	tx := h.db.Begin()

	if err := tx.Where("user_id = ?", userID).Delete(&models.Session{}).Error; err != nil {
		tx.Rollback()
		http.Error(w, "Failed to delete sessions", http.StatusInternalServerError)
		return
	}

	if err := tx.Where("user_id = ?", userID).Delete(&models.RefreshToken{}).Error; err != nil {
		tx.Rollback()
		http.Error(w, "Failed to delete refresh tokens", http.StatusInternalServerError)
		return
	}

	if err := tx.Where("user_id = ?", userID).Delete(&models.PasswordHistory{}).Error; err != nil {
		tx.Rollback()
		http.Error(w, "Failed to delete password history", http.StatusInternalServerError)
		return
	}

	if err := tx.Where("user_id = ?", userID).Delete(&models.TwoFactorRecovery{}).Error; err != nil {
		tx.Rollback()
		http.Error(w, "Failed to delete 2FA recovery", http.StatusInternalServerError)
		return
	}

	if err := tx.Delete(&user).Error; err != nil {
		tx.Rollback()
		http.Error(w, "Failed to delete account", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit().Error; err != nil {
		http.Error(w, "Failed to commit account deletion", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Account deleted successfully",
	})
}

type SessionInfo struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	IPAddress string    `json:"ip_address"`
	UserAgent string    `json:"user_agent"`
	Current   bool      `json:"current"`
}

func (h *AuthHandler) GetSessions(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	sessionID, _ := r.Context().Value(middleware.SessionIDKey).(uuid.UUID)

	var sessions []models.Session
	h.db.Where("user_id = ? AND expires_at > ?", userID, time.Now()).Order("created_at DESC").Find(&sessions)

	sessionList := make([]SessionInfo, len(sessions))
	for i, s := range sessions {
		sessionList[i] = SessionInfo{
			ID:        s.ID,
			CreatedAt: s.CreatedAt,
			ExpiresAt: s.ExpiresAt,
			IPAddress: s.IPAddress,
			UserAgent: s.UserAgent,
			Current:   s.ID == sessionID,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"sessions": sessionList,
		"count":    len(sessionList),
	})
}

func (h *AuthHandler) RevokeSession(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	sessionID, _ := r.Context().Value(middleware.SessionIDKey).(uuid.UUID)

	sessionIDStr := r.URL.Query().Get("id")
	if sessionIDStr == "" {
		http.Error(w, "Session ID required", http.StatusBadRequest)
		return
	}

	revokeSessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		http.Error(w, "Invalid session ID", http.StatusBadRequest)
		return
	}

	if revokeSessionID == sessionID {
		http.Error(w, "Cannot revoke current session", http.StatusBadRequest)
		return
	}

	result := h.db.Where("id = ? AND user_id = ?", revokeSessionID, userID).Delete(&models.Session{})
	if result.Error != nil {
		http.Error(w, "Failed to revoke session", http.StatusInternalServerError)
		return
	}

	if result.RowsAffected == 0 {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Session revoked successfully",
	})
}

func (h *AuthHandler) RevokeAllSessions(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	sessionID, _ := r.Context().Value(middleware.SessionIDKey).(uuid.UUID)

	h.db.Where("user_id = ? AND id != ?", userID, sessionID).Delete(&models.Session{})
	h.db.Where("user_id = ? AND id != ?", userID, sessionID).Delete(&models.RefreshToken{})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "All other sessions revoked successfully",
	})
}

func (h *AuthHandler) GetActiveSessionsCount(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var count int64
	h.db.Model(&models.Session{}).Where("user_id = ? AND expires_at > ?", userID, time.Now()).Count(&count)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"active_sessions": count,
	})
}

func (h *AuthHandler) RotateToken(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	sessionID := uuid.New()
	accessToken, err := h.authService.GenerateToken(&user, sessionID)
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	refreshToken, err := h.authService.GenerateRefreshToken(&user)
	if err != nil {
		http.Error(w, "Failed to generate refresh token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"expires_in":    int64(24 * time.Hour.Seconds()),
	})
}

var _ = context.Background
