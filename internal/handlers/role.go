package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
)

type RoleHandler struct {
	db *database.Database
}

func NewRoleHandler(db *database.Database) *RoleHandler {
	return &RoleHandler{db: db}
}

type Role struct {
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
}

var DefaultRoles = map[string]Role{
	"owner": {
		Name: "owner",
		Permissions: []string{
			"manage_messages",
			"manage_users",
			"manage_settings",
			"delete_room",
			"invite_users",
			"pin_messages",
			"kick_users",
		},
	},
	"admin": {
		Name: "admin",
		Permissions: []string{
			"manage_messages",
			"manage_users",
			"invite_users",
			"pin_messages",
			"kick_users",
		},
	},
	"moderator": {
		Name: "moderator",
		Permissions: []string{
			"manage_messages",
			"kick_users",
			"pin_messages",
		},
	},
	"member": {
		Name: "member",
		Permissions: []string{
			"send_messages",
			"use_voice",
			"react",
		},
	},
	"guest": {
		Name: "guest",
		Permissions: []string{
			"view_messages",
			"use_voice",
		},
	},
}

type UpdateRoleRequest struct {
	UserID uuid.UUID `json:"user_id"`
	RoomID uuid.UUID `json:"room_id"`
	Role   string    `json:"role"`
}

type RoomMemberResponse struct {
	UserID    uuid.UUID `json:"user_id"`
	Username  string    `json:"username"`
	AvatarURL string    `json:"avatar_url"`
	Role      string    `json:"role"`
	JoinedAt  string    `json:"joined_at"`
}

func (h *RoleHandler) GetMembers(w http.ResponseWriter, r *http.Request) {
	roomIDStr := r.URL.Query().Get("room_id")
	if roomIDStr == "" {
		http.Error(w, "room_id is required", http.StatusBadRequest)
		return
	}

	roomID, err := uuid.Parse(roomIDStr)
	if err != nil {
		http.Error(w, "Invalid room_id", http.StatusBadRequest)
		return
	}

	var userRooms []models.UserRoom
	if err := h.db.Where("room_id = ?", roomID).Find(&userRooms).Error; err != nil {
		http.Error(w, "Failed to fetch members", http.StatusInternalServerError)
		return
	}

	// Batch fetch all users at once to avoid N+1 queries
	userIDs := make([]uuid.UUID, 0, len(userRooms))
	for _, ur := range userRooms {
		userIDs = append(userIDs, ur.UserID)
	}
	userMap := make(map[uuid.UUID]models.User)
	if len(userIDs) > 0 {
		var users []models.User
		h.db.Where("id IN ?", userIDs).Find(&users)
		for _, u := range users {
			userMap[u.ID] = u
		}
	}

	members := make([]RoomMemberResponse, 0, len(userRooms))
	for _, ur := range userRooms {
		user, ok := userMap[ur.UserID]
		if !ok {
			continue
		}
		members = append(members, RoomMemberResponse{
			UserID:    user.ID,
			Username:  user.Username,
			AvatarURL: user.AvatarURL,
			Role:      ur.Role,
			JoinedAt:  ur.JoinedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(members); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *RoleHandler) GetUserRole(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	roomIDStr := r.URL.Query().Get("room_id")
	if roomIDStr == "" {
		http.Error(w, "room_id is required", http.StatusBadRequest)
		return
	}

	roomID, err := uuid.Parse(roomIDStr)
	if err != nil {
		http.Error(w, "Invalid room_id", http.StatusBadRequest)
		return
	}

	var userRoom models.UserRoom
	result := h.db.Where("user_id = ? AND room_id = ?", userID, roomID).First(&userRoom)
	if result.Error != nil {
		http.Error(w, "Not a member of this room", http.StatusNotFound)
		return
	}

	roleInfo := Role{
		Name:        userRoom.Role,
		Permissions: GetPermissionsForRole(userRoom.Role),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(roleInfo); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *RoleHandler) UpdateRole(w http.ResponseWriter, r *http.Request) {
	currentUserID := middleware.GetUserID(r)
	if currentUserID == uuid.Nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req UpdateRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if _, ok := DefaultRoles[req.Role]; !ok {
		http.Error(w, "Invalid role", http.StatusBadRequest)
		return
	}

	var currentUserRoom models.UserRoom
	if err := h.db.Where("user_id = ? AND room_id = ?", currentUserID, req.RoomID).First(&currentUserRoom).Error; err != nil {
		http.Error(w, "You are not a member of this room", http.StatusForbidden)
		return
	}

	if !CanManageUsers(currentUserRoom.Role) {
		http.Error(w, "You don't have permission to manage users", http.StatusForbidden)
		return
	}

	if req.UserID == currentUserID {
		http.Error(w, "Cannot change your own role", http.StatusBadRequest)
		return
	}

	var targetUserRoom models.UserRoom
	if err := h.db.Where("user_id = ? AND room_id = ?", req.UserID, req.RoomID).First(&targetUserRoom).Error; err != nil {
		http.Error(w, "Target user is not a member of this room", http.StatusNotFound)
		return
	}

	if !CanManageHigherRole(currentUserRoom.Role, targetUserRoom.Role) {
		http.Error(w, "Cannot modify role of user with higher or equal rank", http.StatusForbidden)
		return
	}

	if err := h.db.Model(&targetUserRoom).Update("role", req.Role).Error; err != nil {
		http.Error(w, "Failed to update role", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "updated", "role": req.Role}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *RoleHandler) KickUser(w http.ResponseWriter, r *http.Request) {
	currentUserID := middleware.GetUserID(r)

	var req struct {
		UserID uuid.UUID `json:"user_id"`
		RoomID uuid.UUID `json:"room_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var currentUserRoom models.UserRoom
	if err := h.db.Where("user_id = ? AND room_id = ?", currentUserID, req.RoomID).First(&currentUserRoom).Error; err != nil {
		http.Error(w, "You are not a member of this room", http.StatusForbidden)
		return
	}

	if !CanManageUsers(currentUserRoom.Role) {
		http.Error(w, "You don't have permission to kick users", http.StatusForbidden)
		return
	}

	var targetUserRoom models.UserRoom
	if err := h.db.Where("user_id = ? AND room_id = ?", req.UserID, req.RoomID).First(&targetUserRoom).Error; err != nil {
		http.Error(w, "Target user is not a member of this room", http.StatusNotFound)
		return
	}

	if !CanManageHigherRole(currentUserRoom.Role, targetUserRoom.Role) {
		http.Error(w, "Cannot kick user with higher or equal rank", http.StatusForbidden)
		return
	}

	if err := h.db.Delete(&targetUserRoom).Error; err != nil {
		http.Error(w, "Failed to kick user", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "kicked"}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *RoleHandler) GetRoles(w http.ResponseWriter, r *http.Request) {
	roles := make([]Role, 0, len(DefaultRoles))
	for _, role := range DefaultRoles {
		roles = append(roles, role)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(roles); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func GetPermissionsForRole(role string) []string {
	if r, ok := DefaultRoles[role]; ok {
		return r.Permissions
	}
	return DefaultRoles["guest"].Permissions
}

func HasPermission(role string, permission string) bool {
	perms := GetPermissionsForRole(role)
	for _, p := range perms {
		if p == permission {
			return true
		}
	}
	return false
}

func CanManageUsers(role string) bool {
	return HasPermission(role, "manage_users")
}

func CanManageMessages(role string) bool {
	return HasPermission(role, "manage_messages")
}

func CanInviteUsers(role string) bool {
	return HasPermission(role, "invite_users")
}

func CanKickUsers(role string) bool {
	return HasPermission(role, "kick_users")
}

func CanPinMessages(role string) bool {
	return HasPermission(role, "pin_messages")
}

var roleHierarchy = map[string]int{
	"owner":     4,
	"admin":     3,
	"moderator": 2,
	"member":    1,
	"guest":     0,
}

func CanManageHigherRole(managerRole, targetRole string) bool {
	managerLevel, ok1 := roleHierarchy[managerRole]
	targetLevel, ok2 := roleHierarchy[targetRole]
	if !ok1 || !ok2 {
		return false
	}
	return managerLevel > targetLevel
}
