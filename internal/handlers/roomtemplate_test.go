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

func setupRoomTemplateHandlerDB(t *testing.T) *gorm.DB {
	db := openTestDB(t)

	err := db.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.UserRoom{},
		&models.Category{},
		&models.RoomTemplate{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupRoomTemplateHandlerTest(t *testing.T) (*RoomTemplateHandler, *database.Database, uuid.UUID) {
	db := setupRoomTemplateHandlerDB(t)
	testDB := &database.Database{DB: db}
	handler := NewRoomTemplateHandler(testDB)

	user := models.User{
		ID:       uuid.New(),
		Username: "testuser",
		Email:    "test@example.com",
		IsAdmin:  true,
	}
	db.Create(&user)

	return handler, testDB, user.ID
}

func TestRoomTemplateHandler_CreateTemplate(t *testing.T) {
	handler, _, userID := setupRoomTemplateHandlerTest(t)

	body := CreateRoomTemplateRequest{
		Name:        "Gaming Room",
		Description: "A template for gaming rooms",
		IsPublic:    true,
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/room-template/create", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.CreateTemplate(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
	}

	var result RoomTemplateResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if result.Name != "Gaming Room" {
		t.Errorf("expected name 'Gaming Room', got '%s'", result.Name)
	}
}

func TestRoomTemplateHandler_CreateTemplate_NoName(t *testing.T) {
	handler, _, userID := setupRoomTemplateHandlerTest(t)

	body := CreateRoomTemplateRequest{
		Description: "Missing name",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/room-template/create", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.CreateTemplate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestRoomTemplateHandler_CreateTemplate_Unauthorized(t *testing.T) {
	handler, _, _ := setupRoomTemplateHandlerTest(t)

	body := CreateRoomTemplateRequest{Name: "Test"}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/room-template/create", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler.CreateTemplate(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestRoomTemplateHandler_GetTemplates(t *testing.T) {
	handler, db, userID := setupRoomTemplateHandlerTest(t)

	template := models.RoomTemplate{
		ID:        uuid.New(),
		Name:      "Test Template",
		CreatedBy: userID,
		IsPublic:  true,
	}
	db.Create(&template)

	req := httptest.NewRequest(http.MethodGet, "/api/room-templates", nil)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.GetTemplates(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var results []RoomTemplateResponse
	if err := json.Unmarshal(w.Body.Bytes(), &results); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(results) < 1 {
		t.Errorf("expected at least 1 template, got %d", len(results))
	}
}

func TestRoomTemplateHandler_GetTemplate(t *testing.T) {
	handler, db, userID := setupRoomTemplateHandlerTest(t)

	template := models.RoomTemplate{
		ID:        uuid.New(),
		Name:      "Specific Template",
		CreatedBy: userID,
	}
	db.Create(&template)

	req := httptest.NewRequest(http.MethodGet, "/api/room-template?id="+template.ID.String(), nil)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.GetTemplate(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestRoomTemplateHandler_UpdateTemplate(t *testing.T) {
	handler, db, userID := setupRoomTemplateHandlerTest(t)

	template := models.RoomTemplate{
		ID:        uuid.New(),
		Name:      "Old Name",
		CreatedBy: userID,
	}
	db.Create(&template)

	body := UpdateRoomTemplateRequest{
		ID:   template.ID,
		Name: "New Name",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/api/room-template/update", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.UpdateTemplate(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}
}

func TestRoomTemplateHandler_DeleteTemplate(t *testing.T) {
	handler, db, userID := setupRoomTemplateHandlerTest(t)

	template := models.RoomTemplate{
		ID:        uuid.New(),
		Name:      "To Delete",
		CreatedBy: userID,
	}
	db.Create(&template)

	req := httptest.NewRequest(http.MethodDelete, "/api/room-template/delete?id="+template.ID.String(), nil)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.DeleteTemplate(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var count int64
	db.Model(&models.RoomTemplate{}).Where("id = ?", template.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected template to be deleted")
	}
}

func TestRoomTemplateHandler_CreateRoomFromTemplate(t *testing.T) {
	handler, db, userID := setupRoomTemplateHandlerTest(t)

	template := models.RoomTemplate{
		ID:          uuid.New(),
		Name:        "Gaming Template",
		Description: "A gaming room",
		CreatedBy:   userID,
	}
	db.Create(&template)

	body := CreateRoomFromTemplateRequest{
		TemplateID: template.ID,
		RoomName:   "My Gaming Room",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/room-template/create-room", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.CreateRoomFromTemplate(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if result["room_name"] != "My Gaming Room" {
		t.Errorf("expected room_name 'My Gaming Room', got '%v'", result["room_name"])
	}
}

func TestRoomTemplateHandler_CreateRoomFromTemplate_NoName(t *testing.T) {
	handler, db, userID := setupRoomTemplateHandlerTest(t)

	template := models.RoomTemplate{
		ID:        uuid.New(),
		Name:      "Template",
		CreatedBy: userID,
	}
	db.Create(&template)

	body := CreateRoomFromTemplateRequest{
		TemplateID: template.ID,
		RoomName:   "",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/room-template/create-room", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.CreateRoomFromTemplate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}
