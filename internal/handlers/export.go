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

type ExportHandler struct {
	db *database.Database
}

func NewExportHandler(db *database.Database) *ExportHandler {
	return &ExportHandler{db: db}
}

type ServerExport struct {
	ExportDate time.Time        `json:"export_date"`
	Version    string           `json:"version"`
	Users      []UserExport     `json:"users"`
	Rooms      []RoomExport     `json:"rooms"`
	Categories []CategoryExport `json:"categories"`
	Messages   []MessageExport  `json:"messages"`
	Invites    []InviteExport   `json:"invites"`
}

type UserExport struct {
	ID          uuid.UUID `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	AvatarURL   string    `json:"avatar_url"`
	IsAdmin     bool      `json:"is_admin"`
	CreatedAt   time.Time `json:"created_at"`
}

type RoomExport struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	IsPrivate   bool       `json:"is_private"`
	MaxUsers    int        `json:"max_users"`
	CategoryID  *uuid.UUID `json:"category_id"`
	CreatedAt   time.Time  `json:"created_at"`
}

type CategoryExport struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
}

type MessageExport struct {
	ID        uuid.UUID `json:"id"`
	RoomID    uuid.UUID `json:"room_id"`
	UserID    uuid.UUID `json:"user_id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type InviteExport struct {
	ID        uuid.UUID `json:"id"`
	Code      string    `json:"code"`
	RoomID    uuid.UUID `json:"room_id"`
	CreatedBy uuid.UUID `json:"created_by"`
	ExpiresAt time.Time `json:"expires_at"`
	MaxUses   int       `json:"max_uses"`
	Uses      int       `json:"uses"`
	IsRevoked bool      `json:"is_revoked"`
	CreatedAt time.Time `json:"created_at"`
}

func (h *ExportHandler) ExportServer(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var user models.User
	if err := h.db.DB.First(&user, "id = ?", userID).Error; err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	if !user.IsAdmin {
		http.Error(w, "Admin access required", http.StatusForbidden)
		return
	}

	export := ServerExport{
		ExportDate: time.Now(),
		Version:    "1.0.0",
	}

	var users []models.User
	h.db.DB.Select("id, username, display_name, avatar_url, is_admin, created_at").Find(&users)
	for _, u := range users {
		export.Users = append(export.Users, UserExport{
			ID:          u.ID,
			Username:    u.Username,
			DisplayName: u.DisplayName,
			AvatarURL:   u.AvatarURL,
			IsAdmin:     u.IsAdmin,
			CreatedAt:   u.CreatedAt,
		})
	}

	var rooms []models.Room
	h.db.DB.Select("id, name, description, is_private, max_users, category_id, created_at").Find(&rooms)
	for _, r := range rooms {
		export.Rooms = append(export.Rooms, RoomExport{
			ID:          r.ID,
			Name:        r.Name,
			Description: r.Description,
			IsPrivate:   r.IsPrivate,
			MaxUsers:    r.MaxUsers,
			CategoryID:  r.CategoryID,
			CreatedAt:   r.CreatedAt,
		})
	}

	var categories []models.Category
	h.db.DB.Select("id, name, sort_order, created_at").Find(&categories)
	for _, c := range categories {
		export.Categories = append(export.Categories, CategoryExport{
			ID:        c.ID,
			Name:      c.Name,
			SortOrder: c.SortOrder,
			CreatedAt: c.CreatedAt,
		})
	}

	var messages []models.Message
	h.db.DB.Select("id, room_id, user_id, content, created_at, updated_at").Find(&messages)
	for _, m := range messages {
		export.Messages = append(export.Messages, MessageExport{
			ID:        m.ID,
			RoomID:    m.RoomID,
			UserID:    m.UserID,
			Content:   m.Content,
			CreatedAt: m.CreatedAt,
			UpdatedAt: m.UpdatedAt,
		})
	}

	var invites []models.Invite
	h.db.DB.Select("id, code, room_id, created_by, expires_at, max_uses, uses, is_revoked, created_at").Find(&invites)
	for _, i := range invites {
		export.Invites = append(export.Invites, InviteExport{
			ID:        i.ID,
			Code:      i.Code,
			RoomID:    i.RoomID,
			CreatedBy: i.CreatedBy,
			ExpiresAt: i.ExpiresAt,
			MaxUses:   i.MaxUses,
			Uses:      i.Uses,
			IsRevoked: i.IsRevoked,
			CreatedAt: i.CreatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=server-export.json")
	if err := json.NewEncoder(w).Encode(export); err != nil {
		http.Error(w, "Failed to encode export", http.StatusInternalServerError)
		return
	}
}
