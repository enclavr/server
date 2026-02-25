package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/google/uuid"
)

type CategoryHandler struct {
	db *database.Database
}

func NewCategoryHandler(db *database.Database) *CategoryHandler {
	return &CategoryHandler{db: db}
}

type CreateCategoryRequest struct {
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
}

type CategoryResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	SortOrder int       `json:"sort_order"`
	CreatedAt string    `json:"created_at"`
	RoomCount int       `json:"room_count"`
}

func (h *CategoryHandler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	var req CreateCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "Category name is required", http.StatusBadRequest)
		return
	}

	category := models.Category{
		Name:      req.Name,
		SortOrder: req.SortOrder,
	}

	if err := h.db.Create(&category).Error; err != nil {
		http.Error(w, "Failed to create category", http.StatusInternalServerError)
		return
	}

	h.sendCategoryResponse(w, &category, 0)
}

func (h *CategoryHandler) GetCategories(w http.ResponseWriter, r *http.Request) {
	var categories []models.Category
	if err := h.db.Order("sort_order ASC, name ASC").Find(&categories).Error; err != nil {
		http.Error(w, "Failed to fetch categories", http.StatusInternalServerError)
		return
	}

	responses := make([]CategoryResponse, 0, len(categories))
	for _, cat := range categories {
		var roomCount int64
		h.db.Model(&models.Room{}).Where("category_id = ?", cat.ID).Count(&roomCount)
		responses = append(responses, h.categoryToResponse(&cat, int(roomCount)))
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(responses); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *CategoryHandler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID        uuid.UUID `json:"id"`
		Name      string    `json:"name"`
		SortOrder int       `json:"sort_order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var category models.Category
	if err := h.db.First(&category, req.ID).Error; err != nil {
		http.Error(w, "Category not found", http.StatusNotFound)
		return
	}

	updates := map[string]interface{}{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	updates["sort_order"] = req.SortOrder
	updates["updated_at"] = time.Now()

	if err := h.db.Model(&category).Updates(updates).Error; err != nil {
		http.Error(w, "Failed to update category", http.StatusInternalServerError)
		return
	}

	var roomCount int64
	h.db.Model(&models.Room{}).Where("category_id = ?", category.ID).Count(&roomCount)
	h.sendCategoryResponse(w, &category, int(roomCount))
}

func (h *CategoryHandler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID uuid.UUID `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.db.Delete(&models.Category{}, req.ID).Error; err != nil {
		http.Error(w, "Failed to delete category", http.StatusInternalServerError)
		return
	}

	if err := h.db.Model(&models.Room{}).Where("category_id = ?", req.ID).Update("category_id", nil).Error; err != nil {
		log.Printf("Error clearing category from rooms: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "deleted"}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *CategoryHandler) categoryToResponse(category *models.Category, roomCount int) CategoryResponse {
	return CategoryResponse{
		ID:        category.ID,
		Name:      category.Name,
		SortOrder: category.SortOrder,
		CreatedAt: category.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		RoomCount: roomCount,
	}
}

func (h *CategoryHandler) sendCategoryResponse(w http.ResponseWriter, category *models.Category, roomCount int) {
	response := h.categoryToResponse(category, roomCount)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}
