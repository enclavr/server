package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/enclavr/server/internal/auth"
	"github.com/enclavr/server/internal/config"
	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
)

type SessionHandler struct {
	db   *database.Database
	cfg  *config.AuthConfig
	auth *auth.AuthService
}

func NewSessionHandler(db *database.Database, cfg *config.AuthConfig, authSvc *auth.AuthService) *SessionHandler {
	return &SessionHandler{
		db:   db,
		cfg:  cfg,
		auth: authSvc,
	}
}

type SessionResponse struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	IPAddress string    `json:"ip_address"`
	UserAgent string    `json:"user_agent"`
	Current   bool      `json:"current"`
}

func (h *SessionHandler) GetSessions(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var sessions []models.Session
	if err := h.db.Where("user_id = ?", userID).Order("created_at DESC").Limit(50).Find(&sessions).Error; err != nil {
		http.Error(w, "Failed to fetch sessions", http.StatusInternalServerError)
		return
	}

	currentToken := r.Header.Get("Authorization")
	var currentSessionID uuid.UUID

	if len(currentToken) > 7 && currentToken[:7] == "Bearer " {
		claims, err := h.auth.ValidateToken(currentToken[7:])
		if err == nil {
			currentSessionID = claims.SessionID
		}
	}

	response := make([]SessionResponse, len(sessions))
	for i, s := range sessions {
		response[i] = SessionResponse{
			ID:        s.ID,
			CreatedAt: s.CreatedAt,
			ExpiresAt: s.ExpiresAt,
			IPAddress: s.IPAddress,
			UserAgent: s.UserAgent,
			Current:   s.ID == currentSessionID && time.Now().Before(s.ExpiresAt),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"sessions": response,
		"count":    len(response),
	})
}

func (h *SessionHandler) RevokeSession(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	sessionID := r.URL.Query().Get("id")
	if sessionID == "" {
		http.Error(w, "Session ID is required", http.StatusBadRequest)
		return
	}

	sessionUUID, err := uuid.Parse(sessionID)
	if err != nil {
		http.Error(w, "Invalid session ID", http.StatusBadRequest)
		return
	}

	result := h.db.Where("id = ? AND user_id = ?", sessionUUID, userID).Delete(&models.Session{})
	if result.Error != nil {
		http.Error(w, "Failed to revoke session", http.StatusInternalServerError)
		return
	}

	if result.RowsAffected == 0 {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Session revoked successfully"})
}

func (h *SessionHandler) RevokeAllSessions(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if err := h.db.Where("user_id = ?", userID).Delete(&models.Session{}).Error; err != nil {
		http.Error(w, "Failed to revoke sessions", http.StatusInternalServerError)
		return
	}

	if err := h.db.Where("user_id = ?", userID).Delete(&models.RefreshToken{}).Error; err != nil {
		log.Printf("Failed to revoke refresh tokens: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "All sessions revoked successfully"})
}

func (h *SessionHandler) RotateToken(w http.ResponseWriter, r *http.Request) {
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
	accessToken, err := h.auth.GenerateToken(&user, sessionID)
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	refreshToken, err := h.auth.GenerateRefreshToken(&user)
	if err != nil {
		http.Error(w, "Failed to generate refresh token", http.StatusInternalServerError)
		return
	}

	h.storeRefreshToken(userID, refreshToken)

	session := models.Session{
		ID:        sessionID,
		UserID:    userID,
		Token:     accessToken,
		ExpiresAt: time.Now().Add(h.cfg.JWTExpiration),
		CreatedAt: time.Now(),
		IPAddress: r.RemoteAddr,
		UserAgent: r.UserAgent(),
	}
	h.db.Create(&session)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"expires_in":    int64(h.cfg.JWTExpiration.Seconds()),
		"token_type":    "Bearer",
	})
}

func (h *SessionHandler) storeRefreshToken(userID uuid.UUID, token string) {
	hashedToken, err := h.auth.HashPassword(token)
	if err != nil {
		log.Printf("Failed to hash refresh token: %v", err)
		return
	}

	refreshToken := models.RefreshToken{
		ID:        uuid.New(),
		UserID:    userID,
		Token:     hashedToken,
		ExpiresAt: time.Now().Add(h.cfg.RefreshExpiration),
		CreatedAt: time.Now(),
	}

	if err := h.db.Create(&refreshToken).Error; err != nil {
		log.Printf("Failed to store refresh token: %v", err)
	}
}

func (h *SessionHandler) GetActiveSessionsCount(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var count int64
	h.db.Model(&models.Session{}).Where("user_id = ? AND expires_at > ?", userID, time.Now()).Count(&count)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"active_sessions": count,
	})
}
