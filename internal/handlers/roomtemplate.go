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

type RoomTemplateHandler struct {
	db *database.Database
}

func NewRoomTemplateHandler(db *database.Database) *RoomTemplateHandler {
	return &RoomTemplateHandler{db: db}
}

type CreateRoomTemplateRequest struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	CategoryID  *uuid.UUID `json:"category_id"`
	Settings    string     `json:"settings"`
	IsPublic    bool       `json:"is_public"`
}

type UpdateRoomTemplateRequest struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	CategoryID  *uuid.UUID `json:"category_id"`
	Settings    string     `json:"settings"`
	IsPublic    *bool      `json:"is_public"`
}

type RoomTemplateResponse struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	CategoryID  *uuid.UUID `json:"category_id,omitempty"`
	Settings    string     `json:"settings"`
	IsPublic    bool       `json:"is_public"`
	UseCount    int        `json:"use_count"`
	CreatedBy   uuid.UUID  `json:"created_by"`
	CreatedAt   string     `json:"created_at"`
	UpdatedAt   string     `json:"updated_at"`
}

type CreateRoomFromTemplateRequest struct {
	TemplateID uuid.UUID `json:"template_id"`
	RoomName   string    `json:"room_name"`
}

func (h *RoomTemplateHandler) CreateTemplate(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req CreateRoomTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "Template name is required", http.StatusBadRequest)
		return
	}

	template := models.RoomTemplate{
		Name:        req.Name,
		Description: req.Description,
		CategoryID:  req.CategoryID,
		Settings:    req.Settings,
		IsPublic:    req.IsPublic,
		CreatedBy:   userID,
	}

	if err := h.db.Create(&template).Error; err != nil {
		log.Printf("Error creating room template: %v", err)
		http.Error(w, "Failed to create template", http.StatusInternalServerError)
		return
	}

	h.sendTemplateResponse(w, &template, http.StatusCreated)
}

func (h *RoomTemplateHandler) GetTemplates(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var templates []models.RoomTemplate
	if err := h.db.Where("created_by = ? OR is_public = ?", userID, true).
		Order("use_count DESC, created_at DESC").
		Find(&templates).Error; err != nil {
		log.Printf("Error fetching templates: %v", err)
		http.Error(w, "Failed to fetch templates", http.StatusInternalServerError)
		return
	}

	responses := make([]RoomTemplateResponse, len(templates))
	for i, t := range templates {
		responses[i] = h.templateToResponse(&t)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(responses); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *RoomTemplateHandler) GetTemplate(w http.ResponseWriter, r *http.Request) {
	templateIDStr := r.URL.Query().Get("id")
	if templateIDStr == "" {
		http.Error(w, "Template ID is required", http.StatusBadRequest)
		return
	}

	templateID, err := uuid.Parse(templateIDStr)
	if err != nil {
		http.Error(w, "Invalid template ID", http.StatusBadRequest)
		return
	}

	var template models.RoomTemplate
	if err := h.db.First(&template, "id = ?", templateID).Error; err != nil {
		http.Error(w, "Template not found", http.StatusNotFound)
		return
	}

	h.sendTemplateResponse(w, &template, http.StatusOK)
}

func (h *RoomTemplateHandler) UpdateTemplate(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req UpdateRoomTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var template models.RoomTemplate
	if err := h.db.First(&template, "id = ?", req.ID).Error; err != nil {
		http.Error(w, "Template not found", http.StatusNotFound)
		return
	}

	if template.CreatedBy != userID {
		var user models.User
		if err := h.db.First(&user, "id = ?", userID).Error; err != nil || !user.IsAdmin {
			http.Error(w, "Not authorized to update this template", http.StatusForbidden)
			return
		}
	}

	updates := map[string]interface{}{
		"updated_at": time.Now(),
	}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}
	if req.CategoryID != nil {
		updates["category_id"] = req.CategoryID
	}
	if req.Settings != "" {
		updates["settings"] = req.Settings
	}
	if req.IsPublic != nil {
		updates["is_public"] = *req.IsPublic
	}

	if err := h.db.Model(&template).Updates(updates).Error; err != nil {
		log.Printf("Error updating template: %v", err)
		http.Error(w, "Failed to update template", http.StatusInternalServerError)
		return
	}

	h.db.First(&template, "id = ?", req.ID)
	h.sendTemplateResponse(w, &template, http.StatusOK)
}

func (h *RoomTemplateHandler) DeleteTemplate(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	templateIDStr := r.URL.Query().Get("id")
	if templateIDStr == "" {
		http.Error(w, "Template ID is required", http.StatusBadRequest)
		return
	}

	templateID, err := uuid.Parse(templateIDStr)
	if err != nil {
		http.Error(w, "Invalid template ID", http.StatusBadRequest)
		return
	}

	var template models.RoomTemplate
	if err := h.db.First(&template, "id = ?", templateID).Error; err != nil {
		http.Error(w, "Template not found", http.StatusNotFound)
		return
	}

	if template.CreatedBy != userID {
		var user models.User
		if err := h.db.First(&user, "id = ?", userID).Error; err != nil || !user.IsAdmin {
			http.Error(w, "Not authorized to delete this template", http.StatusForbidden)
			return
		}
	}

	if err := h.db.Delete(&template).Error; err != nil {
		log.Printf("Error deleting template: %v", err)
		http.Error(w, "Failed to delete template", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "deleted"}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *RoomTemplateHandler) CreateRoomFromTemplate(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req CreateRoomFromTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.RoomName == "" {
		http.Error(w, "Room name is required", http.StatusBadRequest)
		return
	}

	var template models.RoomTemplate
	if err := h.db.First(&template, "id = ?", req.TemplateID).Error; err != nil {
		http.Error(w, "Template not found", http.StatusNotFound)
		return
	}

	room := models.Room{
		Name:        req.RoomName,
		Description: template.Description,
		CategoryID:  template.CategoryID,
		CreatedBy:   userID,
		MaxUsers:    50,
	}

	if err := h.db.Create(&room).Error; err != nil {
		log.Printf("Error creating room from template: %v", err)
		http.Error(w, "Failed to create room", http.StatusInternalServerError)
		return
	}

	h.db.Model(&template).Updates(map[string]interface{}{
		"use_count":  template.UseCount + 1,
		"updated_at": time.Now(),
	})

	h.db.Create(&models.UserRoom{
		UserID: userID,
		RoomID: room.ID,
		Role:   "owner",
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"room_id":     room.ID,
		"room_name":   room.Name,
		"template_id": template.ID,
	}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *RoomTemplateHandler) templateToResponse(template *models.RoomTemplate) RoomTemplateResponse {
	return RoomTemplateResponse{
		ID:          template.ID,
		Name:        template.Name,
		Description: template.Description,
		CategoryID:  template.CategoryID,
		Settings:    template.Settings,
		IsPublic:    template.IsPublic,
		UseCount:    template.UseCount,
		CreatedBy:   template.CreatedBy,
		CreatedAt:   template.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   template.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func (h *RoomTemplateHandler) sendTemplateResponse(w http.ResponseWriter, template *models.RoomTemplate, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(h.templateToResponse(template)); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}
