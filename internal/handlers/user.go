package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
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
