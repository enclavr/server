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
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDBForCategory(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	err = db.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.Category{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupCategoryHandlerWithUser(t *testing.T) (*CategoryHandler, *database.Database, uuid.UUID) {
	db := setupTestDBForCategory(t)
	testDB := &database.Database{DB: db}
	handler := NewCategoryHandler(testDB)

	user := models.User{
		ID:       uuid.New(),
		Username: "testuser",
		Email:    "test@example.com",
	}
	db.Create(&user)

	return handler, testDB, user.ID
}

func TestCreateCategory(t *testing.T) {
	handler, _, userID := setupCategoryHandlerWithUser(t)

	tests := []struct {
		name           string
		body           CreateCategoryRequest
		expectedStatus int
		userID         uuid.UUID
	}{
		{
			name: "valid category creation",
			body: CreateCategoryRequest{
				Name:      "Test Category",
				SortOrder: 1,
			},
			expectedStatus: http.StatusOK,
			userID:         userID,
		},
		{
			name: "category name required",
			body: CreateCategoryRequest{
				SortOrder: 1,
			},
			expectedStatus: http.StatusBadRequest,
			userID:         userID,
		},
		{
			name: "empty category name",
			body: CreateCategoryRequest{
				Name:      "",
				SortOrder: 1,
			},
			expectedStatus: http.StatusBadRequest,
			userID:         userID,
		},
		{
			name: "default sort order",
			body: CreateCategoryRequest{
				Name: "Default Sort Order",
			},
			expectedStatus: http.StatusOK,
			userID:         userID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/category/create", bytes.NewReader(body))
			req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, tt.userID))
			w := httptest.NewRecorder()

			handler.CreateCategory(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestGetCategories(t *testing.T) {
	handler, db, userID := setupCategoryHandlerWithUser(t)

	category1 := models.Category{
		ID:        uuid.New(),
		Name:      "Category 1",
		SortOrder: 1,
	}
	category2 := models.Category{
		ID:        uuid.New(),
		Name:      "Category 2",
		SortOrder: 2,
	}
	db.Create(&category1)
	db.Create(&category2)

	tests := []struct {
		name           string
		expectedStatus int
		userID         uuid.UUID
		expectedCount  int
	}{
		{
			name:           "get all categories",
			expectedStatus: http.StatusOK,
			userID:         userID,
			expectedCount:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/categories", nil)
			req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, tt.userID))
			w := httptest.NewRecorder()

			handler.GetCategories(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			var responses []CategoryResponse
			if err := json.Unmarshal(w.Body.Bytes(), &responses); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}

			if len(responses) != tt.expectedCount {
				t.Errorf("expected %d categories, got %d", tt.expectedCount, len(responses))
			}
		})
	}
}

func TestUpdateCategory(t *testing.T) {
	handler, db, userID := setupCategoryHandlerWithUser(t)

	tests := []struct {
		name           string
		body           map[string]interface{}
		expectedStatus int
		userID         uuid.UUID
		expectedName   string
		setup          func() uuid.UUID
	}{
		{
			name: "valid update",
			body: map[string]interface{}{
				"name":       "Updated Name",
				"sort_order": 5,
			},
			expectedStatus: http.StatusOK,
			userID:         userID,
			expectedName:   "Updated Name",
			setup: func() uuid.UUID {
				category := models.Category{
					ID:        uuid.New(),
					Name:      "Original Name",
					SortOrder: 1,
				}
				db.Create(&category)
				return category.ID
			},
		},
		{
			name: "category not found",
			body: map[string]interface{}{
				"id":         uuid.New().String(),
				"name":       "Non-existent",
				"sort_order": 1,
			},
			expectedStatus: http.StatusNotFound,
			userID:         userID,
			expectedName:   "",
			setup: func() uuid.UUID {
				return uuid.New()
			},
		},
		{
			name: "invalid category id",
			body: map[string]interface{}{
				"id":         "invalid-uuid",
				"name":       "Invalid ID",
				"sort_order": 1,
			},
			expectedStatus: http.StatusNotFound,
			userID:         userID,
			expectedName:   "",
			setup: func() uuid.UUID {
				return uuid.New()
			},
		},
		{
			name: "update sort order only",
			body: map[string]interface{}{
				"sort_order": 10,
			},
			expectedStatus: http.StatusOK,
			userID:         userID,
			expectedName:   "Original Name",
			setup: func() uuid.UUID {
				category := models.Category{
					ID:        uuid.New(),
					Name:      "Original Name",
					SortOrder: 1,
				}
				db.Create(&category)
				return category.ID
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			catID := tt.setup()
			tt.body["id"] = catID.String()

			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPut, "/api/category/update", bytes.NewReader(body))
			req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, tt.userID))
			w := httptest.NewRecorder()

			handler.UpdateCategory(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedStatus == http.StatusOK {
				var response CategoryResponse
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Fatalf("failed to unmarshal response: %v", err)
				}
				if response.Name != tt.expectedName {
					t.Errorf("expected name %s, got %s", tt.expectedName, response.Name)
				}
			}
		})
	}
}

func TestDeleteCategory(t *testing.T) {
	handler, db, userID := setupCategoryHandlerWithUser(t)

	tests := []struct {
		name           string
		body           map[string]interface{}
		expectedStatus int
		userID         uuid.UUID
		setup          func() string
	}{
		{
			name:           "valid delete",
			body:           map[string]interface{}{},
			expectedStatus: http.StatusOK,
			userID:         userID,
			setup: func() string {
				category := models.Category{
					ID:        uuid.New(),
					Name:      "To Delete",
					SortOrder: 1,
				}
				db.Create(&category)

				room := models.Room{
					ID:         uuid.New(),
					Name:       "Test Room",
					CategoryID: &category.ID,
				}
				db.Create(&room)

				return category.ID.String()
			},
		},
		{
			name: "category not found",
			body: map[string]interface{}{
				"id": uuid.New().String(),
			},
			expectedStatus: http.StatusOK,
			userID:         userID,
			setup: func() string {
				return uuid.New().String()
			},
		},
		{
			name: "invalid category id",
			body: map[string]interface{}{
				"id": "invalid-uuid",
			},
			expectedStatus: http.StatusBadRequest,
			userID:         userID,
			setup: func() string {
				return "invalid-uuid"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			catID := tt.setup()
			tt.body["id"] = catID

			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodDelete, "/api/category/delete", bytes.NewReader(body))
			req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, tt.userID))
			w := httptest.NewRecorder()

			handler.DeleteCategory(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}
