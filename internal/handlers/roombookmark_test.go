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

func setupRoomBookmarkDB(t *testing.T) *gorm.DB {
	db := openTestDB(t)

	err := db.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.RoomBookmark{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupRoomBookmarkTest(t *testing.T) (*RoomBookmarkHandler, *database.Database, uuid.UUID, uuid.UUID) {
	db := setupRoomBookmarkDB(t)
	testDB := &database.Database{DB: db}
	handler := NewRoomBookmarkHandler(testDB)

	user := models.User{
		ID:       uuid.New(),
		Username: "testuser",
		Email:    "test@example.com",
	}
	db.Create(&user)

	room := models.Room{
		ID:        uuid.New(),
		Name:      "test-room",
		CreatedBy: user.ID,
	}
	db.Create(&room)

	return handler, testDB, user.ID, room.ID
}

func TestRoomBookmarkHandler_CreateRoomBookmark(t *testing.T) {
	handler, _, userID, roomID := setupRoomBookmarkTest(t)

	reqBody := CreateRoomBookmarkRequest{
		RoomID:   roomID,
		Note:     "My favorite room",
		Position: 1,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/room-bookmark/create", bytes.NewReader(body))
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler.CreateRoomBookmark(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d", http.StatusCreated, w.Code)
	}

	var result RoomBookmarkResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if result.Note != "My favorite room" {
		t.Errorf("expected note 'My favorite room', got '%s'", result.Note)
	}

	if result.RoomID != roomID {
		t.Errorf("expected room_id %s, got %s", roomID, result.RoomID)
	}
}

func TestRoomBookmarkHandler_CreateRoomBookmark_Duplicate(t *testing.T) {
	handler, db, userID, roomID := setupRoomBookmarkTest(t)

	bookmark := models.RoomBookmark{
		UserID: userID,
		RoomID: roomID,
		Note:   "Existing bookmark",
	}
	db.Create(&bookmark)

	reqBody := CreateRoomBookmarkRequest{
		RoomID: roomID,
		Note:   "Duplicate",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/room-bookmark/create", bytes.NewReader(body))
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler.CreateRoomBookmark(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected status %d, got %d", http.StatusConflict, w.Code)
	}
}

func TestRoomBookmarkHandler_GetRoomBookmarks(t *testing.T) {
	handler, db, userID, roomID := setupRoomBookmarkTest(t)

	room2 := models.Room{
		ID:        uuid.New(),
		Name:      "test-room-2",
		CreatedBy: userID,
	}
	db.Create(&room2)

	bookmark1 := models.RoomBookmark{
		UserID:   userID,
		RoomID:   roomID,
		Note:     "First",
		Position: 2,
	}
	bookmark2 := models.RoomBookmark{
		UserID:   userID,
		RoomID:   room2.ID,
		Note:     "Second",
		Position: 1,
	}
	db.Create(&bookmark1)
	db.Create(&bookmark2)

	req := httptest.NewRequest(http.MethodGet, "/api/room-bookmarks", nil)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.GetRoomBookmarks(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var results []RoomBookmarkResponse
	if err := json.Unmarshal(w.Body.Bytes(), &results); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 bookmarks, got %d", len(results))
	}

	if results[0].Note != "Second" {
		t.Errorf("expected first bookmark by position to be 'Second', got '%s'", results[0].Note)
	}
}

func TestRoomBookmarkHandler_DeleteRoomBookmark(t *testing.T) {
	handler, db, userID, roomID := setupRoomBookmarkTest(t)

	bookmark := models.RoomBookmark{
		UserID: userID,
		RoomID: roomID,
		Note:   "To delete",
	}
	db.Create(&bookmark)

	req := httptest.NewRequest(http.MethodDelete, "/api/room-bookmark/delete?id="+bookmark.ID.String(), nil)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.DeleteRoomBookmark(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var count int64
	db.Model(&models.RoomBookmark{}).Where("id = ?", bookmark.ID).Count(&count)
	if count != 0 {
		t.Error("expected bookmark to be deleted")
	}
}

func TestRoomBookmarkHandler_UpdateRoomBookmark(t *testing.T) {
	handler, db, userID, roomID := setupRoomBookmarkTest(t)

	bookmark := models.RoomBookmark{
		UserID: userID,
		RoomID: roomID,
		Note:   "Old note",
	}
	db.Create(&bookmark)

	newNote := "Updated note"
	reqBody := UpdateRoomBookmarkRequest{
		Note: &newNote,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPut, "/api/room-bookmark/update?id="+bookmark.ID.String(), bytes.NewReader(body))
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler.UpdateRoomBookmark(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var result RoomBookmarkResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if result.Note != "Updated note" {
		t.Errorf("expected note 'Updated note', got '%s'", result.Note)
	}
}
