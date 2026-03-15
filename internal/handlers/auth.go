package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
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
	db           *database.Database
	authService  *auth.AuthService
	emailService *services.EmailService
	oauthService *services.OAuthService
	cfg          *config.Config
	firstIsAdmin bool
	loginTracker *auth.LoginAttemptTracker
}

func NewAuthHandler(db *database.Database, authService *auth.AuthService, emailService *services.EmailService, oauthService *services.OAuthService, cfg *config.Config, firstIsAdmin bool, loginTracker *auth.LoginAttemptTracker) *AuthHandler {
	return &AuthHandler{
		db:           db,
		authService:  authService,
		emailService: emailService,
		oauthService: oauthService,
		cfg:          cfg,
		firstIsAdmin: firstIsAdmin,
		loginTracker: loginTracker,
	}
}

type RegisterRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

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
			verifyURL := h.cfg.Server.AllowedOrigins[0] + "/verify-email?token=" + token
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

	if !h.authService.CheckPassword(req.Password, user.PasswordHash) {
		if h.loginTracker != nil {
			h.loginTracker.RecordFailure(identifier)
		}
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	if h.loginTracker != nil {
		h.loginTracker.RecordSuccess(identifier)
	}

	if user.TwoFactorEnabled {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"require_2fa": "true", "user_id": user.ID.String()})
		return
	}

	h.sendAuthResponse(w, &user, r)
}

func (h *AuthHandler) LoginWith2FA(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID string `json:"user_id"`
		Code   string `json:"code"`
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

	if !h.authService.ValidateTwoFactorCode(user.TwoFactorSecret, req.Code) {
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

	redirectURI := h.cfg.Server.AllowedOrigins[0] + "/api/v1/auth/oauth/callback"
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
	redirectURI := h.cfg.Server.AllowedOrigins[0] + "/api/v1/auth/oauth/callback"
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

	h.db.Where("user_id = ? AND token = ?", user.ID, req.RefreshToken).Delete(&models.RefreshToken{})

	h.sendAuthResponse(w, &user, r)
}

func (h *AuthHandler) sendAuthResponse(w http.ResponseWriter, user *models.User, r *http.Request) {
	sessionID := uuid.New()
	accessToken, err := h.authService.GenerateToken(user, sessionID)
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	refreshToken, err := h.authService.GenerateRefreshToken(user)
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
	h.db.Create(&session)

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
			EmailVerified:    user.OIDCIssuer != "",
			TwoFactorEnabled: user.TwoFactorEnabled,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
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
		EmailVerified:    user.OIDCIssuer != "",
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

	h.db.Model(&user).Updates(map[string]interface{}{
		"two_factor_enabled": true,
		"two_factor_secret":  req.Secret,
	})

	h.db.Create(&models.TwoFactorRecovery{
		ID:        uuid.New(),
		UserID:    user.ID,
		Codes:     hashedCodes,
		CreatedAt: time.Now(),
	})

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
		resetURL := h.cfg.Server.AllowedOrigins[0] + "/reset-password?token=" + token
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

	hashedPassword, err := h.authService.HashPassword(req.NewPassword)
	if err != nil {
		http.Error(w, "Failed to hash password", http.StatusInternalServerError)
		return
	}

	var user models.User
	if err := h.db.First(&user, claims.UserID).Error; err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	h.db.Model(&user).Update("password_hash", hashedPassword)
	h.db.Model(&resetToken).Update("used", true)
	h.db.Where("user_id = ?", user.ID).Delete(&models.TwoFactorRecovery{})

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

	verifyURL := h.cfg.Server.AllowedOrigins[0] + "/verify-email?token=" + token

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

	if user.OIDCIssuer != "" {
		http.Error(w, "Email already verified", http.StatusBadRequest)
		return
	}

	user.OIDCIssuer = "verified"
	h.db.Save(&user)

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

	hashedPassword, err := h.authService.HashPassword(req.NewPassword)
	if err != nil {
		http.Error(w, "Failed to hash password", http.StatusInternalServerError)
		return
	}

	h.db.Model(&user).Update("password_hash", hashedPassword)

	h.db.Where("user_id = ?", user.ID).Delete(&models.RefreshToken{})

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

var _ = context.Background
