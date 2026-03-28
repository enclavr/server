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

type PresenceHandler struct {
	db *database.Database
}

func NewPresenceHandler(db *database.Database) *PresenceHandler {
	return &PresenceHandler{db: db}
}

type UpdatePresenceRequest struct {
	Status string     `json:"status"`
	RoomID *uuid.UUID `json:"room_id,omitempty"`
}

type PresenceResponse struct {
	UserID   uuid.UUID  `json:"user_id"`
	Username string     `json:"username"`
	Status   string     `json:"status"`
	RoomID   *uuid.UUID `json:"room_id,omitempty"`
	LastSeen time.Time  `json:"last_seen"`
}

func (h *PresenceHandler) UpdatePresence(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req UpdatePresenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var status models.PresenceStatus
	switch req.Status {
	case "online":
		status = models.PresenceOnline
	case "away":
		status = models.PresenceAway
	case "busy":
		status = models.PresenceBusy
	case "offline":
		status = models.PresenceOffline
	default:
		status = models.PresenceOnline
	}

	var presence models.Presence
	result := h.db.First(&presence, "user_id = ?", userID)

	if result.Error != nil {
		presence = models.Presence{
			UserID:   userID,
			Status:   status,
			RoomID:   req.RoomID,
			LastSeen: time.Now(),
		}
		h.db.Create(&presence)
	} else {
		presence.Status = status
		presence.RoomID = req.RoomID
		presence.LastSeen = time.Now()
		h.db.Save(&presence)
	}

	var user models.User
	h.db.First(&user, "id = ?", userID)

	response := PresenceResponse{
		UserID:   presence.UserID,
		Username: user.Username,
		Status:   string(presence.Status),
		RoomID:   presence.RoomID,
		LastSeen: presence.LastSeen,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		return
	}
}

func (h *PresenceHandler) GetPresence(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

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
	if err := h.db.First(&userRoom, "user_id = ? AND room_id = ?", userID, roomID).Error; err != nil {
		http.Error(w, "You are not a member of this room", http.StatusForbidden)
		return
	}

	var presences []models.Presence
	if err := h.db.Joins("JOIN user_rooms ON user_rooms.user_id = presences.user_id").
		Where("user_rooms.room_id = ? AND presences.status != ?", roomID, models.PresenceOffline).
		Find(&presences).Error; err != nil {
		http.Error(w, "Failed to fetch presence", http.StatusInternalServerError)
		return
	}

	// Batch fetch all users at once to avoid N+1 queries
	userIDs := make([]uuid.UUID, 0, len(presences))
	for _, p := range presences {
		userIDs = append(userIDs, p.UserID)
	}
	userMap := make(map[uuid.UUID]models.User)
	if len(userIDs) > 0 {
		var users []models.User
		h.db.Where("id IN ?", userIDs).Find(&users)
		for _, u := range users {
			userMap[u.ID] = u
		}
	}

	var response []PresenceResponse
	for _, p := range presences {
		user := userMap[p.UserID]
		response = append(response, PresenceResponse{
			UserID:   p.UserID,
			Username: user.Username,
			Status:   string(p.Status),
			RoomID:   p.RoomID,
			LastSeen: p.LastSeen,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		return
	}
}

func (h *PresenceHandler) GetUserPresence(w http.ResponseWriter, r *http.Request) {
	targetUserIDStr := r.URL.Query().Get("user_id")
	if targetUserIDStr == "" {
		http.Error(w, "user_id is required", http.StatusBadRequest)
		return
	}

	targetUserID, err := uuid.Parse(targetUserIDStr)
	if err != nil {
		http.Error(w, "Invalid user_id", http.StatusBadRequest)
		return
	}

	var presence models.Presence
	if err := h.db.First(&presence, "user_id = ?", targetUserID).Error; err != nil {
		presence = models.Presence{
			UserID:   targetUserID,
			Status:   models.PresenceOffline,
			LastSeen: time.Now(),
		}
	}

	var user models.User
	h.db.First(&user, "id = ?", targetUserID)

	response := PresenceResponse{
		UserID:   presence.UserID,
		Username: user.Username,
		Status:   string(presence.Status),
		RoomID:   presence.RoomID,
		LastSeen: presence.LastSeen,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		return
	}
}
