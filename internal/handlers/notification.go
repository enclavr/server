package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
)

type NotificationHandler struct {
	db *database.Database
}

func NewNotificationHandler(db *database.Database) *NotificationHandler {
	return &NotificationHandler{db: db}
}

type CreateNotificationRequest struct {
	Type      models.NotificationType `json:"type"`
	Title     string                  `json:"title"`
	Body      string                  `json:"body"`
	Link      string                  `json:"link"`
	ActorID   *uuid.UUID              `json:"actor_id"`
	ActorName string                  `json:"actor_name"`
	RoomID    *uuid.UUID              `json:"room_id"`
	MessageID *uuid.UUID              `json:"message_id"`
	Data      map[string]interface{}  `json:"data"`
}

type UpdateNotificationRequest struct {
	IsRead   *bool `json:"is_read"`
	Archived *bool `json:"archived"`
}

type NotificationResponse struct {
	ID        uuid.UUID               `json:"id"`
	Type      models.NotificationType `json:"type"`
	Title     string                  `json:"title"`
	Body      string                  `json:"body"`
	Link      string                  `json:"link"`
	ActorID   *uuid.UUID              `json:"actor_id"`
	ActorName string                  `json:"actor_name"`
	RoomID    *uuid.UUID              `json:"room_id"`
	MessageID *uuid.UUID              `json:"message_id"`
	IsRead    bool                    `json:"is_read"`
	Archived  bool                    `json:"archived"`
	Data      map[string]interface{}  `json:"data"`
	CreatedAt string                  `json:"created_at"`
	ReadAt    *string                 `json:"read_at"`
}

func (h *NotificationHandler) GetNotifications(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	query := h.db.Where("user_id = ?", userID)

	archived := r.URL.Query().Get("archived")
	if archived == "true" {
		query = query.Where("archived = ?", true)
	} else if archived == "false" || archived == "" {
		query = query.Where("archived = ?", false)
	}

	isRead := r.URL.Query().Get("is_read")
	if isRead == "true" {
		query = query.Where("is_read = ?", true)
	} else if isRead == "false" {
		query = query.Where("is_read = ?", false)
	}

	notificationType := r.URL.Query().Get("type")
	if notificationType != "" {
		query = query.Where("type = ?", notificationType)
	}

	var notifications []models.Notification
	if err := query.Order("created_at DESC").Limit(100).Find(&notifications).Error; err != nil {
		http.Error(w, "Failed to fetch notifications", http.StatusInternalServerError)
		return
	}

	results := make([]NotificationResponse, 0, len(notifications))
	for _, n := range notifications {
		results = append(results, h.toResponse(n))
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(results)
}

func (h *NotificationHandler) GetUnreadCount(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var count int64
	if err := h.db.Model(&models.Notification{}).
		Where("user_id = ? AND is_read = ? AND archived = ?", userID, false, false).
		Count(&count).Error; err != nil {
		http.Error(w, "Failed to count notifications", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]int64{"unread_count": count})
}

func (h *NotificationHandler) MarkAsRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	notificationID := r.PathValue("id")
	if notificationID == "" {
		http.Error(w, "Notification ID required", http.StatusBadRequest)
		return
	}

	id, err := uuid.Parse(notificationID)
	if err != nil {
		http.Error(w, "Invalid notification ID", http.StatusBadRequest)
		return
	}

	var notification models.Notification
	if err := h.db.First(&notification, "id = ? AND user_id = ?", id, userID).Error; err != nil {
		http.Error(w, "Notification not found", http.StatusNotFound)
		return
	}

	notification.IsRead = true
	now := notification.CreatedAt
	notification.ReadAt = &now

	if err := h.db.Save(&notification).Error; err != nil {
		http.Error(w, "Failed to update notification", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h.toResponse(notification))
}

func (h *NotificationHandler) MarkAllAsRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	now := time.Now()
	result := h.db.Model(&models.Notification{}).
		Where("user_id = ? AND is_read = ? AND archived = ?", userID, false, false).
		Updates(map[string]interface{}{
			"is_read": true,
			"read_at": now,
		})

	if result.Error != nil {
		http.Error(w, "Failed to mark notifications as read", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]int64{"updated_count": result.RowsAffected})
}

func (h *NotificationHandler) ArchiveNotification(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	notificationID := r.PathValue("id")
	if notificationID == "" {
		http.Error(w, "Notification ID required", http.StatusBadRequest)
		return
	}

	id, err := uuid.Parse(notificationID)
	if err != nil {
		http.Error(w, "Invalid notification ID", http.StatusBadRequest)
		return
	}

	var notification models.Notification
	if err := h.db.First(&notification, "id = ? AND user_id = ?", id, userID).Error; err != nil {
		http.Error(w, "Notification not found", http.StatusNotFound)
		return
	}

	notification.Archived = true

	if err := h.db.Save(&notification).Error; err != nil {
		http.Error(w, "Failed to archive notification", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h.toResponse(notification))
}

func (h *NotificationHandler) DeleteNotification(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	notificationID := r.PathValue("id")
	if notificationID == "" {
		http.Error(w, "Notification ID required", http.StatusBadRequest)
		return
	}

	id, err := uuid.Parse(notificationID)
	if err != nil {
		http.Error(w, "Invalid notification ID", http.StatusBadRequest)
		return
	}

	result := h.db.Delete(&models.Notification{}, "id = ? AND user_id = ?", id, userID)
	if result.Error != nil {
		http.Error(w, "Failed to delete notification", http.StatusInternalServerError)
		return
	}

	if result.RowsAffected == 0 {
		http.Error(w, "Notification not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *NotificationHandler) CreateNotification(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req CreateNotificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Title == "" {
		http.Error(w, "Title is required", http.StatusBadRequest)
		return
	}

	if req.Type == "" {
		req.Type = models.NotificationTypeSystem
	}

	var dataJSON string
	if req.Data != nil {
		dataBytes, _ := json.Marshal(req.Data)
		dataJSON = string(dataBytes)
	}

	notification := models.Notification{
		UserID:    userID,
		Type:      req.Type,
		Title:     req.Title,
		Body:      req.Body,
		Link:      req.Link,
		ActorID:   req.ActorID,
		ActorName: req.ActorName,
		RoomID:    req.RoomID,
		MessageID: req.MessageID,
		Data:      dataJSON,
	}

	if err := h.db.Create(&notification).Error; err != nil {
		http.Error(w, "Failed to create notification", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(h.toResponse(notification))
}

func (h *NotificationHandler) toResponse(n models.Notification) NotificationResponse {
	var readAt *string
	if n.ReadAt != nil {
		formatted := n.ReadAt.Format("2006-01-02T15:04:05Z07:00")
		readAt = &formatted
	}

	var data map[string]interface{}
	if n.Data != "" {
		_ = json.Unmarshal([]byte(n.Data), &data)
	}

	return NotificationResponse{
		ID:        n.ID,
		Type:      n.Type,
		Title:     n.Title,
		Body:      n.Body,
		Link:      n.Link,
		ActorID:   n.ActorID,
		ActorName: n.ActorName,
		RoomID:    n.RoomID,
		MessageID: n.MessageID,
		IsRead:    n.IsRead,
		Archived:  n.Archived,
		Data:      data,
		CreatedAt: n.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		ReadAt:    readAt,
	}
}
