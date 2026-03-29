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

// VoiceChannelPermissionHandler manages voice channel permissions.
type VoiceChannelPermissionHandler struct {
	db *database.Database
}

// NewVoiceChannelPermissionHandler creates a new VoiceChannelPermissionHandler.
func NewVoiceChannelPermissionHandler(db *database.Database) *VoiceChannelPermissionHandler {
	return &VoiceChannelPermissionHandler{db: db}
}

// SetPermissionRequest represents the request body for setting a permission.
type SetPermissionRequest struct {
	ChannelID       uuid.UUID `json:"channel_id"`
	UserID          uuid.UUID `json:"user_id"`
	CanJoin         *bool     `json:"can_join,omitempty"`
	CanSpeak        *bool     `json:"can_speak,omitempty"`
	CanMuteOthers   *bool     `json:"can_mute_others,omitempty"`
	CanDeafenOthers *bool     `json:"can_deafen_others,omitempty"`
	CanMoveUsers    *bool     `json:"can_move_users,omitempty"`
	IsPriority      *bool     `json:"is_priority,omitempty"`
}

// PermissionResponse represents the JSON response for a voice channel permission.
type PermissionResponse struct {
	ID              uuid.UUID `json:"id"`
	ChannelID       uuid.UUID `json:"channel_id"`
	UserID          uuid.UUID `json:"user_id"`
	Username        string    `json:"username,omitempty"`
	CanJoin         bool      `json:"can_join"`
	CanSpeak        bool      `json:"can_speak"`
	CanMuteOthers   bool      `json:"can_mute_others"`
	CanDeafenOthers bool      `json:"can_deafen_others"`
	CanMoveUsers    bool      `json:"can_move_users"`
	IsPriority      bool      `json:"is_priority"`
	GrantedBy       uuid.UUID `json:"granted_by"`
	CreatedAt       string    `json:"created_at"`
	UpdatedAt       string    `json:"updated_at"`
}

func sendPermissionJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error encoding permission response: %v", err)
	}
}

// isChannelAdmin checks if the user is a room admin, channel creator, or has moderation permissions.
func (h *VoiceChannelPermissionHandler) isChannelAdmin(userID uuid.UUID, channel *models.VoiceChannel) bool {
	if channel.CreatedBy == userID {
		return true
	}

	var userRoom models.UserRoom
	if err := h.db.Where("user_id = ? AND room_id = ? AND role IN ?",
		userID, channel.RoomID, []string{"owner", "admin"}).First(&userRoom).Error; err == nil {
		return true
	}

	return false
}

// SetPermission creates or updates a user's permissions for a voice channel.
func (h *VoiceChannelPermissionHandler) SetPermission(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req SetPermissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ChannelID == uuid.Nil {
		http.Error(w, "Channel ID is required", http.StatusBadRequest)
		return
	}

	if req.UserID == uuid.Nil {
		http.Error(w, "User ID is required", http.StatusBadRequest)
		return
	}

	var channel models.VoiceChannel
	if err := h.db.Where("id = ?", req.ChannelID).First(&channel).Error; err != nil {
		http.Error(w, "Voice channel not found", http.StatusNotFound)
		return
	}

	if !h.isChannelAdmin(userID, &channel) {
		http.Error(w, "You don't have permission to manage this channel", http.StatusForbidden)
		return
	}

	var targetUserRoom models.UserRoom
	if err := h.db.Where("user_id = ? AND room_id = ?", req.UserID, channel.RoomID).First(&targetUserRoom).Error; err != nil {
		http.Error(w, "Target user is not a member of this room", http.StatusBadRequest)
		return
	}

	var permission models.VoiceChannelPermission
	result := h.db.Where("channel_id = ? AND user_id = ?", req.ChannelID, req.UserID).First(&permission)

	if result.Error != nil {
		permission = models.VoiceChannelPermission{
			ChannelID:       req.ChannelID,
			UserID:          req.UserID,
			CanJoin:         true,
			CanSpeak:        true,
			CanMuteOthers:   false,
			CanDeafenOthers: false,
			CanMoveUsers:    false,
			IsPriority:      false,
			GrantedBy:       userID,
		}

		if req.CanJoin != nil {
			permission.CanJoin = *req.CanJoin
		}
		if req.CanSpeak != nil {
			permission.CanSpeak = *req.CanSpeak
		}
		if req.CanMuteOthers != nil {
			permission.CanMuteOthers = *req.CanMuteOthers
		}
		if req.CanDeafenOthers != nil {
			permission.CanDeafenOthers = *req.CanDeafenOthers
		}
		if req.CanMoveUsers != nil {
			permission.CanMoveUsers = *req.CanMoveUsers
		}
		if req.IsPriority != nil {
			permission.IsPriority = *req.IsPriority
		}

		if err := h.db.Create(&permission).Error; err != nil {
			log.Printf("Error creating voice channel permission: %v", err)
			http.Error(w, "Failed to create permission", http.StatusInternalServerError)
			return
		}
	} else {
		updates := map[string]interface{}{
			"updated_at": time.Now(),
			"granted_by": userID,
		}
		if req.CanJoin != nil {
			updates["can_join"] = *req.CanJoin
		}
		if req.CanSpeak != nil {
			updates["can_speak"] = *req.CanSpeak
		}
		if req.CanMuteOthers != nil {
			updates["can_mute_others"] = *req.CanMuteOthers
		}
		if req.CanDeafenOthers != nil {
			updates["can_deafen_others"] = *req.CanDeafenOthers
		}
		if req.CanMoveUsers != nil {
			updates["can_move_users"] = *req.CanMoveUsers
		}
		if req.IsPriority != nil {
			updates["is_priority"] = *req.IsPriority
		}

		if err := h.db.Model(&permission).Updates(updates).Error; err != nil {
			log.Printf("Error updating voice channel permission: %v", err)
			http.Error(w, "Failed to update permission", http.StatusInternalServerError)
			return
		}

		h.db.Where("channel_id = ? AND user_id = ?", req.ChannelID, req.UserID).First(&permission)
	}

	var targetUser models.User
	h.db.Where("id = ?", req.UserID).First(&targetUser)

	resp := PermissionResponse{
		ID:              permission.ID,
		ChannelID:       permission.ChannelID,
		UserID:          permission.UserID,
		Username:        targetUser.Username,
		CanJoin:         permission.CanJoin,
		CanSpeak:        permission.CanSpeak,
		CanMuteOthers:   permission.CanMuteOthers,
		CanDeafenOthers: permission.CanDeafenOthers,
		CanMoveUsers:    permission.CanMoveUsers,
		IsPriority:      permission.IsPriority,
		GrantedBy:       permission.GrantedBy,
		CreatedAt:       permission.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       permission.UpdatedAt.Format(time.RFC3339),
	}

	sendPermissionJSON(w, http.StatusOK, resp)
}

// GetPermissions returns all permissions for a voice channel.
func (h *VoiceChannelPermissionHandler) GetPermissions(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	channelIDStr := r.URL.Query().Get("channel_id")
	if channelIDStr == "" {
		http.Error(w, "Channel ID is required", http.StatusBadRequest)
		return
	}

	channelID, err := uuid.Parse(channelIDStr)
	if err != nil {
		http.Error(w, "Invalid channel ID", http.StatusBadRequest)
		return
	}

	var channel models.VoiceChannel
	if err := h.db.Where("id = ?", channelID).First(&channel).Error; err != nil {
		http.Error(w, "Voice channel not found", http.StatusNotFound)
		return
	}

	var userRoom models.UserRoom
	if err := h.db.Where("user_id = ? AND room_id = ?", userID, channel.RoomID).First(&userRoom).Error; err != nil {
		http.Error(w, "You are not a member of this room", http.StatusForbidden)
		return
	}

	var permissions []models.VoiceChannelPermission
	h.db.Preload("User").Where("channel_id = ?", channelID).Find(&permissions)

	responses := make([]PermissionResponse, 0, len(permissions))
	for _, p := range permissions {
		responses = append(responses, PermissionResponse{
			ID:              p.ID,
			ChannelID:       p.ChannelID,
			UserID:          p.UserID,
			Username:        p.User.Username,
			CanJoin:         p.CanJoin,
			CanSpeak:        p.CanSpeak,
			CanMuteOthers:   p.CanMuteOthers,
			CanDeafenOthers: p.CanDeafenOthers,
			CanMoveUsers:    p.CanMoveUsers,
			IsPriority:      p.IsPriority,
			GrantedBy:       p.GrantedBy,
			CreatedAt:       p.CreatedAt.Format(time.RFC3339),
			UpdatedAt:       p.UpdatedAt.Format(time.RFC3339),
		})
	}

	sendPermissionJSON(w, http.StatusOK, responses)
}

// GetUserPermission returns a specific user's permission for a voice channel.
func (h *VoiceChannelPermissionHandler) GetUserPermission(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	channelIDStr := r.URL.Query().Get("channel_id")
	if channelIDStr == "" {
		http.Error(w, "Channel ID is required", http.StatusBadRequest)
		return
	}

	channelID, err := uuid.Parse(channelIDStr)
	if err != nil {
		http.Error(w, "Invalid channel ID", http.StatusBadRequest)
		return
	}

	var channel models.VoiceChannel
	if err := h.db.Where("id = ?", channelID).First(&channel).Error; err != nil {
		http.Error(w, "Voice channel not found", http.StatusNotFound)
		return
	}

	var userRoom models.UserRoom
	if err := h.db.Where("user_id = ? AND room_id = ?", userID, channel.RoomID).First(&userRoom).Error; err != nil {
		http.Error(w, "You are not a member of this room", http.StatusForbidden)
		return
	}

	targetUserIDStr := r.URL.Query().Get("user_id")
	if targetUserIDStr == "" {
		targetUserIDStr = userID.String()
	}

	targetUserID, err := uuid.Parse(targetUserIDStr)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	var permission models.VoiceChannelPermission
	if err := h.db.Where("channel_id = ? AND user_id = ?", channelID, targetUserID).First(&permission).Error; err != nil {
		resp := PermissionResponse{
			ChannelID:       channelID,
			UserID:          targetUserID,
			CanJoin:         !channel.IsPrivate,
			CanSpeak:        true,
			CanMuteOthers:   false,
			CanDeafenOthers: false,
			CanMoveUsers:    false,
			IsPriority:      false,
		}
		sendPermissionJSON(w, http.StatusOK, resp)
		return
	}

	var targetUser models.User
	h.db.Where("id = ?", targetUserID).First(&targetUser)

	resp := PermissionResponse{
		ID:              permission.ID,
		ChannelID:       permission.ChannelID,
		UserID:          permission.UserID,
		Username:        targetUser.Username,
		CanJoin:         permission.CanJoin,
		CanSpeak:        permission.CanSpeak,
		CanMuteOthers:   permission.CanMuteOthers,
		CanDeafenOthers: permission.CanDeafenOthers,
		CanMoveUsers:    permission.CanMoveUsers,
		IsPriority:      permission.IsPriority,
		GrantedBy:       permission.GrantedBy,
		CreatedAt:       permission.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       permission.UpdatedAt.Format(time.RFC3339),
	}

	sendPermissionJSON(w, http.StatusOK, resp)
}

// DeletePermission removes a user's permission entry from a voice channel.
func (h *VoiceChannelPermissionHandler) DeletePermission(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	channelIDStr := r.URL.Query().Get("channel_id")
	userIDStr := r.URL.Query().Get("user_id")

	if channelIDStr == "" || userIDStr == "" {
		http.Error(w, "Channel ID and User ID are required", http.StatusBadRequest)
		return
	}

	channelID, err := uuid.Parse(channelIDStr)
	if err != nil {
		http.Error(w, "Invalid channel ID", http.StatusBadRequest)
		return
	}

	targetUserID, err := uuid.Parse(userIDStr)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	var channel models.VoiceChannel
	if err := h.db.Where("id = ?", channelID).First(&channel).Error; err != nil {
		http.Error(w, "Voice channel not found", http.StatusNotFound)
		return
	}

	if !h.isChannelAdmin(userID, &channel) {
		http.Error(w, "You don't have permission to manage this channel", http.StatusForbidden)
		return
	}

	result := h.db.Where("channel_id = ? AND user_id = ?", channelID, targetUserID).Delete(&models.VoiceChannelPermission{})
	if result.RowsAffected == 0 {
		http.Error(w, "Permission not found", http.StatusNotFound)
		return
	}

	if result.Error != nil {
		log.Printf("Error deleting voice channel permission: %v", result.Error)
		http.Error(w, "Failed to delete permission", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(`{"message":"Permission deleted"}`)); err != nil {
		log.Printf("Error writing response: %v", err)
	}
}

// CheckPermission checks if a user can perform a specific action on a voice channel.
func (h *VoiceChannelPermissionHandler) CheckPermission(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	channelIDStr := r.URL.Query().Get("channel_id")
	action := r.URL.Query().Get("action")

	if channelIDStr == "" {
		http.Error(w, "Channel ID is required", http.StatusBadRequest)
		return
	}

	channelID, err := uuid.Parse(channelIDStr)
	if err != nil {
		http.Error(w, "Invalid channel ID", http.StatusBadRequest)
		return
	}

	var channel models.VoiceChannel
	if err := h.db.Where("id = ?", channelID).First(&channel).Error; err != nil {
		http.Error(w, "Voice channel not found", http.StatusNotFound)
		return
	}

	var userRoom models.UserRoom
	if err := h.db.Where("user_id = ? AND room_id = ?", userID, channel.RoomID).First(&userRoom).Error; err != nil {
		sendPermissionJSON(w, http.StatusOK, map[string]interface{}{
			"allowed": false,
			"reason":  "not a room member",
		})
		return
	}

	var permission models.VoiceChannelPermission
	hasExplicit := h.db.Where("channel_id = ? AND user_id = ?", channelID, userID).First(&permission).Error == nil

	allowed := true
	reason := "allowed"

	switch action {
	case "join":
		if hasExplicit {
			allowed = permission.CanJoin
			if !allowed {
				reason = "explicitly denied"
			}
		} else if channel.IsPrivate {
			allowed = false
			reason = "channel is private"
		}
	case "speak":
		if hasExplicit {
			allowed = permission.CanSpeak
			if !allowed {
				reason = "explicitly denied"
			}
		}
	case "mute_others":
		if h.isChannelAdmin(userID, &channel) {
			allowed = true
		} else if hasExplicit {
			allowed = permission.CanMuteOthers
			if !allowed {
				reason = "no mute permission"
			}
		} else {
			allowed = false
			reason = "no mute permission"
		}
	case "deafen_others":
		if h.isChannelAdmin(userID, &channel) {
			allowed = true
		} else if hasExplicit {
			allowed = permission.CanDeafenOthers
			if !allowed {
				reason = "no deafen permission"
			}
		} else {
			allowed = false
			reason = "no deafen permission"
		}
	case "move_users":
		if h.isChannelAdmin(userID, &channel) {
			allowed = true
		} else if hasExplicit {
			allowed = permission.CanMoveUsers
			if !allowed {
				reason = "no move permission"
			}
		} else {
			allowed = false
			reason = "no move permission"
		}
	default:
		if action == "" {
			allowed = true
			reason = "no action specified"
		} else {
			allowed = false
			reason = "unknown action"
		}
	}

	sendPermissionJSON(w, http.StatusOK, map[string]interface{}{
		"allowed":    allowed,
		"action":     action,
		"channel_id": channelID,
		"user_id":    userID,
		"reason":     reason,
	})
}

// BulkSetPermissions sets permissions for multiple users at once.
func (h *VoiceChannelPermissionHandler) BulkSetPermissions(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		ChannelID   uuid.UUID              `json:"channel_id"`
		Permissions []SetPermissionRequest `json:"permissions"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ChannelID == uuid.Nil {
		http.Error(w, "Channel ID is required", http.StatusBadRequest)
		return
	}

	var channel models.VoiceChannel
	if err := h.db.Where("id = ?", req.ChannelID).First(&channel).Error; err != nil {
		http.Error(w, "Voice channel not found", http.StatusNotFound)
		return
	}

	if !h.isChannelAdmin(userID, &channel) {
		http.Error(w, "You don't have permission to manage this channel", http.StatusForbidden)
		return
	}

	responses := make([]PermissionResponse, 0, len(req.Permissions))

	err := h.db.Transaction(func(tx *gorm.DB) error {
		for _, perm := range req.Permissions {
			if perm.UserID == uuid.Nil {
				continue
			}

			var existing models.VoiceChannelPermission
			result := tx.Where("channel_id = ? AND user_id = ?", req.ChannelID, perm.UserID).First(&existing)

			if result.Error != nil {
				newPerm := models.VoiceChannelPermission{
					ChannelID:       req.ChannelID,
					UserID:          perm.UserID,
					CanJoin:         true,
					CanSpeak:        true,
					CanMuteOthers:   false,
					CanDeafenOthers: false,
					CanMoveUsers:    false,
					IsPriority:      false,
					GrantedBy:       userID,
				}
				if perm.CanJoin != nil {
					newPerm.CanJoin = *perm.CanJoin
				}
				if perm.CanSpeak != nil {
					newPerm.CanSpeak = *perm.CanSpeak
				}
				if perm.CanMuteOthers != nil {
					newPerm.CanMuteOthers = *perm.CanMuteOthers
				}
				if perm.CanDeafenOthers != nil {
					newPerm.CanDeafenOthers = *perm.CanDeafenOthers
				}
				if perm.CanMoveUsers != nil {
					newPerm.CanMoveUsers = *perm.CanMoveUsers
				}
				if perm.IsPriority != nil {
					newPerm.IsPriority = *perm.IsPriority
				}

				if err := tx.Create(&newPerm).Error; err != nil {
					return err
				}
				existing = newPerm
			} else {
				updates := map[string]interface{}{
					"updated_at": time.Now(),
					"granted_by": userID,
				}
				if perm.CanJoin != nil {
					updates["can_join"] = *perm.CanJoin
				}
				if perm.CanSpeak != nil {
					updates["can_speak"] = *perm.CanSpeak
				}
				if perm.CanMuteOthers != nil {
					updates["can_mute_others"] = *perm.CanMuteOthers
				}
				if perm.CanDeafenOthers != nil {
					updates["can_deafen_others"] = *perm.CanDeafenOthers
				}
				if perm.CanMoveUsers != nil {
					updates["can_move_users"] = *perm.CanMoveUsers
				}
				if perm.IsPriority != nil {
					updates["is_priority"] = *perm.IsPriority
				}

				if err := tx.Model(&existing).Updates(updates).Error; err != nil {
					return err
				}
				tx.Where("channel_id = ? AND user_id = ?", req.ChannelID, perm.UserID).First(&existing)
			}

			var targetUser models.User
			tx.Where("id = ?", perm.UserID).First(&targetUser)

			responses = append(responses, PermissionResponse{
				ID:              existing.ID,
				ChannelID:       existing.ChannelID,
				UserID:          existing.UserID,
				Username:        targetUser.Username,
				CanJoin:         existing.CanJoin,
				CanSpeak:        existing.CanSpeak,
				CanMuteOthers:   existing.CanMuteOthers,
				CanDeafenOthers: existing.CanDeafenOthers,
				CanMoveUsers:    existing.CanMoveUsers,
				IsPriority:      existing.IsPriority,
				GrantedBy:       existing.GrantedBy,
				CreatedAt:       existing.CreatedAt.Format(time.RFC3339),
				UpdatedAt:       existing.UpdatedAt.Format(time.RFC3339),
			})
		}
		return nil
	})

	if err != nil {
		log.Printf("Error bulk setting permissions: %v", err)
		http.Error(w, "Failed to set permissions", http.StatusInternalServerError)
		return
	}

	sendPermissionJSON(w, http.StatusOK, responses)
}
