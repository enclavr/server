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

type BanHandler struct {
	db *database.Database
}

func NewBanHandler(db *database.Database) *BanHandler {
	return &BanHandler{db: db}
}

type CreateBanRequest struct {
	UserID    uuid.UUID  `json:"user_id"`
	RoomID    uuid.UUID  `json:"room_id"`
	Reason    string     `json:"reason"`
	ExpiresAt *time.Time `json:"expires_at"`
}

type UpdateBanRequest struct {
	Reason    string     `json:"reason"`
	ExpiresAt *time.Time `json:"expires_at"`
}

func (h *BanHandler) CreateBan(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req CreateBanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.UserID == uuid.Nil {
		http.Error(w, "user_id is required", http.StatusBadRequest)
		return
	}

	if req.RoomID == uuid.Nil {
		http.Error(w, "room_id is required", http.StatusBadRequest)
		return
	}

	var room models.Room
	if err := h.db.First(&room, req.RoomID).Error; err != nil {
		http.Error(w, "Room not found", http.StatusNotFound)
		return
	}

	var requesterRoom models.UserRoom
	if err := h.db.Where("user_id = ? AND room_id = ?", userID, req.RoomID).First(&requesterRoom).Error; err != nil {
		http.Error(w, "You are not a member of this room", http.StatusForbidden)
		return
	}
	if requesterRoom.Role != "owner" && requesterRoom.Role != "admin" {
		http.Error(w, "Only room owners and admins can ban users", http.StatusForbidden)
		return
	}

	var user models.User
	if err := h.db.First(&user, req.UserID).Error; err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	var existingBan models.Ban
	if err := h.db.Where("user_id = ? AND room_id = ? AND deleted_at IS NULL", req.UserID, req.RoomID).First(&existingBan).Error; err == nil {
		http.Error(w, "User is already banned from this room", http.StatusConflict)
		return
	}

	ban := models.Ban{
		UserID:    req.UserID,
		RoomID:    req.RoomID,
		BannedBy:  userID,
		Reason:    req.Reason,
		ExpiresAt: req.ExpiresAt,
	}

	if err := h.db.Create(&ban).Error; err != nil {
		http.Error(w, "Failed to create ban", http.StatusInternalServerError)
		return
	}

	auditLog := models.AuditLog{
		UserID:     userID,
		Action:     models.AuditActionUserBan,
		TargetType: "user",
		TargetID:   req.UserID,
		Details:    "Banned from room: " + room.Name,
	}
	h.db.Create(&auditLog)

	if err := h.db.Where("user_id = ? AND room_id = ?", req.UserID, req.RoomID).Delete(&models.UserRoom{}).Error; err != nil {
		http.Error(w, "Failed to remove user from room", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"message": "ban created successfully", "ban": ban})
}

func (h *BanHandler) GetBans(w http.ResponseWriter, r *http.Request) {
	roomID := r.URL.Query().Get("room_id")
	if roomID == "" {
		http.Error(w, "room_id is required", http.StatusBadRequest)
		return
	}

	roomUUID, err := uuid.Parse(roomID)
	if err != nil {
		http.Error(w, "Invalid room_id", http.StatusBadRequest)
		return
	}

	var bans []models.Ban
	if err := h.db.Where("room_id = ? AND deleted_at IS NULL", roomUUID).Find(&bans).Error; err != nil {
		http.Error(w, "Failed to fetch bans", http.StatusInternalServerError)
		return
	}

	type BanResponse struct {
		ID        uuid.UUID  `json:"id"`
		UserID    uuid.UUID  `json:"user_id"`
		RoomID    uuid.UUID  `json:"room_id"`
		BannedBy  uuid.UUID  `json:"banned_by"`
		Reason    string     `json:"reason"`
		ExpiresAt *time.Time `json:"expires_at"`
		CreatedAt time.Time  `json:"created_at"`
		User      struct {
			ID          uuid.UUID `json:"id"`
			Username    string    `json:"username"`
			DisplayName string    `json:"display_name"`
			AvatarURL   string    `json:"avatar_url"`
		} `json:"user"`
	}

	response := make([]BanResponse, len(bans))

	userIDs := make([]uuid.UUID, len(bans))
	for i, ban := range bans {
		userIDs[i] = ban.UserID
	}

	userMap := make(map[uuid.UUID]models.User)
	var users []models.User
	if len(userIDs) > 0 {
		if err := h.db.Where("id IN ?", userIDs).Find(&users).Error; err == nil {
			for _, u := range users {
				userMap[u.ID] = u
			}
		}
	}

	for i, ban := range bans {
		bannedUser := userMap[ban.UserID]

		response[i] = BanResponse{
			ID:        ban.ID,
			UserID:    ban.UserID,
			RoomID:    ban.RoomID,
			BannedBy:  ban.BannedBy,
			Reason:    ban.Reason,
			ExpiresAt: ban.ExpiresAt,
			CreatedAt: ban.CreatedAt,
			User: struct {
				ID          uuid.UUID `json:"id"`
				Username    string    `json:"username"`
				DisplayName string    `json:"display_name"`
				AvatarURL   string    `json:"avatar_url"`
			}{
				ID:          bannedUser.ID,
				Username:    bannedUser.Username,
				DisplayName: bannedUser.DisplayName,
				AvatarURL:   bannedUser.AvatarURL,
			},
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"bans": response})
}

func (h *BanHandler) GetBan(w http.ResponseWriter, r *http.Request) {
	banID := r.URL.Query().Get("id")
	if banID == "" {
		http.Error(w, "ban_id is required", http.StatusBadRequest)
		return
	}

	banUUID, err := uuid.Parse(banID)
	if err != nil {
		http.Error(w, "Invalid ban_id", http.StatusBadRequest)
		return
	}

	var ban models.Ban
	if err := h.db.First(&ban, banUUID).Error; err != nil {
		http.Error(w, "Ban not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ban": ban})
}

func (h *BanHandler) UpdateBan(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	banID := r.URL.Query().Get("id")
	if banID == "" {
		http.Error(w, "ban_id is required", http.StatusBadRequest)
		return
	}

	banUUID, err := uuid.Parse(banID)
	if err != nil {
		http.Error(w, "Invalid ban_id", http.StatusBadRequest)
		return
	}

	var req UpdateBanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var ban models.Ban
	if err := h.db.First(&ban, banUUID).Error; err != nil {
		http.Error(w, "Ban not found", http.StatusNotFound)
		return
	}

	var requesterRoom models.UserRoom
	if err := h.db.Where("user_id = ? AND room_id = ?", userID, ban.RoomID).First(&requesterRoom).Error; err != nil {
		http.Error(w, "You are not a member of this room", http.StatusForbidden)
		return
	}
	if requesterRoom.Role != "owner" && requesterRoom.Role != "admin" {
		http.Error(w, "Only room owners and admins can update bans", http.StatusForbidden)
		return
	}

	if req.Reason != "" {
		ban.Reason = req.Reason
	}
	if req.ExpiresAt != nil {
		ban.ExpiresAt = req.ExpiresAt
	}

	if err := h.db.Save(&ban).Error; err != nil {
		http.Error(w, "Failed to update ban", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"message": "ban updated successfully", "ban": ban})
}

func (h *BanHandler) DeleteBan(w http.ResponseWriter, r *http.Request) {
	banID := r.URL.Query().Get("id")
	if banID == "" {
		http.Error(w, "ban_id is required", http.StatusBadRequest)
		return
	}

	banUUID, err := uuid.Parse(banID)
	if err != nil {
		http.Error(w, "Invalid ban_id", http.StatusBadRequest)
		return
	}

	userID := middleware.GetUserID(r)

	var ban models.Ban
	if err := h.db.First(&ban, banUUID).Error; err != nil {
		http.Error(w, "Ban not found", http.StatusNotFound)
		return
	}

	var requesterRoom models.UserRoom
	if err := h.db.Where("user_id = ? AND room_id = ?", userID, ban.RoomID).First(&requesterRoom).Error; err != nil {
		http.Error(w, "You are not a member of this room", http.StatusForbidden)
		return
	}
	if requesterRoom.Role != "owner" && requesterRoom.Role != "admin" {
		http.Error(w, "Only room owners and admins can delete bans", http.StatusForbidden)
		return
	}

	if err := h.db.Delete(&ban).Error; err != nil {
		http.Error(w, "Failed to delete ban", http.StatusInternalServerError)
		return
	}

	var room models.Room
	h.db.First(&room, ban.RoomID)

	auditLog := models.AuditLog{
		UserID:     userID,
		Action:     models.AuditActionUserUnban,
		TargetType: "user",
		TargetID:   ban.UserID,
		Details:    "Unbanned from room: " + room.Name,
	}
	h.db.Create(&auditLog)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"message": "ban removed successfully"})
}

func (h *BanHandler) CheckUserBan(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	roomID := r.URL.Query().Get("room_id")

	if userID == "" || roomID == "" {
		http.Error(w, "user_id and room_id are required", http.StatusBadRequest)
		return
	}

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		http.Error(w, "Invalid user_id", http.StatusBadRequest)
		return
	}

	roomUUID, err := uuid.Parse(roomID)
	if err != nil {
		http.Error(w, "Invalid room_id", http.StatusBadRequest)
		return
	}

	var ban models.Ban
	if err := h.db.Where("user_id = ? AND room_id = ? AND deleted_at IS NULL", userUUID, roomUUID).First(&ban).Error; err != nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"banned": false})
		return
	}

	if ban.ExpiresAt != nil && time.Now().After(*ban.ExpiresAt) {
		h.db.Delete(&ban)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"banned": false})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"banned": true, "ban": ban})
}
