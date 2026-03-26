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

type CategoryPermissionHandler struct {
	db *database.Database
}

func NewCategoryPermissionHandler(db *database.Database) *CategoryPermissionHandler {
	return &CategoryPermissionHandler{db: db}
}

type CreateCategoryPermissionRequest struct {
	CategoryID uuid.UUID  `json:"category_id"`
	UserID     *uuid.UUID `json:"user_id"`
	RoleID     *uuid.UUID `json:"role_id"`
	Permission string     `json:"permission"`
	CanView    bool       `json:"can_view"`
	CanCreate  bool       `json:"can_create"`
	CanEdit    bool       `json:"can_edit"`
	CanDelete  bool       `json:"can_delete"`
}

type UpdateCategoryPermissionRequest struct {
	ID        uuid.UUID `json:"id"`
	CanView   *bool     `json:"can_view"`
	CanCreate *bool     `json:"can_create"`
	CanEdit   *bool     `json:"can_edit"`
	CanDelete *bool     `json:"can_delete"`
}

type CategoryPermissionResponse struct {
	ID         uuid.UUID  `json:"id"`
	CategoryID uuid.UUID  `json:"category_id"`
	UserID     *uuid.UUID `json:"user_id,omitempty"`
	RoleID     *uuid.UUID `json:"role_id,omitempty"`
	Permission string     `json:"permission"`
	CanView    bool       `json:"can_view"`
	CanCreate  bool       `json:"can_create"`
	CanEdit    bool       `json:"can_edit"`
	CanDelete  bool       `json:"can_delete"`
	CreatedAt  string     `json:"created_at"`
	UpdatedAt  string     `json:"updated_at"`
}

func (h *CategoryPermissionHandler) CreatePermission(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req CreateCategoryPermissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.CategoryID == uuid.Nil {
		http.Error(w, "Category ID is required", http.StatusBadRequest)
		return
	}

	if req.UserID == nil && req.RoleID == nil {
		http.Error(w, "Either user_id or role_id is required", http.StatusBadRequest)
		return
	}

	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil || !user.IsAdmin {
		var category models.Category
		if err := h.db.First(&category, "id = ?", req.CategoryID).Error; err != nil {
			http.Error(w, "Category not found", http.StatusNotFound)
			return
		}
		if category.CreatedBy != userID {
			http.Error(w, "Admin or category owner access required", http.StatusForbidden)
			return
		}
	}

	permission := models.CategoryPermission{
		CategoryID: req.CategoryID,
		UserID:     req.UserID,
		RoleID:     req.RoleID,
		Permission: req.Permission,
		CanView:    req.CanView,
		CanCreate:  req.CanCreate,
		CanEdit:    req.CanEdit,
		CanDelete:  req.CanDelete,
	}

	if err := h.db.Create(&permission).Error; err != nil {
		log.Printf("Error creating category permission: %v", err)
		http.Error(w, "Failed to create permission", http.StatusInternalServerError)
		return
	}

	h.sendPermissionResponse(w, &permission, http.StatusCreated)
}

func (h *CategoryPermissionHandler) GetCategoryPermissions(w http.ResponseWriter, r *http.Request) {
	categoryIDStr := r.URL.Query().Get("category_id")
	if categoryIDStr == "" {
		http.Error(w, "Category ID is required", http.StatusBadRequest)
		return
	}

	categoryID, err := uuid.Parse(categoryIDStr)
	if err != nil {
		http.Error(w, "Invalid category ID", http.StatusBadRequest)
		return
	}

	var permissions []models.CategoryPermission
	if err := h.db.Where("category_id = ?", categoryID).
		Order("created_at ASC").
		Find(&permissions).Error; err != nil {
		log.Printf("Error fetching permissions: %v", err)
		http.Error(w, "Failed to fetch permissions", http.StatusInternalServerError)
		return
	}

	responses := make([]CategoryPermissionResponse, len(permissions))
	for i, p := range permissions {
		responses[i] = h.permissionToResponse(&p)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(responses); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *CategoryPermissionHandler) UpdatePermission(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req UpdateCategoryPermissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var permission models.CategoryPermission
	if err := h.db.First(&permission, "id = ?", req.ID).Error; err != nil {
		http.Error(w, "Permission not found", http.StatusNotFound)
		return
	}

	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil || !user.IsAdmin {
		var category models.Category
		if err := h.db.First(&category, "id = ?", permission.CategoryID).Error; err != nil {
			http.Error(w, "Category not found", http.StatusNotFound)
			return
		}
		if category.CreatedBy != userID {
			http.Error(w, "Admin or category owner access required", http.StatusForbidden)
			return
		}
	}

	updates := map[string]interface{}{
		"updated_at": time.Now(),
	}
	if req.CanView != nil {
		updates["can_view"] = *req.CanView
	}
	if req.CanCreate != nil {
		updates["can_create"] = *req.CanCreate
	}
	if req.CanEdit != nil {
		updates["can_edit"] = *req.CanEdit
	}
	if req.CanDelete != nil {
		updates["can_delete"] = *req.CanDelete
	}

	if err := h.db.Model(&permission).Updates(updates).Error; err != nil {
		log.Printf("Error updating permission: %v", err)
		http.Error(w, "Failed to update permission", http.StatusInternalServerError)
		return
	}

	h.db.First(&permission, "id = ?", req.ID)
	h.sendPermissionResponse(w, &permission, http.StatusOK)
}

func (h *CategoryPermissionHandler) DeletePermission(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	permissionIDStr := r.URL.Query().Get("id")
	if permissionIDStr == "" {
		http.Error(w, "Permission ID is required", http.StatusBadRequest)
		return
	}

	permissionID, err := uuid.Parse(permissionIDStr)
	if err != nil {
		http.Error(w, "Invalid permission ID", http.StatusBadRequest)
		return
	}

	var permission models.CategoryPermission
	if err := h.db.First(&permission, "id = ?", permissionID).Error; err != nil {
		http.Error(w, "Permission not found", http.StatusNotFound)
		return
	}

	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil || !user.IsAdmin {
		var category models.Category
		if err := h.db.First(&category, "id = ?", permission.CategoryID).Error; err != nil {
			http.Error(w, "Category not found", http.StatusNotFound)
			return
		}
		if category.CreatedBy != userID {
			http.Error(w, "Admin or category owner access required", http.StatusForbidden)
			return
		}
	}

	if err := h.db.Delete(&permission).Error; err != nil {
		log.Printf("Error deleting permission: %v", err)
		http.Error(w, "Failed to delete permission", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "deleted"}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *CategoryPermissionHandler) CheckPermission(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	categoryIDStr := r.URL.Query().Get("category_id")
	action := r.URL.Query().Get("action")

	if categoryIDStr == "" || action == "" {
		http.Error(w, "category_id and action are required", http.StatusBadRequest)
		return
	}

	categoryID, err := uuid.Parse(categoryIDStr)
	if err != nil {
		http.Error(w, "Invalid category ID", http.StatusBadRequest)
		return
	}

	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err == nil && user.IsAdmin {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"allowed": true,
			"reason":  "admin",
		}); err != nil {
			log.Printf("Error encoding response: %v", err)
		}
		return
	}

	var userRoles []models.UserRole
	h.db.Where("user_id = ?", userID).Find(&userRoles)

	roleIDs := make([]uuid.UUID, len(userRoles))
	for i, ur := range userRoles {
		roleIDs[i] = ur.RoleID
	}

	var permissions []models.CategoryPermission
	query := h.db.Where("category_id = ?", categoryID)
	if len(roleIDs) > 0 {
		query = query.Where("user_id = ? OR role_id IN ?", userID, roleIDs)
	} else {
		query = query.Where("user_id = ?", userID)
	}
	query.Find(&permissions)

	allowed := false
	for _, p := range permissions {
		switch action {
		case "view":
			if p.CanView {
				allowed = true
			}
		case "create":
			if p.CanCreate {
				allowed = true
			}
		case "edit":
			if p.CanEdit {
				allowed = true
			}
		case "delete":
			if p.CanDelete {
				allowed = true
			}
		}
		if allowed {
			break
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"allowed":     allowed,
		"category_id": categoryID,
		"user_id":     userID,
		"action":      action,
	}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *CategoryPermissionHandler) permissionToResponse(p *models.CategoryPermission) CategoryPermissionResponse {
	return CategoryPermissionResponse{
		ID:         p.ID,
		CategoryID: p.CategoryID,
		UserID:     p.UserID,
		RoleID:     p.RoleID,
		Permission: p.Permission,
		CanView:    p.CanView,
		CanCreate:  p.CanCreate,
		CanEdit:    p.CanEdit,
		CanDelete:  p.CanDelete,
		CreatedAt:  p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:  p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func (h *CategoryPermissionHandler) sendPermissionResponse(w http.ResponseWriter, p *models.CategoryPermission, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(h.permissionToResponse(p)); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}
