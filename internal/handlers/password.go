package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"time"

	"github.com/enclavr/server/internal/auth"
	"github.com/enclavr/server/internal/config"
	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
)

type PasswordResetHandler struct {
	db          *database.Database
	authService *auth.AuthService
	cfg         *config.AuthConfig
	email       *config.EmailConfig
}

func NewPasswordResetHandler(db *database.Database, authService *auth.AuthService, cfg *config.AuthConfig, email *config.EmailConfig) *PasswordResetHandler {
	return &PasswordResetHandler{
		db:          db,
		authService: authService,
		cfg:         cfg,
		email:       email,
	}
}

type ForgotPasswordRequest struct {
	Email string `json:"email"`
}

type ResetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

func (h *PasswordResetHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req ForgotPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Email == "" {
		http.Error(w, "Email is required", http.StatusBadRequest)
		return
	}

	var user models.User
	result := h.db.Where("email = ?", req.Email).First(&user)
	if result.Error != nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "If the email exists, a reset link has been sent"})
		return
	}

	tokenBytes := make([]byte, 32)
	_, _ = rand.Read(tokenBytes)
	token := base64.URLEncoding.EncodeToString(tokenBytes)

	reset := models.PasswordReset{
		ID:        uuid.New(),
		UserID:    user.ID,
		Token:     token,
		ExpiresAt: time.Now().Add(1 * time.Hour),
		Used:      false,
		CreatedAt: time.Now(),
	}

	if err := h.db.Create(&reset).Error; err != nil {
		log.Printf("Failed to create password reset: %v", err)
		http.Error(w, "Failed to create reset token", http.StatusInternalServerError)
		return
	}

	go h.sendPasswordResetEmail(user.Email, token)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "If the email exists, a reset link has been sent"})
}

func (h *PasswordResetHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req ResetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Token == "" || req.NewPassword == "" {
		http.Error(w, "Token and new password are required", http.StatusBadRequest)
		return
	}

	if err := h.authService.ValidatePasswordStrength(req.NewPassword); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var reset models.PasswordReset
	result := h.db.Where("token = ? AND used = ? AND expires_at > ?", req.Token, false, time.Now()).First(&reset)
	if result.Error != nil {
		http.Error(w, "Invalid or expired token", http.StatusBadRequest)
		return
	}

	var user models.User
	if err := h.db.First(&user, reset.UserID).Error; err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	hashedPassword, err := h.authService.HashPassword(req.NewPassword)
	if err != nil {
		http.Error(w, "Failed to hash password", http.StatusInternalServerError)
		return
	}

	user.PasswordHash = hashedPassword

	if err := h.db.Save(&user).Error; err != nil {
		http.Error(w, "Failed to update password", http.StatusInternalServerError)
		return
	}

	reset.Used = true
	h.db.Save(&reset)

	h.db.Where("user_id = ?", user.ID).Delete(&models.RefreshToken{})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Password reset successfully"})
}

func (h *PasswordResetHandler) ValidateToken(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "Token is required", http.StatusBadRequest)
		return
	}

	var reset models.PasswordReset
	result := h.db.Where("token = ? AND used = ? AND expires_at > ?", token, false, time.Now()).First(&reset)
	if result.Error != nil {
		http.Error(w, "Invalid or expired token", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"valid": true})
}

func (h *PasswordResetHandler) sendPasswordResetEmail(email, token string) {
	if h.email.SMTPHost == "" {
		log.Printf("SMTP not configured, would send password reset email to %s with token: %s", email, token)
		return
	}

	resetURL := fmt.Sprintf("https://enclavr.local/reset-password?token=%s", token)

	subject := "Password Reset Request"
	body := fmt.Sprintf(`You requested a password reset for your Enclavr account.

If you didn't request this, please ignore this email.

To reset your password, click the link below:
%s

This link will expire in 1 hour.

If the link doesn't work, copy and paste it into your browser.
`, resetURL)

	err := h.sendEmail(email, subject, body)
	if err != nil {
		log.Printf("Failed to send password reset email: %v", err)
	}
}

func (h *PasswordResetHandler) sendEmail(to, subject, body string) error {
	from := h.email.SMTPFrom
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s", from, to, subject, body)

	addr := fmt.Sprintf("%s:%s", h.email.SMTPHost, h.email.SMTPPort)

	var auth smtp.Auth
	if h.email.SMTPUsername != "" {
		auth = smtp.PlainAuth("", h.email.SMTPUsername, h.email.SMTPPassword, h.email.SMTPHost)
	}

	var err error
	if h.email.UseTLS {
		err = smtp.SendMail(addr, auth, from, []string{to}, []byte(msg))
	} else {
		err = smtp.SendMail(addr, auth, from, []string{to}, []byte(msg))
	}

	return err
}

type EmailVerificationHandler struct {
	db    *database.Database
	cfg   *config.AuthConfig
	email *config.EmailConfig
}

func NewEmailVerificationHandler(db *database.Database, cfg *config.AuthConfig, email *config.EmailConfig) *EmailVerificationHandler {
	return &EmailVerificationHandler{
		db:    db,
		cfg:   cfg,
		email: email,
	}
}

type VerifyEmailRequest struct {
	Token string `json:"token"`
}

func (h *EmailVerificationHandler) SendVerification(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	if user.Email == "" {
		http.Error(w, "User has no email", http.StatusBadRequest)
		return
	}

	tokenBytes := make([]byte, 32)
	_, _ = rand.Read(tokenBytes)
	token := base64.URLEncoding.EncodeToString(tokenBytes)

	verification := models.EmailVerification{
		ID:        uuid.New(),
		UserID:    userID,
		Token:     token,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Used:      false,
		CreatedAt: time.Now(),
	}

	if err := h.db.Create(&verification).Error; err != nil {
		http.Error(w, "Failed to create verification", http.StatusInternalServerError)
		return
	}

	go h.sendVerificationEmail(user.Email, token)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Verification email sent"})
}

func (h *EmailVerificationHandler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	var req VerifyEmailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var verification models.EmailVerification
	result := h.db.Where("token = ? AND used = ? AND expires_at > ?", req.Token, false, time.Now()).First(&verification)
	if result.Error != nil {
		http.Error(w, "Invalid or expired token", http.StatusBadRequest)
		return
	}

	var user models.User
	if err := h.db.First(&user, verification.UserID).Error; err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	verification.Used = true
	h.db.Save(&verification)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Email verified successfully"})
}

func (h *EmailVerificationHandler) sendVerificationEmail(email, token string) {
	if h.email.SMTPHost == "" {
		log.Printf("SMTP not configured, would send verification email to %s with token: %s", email, token)
		return
	}

	verifyURL := fmt.Sprintf("https://enclavr.local/verify-email?token=%s", token)

	subject := "Verify your email"
	body := fmt.Sprintf(`Welcome to Enclavr!

Please verify your email address by clicking the link below:
%s

This link will expire in 24 hours.
`, verifyURL)

	err := h.sendEmail(email, subject, body)
	if err != nil {
		log.Printf("Failed to send verification email: %v", err)
	}
}

func (h *EmailVerificationHandler) sendEmail(to, subject, body string) error {
	from := h.email.SMTPFrom
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s", from, to, subject, body)

	addr := fmt.Sprintf("%s:%s", h.email.SMTPHost, h.email.SMTPPort)

	var auth smtp.Auth
	if h.email.SMTPUsername != "" {
		auth = smtp.PlainAuth("", h.email.SMTPUsername, h.email.SMTPPassword, h.email.SMTPHost)
	}

	return smtp.SendMail(addr, auth, from, []string{to}, []byte(msg))
}

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

func (h *PasswordResetHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.OldPassword == "" || req.NewPassword == "" {
		http.Error(w, "Old and new passwords are required", http.StatusBadRequest)
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

	if user.PasswordHash != "" {
		if !h.authService.CheckPassword(req.OldPassword, user.PasswordHash) {
			http.Error(w, "Current password is incorrect", http.StatusUnauthorized)
			return
		}
	}

	hashedPassword, err := h.authService.HashPassword(req.NewPassword)
	if err != nil {
		http.Error(w, "Failed to hash password", http.StatusInternalServerError)
		return
	}

	user.PasswordHash = hashedPassword

	if err := h.db.Save(&user).Error; err != nil {
		http.Error(w, "Failed to update password", http.StatusInternalServerError)
		return
	}

	h.db.Where("user_id = ?", user.ID).Delete(&models.RefreshToken{})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Password changed successfully"})
}
