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
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func setupStatusHandlerDB(t *testing.T) *gorm.DB {
	db := openTestDB(t)

	err := db.AutoMigrate(
		&models.User{},
		&models.UserStatusModel{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupStatusHandlerTest(t *testing.T) (*StatusHandler, *database.Database, uuid.UUID) {
	db := setupStatusHandlerDB(t)
	testDB := &database.Database{DB: db}
	handler := NewStatusHandler(testDB)

	user := models.User{
		ID:       uuid.New(),
		Username: "testuser",
		Email:    "test@example.com",
	}
	db.Create(&user)

	return handler, testDB, user.ID
}

func TestStatusHandler_GetStatus(t *testing.T) {
	handler, _, userID := setupStatusHandlerTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.GetStatus(c.Writer, c.Request)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response StatusResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if response.UserID != userID {
		t.Errorf("expected user_id %s, got %s", userID, response.UserID)
	}

	if response.Status != models.UserStatusOffline {
		t.Errorf("expected status %s, got %s", models.UserStatusOffline, response.Status)
	}
}

func TestStatusHandler_GetStatus_ExistingStatus(t *testing.T) {
	handler, db, userID := setupStatusHandlerTest(t)

	status := models.UserStatusModel{
		UserID:      userID,
		Status:      models.UserStatusDND,
		StatusText:  "In a meeting",
		StatusEmoji: ":calendar:",
	}
	db.Create(&status)

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.GetStatus(c.Writer, c.Request)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response StatusResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if response.Status != models.UserStatusDND {
		t.Errorf("expected status %s, got %s", models.UserStatusDND, response.Status)
	}

	if response.StatusText != "In a meeting" {
		t.Errorf("expected status_text 'In a meeting', got '%s'", response.StatusText)
	}

	if response.StatusEmoji != ":calendar:" {
		t.Errorf("expected status_emoji ':calendar:', got '%s'", response.StatusEmoji)
	}
}

func TestStatusHandler_UpdateStatus(t *testing.T) {
	handler, _, userID := setupStatusHandlerTest(t)

	updateReq := UpdateStatusRequest{
		Status:      stringPtr("away"),
		StatusText:  stringPtr("BRB"),
		StatusEmoji: stringPtr(":coffee:"),
	}
	body, _ := json.Marshal(updateReq)

	req := httptest.NewRequest(http.MethodPost, "/api/status/update", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.UpdateStatus(c.Writer, c.Request)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response StatusResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if response.Status != models.UserStatusAway {
		t.Errorf("expected status %s, got %s", models.UserStatusAway, response.Status)
	}

	if response.StatusText != "BRB" {
		t.Errorf("expected status_text 'BRB', got '%s'", response.StatusText)
	}

	if response.StatusEmoji != ":coffee:" {
		t.Errorf("expected status_emoji ':coffee:', got '%s'", response.StatusEmoji)
	}
}

func TestStatusHandler_UpdateStatus_InvalidStatus(t *testing.T) {
	handler, _, userID := setupStatusHandlerTest(t)

	updateReq := UpdateStatusRequest{
		Status: stringPtr("invalid_status"),
	}
	body, _ := json.Marshal(updateReq)

	req := httptest.NewRequest(http.MethodPost, "/api/status/update", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.UpdateStatus(c.Writer, c.Request)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestStatusHandler_UpdateStatus_StatusTextTooLong(t *testing.T) {
	handler, _, userID := setupStatusHandlerTest(t)

	longText := string(make([]byte, 151))
	for i := range longText {
		longText = longText[:i] + "a" + longText[i+1:]
	}

	updateReq := UpdateStatusRequest{
		Status:     stringPtr("online"),
		StatusText: &longText,
	}
	body, _ := json.Marshal(updateReq)

	req := httptest.NewRequest(http.MethodPost, "/api/status/update", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.UpdateStatus(c.Writer, c.Request)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestStatusHandler_UpdateStatus_ExpiresIn(t *testing.T) {
	handler, _, userID := setupStatusHandlerTest(t)

	updateReq := UpdateStatusRequest{
		Status:    stringPtr("away"),
		ExpiresIn: intPtr(30),
	}
	body, _ := json.Marshal(updateReq)

	req := httptest.NewRequest(http.MethodPost, "/api/status/update", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.UpdateStatus(c.Writer, c.Request)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response StatusResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if response.ExpiresAt == nil {
		t.Error("expected expires_at to be set")
	}
}

func TestStatusHandler_GetUserStatus(t *testing.T) {
	handler, db, userID := setupStatusHandlerTest(t)

	status := models.UserStatusModel{
		UserID:     userID,
		Status:     models.UserStatusOnline,
		StatusText: "Working",
	}
	db.Create(&status)

	req := httptest.NewRequest(http.MethodGet, "/api/status/user?user_id="+userID.String(), nil)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.GetUserStatus(c.Writer, c.Request)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response StatusResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if response.Status != models.UserStatusOnline {
		t.Errorf("expected status %s, got %s", models.UserStatusOnline, response.Status)
	}
}

func TestStatusHandler_GetUserStatus_MissingUserID(t *testing.T) {
	handler, _, userID := setupStatusHandlerTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/status/user", nil)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.GetUserStatus(c.Writer, c.Request)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestStatusHandler_UpdateStatus_OfflineClearsText(t *testing.T) {
	handler, _, userID := setupStatusHandlerTest(t)

	updateReq := UpdateStatusRequest{
		Status:      stringPtr("online"),
		StatusText:  stringPtr("Some text"),
		StatusEmoji: stringPtr(":smile:"),
	}
	body, _ := json.Marshal(updateReq)

	req := httptest.NewRequest(http.MethodPost, "/api/status/update", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.UpdateStatus(c.Writer, c.Request)

	updateReq2 := UpdateStatusRequest{
		Status: stringPtr("offline"),
	}
	body2, _ := json.Marshal(updateReq2)

	req2 := httptest.NewRequest(http.MethodPost, "/api/status/update", bytes.NewBuffer(body2))
	req2.Header.Set("Content-Type", "application/json")
	ctx2 := context.WithValue(req2.Context(), middleware.UserIDKey, userID)
	req2 = req2.WithContext(ctx2)

	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = req2

	handler.UpdateStatus(c2.Writer, c2.Request)

	if w2.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w2.Code)
	}

	var response StatusResponse
	if err := json.Unmarshal(w2.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if response.StatusText != "" {
		t.Errorf("expected status_text to be cleared, got '%s'", response.StatusText)
	}

	if response.StatusEmoji != "" {
		t.Errorf("expected status_emoji to be cleared, got '%s'", response.StatusEmoji)
	}
}

func stringPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}
