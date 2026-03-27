package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
)

type UserHandler struct {
	db *database.Database
}

func NewUserHandler(db *database.Database) *UserHandler {
	return &UserHandler{db: db}
}

type UserSearchResult struct {
	ID          uuid.UUID `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	AvatarURL   string    `json:"avatar_url"`
}

type UpdateUserRequest struct {
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
}

type UserUpdateResponse struct {
	ID          uuid.UUID `json:"id"`
	Username    string    `json:"username"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	AvatarURL   string    `json:"avatar_url"`
	IsAdmin     bool      `json:"is_admin"`
}

type UserProfileResponse struct {
	ID          uuid.UUID `json:"id"`
	Username    string    `json:"username"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	AvatarURL   string    `json:"avatar_url"`
	CreatedAt   string    `json:"created_at"`
	IsAdmin     bool      `json:"is_admin"`
	Stats       UserStats `json:"stats"`
}

type UserStats struct {
	RoomsJoined    int64 `json:"rooms_joined"`
	MessagesSent   int64 `json:"messages_sent"`
	DMsReceived    int64 `json:"dms_received"`
	ReactionsGiven int64 `json:"reactions_given"`
}

func (h *UserHandler) SearchUsers(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("username")
	if query == "" {
		http.Error(w, "Query parameter required", http.StatusBadRequest)
		return
	}

	var users []models.User
	if err := h.db.Where("LOWER(username) LIKE ?", "%"+query+"%").Limit(10).Find(&users).Error; err != nil {
		http.Error(w, "Failed to search users", http.StatusInternalServerError)
		return
	}

	results := make([]UserSearchResult, 0, len(users))
	for _, user := range users {
		results = append(results, UserSearchResult{
			ID:          user.ID,
			Username:    user.Username,
			DisplayName: user.DisplayName,
			AvatarURL:   user.AvatarURL,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(results); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func (h *UserHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	if req.DisplayName != "" {
		user.DisplayName = req.DisplayName
	}
	if req.AvatarURL != "" {
		user.AvatarURL = req.AvatarURL
	}

	if err := h.db.Save(&user).Error; err != nil {
		http.Error(w, "Failed to update user", http.StatusInternalServerError)
		return
	}

	sanitized := UserResponse{
		ID:          user.ID,
		Username:    user.Username,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		AvatarURL:   user.AvatarURL,
		IsAdmin:     user.IsAdmin,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(sanitized); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func (h *UserHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	var stats UserStats
	h.db.Model(&models.UserRoom{}).Where("user_id = ?", userID).Count(&stats.RoomsJoined)
	h.db.Model(&models.Message{}).Where("user_id = ?", userID).Count(&stats.MessagesSent)
	h.db.Model(&models.DirectMessage{}).Where("receiver_id = ?", userID).Count(&stats.DMsReceived)
	h.db.Model(&models.MessageReaction{}).Where("user_id = ?", userID).Count(&stats.ReactionsGiven)

	profile := UserProfileResponse{
		ID:        user.ID,
		Username:  user.Username,
		Email:     user.Email,
		CreatedAt: user.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		IsAdmin:   user.IsAdmin,
		Stats:     stats,
	}

	if user.DisplayName != "" {
		profile.DisplayName = user.DisplayName
	}
	if user.AvatarURL != "" {
		profile.AvatarURL = user.AvatarURL
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(profile); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}
