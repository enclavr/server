package handlers

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/enclavr/server/internal/auth"
	"github.com/enclavr/server/internal/config"
	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

type TwoFactorHandler struct {
	db          *database.Database
	authService *auth.AuthService
	cfg         *config.AuthConfig
}

func NewTwoFactorHandler(db *database.Database, authService *auth.AuthService, cfg *config.AuthConfig) *TwoFactorHandler {
	return &TwoFactorHandler{
		db:          db,
		authService: authService,
		cfg:         cfg,
	}
}

type SetupTwoFactorResponse struct {
	Secret        string   `json:"secret"`
	QRCodeURL     string   `json:"qr_code_url"`
	RecoveryCodes []string `json:"recovery_codes"`
}

func (h *TwoFactorHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
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

	response := map[string]interface{}{
		"enabled": user.TwoFactorEnabled,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func (h *TwoFactorHandler) Setup(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "Two-factor authentication already enabled", http.StatusBadRequest)
		return
	}

	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "Enclavr",
		AccountName: user.Email,
		Algorithm:   otp.AlgorithmSHA1,
		Digits:      otp.DigitsSix,
		Period:      30,
	})
	if err != nil {
		log.Printf("[ERROR] Failed to generate TOTP secret: %v", err)
		http.Error(w, "Failed to generate secret", http.StatusInternalServerError)
		return
	}

	recoveryCodes := generateRecoveryCodes(10)

	hashedRecoveryCodes := make([]string, len(recoveryCodes))
	for i, code := range recoveryCodes {
		hashed, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
		if err != nil {
			http.Error(w, "Failed to hash recovery codes", http.StatusInternalServerError)
			return
		}
		hashedRecoveryCodes[i] = string(hashed)
	}

	user.TwoFactorSecret = key.Secret()
	user.TwoFactorEnabled = false

	if err := h.db.Save(&user).Error; err != nil {
		http.Error(w, "Failed to save user", http.StatusInternalServerError)
		return
	}

	var recoveryCodesData string
	for i, code := range recoveryCodes {
		if i > 0 {
			recoveryCodesData += ","
		}
		recoveryCodesData += fmt.Sprintf(`{"code":"%s","hashed":"%s"}`, code, hashedRecoveryCodes[i])
	}

	recovery := models.TwoFactorRecovery{
		ID:        uuid.New(),
		UserID:    userID,
		Codes:     recoveryCodesData,
		CreatedAt: time.Now(),
	}

	if err := h.db.Create(&recovery).Error; err != nil {
		log.Printf("Failed to save recovery codes: %v", err)
	}

	response := SetupTwoFactorResponse{
		Secret:        key.Secret(),
		QRCodeURL:     key.URL(),
		RecoveryCodes: recoveryCodes,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func (h *TwoFactorHandler) Enable(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Code string `json:"code"`
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

	if user.TwoFactorEnabled {
		http.Error(w, "Two-factor authentication already enabled", http.StatusBadRequest)
		return
	}

	valid := totp.Validate(req.Code, user.TwoFactorSecret)
	if !valid {
		http.Error(w, "Invalid verification code", http.StatusBadRequest)
		return
	}

	user.TwoFactorEnabled = true

	if err := h.db.Save(&user).Error; err != nil {
		http.Error(w, "Failed to enable two-factor authentication", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"enabled": true})
}

func (h *TwoFactorHandler) Disable(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Code string `json:"code"`
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

	if !user.TwoFactorEnabled {
		http.Error(w, "Two-factor authentication not enabled", http.StatusBadRequest)
		return
	}

	valid := totp.Validate(req.Code, user.TwoFactorSecret)
	if !valid {
		http.Error(w, "Invalid verification code", http.StatusBadRequest)
		return
	}

	user.TwoFactorEnabled = false
	user.TwoFactorSecret = ""

	if err := h.db.Save(&user).Error; err != nil {
		http.Error(w, "Failed to disable two-factor authentication", http.StatusInternalServerError)
		return
	}

	h.db.Where("user_id = ?", userID).Delete(&models.TwoFactorRecovery{})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"enabled": false})
}

func (h *TwoFactorHandler) Verify(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	if !user.TwoFactorEnabled {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"valid": true})
		return
	}

	isValid := totp.Validate(req.Code, user.TwoFactorSecret)

	if !isValid {
		var recovery models.TwoFactorRecovery
		if err := h.db.Where("user_id = ?", userID).First(&recovery).Error; err == nil {
			isValid = h.authService.ValidateRecoveryCode(recovery.Codes, req.Code)
			if isValid {
				newCodes, err := h.authService.RemoveUsedRecoveryCode(recovery.Codes, req.Code)
				if err == nil {
					h.db.Model(&recovery).Update("codes", newCodes)
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"valid": isValid})
}

func (h *TwoFactorHandler) GetRecoveryCodes(w http.ResponseWriter, r *http.Request) {
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

	if !user.TwoFactorEnabled {
		http.Error(w, "Two-factor authentication not enabled", http.StatusBadRequest)
		return
	}

	var recovery models.TwoFactorRecovery
	if err := h.db.Where("user_id = ?", userID).First(&recovery).Error; err != nil {
		http.Error(w, "Recovery codes not found", http.StatusNotFound)
		return
	}

	if strings.Contains(recovery.Codes, ",") && !strings.Contains(recovery.Codes, "|||") {
		codes := parseRecoveryCodes(recovery.Codes)
		var codeList []string
		for _, c := range codes {
			codeList = append(codeList, c.Code)
		}
		hashedCodes, _ := h.authService.HashRecoveryCodes(codeList)
		h.db.Model(&recovery).Update("codes", hashedCodes)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"codes": codeList,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"message": "Recovery codes were provided during 2FA setup. Store them securely - they cannot be retrieved again.",
	})
}

func generateRecoveryCodes(count int) []string {
	codes := make([]string, count)
	for i := 0; i < count; i++ {
		bytes := make([]byte, 4)
		_, _ = rand.Read(bytes)
		codes[i] = fmt.Sprintf("%08x", bytes)
	}
	return codes
}

type recoveryCode struct {
	Code   string `json:"code"`
	Hashed string `json:"hashed"`
}

func parseRecoveryCodes(codesJSON string) []recoveryCode {
	var codes []recoveryCode
	if codesJSON == "" {
		return codes
	}

	codesJSON = "[" + codesJSON + "]"
	_ = json.Unmarshal([]byte(codesJSON), &codes)
	return codes
}

// generateTOTPSecret is currently unused but may be needed for future TOTP implementation
// func generateTOTPSecret() (string, error) {
// 	secret := make([]byte, 20)
// 	_, err := rand.Read(secret)
// 	if err != nil {
// 		return "", err
// 	}
// 	return base32.StdEncoding.EncodeToString(secret), nil
// }
