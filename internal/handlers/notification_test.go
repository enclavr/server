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

func setupNotificationHandlerDB(t *testing.T) *gorm.DB {
	db := openTestDB(t)

	err := db.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.Notification{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupNotificationHandlerTest(t *testing.T) (*NotificationHandler, *database.Database, uuid.UUID) {
	db := setupNotificationHandlerDB(t)
	testDB := &database.Database{DB: db}
	handler := NewNotificationHandler(testDB)

	user := models.User{
		ID:       uuid.New(),
		Username: "testuser",
		Email:    "test@example.com",
	}
	db.Create(&user)

	return handler, testDB, user.ID
}

func TestNotificationHandler_GetNotifications(t *testing.T) {
	handler, db, userID := setupNotificationHandlerTest(t)

	notification := models.Notification{
		ID:     uuid.New(),
		UserID: userID,
		Type:   models.NotificationTypeMention,
		Title:  "You were mentioned",
		Body:   "Test message",
	}
	db.Create(&notification)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications", nil)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.GetNotifications(c.Writer, c.Request)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var results []NotificationResponse
	if err := json.Unmarshal(w.Body.Bytes(), &results); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 notification, got %d", len(results))
	}

	if results[0].Title != "You were mentioned" {
		t.Errorf("expected title 'You were mentioned', got '%s'", results[0].Title)
	}
}

func TestNotificationHandler_GetUnreadCount(t *testing.T) {
	handler, db, userID := setupNotificationHandlerTest(t)

	notification1 := models.Notification{
		ID:     uuid.New(),
		UserID: userID,
		Type:   models.NotificationTypeReply,
		Title:  "New reply",
		IsRead: false,
	}
	notification2 := models.Notification{
		ID:     uuid.New(),
		UserID: userID,
		Type:   models.NotificationTypeSystem,
		Title:  "System message",
		IsRead: true,
	}
	db.Create(&notification1)
	db.Create(&notification2)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/unread-count", nil)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.GetUnreadCount(c.Writer, c.Request)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var result map[string]int64
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if result["unread_count"] != 1 {
		t.Errorf("expected unread count 1, got %d", result["unread_count"])
	}
}

func TestNotificationHandler_CreateNotification(t *testing.T) {
	handler, _, userID := setupNotificationHandlerTest(t)

	reqBody := CreateNotificationRequest{
		Type:      models.NotificationTypeDirectMessage,
		Title:     "New direct message",
		Body:      "Hello!",
		Link:      "/dm/123",
		ActorName: "John Doe",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications", bytes.NewReader(body))
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.CreateNotification(c.Writer, c.Request)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d", http.StatusCreated, w.Code)
	}

	var result NotificationResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if result.Title != "New direct message" {
		t.Errorf("expected title 'New direct message', got '%s'", result.Title)
	}

	if result.Type != models.NotificationTypeDirectMessage {
		t.Errorf("expected type 'direct_message', got '%s'", result.Type)
	}
}

func setupTestRouter(handler *NotificationHandler, userID uuid.UUID) *gin.Engine {
	router := gin.New()
	router.PUT("/api/v1/notifications/:id/read", func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.UserIDKey, userID)
		c.Request = c.Request.WithContext(ctx)
		handler.MarkAsRead(c)
	})
	router.DELETE("/api/v1/notifications/:id", func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.UserIDKey, userID)
		c.Request = c.Request.WithContext(ctx)
		handler.DeleteNotification(c)
	})
	router.PUT("/api/v1/notifications/:id/archive", func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.UserIDKey, userID)
		c.Request = c.Request.WithContext(ctx)
		handler.ArchiveNotification(c)
	})
	return router
}

func TestNotificationHandler_MarkAsRead(t *testing.T) {
	handler, db, userID := setupNotificationHandlerTest(t)

	notification := models.Notification{
		ID:     uuid.New(),
		UserID: userID,
		Type:   models.NotificationTypeMention,
		Title:  "Test notification",
		IsRead: false,
	}
	db.Create(&notification)

	router := setupTestRouter(handler, userID)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/notifications/"+notification.ID.String()+"/read", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d, body: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var result NotificationResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if !result.IsRead {
		t.Error("expected notification to be marked as read")
	}

	if result.ReadAt == nil {
		t.Error("expected read_at to be set")
	}
}

func TestNotificationHandler_DeleteNotification(t *testing.T) {
	handler, db, userID := setupNotificationHandlerTest(t)

	notification := models.Notification{
		ID:     uuid.New(),
		UserID: userID,
		Type:   models.NotificationTypeSystem,
		Title:  "To be deleted",
	}
	db.Create(&notification)

	router := setupTestRouter(handler, userID)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/notifications/"+notification.ID.String(), nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status %d, got %d, body: %s", http.StatusNoContent, w.Code, w.Body.String())
	}

	var count int64
	db.Model(&models.Notification{}).Where("id = ?", notification.ID).Count(&count)
	if count != 0 {
		t.Error("expected notification to be deleted")
	}
}

func TestNotificationHandler_ArchiveNotification(t *testing.T) {
	handler, db, userID := setupNotificationHandlerTest(t)

	notification := models.Notification{
		ID:       uuid.New(),
		UserID:   userID,
		Type:     models.NotificationTypePollVote,
		Title:    "Vote notification",
		Archived: false,
	}
	db.Create(&notification)

	router := setupTestRouter(handler, userID)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/notifications/"+notification.ID.String()+"/archive", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d, body: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var result NotificationResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if !result.Archived {
		t.Error("expected notification to be archived")
	}
}
