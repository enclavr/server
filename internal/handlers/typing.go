package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	// TYPING_EXPIRY_DURATION is how long a typing indicator remains active.
	TYPING_EXPIRY_DURATION = 10 * time.Second
)

// TypingIndicatorHandler handles typing indicator start/stop and queries.
type TypingIndicatorHandler struct {
	db *database.Database
}

// NewTypingIndicatorHandler creates a new TypingIndicatorHandler instance.
func NewTypingIndicatorHandler(db *database.Database) *TypingIndicatorHandler {
	return &TypingIndicatorHandler{db: db}
}

// StartTypingRequest represents the request body for starting a typing indicator.
type StartTypingRequest struct {
	RoomID   *uuid.UUID `json:"room_id,omitempty"`
	DMUserID *uuid.UUID `json:"dm_user_id,omitempty"`
}

// TypingUserResponse represents a user who is currently typing.
type TypingUserResponse struct {
	UserID    uuid.UUID `json:"user_id"`
	Username  string    `json:"username"`
	StartedAt string    `json:"started_at"`
}

// StartTyping creates or refreshes a typing indicator for the authenticated user.
func (h *TypingIndicatorHandler) StartTyping(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req StartTypingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.RoomID == nil && req.DMUserID == nil {
		http.Error(w, "Either room_id or dm_user_id is required", http.StatusBadRequest)
		return
	}

	if req.RoomID != nil && req.DMUserID != nil {
		http.Error(w, "Provide either room_id or dm_user_id, not both", http.StatusBadRequest)
		return
	}

	if req.RoomID != nil {
		var userRoom models.UserRoom
		if err := h.db.Where("user_id = ? AND room_id = ?", userID, *req.RoomID).First(&userRoom).Error; err != nil {
			http.Error(w, "You are not a member of this room", http.StatusForbidden)
			return
		}
	}

	now := time.Now()
	expiresAt := now.Add(TYPING_EXPIRY_DURATION)

	var existing models.TypingIndicator
	query := h.db.Where("user_id = ?", userID)
	if req.RoomID != nil {
		query = query.Where("room_id = ?", *req.RoomID)
	} else {
		query = query.Where("dm_user_id = ?", *req.DMUserID)
	}

	if err := query.First(&existing).Error; err == nil {
		updates := map[string]interface{}{
			"started_at": now,
			"expires_at": expiresAt,
		}
		if err := h.db.Model(&existing).Updates(updates).Error; err != nil {
			log.Printf("Error updating typing indicator: %v", err)
			http.Error(w, "Failed to update typing indicator", http.StatusInternalServerError)
			return
		}
	} else {
		indicator := models.TypingIndicator{
			UserID:    userID,
			RoomID:    req.RoomID,
			DMUserID:  req.DMUserID,
			StartedAt: now,
			ExpiresAt: expiresAt,
		}
		if err := h.db.Create(&indicator).Error; err != nil {
			log.Printf("Error creating typing indicator: %v", err)
			http.Error(w, "Failed to create typing indicator", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(`{"status":"ok"}`)); err != nil {
		log.Printf("Error writing response: %v", err)
	}
}

// StopTyping removes a typing indicator for the authenticated user.
func (h *TypingIndicatorHandler) StopTyping(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	roomIDStr := r.URL.Query().Get("room_id")
	dmUserIDStr := r.URL.Query().Get("dm_user_id")

	if roomIDStr == "" && dmUserIDStr == "" {
		http.Error(w, "Either room_id or dm_user_id is required", http.StatusBadRequest)
		return
	}

	query := h.db.Where("user_id = ?", userID)
	if roomIDStr != "" {
		roomID, err := uuid.Parse(roomIDStr)
		if err != nil {
			http.Error(w, "Invalid room_id", http.StatusBadRequest)
			return
		}
		query = query.Where("room_id = ?", roomID)
	} else {
		dmUserID, err := uuid.Parse(dmUserIDStr)
		if err != nil {
			http.Error(w, "Invalid dm_user_id", http.StatusBadRequest)
			return
		}
		query = query.Where("dm_user_id = ?", dmUserID)
	}

	query.Delete(&models.TypingIndicator{})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(`{"status":"ok"}`)); err != nil {
		log.Printf("Error writing response: %v", err)
	}
}

// GetTypingUsers returns all users currently typing in a room or DM conversation.
func (h *TypingIndicatorHandler) GetTypingUsers(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	_ = userID

	roomIDStr := r.URL.Query().Get("room_id")
	dmUserIDStr := r.URL.Query().Get("dm_user_id")

	if roomIDStr == "" && dmUserIDStr == "" {
		http.Error(w, "Either room_id or dm_user_id is required", http.StatusBadRequest)
		return
	}

	h.cleanExpiredIndicators()

	var indicators []models.TypingIndicator
	query := h.db.Preload("User")
	if roomIDStr != "" {
		roomID, err := uuid.Parse(roomIDStr)
		if err != nil {
			http.Error(w, "Invalid room_id", http.StatusBadRequest)
			return
		}
		query = query.Where("room_id = ? AND expires_at > ?", roomID, time.Now())
	} else {
		dmUserID, err := uuid.Parse(dmUserIDStr)
		if err != nil {
			http.Error(w, "Invalid dm_user_id", http.StatusBadRequest)
			return
		}
		query = query.Where("dm_user_id = ? AND expires_at > ?", dmUserID, time.Now())
	}

	query.Find(&indicators)

	responses := make([]TypingUserResponse, 0, len(indicators))
	for _, ind := range indicators {
		responses = append(responses, TypingUserResponse{
			UserID:    ind.UserID,
			Username:  ind.User.Username,
			StartedAt: ind.StartedAt.Format(time.RFC3339),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(responses); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

// cleanExpiredIndicators removes expired typing indicators from the database.
func (h *TypingIndicatorHandler) cleanExpiredIndicators() {
	h.db.Where("expires_at <= ?", time.Now()).Delete(&models.TypingIndicator{})
}

// CleanupExpiredTypingIndicators is a periodic cleanup function that removes expired indicators.
// Call this from a goroutine in main.go.
func CleanupExpiredTypingIndicators(db *gorm.DB, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		result := db.Where("expires_at <= ?", time.Now()).Delete(&models.TypingIndicator{})
		if result.Error != nil {
			log.Printf("Error cleaning up expired typing indicators: %v", result.Error)
		}
	}
}
