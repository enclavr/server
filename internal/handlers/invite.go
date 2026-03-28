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
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type InviteHandler struct {
	db *database.Database
}

func NewInviteHandler(db *database.Database) *InviteHandler {
	return &InviteHandler{db: db}
}

type CreateInviteRequest struct {
	RoomID    *uuid.UUID `json:"room_id"`
	MaxUses   int        `json:"max_uses"`
	ExpiresIn int        `json:"expires_in"`
}

type InviteResponse struct {
	ID        uuid.UUID `json:"id"`
	Code      string    `json:"code"`
	RoomID    uuid.UUID `json:"room_id"`
	RoomName  string    `json:"room_name"`
	CreatedBy uuid.UUID `json:"created_by"`
	ExpiresAt string    `json:"expires_at"`
	MaxUses   int       `json:"max_uses"`
	Uses      int       `json:"uses"`
	IsRevoked bool      `json:"is_revoked"`
	CreatedAt string    `json:"created_at"`
}

func (h *InviteHandler) CreateInvite(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req CreateInviteRequest
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
		http.Error(w, "You must be a member of the room to create an invite", http.StatusForbidden)
		return
	}

	invite := models.Invite{
		RoomID:    *req.RoomID,
		CreatedBy: userID,
		MaxUses:   req.MaxUses,
	}

	if req.ExpiresIn > 0 {
		invite.ExpiresAt = time.Now().Add(time.Duration(req.ExpiresIn) * time.Hour)
	}

	if err := h.db.Create(&invite).Error; err != nil {
		http.Error(w, "Failed to create invite", http.StatusInternalServerError)
		return
	}

	h.sendInviteResponse(w, &invite, &room)
}

func (h *InviteHandler) GetInvites(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "You must be a member of the room to view invites", http.StatusForbidden)
		return
	}

	var invites []models.Invite
	if err := h.db.Where("room_id = ?", roomID).Find(&invites).Error; err != nil {
		http.Error(w, "Failed to fetch invites", http.StatusInternalServerError)
		return
	}

	responses := make([]InviteResponse, 0, len(invites))
	for _, invite := range invites {
		var room models.Room
		h.db.First(&room, invite.RoomID)
		responses = append(responses, h.inviteToResponse(&invite, &room))
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(responses); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *InviteHandler) UseInvite(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Code == "" {
		http.Error(w, "Invite code is required", http.StatusBadRequest)
		return
	}

	var invite models.Invite
	if err := h.db.Where("code = ?", req.Code).First(&invite).Error; err != nil {
		http.Error(w, "Invalid invite code", http.StatusNotFound)
		return
	}

	if invite.IsRevoked {
		http.Error(w, "This invite has been revoked", http.StatusForbidden)
		return
	}

	if !invite.ExpiresAt.IsZero() && time.Now().After(invite.ExpiresAt) {
		http.Error(w, "This invite has expired", http.StatusForbidden)
		return
	}

	var existingUser models.UserRoom
	result := h.db.Where("user_id = ? AND room_id = ?", userID, invite.RoomID).First(&existingUser)
	if result.Error == nil {
		http.Error(w, "Already in room", http.StatusConflict)
		return
	}

	var room models.Room
	if err := h.db.First(&room, invite.RoomID).Error; err != nil {
		http.Error(w, "Room not found", http.StatusNotFound)
		return
	}

	if room.IsPrivate && room.Password != "" {
		var pwReq struct {
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&pwReq); err != nil || pwReq.Password == "" {
			http.Error(w, "Password required for private room", http.StatusForbidden)
			return
		}
		if err := bcrypt.CompareHashAndPassword([]byte(room.Password), []byte(pwReq.Password)); err != nil {
			http.Error(w, "Invalid password", http.StatusForbidden)
			return
		}
	}

	tx := h.db.Begin()

	if invite.MaxUses > 0 {
		result := tx.Model(&models.Invite{}).
			Where("id = ? AND uses < ?", invite.ID, invite.MaxUses).
			Update("uses", gorm.Expr("uses + 1"))
		if result.Error != nil {
			tx.Rollback()
			http.Error(w, "Failed to use invite", http.StatusInternalServerError)
			return
		}
		if result.RowsAffected == 0 {
			tx.Rollback()
			http.Error(w, "This invite has reached its maximum uses", http.StatusForbidden)
			return
		}
	} else {
		tx.Model(&invite).Update("uses", gorm.Expr("uses + 1"))
	}

	userRoom := models.UserRoom{
		UserID: userID,
		RoomID: invite.RoomID,
		Role:   "member",
	}
	if err := tx.Create(&userRoom).Error; err != nil {
		tx.Rollback()
		http.Error(w, "Failed to join room", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit().Error; err != nil {
		http.Error(w, "Failed to join room", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "joined",
		"room_id":   invite.RoomID,
		"room_name": room.Name,
	}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *InviteHandler) RevokeInvite(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		InviteID uuid.UUID `json:"invite_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var invite models.Invite
	if err := h.db.First(&invite, req.InviteID).Error; err != nil {
		http.Error(w, "Invite not found", http.StatusNotFound)
		return
	}

	var userRoom models.UserRoom
	if err := h.db.Where("user_id = ? AND room_id = ?", userID, invite.RoomID).First(&userRoom).Error; err != nil {
		http.Error(w, "You must be a member of the room to revoke invites", http.StatusForbidden)
		return
	}

	if userRoom.Role != "owner" && userRoom.Role != "admin" {
		http.Error(w, "Only room owners and admins can revoke invites", http.StatusForbidden)
		return
	}

	invite.IsRevoked = true
	h.db.Save(&invite)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "revoked"}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *InviteHandler) inviteToResponse(invite *models.Invite, room *models.Room) InviteResponse {
	return InviteResponse{
		ID:        invite.ID,
		Code:      invite.Code,
		RoomID:    invite.RoomID,
		RoomName:  room.Name,
		CreatedBy: invite.CreatedBy,
		ExpiresAt: invite.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
		MaxUses:   invite.MaxUses,
		Uses:      invite.Uses,
		IsRevoked: invite.IsRevoked,
		CreatedAt: invite.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func (h *InviteHandler) sendInviteResponse(w http.ResponseWriter, invite *models.Invite, room *models.Room) {
	response := h.inviteToResponse(invite, room)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}
