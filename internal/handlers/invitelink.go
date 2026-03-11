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

type InviteLinkHandler struct {
	db *database.Database
}

func NewInviteLinkHandler(db *database.Database) *InviteLinkHandler {
	return &InviteLinkHandler{db: db}
}

type CreateInviteLinkRequest struct {
	RoomID      *uuid.UUID `json:"room_id"`
	Code        string     `json:"code"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	MaxUses     int        `json:"max_uses"`
	ExpiresIn   int        `json:"expires_in"`
}

type UpdateInviteLinkRequest struct {
	ID          uuid.UUID `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	MaxUses     int       `json:"max_uses"`
	IsEnabled   bool      `json:"is_enabled"`
	ExpiresIn   *int      `json:"expires_in"`
}

type InviteLinkResponse struct {
	ID          uuid.UUID  `json:"id"`
	Code        string     `json:"code"`
	RoomID      uuid.UUID  `json:"room_id"`
	RoomName    string     `json:"room_name"`
	CreatedBy   uuid.UUID  `json:"created_by"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	MaxUses     int        `json:"max_uses"`
	Uses        int        `json:"uses"`
	IsPermanent bool       `json:"is_permanent"`
	IsEnabled   bool       `json:"is_enabled"`
	ExpiresAt   *time.Time `json:"expires_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	URL         string     `json:"url"`
}

func (h *InviteLinkHandler) CreateInviteLink(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req CreateInviteLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.RoomID == nil {
		http.Error(w, "Room ID is required", http.StatusBadRequest)
		return
	}

	var room models.Room
	if err := h.db.First(&room, *req.RoomID).Error; err != nil {
		http.Error(w, "Room not found", http.StatusNotFound)
		return
	}

	var userRoom models.UserRoom
	if err := h.db.Where("user_id = ? AND room_id = ?", userID, *req.RoomID).First(&userRoom).Error; err != nil {
		http.Error(w, "You must be a member of the room to create an invite link", http.StatusForbidden)
		return
	}

	if userRoom.Role != "owner" && userRoom.Role != "admin" {
		http.Error(w, "Only room owners and admins can create invite links", http.StatusForbidden)
		return
	}

	if req.Code != "" {
		var existing models.InviteLink
		if err := h.db.Where("code = ?", req.Code).First(&existing).Error; err == nil {
			http.Error(w, "This invite code is already taken", http.StatusConflict)
			return
		}
	}

	inviteLink := models.InviteLink{
		RoomID:      *req.RoomID,
		CreatedBy:   userID,
		Title:       req.Title,
		Description: req.Description,
		MaxUses:     req.MaxUses,
		IsPermanent: req.MaxUses == 0 && req.ExpiresIn == 0,
		IsEnabled:   true,
	}

	if req.Code != "" {
		inviteLink.Code = req.Code
	}

	if req.ExpiresIn > 0 {
		expireTime := time.Now().Add(time.Duration(req.ExpiresIn) * time.Hour)
		inviteLink.ExpiresAt = &expireTime
		inviteLink.IsPermanent = false
	}

	if err := h.db.Create(&inviteLink).Error; err != nil {
		http.Error(w, "Failed to create invite link", http.StatusInternalServerError)
		return
	}

	h.sendInviteLinkResponse(w, &inviteLink, &room)
}

func (h *InviteLinkHandler) GetInviteLinks(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	roomIDStr := r.URL.Query().Get("room_id")
	if roomIDStr == "" {
		http.Error(w, "Room ID is required", http.StatusBadRequest)
		return
	}

	roomID, err := uuid.Parse(roomIDStr)
	if err != nil {
		http.Error(w, "Invalid room ID", http.StatusBadRequest)
		return
	}

	var userRoom models.UserRoom
	if err := h.db.Where("user_id = ? AND room_id = ?", userID, roomID).First(&userRoom).Error; err != nil {
		http.Error(w, "You must be a member of the room to view invite links", http.StatusForbidden)
		return
	}

	var inviteLinks []models.InviteLink
	if err := h.db.Where("room_id = ?", roomID).Find(&inviteLinks).Error; err != nil {
		http.Error(w, "Failed to fetch invite links", http.StatusInternalServerError)
		return
	}

	var room models.Room
	h.db.First(&room, roomID)

	responses := make([]InviteLinkResponse, 0, len(inviteLinks))
	for _, link := range inviteLinks {
		responses = append(responses, h.inviteLinkToResponse(&link, &room))
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(responses); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *InviteLinkHandler) UpdateInviteLink(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req UpdateInviteLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var inviteLink models.InviteLink
	if err := h.db.First(&inviteLink, req.ID).Error; err != nil {
		http.Error(w, "Invite link not found", http.StatusNotFound)
		return
	}

	var userRoom models.UserRoom
	if err := h.db.Where("user_id = ? AND room_id = ?", userID, inviteLink.RoomID).First(&userRoom).Error; err != nil {
		http.Error(w, "You must be a member of the room to update invite links", http.StatusForbidden)
		return
	}

	if userRoom.Role != "owner" && userRoom.Role != "admin" {
		http.Error(w, "Only room owners and admins can update invite links", http.StatusForbidden)
		return
	}

	if req.Title != "" {
		inviteLink.Title = req.Title
	}
	if req.Description != "" {
		inviteLink.Description = req.Description
	}
	if req.MaxUses > 0 {
		inviteLink.MaxUses = req.MaxUses
	}
	inviteLink.IsEnabled = req.IsEnabled

	if req.ExpiresIn != nil && *req.ExpiresIn > 0 {
		expireTime := time.Now().Add(time.Duration(*req.ExpiresIn) * time.Hour)
		inviteLink.ExpiresAt = &expireTime
		inviteLink.IsPermanent = false
	} else if req.ExpiresIn != nil && *req.ExpiresIn == 0 {
		inviteLink.ExpiresAt = nil
		inviteLink.IsPermanent = true
	}

	inviteLink.UpdatedAt = time.Now()

	if err := h.db.Save(&inviteLink).Error; err != nil {
		http.Error(w, "Failed to update invite link", http.StatusInternalServerError)
		return
	}

	var room models.Room
	h.db.First(&room, inviteLink.RoomID)

	h.sendInviteLinkResponse(w, &inviteLink, &room)
}

func (h *InviteLinkHandler) DeleteInviteLink(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	inviteIDStr := r.URL.Query().Get("id")
	if inviteIDStr == "" {
		http.Error(w, "Invite link ID is required", http.StatusBadRequest)
		return
	}

	inviteID, err := uuid.Parse(inviteIDStr)
	if err != nil {
		http.Error(w, "Invalid invite link ID", http.StatusBadRequest)
		return
	}

	var inviteLink models.InviteLink
	if err := h.db.First(&inviteLink, inviteID).Error; err != nil {
		http.Error(w, "Invite link not found", http.StatusNotFound)
		return
	}

	var userRoom models.UserRoom
	if err := h.db.Where("user_id = ? AND room_id = ?", userID, inviteLink.RoomID).First(&userRoom).Error; err != nil {
		http.Error(w, "You must be a member of the room to delete invite links", http.StatusForbidden)
		return
	}

	if userRoom.Role != "owner" && userRoom.Role != "admin" {
		http.Error(w, "Only room owners and admins can delete invite links", http.StatusForbidden)
		return
	}

	if err := h.db.Delete(&inviteLink).Error; err != nil {
		http.Error(w, "Failed to delete invite link", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "deleted"}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *InviteLinkHandler) ResolveInviteLink(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Invite code is required", http.StatusBadRequest)
		return
	}

	var inviteLink models.InviteLink
	if err := h.db.Where("code = ?", code).First(&inviteLink).Error; err != nil {
		http.Error(w, "Invite link not found", http.StatusNotFound)
		return
	}

	if !inviteLink.IsEnabled {
		http.Error(w, "This invite link is disabled", http.StatusForbidden)
		return
	}

	if inviteLink.ExpiresAt != nil && time.Now().After(*inviteLink.ExpiresAt) {
		http.Error(w, "This invite link has expired", http.StatusForbidden)
		return
	}

	if inviteLink.MaxUses > 0 && inviteLink.Uses >= inviteLink.MaxUses {
		http.Error(w, "This invite link has reached its maximum uses", http.StatusForbidden)
		return
	}

	var room models.Room
	if err := h.db.First(&room, inviteLink.RoomID).Error; err != nil {
		http.Error(w, "Room not found", http.StatusNotFound)
		return
	}

	response := h.inviteLinkToResponse(&inviteLink, &room)
	response.URL = "/invite/" + inviteLink.Code

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *InviteLinkHandler) UseInviteLink(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Invite code is required", http.StatusBadRequest)
		return
	}

	var inviteLink models.InviteLink
	if err := h.db.Where("code = ?", code).First(&inviteLink).Error; err != nil {
		http.Error(w, "Invalid invite code", http.StatusNotFound)
		return
	}

	if !inviteLink.IsEnabled {
		http.Error(w, "This invite link is disabled", http.StatusForbidden)
		return
	}

	if inviteLink.ExpiresAt != nil && time.Now().After(*inviteLink.ExpiresAt) {
		http.Error(w, "This invite link has expired", http.StatusForbidden)
		return
	}

	if inviteLink.MaxUses > 0 && inviteLink.Uses >= inviteLink.MaxUses {
		http.Error(w, "This invite link has reached its maximum uses", http.StatusForbidden)
		return
	}

	var existingUser models.UserRoom
	result := h.db.Where("user_id = ? AND room_id = ?", userID, inviteLink.RoomID).First(&existingUser)
	if result.Error == nil {
		http.Error(w, "Already in room", http.StatusConflict)
		return
	}

	var room models.Room
	if err := h.db.First(&room, inviteLink.RoomID).Error; err != nil {
		http.Error(w, "Room not found", http.StatusNotFound)
		return
	}

	userRoom := models.UserRoom{
		UserID: userID,
		RoomID: inviteLink.RoomID,
		Role:   "member",
	}
	if err := h.db.Create(&userRoom).Error; err != nil {
		http.Error(w, "Failed to join room", http.StatusInternalServerError)
		return
	}

	inviteLink.Uses++
	inviteLink.UpdatedAt = time.Now()
	h.db.Save(&inviteLink)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "joined",
		"room_id":   inviteLink.RoomID,
		"room_name": room.Name,
	}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *InviteLinkHandler) inviteLinkToResponse(link *models.InviteLink, room *models.Room) InviteLinkResponse {
	return InviteLinkResponse{
		ID:          link.ID,
		Code:        link.Code,
		RoomID:      link.RoomID,
		RoomName:    room.Name,
		CreatedBy:   link.CreatedBy,
		Title:       link.Title,
		Description: link.Description,
		MaxUses:     link.MaxUses,
		Uses:        link.Uses,
		IsPermanent: link.IsPermanent,
		IsEnabled:   link.IsEnabled,
		ExpiresAt:   link.ExpiresAt,
		CreatedAt:   link.CreatedAt,
		UpdatedAt:   link.UpdatedAt,
	}
}

func (h *InviteLinkHandler) sendInviteLinkResponse(w http.ResponseWriter, link *models.InviteLink, room *models.Room) {
	response := h.inviteLinkToResponse(link, room)
	response.URL = "/invite/" + link.Code

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}
