package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func setupCategoryPermissionHandlerDB(t *testing.T) *gorm.DB {
	db := openTestDB(t)

	err := db.AutoMigrate(
		&models.User{},
		&models.Category{},
		&models.CategoryPermission{},
		&models.UserRole{},
		&models.Role{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupCategoryPermissionHandlerTest(t *testing.T) (*CategoryPermissionHandler, *database.Database, uuid.UUID, uuid.UUID) {
	db := setupCategoryPermissionHandlerDB(t)
	testDB := &database.Database{DB: db}
	handler := NewCategoryPermissionHandler(testDB)

	user := models.User{
		ID:       uuid.New(),
		Username: "admin",
		Email:    "admin@example.com",
		IsAdmin:  true,
	}
	db.Create(&user)

	category := models.Category{
		ID:        uuid.New(),
		Name:      "Test Category",
		CreatedBy: user.ID,
	}
	db.Create(&category)

	return handler, testDB, user.ID, category.ID
}

func TestCategoryPermissionHandler_CreatePermission(t *testing.T) {
	handler, _, userID, categoryID := setupCategoryPermissionHandlerTest(t)

	targetUser := uuid.New()
	body := CreateCategoryPermissionRequest{
		CategoryID: categoryID,
		UserID:     &targetUser,
		Permission: "read",
		CanView:    true,
		CanCreate:  false,
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/category-permission/create", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.CreatePermission(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
	}

	var result CategoryPermissionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if !result.CanView {
		t.Error("expected can_view to be true")
	}
}

func TestCategoryPermissionHandler_CreatePermission_NoTarget(t *testing.T) {
	handler, _, userID, categoryID := setupCategoryPermissionHandlerTest(t)

	body := CreateCategoryPermissionRequest{
		CategoryID: categoryID,
		Permission: "read",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/category-permission/create", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.CreatePermission(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestCategoryPermissionHandler_GetCategoryPermissions(t *testing.T) {
	handler, db, userID, categoryID := setupCategoryPermissionHandlerTest(t)

	perm := models.CategoryPermission{
		ID:         uuid.New(),
		CategoryID: categoryID,
		UserID:     &userID,
		Permission: "read",
		CanView:    true,
	}
	db.Create(&perm)

	req := httptest.NewRequest(http.MethodGet, "/api/category-permissions?category_id="+categoryID.String(), nil)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.GetCategoryPermissions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var results []CategoryPermissionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &results); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(results) < 1 {
		t.Errorf("expected at least 1 permission, got %d", len(results))
	}
}

func TestCategoryPermissionHandler_UpdatePermission(t *testing.T) {
	handler, db, userID, categoryID := setupCategoryPermissionHandlerTest(t)

	perm := models.CategoryPermission{
		ID:         uuid.New(),
		CategoryID: categoryID,
		UserID:     &userID,
		Permission: "read",
		CanView:    true,
		CanCreate:  false,
	}
	db.Create(&perm)

	canEdit := true
	body := UpdateCategoryPermissionRequest{
		ID:      perm.ID,
		CanEdit: &canEdit,
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/api/category-permission/update", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.UpdatePermission(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}
}

func TestCategoryPermissionHandler_DeletePermission(t *testing.T) {
	handler, db, userID, categoryID := setupCategoryPermissionHandlerTest(t)

	perm := models.CategoryPermission{
		ID:         uuid.New(),
		CategoryID: categoryID,
		UserID:     &userID,
		Permission: "read",
		CanView:    true,
	}
	db.Create(&perm)

	req := httptest.NewRequest(http.MethodDelete, "/api/category-permission/delete?id="+perm.ID.String(), nil)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.DeletePermission(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var count int64
	db.Model(&models.CategoryPermission{}).Where("id = ?", perm.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected permission to be deleted")
	}
}

func TestCategoryPermissionHandler_CheckPermission(t *testing.T) {
	handler, db, userID, categoryID := setupCategoryPermissionHandlerTest(t)

	perm := models.CategoryPermission{
		ID:         uuid.New(),
		CategoryID: categoryID,
		UserID:     &userID,
		Permission: "read",
		CanView:    true,
	}
	db.Create(&perm)

	req := httptest.NewRequest(http.MethodGet, "/api/category-permission/check?category_id="+categoryID.String()+"&action=view", nil)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.CheckPermission(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if result["allowed"] != true {
		t.Error("expected permission to be allowed")
	}
}

func TestCategoryPermissionHandler_CheckPermission_AdminBypass(t *testing.T) {
	handler, _, userID, categoryID := setupCategoryPermissionHandlerTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/category-permission/check?category_id="+categoryID.String()+"&action=delete", nil)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.CheckPermission(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if result["allowed"] != true {
		t.Error("expected admin to be allowed")
	}
}
