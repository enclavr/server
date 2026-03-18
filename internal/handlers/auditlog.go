package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	logger "github.com/enclavr/server/pkg/logger"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
)

type AuditHandler struct {
	db *database.Database
}

func NewAuditHandler(db *database.Database) *AuditHandler {
	return &AuditHandler{db: db}
}

type AuditLogResponse struct {
	ID           uuid.UUID          `json:"id"`
	UserID       uuid.UUID          `json:"user_id"`
	Username     string             `json:"username"`
	Action       models.AuditAction `json:"action"`
	TargetType   string             `json:"target_type"`
	TargetID     uuid.UUID          `json:"target_id"`
	Details      string             `json:"details"`
	OldValue     string             `json:"old_value"`
	NewValue     string             `json:"new_value"`
	IPAddress    string             `json:"ip_address"`
	UserAgent    string             `json:"user_agent"`
	Success      bool               `json:"success"`
	ErrorMessage string             `json:"error_message"`
	CreatedAt    time.Time          `json:"created_at"`
}

type AuditLogListResponse struct {
	Logs       []AuditLogResponse `json:"logs"`
	Total      int64              `json:"total"`
	Page       int                `json:"page"`
	PageSize   int                `json:"page_size"`
	TotalPages int64              `json:"total_pages"`
}

func (h *AuditHandler) GetAuditLogs(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	if !user.IsAdmin {
		http.Error(w, "Admin access required", http.StatusForbidden)
		return
	}

	page := 1
	pageSize := 50

	if p := r.URL.Query().Get("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}

	if ps := r.URL.Query().Get("page_size"); ps != "" {
		if parsed, err := strconv.Atoi(ps); err == nil && parsed > 0 && parsed <= 100 {
			pageSize = parsed
		}
	}

	actionFilter := r.URL.Query().Get("action")

	offset := (page - 1) * pageSize

	var logs []models.AuditLog
	query := h.db.Preload("User").Order("created_at DESC").Offset(offset).Limit(pageSize)

	if actionFilter != "" {
		query = query.Where("action = ?", actionFilter)
	}

	if err := query.Find(&logs).Error; err != nil {
		http.Error(w, "Failed to fetch audit logs", http.StatusInternalServerError)
		return
	}

	var total int64
	countQuery := h.db.Model(&models.AuditLog{})
	if actionFilter != "" {
		countQuery = countQuery.Where("action = ?", actionFilter)
	}
	countQuery.Count(&total)

	logResponses := make([]AuditLogResponse, len(logs))

	for i, log := range logs {
		username := ""
		if log.User.ID != uuid.Nil {
			username = log.User.Username
		}
		logResponses[i] = AuditLogResponse{
			ID:           log.ID,
			UserID:       log.UserID,
			Username:     username,
			Action:       log.Action,
			TargetType:   log.TargetType,
			TargetID:     log.TargetID,
			Details:      log.Details,
			OldValue:     log.OldValue,
			NewValue:     log.NewValue,
			IPAddress:    log.IPAddress,
			UserAgent:    log.UserAgent,
			Success:      log.Success,
			ErrorMessage: log.ErrorMessage,
			CreatedAt:    log.CreatedAt,
		}
	}

	totalPages := total / int64(pageSize)
	if total%int64(pageSize) > 0 {
		totalPages++
	}

	response := AuditLogListResponse{
		Logs:       logResponses,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

func (h *AuditHandler) LogAction(userID uuid.UUID, action models.AuditAction, targetType string, targetID uuid.UUID, details string, ipAddress string) {
	h.LogActionWithDetails(userID, action, targetType, targetID, details, "", "", ipAddress, "", true, "")
}

func (h *AuditHandler) LogActionWithDetails(userID uuid.UUID, action models.AuditAction, targetType string, targetID uuid.UUID, details string, oldValue string, newValue string, ipAddress string, userAgent string, success bool, errorMessage string) {
	log := models.AuditLog{
		UserID:       userID,
		Action:       action,
		TargetType:   targetType,
		TargetID:     targetID,
		Details:      details,
		OldValue:     oldValue,
		NewValue:     newValue,
		IPAddress:    ipAddress,
		UserAgent:    userAgent,
		Success:      success,
		ErrorMessage: errorMessage,
	}

	if err := h.db.Create(&log).Error; err != nil {
		logger.Error("Failed to create audit log", map[string]interface{}{"error": err.Error()})
	}
}
