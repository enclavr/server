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

type ReminderHandler struct {
	db *database.Database
}

func NewReminderHandler(db *database.Database) *ReminderHandler {
	return &ReminderHandler{db: db}
}

type CreateReminderRequest struct {
	MessageID uuid.UUID `json:"message_id"`
	RemindAt  time.Time `json:"remind_at"`
	Note      string    `json:"note"`
}

type UpdateReminderRequest struct {
	RemindAt *time.Time `json:"remind_at"`
	Note     *string    `json:"note"`
}

type ReminderResponse struct {
	ID          uuid.UUID `json:"id"`
	MessageID   uuid.UUID `json:"message_id"`
	RemindAt    string    `json:"remind_at"`
	Note        string    `json:"note"`
	IsTriggered bool      `json:"is_triggered"`
	TriggeredAt *string   `json:"triggered_at,omitempty"`
	CreatedAt   string    `json:"created_at"`
}

func (h *ReminderHandler) GetReminders(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	pending := r.URL.Query().Get("pending") == "true"

	var reminders []models.MessageReminder
	query := h.db.Where("user_id = ?", userID)

	if pending {
		query = query.Where("is_triggered = ?", false).Where("remind_at > ?", time.Now())
	}

	if err := query.Order("remind_at ASC").Find(&reminders).Error; err != nil {
		http.Error(w, "Failed to fetch reminders", http.StatusInternalServerError)
		return
	}

	results := make([]ReminderResponse, 0, len(reminders))
	for _, reminder := range reminders {
		resp := ReminderResponse{
			ID:          reminder.ID,
			MessageID:   reminder.MessageID,
			RemindAt:    reminder.RemindAt.Format("2006-01-02T15:04:05Z07:00"),
			Note:        reminder.Note,
			IsTriggered: reminder.IsTriggered,
			CreatedAt:   reminder.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
		if reminder.TriggeredAt != nil {
			triggered := reminder.TriggeredAt.Format("2006-01-02T15:04:05Z07:00")
			resp.TriggeredAt = &triggered
		}
		results = append(results, resp)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(results)
}

func (h *ReminderHandler) CreateReminder(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req CreateReminderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.MessageID == uuid.Nil {
		http.Error(w, "Message ID is required", http.StatusBadRequest)
		return
	}

	if req.RemindAt.IsZero() {
		http.Error(w, "Remind at time is required", http.StatusBadRequest)
		return
	}

	if req.RemindAt.Before(time.Now()) {
		http.Error(w, "Remind at time must be in the future", http.StatusBadRequest)
		return
	}

	var message models.Message
	if err := h.db.First(&message, "id = ?", req.MessageID).Error; err != nil {
		http.Error(w, "Message not found", http.StatusNotFound)
		return
	}

	_ = message

	existingCount := int64(0)
	h.db.Model(&models.MessageReminder{}).Where("user_id = ? AND message_id = ? AND is_triggered = ?", userID, req.MessageID, false).Count(&existingCount)
	if existingCount > 0 {
		http.Error(w, "Reminder already exists for this message", http.StatusConflict)
		return
	}

	reminder := models.MessageReminder{
		UserID:    userID,
		MessageID: req.MessageID,
		RemindAt:  req.RemindAt,
		Note:      req.Note,
	}

	if err := h.db.Create(&reminder).Error; err != nil {
		http.Error(w, "Failed to create reminder", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(ReminderResponse{
		ID:          reminder.ID,
		MessageID:   reminder.MessageID,
		RemindAt:    reminder.RemindAt.Format("2006-01-02T15:04:05Z07:00"),
		Note:        reminder.Note,
		IsTriggered: reminder.IsTriggered,
		CreatedAt:   reminder.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

func (h *ReminderHandler) GetReminder(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	reminderID := r.PathValue("id")
	if reminderID == "" {
		if ctxID, ok := r.Context().Value(middleware.ReminderIDKey).(string); ok {
			reminderID = ctxID
		}
	}
	if reminderID == "" {
		http.Error(w, "Reminder ID required", http.StatusBadRequest)
		return
	}

	id, err := uuid.Parse(reminderID)
	if err != nil {
		http.Error(w, "Invalid reminder ID", http.StatusBadRequest)
		return
	}

	var reminder models.MessageReminder
	if err := h.db.First(&reminder, "id = ? AND user_id = ?", id, userID).Error; err != nil {
		http.Error(w, "Reminder not found", http.StatusNotFound)
		return
	}

	resp := ReminderResponse{
		ID:          reminder.ID,
		MessageID:   reminder.MessageID,
		RemindAt:    reminder.RemindAt.Format("2006-01-02T15:04:05Z07:00"),
		Note:        reminder.Note,
		IsTriggered: reminder.IsTriggered,
		CreatedAt:   reminder.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if reminder.TriggeredAt != nil {
		triggered := reminder.TriggeredAt.Format("2006-01-02T15:04:05Z07:00")
		resp.TriggeredAt = &triggered
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *ReminderHandler) UpdateReminder(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	reminderID := r.PathValue("id")
	if reminderID == "" {
		if ctxID, ok := r.Context().Value(middleware.ReminderIDKey).(string); ok {
			reminderID = ctxID
		}
	}
	if reminderID == "" {
		http.Error(w, "Reminder ID required", http.StatusBadRequest)
		return
	}

	id, err := uuid.Parse(reminderID)
	if err != nil {
		http.Error(w, "Invalid reminder ID", http.StatusBadRequest)
		return
	}

	var reminder models.MessageReminder
	if err := h.db.First(&reminder, "id = ? AND user_id = ?", id, userID).Error; err != nil {
		http.Error(w, "Reminder not found", http.StatusNotFound)
		return
	}

	if reminder.IsTriggered {
		http.Error(w, "Cannot update triggered reminder", http.StatusForbidden)
		return
	}

	var req UpdateReminderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.RemindAt != nil {
		if req.RemindAt.Before(time.Now()) {
			http.Error(w, "Remind at time must be in the future", http.StatusBadRequest)
			return
		}
		reminder.RemindAt = *req.RemindAt
	}

	if req.Note != nil {
		reminder.Note = *req.Note
	}

	reminder.UpdatedAt = time.Now()
	if err := h.db.Save(&reminder).Error; err != nil {
		http.Error(w, "Failed to update reminder", http.StatusInternalServerError)
		return
	}

	resp := ReminderResponse{
		ID:          reminder.ID,
		MessageID:   reminder.MessageID,
		RemindAt:    reminder.RemindAt.Format("2006-01-02T15:04:05Z07:00"),
		Note:        reminder.Note,
		IsTriggered: reminder.IsTriggered,
		CreatedAt:   reminder.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *ReminderHandler) DeleteReminder(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	reminderID := r.PathValue("id")
	if reminderID == "" {
		if ctxID, ok := r.Context().Value(middleware.ReminderIDKey).(string); ok {
			reminderID = ctxID
		}
	}
	if reminderID == "" {
		http.Error(w, "Reminder ID required", http.StatusBadRequest)
		return
	}

	id, err := uuid.Parse(reminderID)
	if err != nil {
		http.Error(w, "Invalid reminder ID", http.StatusBadRequest)
		return
	}

	result := h.db.Delete(&models.MessageReminder{}, "id = ? AND user_id = ?", id, userID)
	if result.Error != nil {
		http.Error(w, "Failed to delete reminder", http.StatusInternalServerError)
		return
	}

	if result.RowsAffected == 0 {
		http.Error(w, "Reminder not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *ReminderHandler) GetPendingReminders(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var reminders []models.MessageReminder
	if err := h.db.Where("user_id = ? AND is_triggered = ? AND remind_at <= ?", userID, false, time.Now()).
		Order("remind_at ASC").
		Find(&reminders).Error; err != nil {
		http.Error(w, "Failed to fetch pending reminders", http.StatusInternalServerError)
		return
	}

	now := time.Now()
	for i := range reminders {
		reminders[i].IsTriggered = true
		reminders[i].TriggeredAt = &now
		reminders[i].UpdatedAt = now
		h.db.Save(&reminders[i])
	}

	results := make([]ReminderResponse, 0, len(reminders))
	for _, reminder := range reminders {
		resp := ReminderResponse{
			ID:          reminder.ID,
			MessageID:   reminder.MessageID,
			RemindAt:    reminder.RemindAt.Format("2006-01-02T15:04:05Z07:00"),
			Note:        reminder.Note,
			IsTriggered: true,
			CreatedAt:   reminder.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
		triggered := now.Format("2006-01-02T15:04:05Z07:00")
		resp.TriggeredAt = &triggered
		results = append(results, resp)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(results)
}
