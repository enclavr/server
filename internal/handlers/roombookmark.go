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

type RoomBookmarkHandler struct {
	db *database.Database
}

func NewRoomBookmarkHandler(db *database.Database) *RoomBookmarkHandler {
	return &RoomBookmarkHandler{db: db}
}

type CreateRoomBookmarkRequest struct {
	RoomID   uuid.UUID `json:"room_id"`
	Note     string    `json:"note"`
	Position int       `json:"position"`
}

type UpdateRoomBookmarkRequest struct {
	Note     *string `json:"note"`
	Position *int    `json:"position"`
}

type RoomBookmarkResponse struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	RoomID    uuid.UUID `json:"room_id"`
	Note      string    `json:"note"`
	Position  int       `json:"position"`
	CreatedAt string    `json:"created_at"`
	UpdatedAt string    `json:"updated_at"`
}

func (h *RoomBookmarkHandler) GetRoomBookmarks(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var bookmarks []models.RoomBookmark
	if err := h.db.Where("user_id = ?", userID).
		Order("position ASC, created_at DESC").
		Find(&bookmarks).Error; err != nil {
		log.Printf("Error fetching room bookmarks: %v", err)
		http.Error(w, "Failed to fetch room bookmarks", http.StatusInternalServerError)
		return
	}

	results := make([]RoomBookmarkResponse, 0, len(bookmarks))
	for _, b := range bookmarks {
		results = append(results, RoomBookmarkResponse{
			ID:        b.ID,
			UserID:    b.UserID,
			RoomID:    b.RoomID,
			Note:      b.Note,
			Position:  b.Position,
			CreatedAt: b.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt: b.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(results); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *RoomBookmarkHandler) CreateRoomBookmark(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req CreateRoomBookmarkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.RoomID == uuid.Nil {
		http.Error(w, "room_id is required", http.StatusBadRequest)
		return
	}

	var room models.Room
	if err := h.db.First(&room, "id = ?", req.RoomID).Error; err != nil {
		http.Error(w, "Room not found", http.StatusNotFound)
		return
	}

	var existing models.RoomBookmark
	if err := h.db.First(&existing, "user_id = ? AND room_id = ?", userID, req.RoomID).Error; err == nil {
		http.Error(w, "Room already bookmarked", http.StatusConflict)
		return
	}

	bookmark := &models.RoomBookmark{
		UserID:   userID,
		RoomID:   req.RoomID,
		Note:     req.Note,
		Position: req.Position,
	}

	if err := h.db.Create(bookmark).Error; err != nil {
		log.Printf("Error creating room bookmark: %v", err)
		http.Error(w, "Failed to create room bookmark", http.StatusInternalServerError)
		return
	}

	response := RoomBookmarkResponse{
		ID:        bookmark.ID,
		UserID:    bookmark.UserID,
		RoomID:    bookmark.RoomID,
		Note:      bookmark.Note,
		Position:  bookmark.Position,
		CreatedAt: bookmark.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt: bookmark.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *RoomBookmarkHandler) UpdateRoomBookmark(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}

	var bookmark models.RoomBookmark
	if err := h.db.First(&bookmark, "id = ? AND user_id = ?", id, userID).Error; err != nil {
		http.Error(w, "Room bookmark not found", http.StatusNotFound)
		return
	}

	var req UpdateRoomBookmarkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Note != nil {
		bookmark.Note = *req.Note
	}
	if req.Position != nil {
		bookmark.Position = *req.Position
	}
	bookmark.UpdatedAt = time.Now()

	if err := h.db.Save(&bookmark).Error; err != nil {
		log.Printf("Error updating room bookmark: %v", err)
		http.Error(w, "Failed to update room bookmark", http.StatusInternalServerError)
		return
	}

	response := RoomBookmarkResponse{
		ID:        bookmark.ID,
		UserID:    bookmark.UserID,
		RoomID:    bookmark.RoomID,
		Note:      bookmark.Note,
		Position:  bookmark.Position,
		CreatedAt: bookmark.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt: bookmark.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *RoomBookmarkHandler) DeleteRoomBookmark(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}

	result := h.db.Where("id = ? AND user_id = ?", id, userID).Delete(&models.RoomBookmark{})
	if result.Error != nil {
		log.Printf("Error deleting room bookmark: %v", result.Error)
		http.Error(w, "Failed to delete room bookmark", http.StatusInternalServerError)
		return
	}

	if result.RowsAffected == 0 {
		http.Error(w, "Room bookmark not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "deleted"}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}
