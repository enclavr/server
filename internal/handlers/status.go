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
)

type StatusHandler struct {
	db *database.Database
}

func NewStatusHandler(db *database.Database) *StatusHandler {
	return &StatusHandler{db: db}
}

type UpdateStatusRequest struct {
	Status      *string `json:"status"`
	StatusText  *string `json:"status_text"`
	StatusEmoji *string `json:"status_emoji"`
	ExpiresIn   *int    `json:"expires_in"`
}

type StatusResponse struct {
	ID          uuid.UUID         `json:"id"`
	UserID      uuid.UUID         `json:"user_id"`
	Status      models.UserStatus `json:"status"`
	StatusText  string            `json:"status_text"`
	StatusEmoji string            `json:"status_emoji"`
	ExpiresAt   *time.Time        `json:"expires_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

func (h *StatusHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var status models.UserStatusModel
	if err := h.db.Where("user_id = ?", userID).First(&status).Error; err != nil {
		if err.Error() == "record not found" {
			status = models.UserStatusModel{
				UserID: userID,
				Status: models.UserStatusOffline,
			}
			if err := h.db.Create(&status).Error; err != nil {
				http.Error(w, "Failed to create status", http.StatusInternalServerError)
				return
			}
		} else {
			http.Error(w, "Failed to fetch status", http.StatusInternalServerError)
			return
		}
	}

	if status.ExpiresAt != nil && time.Now().After(*status.ExpiresAt) {
		status.Status = models.UserStatusOffline
		status.StatusText = ""
		status.StatusEmoji = ""
		status.ExpiresAt = nil
		if err := h.db.Save(&status).Error; err != nil {
			log.Printf("Error clearing expired status: %v", err)
		}
	}

	h.encodeStatus(w, status)
}

func (h *StatusHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req UpdateStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var status models.UserStatusModel
	if err := h.db.Where("user_id = ?", userID).First(&status).Error; err != nil {
		if err.Error() == "record not found" {
			status = models.UserStatusModel{
				UserID: userID,
				Status: models.UserStatusOffline,
			}
		} else {
			http.Error(w, "Failed to fetch status", http.StatusInternalServerError)
			return
		}
	}

	if req.Status != nil {
		newStatus := models.UserStatus(*req.Status)
		if !isValidStatus(newStatus) {
			http.Error(w, "Invalid status value", http.StatusBadRequest)
			return
		}
		status.Status = newStatus
	}

	if req.StatusText != nil {
		if len(*req.StatusText) > 150 {
			http.Error(w, "Status text too long (max 150 characters)", http.StatusBadRequest)
			return
		}
		status.StatusText = *req.StatusText
	}

	if req.StatusEmoji != nil {
		if len(*req.StatusEmoji) > 10 {
			http.Error(w, "Status emoji too long (max 10 characters)", http.StatusBadRequest)
			return
		}
		status.StatusEmoji = *req.StatusEmoji
	}

	if req.ExpiresIn != nil {
		if *req.ExpiresIn > 0 {
			expiresAt := time.Now().Add(time.Duration(*req.ExpiresIn) * time.Minute)
			status.ExpiresAt = &expiresAt
		} else {
			status.ExpiresAt = nil
		}
	}

	if status.Status == models.UserStatusOffline {
		status.StatusText = ""
		status.StatusEmoji = ""
		status.ExpiresAt = nil
	}

	status.UpdatedAt = time.Now()

	if status.ID == uuid.Nil {
		if err := h.db.Create(&status).Error; err != nil {
			http.Error(w, "Failed to create status", http.StatusInternalServerError)
			return
		}
	} else {
		if err := h.db.Save(&status).Error; err != nil {
			http.Error(w, "Failed to update status", http.StatusInternalServerError)
			return
		}
	}

	h.encodeStatus(w, status)
}

func (h *StatusHandler) GetUserStatus(w http.ResponseWriter, r *http.Request) {
	userIDStr := r.URL.Query().Get("user_id")
	if userIDStr == "" {
		http.Error(w, "user_id is required", http.StatusBadRequest)
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		http.Error(w, "Invalid user_id", http.StatusBadRequest)
		return
	}

	var status models.UserStatusModel
	if err := h.db.Where("user_id = ?", userID).First(&status).Error; err != nil {
		if err.Error() == "record not found" {
			status = models.UserStatusModel{
				UserID: userID,
				Status: models.UserStatusOffline,
			}
		} else {
			http.Error(w, "Failed to fetch status", http.StatusInternalServerError)
			return
		}
	}

	if status.ExpiresAt != nil && time.Now().After(*status.ExpiresAt) {
		status.Status = models.UserStatusOffline
		status.StatusText = ""
		status.StatusEmoji = ""
	}

	h.encodeStatus(w, status)
}

func (h *StatusHandler) encodeStatus(w http.ResponseWriter, status models.UserStatusModel) {
	response := StatusResponse{
		ID:          status.ID,
		UserID:      status.UserID,
		Status:      status.Status,
		StatusText:  status.StatusText,
		StatusEmoji: status.StatusEmoji,
		ExpiresAt:   status.ExpiresAt,
		UpdatedAt:   status.UpdatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding status response: %v", err)
	}
}

func isValidStatus(status models.UserStatus) bool {
	switch status {
	case models.UserStatusOnline, models.UserStatusAway, models.UserStatusDND, models.UserStatusInvisible, models.UserStatusOffline:
		return true
	}
	return false
}
