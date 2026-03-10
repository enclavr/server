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

func setupBookmarkHandlerDB(t *testing.T) *gorm.DB {
	db := openTestDB(t)

	err := db.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.Message{},
		&models.Bookmark{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupBookmarkHandlerTest(t *testing.T) (*BookmarkHandler, *database.Database, uuid.UUID) {
	db := setupBookmarkHandlerDB(t)
	testDB := &database.Database{DB: db}
	handler := NewBookmarkHandler(testDB)

	user := models.User{
		ID:       uuid.New(),
		Username: "testuser",
		Email:    "test@example.com",
	}
	db.Create(&user)

	room := models.Room{
		ID:   uuid.New(),
		Name: "test-room",
	}
	db.Create(&room)

	return handler, testDB, user.ID
}

func addBookmarkIDToPath(r *http.Request, bookmarkID uuid.UUID) *http.Request {
	ctx := context.WithValue(r.Context(), middleware.BookmarkIDKey, bookmarkID.String())
	return r.WithContext(ctx)
}

func TestBookmarkHandler_GetBookmarks(t *testing.T) {
	handler, db, userID := setupBookmarkHandlerTest(t)

	message := models.Message{
		ID:      uuid.New(),
		RoomID:  uuid.New(),
		UserID:  userID,
		Content: "Test message",
	}
	db.Create(&message)

	bookmark := models.Bookmark{
		ID:        uuid.New(),
		UserID:    userID,
		MessageID: message.ID,
		Note:      "Important message",
	}
	db.Create(&bookmark)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bookmarks", nil)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.GetBookmarks(c.Writer, c.Request)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var results []BookmarkResponse
	if err := json.Unmarshal(w.Body.Bytes(), &results); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 bookmark, got %d", len(results))
	}
}

func TestBookmarkHandler_GetBookmarks_Unauthorized(t *testing.T) {
	handler, _, _ := setupBookmarkHandlerTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bookmarks", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.GetBookmarks(c.Writer, c.Request)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestBookmarkHandler_CreateBookmark(t *testing.T) {
	handler, db, userID := setupBookmarkHandlerTest(t)

	message := models.Message{
		ID:      uuid.New(),
		RoomID:  uuid.New(),
		UserID:  userID,
		Content: "Test message",
	}
	db.Create(&message)

	body := CreateBookmarkRequest{
		MessageID: message.ID,
		Note:      "Important message",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/bookmarks", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.CreateBookmark(c.Writer, c.Request)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
	}

	var result BookmarkResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if result.Note != "Important message" {
		t.Errorf("expected note 'Important message', got '%s'", result.Note)
	}
}

func TestBookmarkHandler_CreateBookmark_NotFound(t *testing.T) {
	handler, _, userID := setupBookmarkHandlerTest(t)

	body := CreateBookmarkRequest{
		MessageID: uuid.New(),
		Note:      "Non-existent message",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/bookmarks", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.CreateBookmark(c.Writer, c.Request)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestBookmarkHandler_CreateBookmark_Duplicate(t *testing.T) {
	handler, db, userID := setupBookmarkHandlerTest(t)

	message := models.Message{
		ID:      uuid.New(),
		RoomID:  uuid.New(),
		UserID:  userID,
		Content: "Test message",
	}
	db.Create(&message)

	bookmark := models.Bookmark{
		ID:        uuid.New(),
		UserID:    userID,
		MessageID: message.ID,
		Note:      "Already bookmarked",
	}
	db.Create(&bookmark)

	body := CreateBookmarkRequest{
		MessageID: message.ID,
		Note:      "Duplicate",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/bookmarks", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.CreateBookmark(c.Writer, c.Request)

	if w.Code != http.StatusConflict {
		t.Errorf("expected status %d, got %d", http.StatusConflict, w.Code)
	}
}

func TestBookmarkHandler_GetBookmark(t *testing.T) {
	handler, db, userID := setupBookmarkHandlerTest(t)

	message := models.Message{
		ID:      uuid.New(),
		RoomID:  uuid.New(),
		UserID:  userID,
		Content: "Test message",
	}
	db.Create(&message)

	bookmark := models.Bookmark{
		ID:        uuid.New(),
		UserID:    userID,
		MessageID: message.ID,
		Note:      "Test note",
	}
	db.Create(&bookmark)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bookmarks/"+bookmark.ID.String(), nil)
	req = addBookmarkIDToPath(req, bookmark.ID)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.GetBookmark(c.Writer, c.Request)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var result BookmarkResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if result.Note != "Test note" {
		t.Errorf("expected note 'Test note', got '%s'", result.Note)
	}
}

func TestBookmarkHandler_GetBookmark_NotFound(t *testing.T) {
	handler, _, userID := setupBookmarkHandlerTest(t)

	nonexistentID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/bookmarks/"+nonexistentID.String(), nil)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Request = addBookmarkIDToPath(c.Request, nonexistentID)

	handler.GetBookmark(c.Writer, c.Request)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestBookmarkHandler_UpdateBookmark(t *testing.T) {
	handler, db, userID := setupBookmarkHandlerTest(t)

	message := models.Message{
		ID:      uuid.New(),
		RoomID:  uuid.New(),
		UserID:  userID,
		Content: "Test message",
	}
	db.Create(&message)

	bookmark := models.Bookmark{
		ID:        uuid.New(),
		UserID:    userID,
		MessageID: message.ID,
		Note:      "Old note",
	}
	db.Create(&bookmark)

	body := UpdateBookmarkRequest{
		Note: "Updated note",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/bookmarks/"+bookmark.ID.String(), bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Request = addBookmarkIDToPath(c.Request, bookmark.ID)

	handler.UpdateBookmark(c.Writer, c.Request)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var result BookmarkResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if result.Note != "Updated note" {
		t.Errorf("expected note 'Updated note', got '%s'", result.Note)
	}
}

func TestBookmarkHandler_DeleteBookmark(t *testing.T) {
	handler, db, userID := setupBookmarkHandlerTest(t)

	message := models.Message{
		ID:      uuid.New(),
		RoomID:  uuid.New(),
		UserID:  userID,
		Content: "Test message",
	}
	db.Create(&message)

	bookmark := models.Bookmark{
		ID:        uuid.New(),
		UserID:    userID,
		MessageID: message.ID,
		Note:      "To be deleted",
	}
	db.Create(&bookmark)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/bookmarks/"+bookmark.ID.String(), nil)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Request = addBookmarkIDToPath(c.Request, bookmark.ID)

	handler.DeleteBookmark(w, c.Request)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status %d, got %d", http.StatusNoContent, w.Code)
	}

	var count int64
	db.Model(&models.Bookmark{}).Where("id = ?", bookmark.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected bookmark to be deleted, but still exists")
	}
}

func TestBookmarkHandler_DeleteBookmark_NotFound(t *testing.T) {
	handler, _, userID := setupBookmarkHandlerTest(t)

	nonexistentID := uuid.New()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/bookmarks/"+nonexistentID.String(), nil)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Request = addBookmarkIDToPath(c.Request, nonexistentID)

	handler.DeleteBookmark(c.Writer, c.Request)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}
