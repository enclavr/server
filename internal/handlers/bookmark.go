package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
)

type BookmarkHandler struct {
	db *database.Database
}

func NewBookmarkHandler(db *database.Database) *BookmarkHandler {
	return &BookmarkHandler{db: db}
}

type CreateBookmarkRequest struct {
	MessageID uuid.UUID `json:"message_id"`
	Note      string    `json:"note"`
}

type UpdateBookmarkRequest struct {
	Note string `json:"note"`
}

type BookmarkResponse struct {
	ID        uuid.UUID `json:"id"`
	MessageID uuid.UUID `json:"message_id"`
	Note      string    `json:"note"`
	CreatedAt string    `json:"created_at"`
}

func (h *BookmarkHandler) GetBookmarks(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var bookmarks []models.Bookmark
	if err := h.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&bookmarks).Error; err != nil {
		http.Error(w, "Failed to fetch bookmarks", http.StatusInternalServerError)
		return
	}

	results := make([]BookmarkResponse, 0, len(bookmarks))
	for _, b := range bookmarks {
		results = append(results, BookmarkResponse{
			ID:        b.ID,
			MessageID: b.MessageID,
			Note:      b.Note,
			CreatedAt: b.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(results)
}

func (h *BookmarkHandler) CreateBookmark(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req CreateBookmarkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.MessageID == uuid.Nil {
		http.Error(w, "Message ID is required", http.StatusBadRequest)
		return
	}

	var message models.Message
	if err := h.db.First(&message, "id = ?", req.MessageID).Error; err != nil {
		http.Error(w, "Message not found", http.StatusNotFound)
		return
	}

	var existingBookmark models.Bookmark
	if err := h.db.Where("user_id = ? AND message_id = ?", userID, req.MessageID).First(&existingBookmark).Error; err == nil {
		http.Error(w, "Bookmark already exists", http.StatusConflict)
		return
	}

	bookmark := models.Bookmark{
		UserID:    userID,
		MessageID: req.MessageID,
		Note:      req.Note,
	}

	if err := h.db.Create(&bookmark).Error; err != nil {
		http.Error(w, "Failed to create bookmark", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(BookmarkResponse{
		ID:        bookmark.ID,
		MessageID: bookmark.MessageID,
		Note:      bookmark.Note,
		CreatedAt: bookmark.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

func (h *BookmarkHandler) GetBookmark(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	bookmarkID := r.PathValue("id")
	if bookmarkID == "" {
		if ctxID, ok := r.Context().Value(middleware.BookmarkIDKey).(string); ok {
			bookmarkID = ctxID
		}
	}
	if bookmarkID == "" {
		http.Error(w, "Bookmark ID required", http.StatusBadRequest)
		return
	}

	id, err := uuid.Parse(bookmarkID)
	if err != nil {
		http.Error(w, "Invalid bookmark ID", http.StatusBadRequest)
		return
	}

	var bookmark models.Bookmark
	if err := h.db.First(&bookmark, "id = ? AND user_id = ?", id, userID).Error; err != nil {
		http.Error(w, "Bookmark not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(BookmarkResponse{
		ID:        bookmark.ID,
		MessageID: bookmark.MessageID,
		Note:      bookmark.Note,
		CreatedAt: bookmark.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

func (h *BookmarkHandler) UpdateBookmark(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	bookmarkID := r.PathValue("id")
	if bookmarkID == "" {
		if ctxID, ok := r.Context().Value(middleware.BookmarkIDKey).(string); ok {
			bookmarkID = ctxID
		}
	}
	if bookmarkID == "" {
		http.Error(w, "Bookmark ID required", http.StatusBadRequest)
		return
	}

	id, err := uuid.Parse(bookmarkID)
	if err != nil {
		http.Error(w, "Invalid bookmark ID", http.StatusBadRequest)
		return
	}

	var req UpdateBookmarkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var bookmark models.Bookmark
	if err := h.db.First(&bookmark, "id = ? AND user_id = ?", id, userID).Error; err != nil {
		http.Error(w, "Bookmark not found", http.StatusNotFound)
		return
	}

	bookmark.Note = req.Note
	if err := h.db.Save(&bookmark).Error; err != nil {
		http.Error(w, "Failed to update bookmark", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(BookmarkResponse{
		ID:        bookmark.ID,
		MessageID: bookmark.MessageID,
		Note:      bookmark.Note,
		CreatedAt: bookmark.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

func (h *BookmarkHandler) DeleteBookmark(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	bookmarkID := r.PathValue("id")
	if bookmarkID == "" {
		if ctxID, ok := r.Context().Value(middleware.BookmarkIDKey).(string); ok {
			bookmarkID = ctxID
		}
	}
	if bookmarkID == "" {
		http.Error(w, "Bookmark ID required", http.StatusBadRequest)
		return
	}

	id, err := uuid.Parse(bookmarkID)
	if err != nil {
		http.Error(w, "Invalid bookmark ID", http.StatusBadRequest)
		return
	}

	result := h.db.Delete(&models.Bookmark{}, "id = ? AND user_id = ?", id, userID)
	if result.Error != nil {
		http.Error(w, "Failed to delete bookmark", http.StatusInternalServerError)
		return
	}

	if result.RowsAffected == 0 {
		http.Error(w, "Bookmark not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
