package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
)

type AnnouncementHandler struct {
	db *database.Database
}

func NewAnnouncementHandler(db *database.Database) *AnnouncementHandler {
	return &AnnouncementHandler{db: db}
}

type CreateAnnouncementRequest struct {
	Title     string                      `json:"title"`
	Content   string                      `json:"content"`
	Priority  models.AnnouncementPriority `json:"priority"`
	ExpiresAt *time.Time                  `json:"expires_at"`
}

type AnnouncementResponse struct {
	ID         uuid.UUID                   `json:"id"`
	Title      string                      `json:"title"`
	Content    string                      `json:"content"`
	Priority   models.AnnouncementPriority `json:"priority"`
	CreatedBy  uuid.UUID                   `json:"created_by"`
	AuthorName string                      `json:"author_name"`
	IsActive   bool                        `json:"is_active"`
	ExpiresAt  *time.Time                  `json:"expires_at"`
	CreatedAt  string                      `json:"created_at"`
	UpdatedAt  string                      `json:"updated_at"`
}

func (h *AnnouncementHandler) CreateAnnouncement(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "Only admins can create announcements", http.StatusForbidden)
		return
	}

	var req CreateAnnouncementRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Title == "" {
		http.Error(w, "Title is required", http.StatusBadRequest)
		return
	}

	if req.Content == "" {
		http.Error(w, "Content is required", http.StatusBadRequest)
		return
	}

	if len(req.Title) > 200 {
		http.Error(w, "Title must be 200 characters or less", http.StatusBadRequest)
		return
	}

	if req.Priority == "" {
		req.Priority = models.AnnouncementPriorityNormal
	}

	announcement := models.Announcement{
		Title:     req.Title,
		Content:   req.Content,
		Priority:  req.Priority,
		CreatedBy: userID,
		IsActive:  true,
		ExpiresAt: req.ExpiresAt,
	}

	if err := h.db.Create(&announcement).Error; err != nil {
		http.Error(w, "Failed to create announcement", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(h.toResponse(announcement, user.Username))
}

func (h *AnnouncementHandler) GetAnnouncements(w http.ResponseWriter, r *http.Request) {
	_, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	activeOnly := r.URL.Query().Get("active")
	query := h.db.Order("created_at DESC")
	if activeOnly == "true" {
		query = query.Where("is_active = ?", true)
	}

	var announcements []models.Announcement
	if err := query.Limit(limit).Find(&announcements).Error; err != nil {
		http.Error(w, "Failed to fetch announcements", http.StatusInternalServerError)
		return
	}

	userIDs := make([]uuid.UUID, 0, len(announcements))
	for _, a := range announcements {
		userIDs = append(userIDs, a.CreatedBy)
	}

	userMap := make(map[uuid.UUID]string)
	if len(userIDs) > 0 {
		var users []models.User
		h.db.Where("id IN ?", userIDs).Find(&users)
		for _, u := range users {
			userMap[u.ID] = u.Username
		}
	}

	responses := make([]AnnouncementResponse, 0, len(announcements))
	for _, a := range announcements {
		responses = append(responses, h.toResponse(a, userMap[a.CreatedBy]))
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(responses)
}

func (h *AnnouncementHandler) GetActiveAnnouncement(w http.ResponseWriter, r *http.Request) {
	_, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var announcement models.Announcement
	query := h.db.Where("is_active = ?", true).Order("created_at DESC")

	if err := query.First(&announcement).Error; err != nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"announcement": nil})
		return
	}

	if announcement.ExpiresAt != nil && announcement.ExpiresAt.Before(time.Now()) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"announcement": nil})
		return
	}

	var author models.User
	h.db.Select("username").First(&author, "id = ?", announcement.CreatedBy)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"announcement": h.toResponse(announcement, author.Username),
	})
}

func (h *AnnouncementHandler) UpdateAnnouncement(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "Only admins can update announcements", http.StatusForbidden)
		return
	}

	announcementIDStr := r.URL.Query().Get("id")
	if announcementIDStr == "" {
		http.Error(w, "Announcement ID is required", http.StatusBadRequest)
		return
	}

	announcementID, err := uuid.Parse(announcementIDStr)
	if err != nil {
		http.Error(w, "Invalid announcement ID", http.StatusBadRequest)
		return
	}

	var announcement models.Announcement
	if err := h.db.First(&announcement, "id = ?", announcementID).Error; err != nil {
		http.Error(w, "Announcement not found", http.StatusNotFound)
		return
	}

	var req CreateAnnouncementRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Title != "" {
		if len(req.Title) > 200 {
			http.Error(w, "Title must be 200 characters or less", http.StatusBadRequest)
			return
		}
		announcement.Title = req.Title
	}

	if req.Content != "" {
		announcement.Content = req.Content
	}

	if req.Priority != "" {
		announcement.Priority = req.Priority
	}

	if req.ExpiresAt != nil {
		announcement.ExpiresAt = req.ExpiresAt
	}

	announcement.UpdatedAt = time.Now()

	if err := h.db.Save(&announcement).Error; err != nil {
		http.Error(w, "Failed to update announcement", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h.toResponse(announcement, user.Username))
}

func (h *AnnouncementHandler) DeleteAnnouncement(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "Only admins can delete announcements", http.StatusForbidden)
		return
	}

	announcementIDStr := r.URL.Query().Get("id")
	if announcementIDStr == "" {
		http.Error(w, "Announcement ID is required", http.StatusBadRequest)
		return
	}

	announcementID, err := uuid.Parse(announcementIDStr)
	if err != nil {
		http.Error(w, "Invalid announcement ID", http.StatusBadRequest)
		return
	}

	result := h.db.Delete(&models.Announcement{}, "id = ?", announcementID)
	if result.Error != nil {
		http.Error(w, "Failed to delete announcement", http.StatusInternalServerError)
		return
	}

	if result.RowsAffected == 0 {
		http.Error(w, "Announcement not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AnnouncementHandler) DeactivateAnnouncement(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "Only admins can deactivate announcements", http.StatusForbidden)
		return
	}

	announcementIDStr := r.URL.Query().Get("id")
	if announcementIDStr == "" {
		http.Error(w, "Announcement ID is required", http.StatusBadRequest)
		return
	}

	announcementID, err := uuid.Parse(announcementIDStr)
	if err != nil {
		http.Error(w, "Invalid announcement ID", http.StatusBadRequest)
		return
	}

	var announcement models.Announcement
	if err := h.db.First(&announcement, "id = ?", announcementID).Error; err != nil {
		http.Error(w, "Announcement not found", http.StatusNotFound)
		return
	}

	announcement.IsActive = false
	announcement.UpdatedAt = time.Now()

	if err := h.db.Save(&announcement).Error; err != nil {
		http.Error(w, "Failed to deactivate announcement", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h.toResponse(announcement, user.Username))
}

func (h *AnnouncementHandler) toResponse(a models.Announcement, authorName string) AnnouncementResponse {
	return AnnouncementResponse{
		ID:         a.ID,
		Title:      a.Title,
		Content:    a.Content,
		Priority:   a.Priority,
		CreatedBy:  a.CreatedBy,
		AuthorName: authorName,
		IsActive:   a.IsActive,
		ExpiresAt:  a.ExpiresAt,
		CreatedAt:  a.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:  a.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
