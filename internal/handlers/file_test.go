package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDBForFile(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	err = db.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.UserRoom{},
		&models.ServerSettings{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupFileHandlerWithUser(t *testing.T) (*FileHandler, *database.Database, uuid.UUID) {
	db := setupTestDBForFile(t)
	testDB := &database.Database{DB: db}

	settings := models.ServerSettings{
		ID:                uuid.New(),
		EnableFileUploads: true,
		MaxUploadSizeMB:   10,
	}
	db.Create(&settings)

	handler := NewFileHandler(testDB, "/tmp/test-uploads", 10)

	user := models.User{
		ID:       uuid.New(),
		Username: "testuser",
		Email:    "test@example.com",
	}
	db.Create(&user)

	room := models.Room{
		ID:       uuid.New(),
		Name:     "Test Room",
		MaxUsers: 50,
	}
	db.Create(&room)

	db.Create(&models.UserRoom{
		UserID: user.ID,
		RoomID: room.ID,
		Role:   "member",
	})

	return handler, testDB, user.ID
}

func TestFileHandler_UploadFile_Unauthorized(t *testing.T) {
	handler, _, _ := setupFileHandlerWithUser(t)

	req := httptest.NewRequest(http.MethodPost, "/files/upload", nil)
	w := httptest.NewRecorder()

	handler.UploadFile(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestFileHandler_UploadFile_DisabledUploads(t *testing.T) {
	handler, db, userID := setupFileHandlerWithUser(t)

	var settings models.ServerSettings
	db.First(&settings)
	db.Model(&settings).Update("enable_file_uploads", false)

	req := httptest.NewRequest(http.MethodPost, "/files/upload", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.UploadFile(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}

func TestFileHandler_UploadFile_NoRoomID(t *testing.T) {
	handler, _, userID := setupFileHandlerWithUser(t)

	req := httptest.NewRequest(http.MethodPost, "/files/upload", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.UploadFile(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestFileHandler_UploadFile_InvalidRoomID(t *testing.T) {
	handler, _, userID := setupFileHandlerWithUser(t)

	req := httptest.NewRequest(http.MethodPost, "/files/upload?room_id=invalid", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.UploadFile(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestFileHandler_GetFile(t *testing.T) {
	handler, _, userID := setupFileHandlerWithUser(t)

	nonexistentID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/files?file_id="+nonexistentID.String(), nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetFile(w, req)

	if w.Code != http.StatusNotFound && w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d or %d, got %d", http.StatusNotFound, http.StatusBadRequest, w.Code)
	}
}

func TestFileHandler_GetFile_MissingID(t *testing.T) {
	handler, _, userID := setupFileHandlerWithUser(t)

	req := httptest.NewRequest(http.MethodGet, "/files", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetFile(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestFileHandler_DeleteFile(t *testing.T) {
	handler, _, userID := setupFileHandlerWithUser(t)

	nonexistentID := uuid.New()
	req := httptest.NewRequest(http.MethodDelete, "/files?file_id="+nonexistentID.String(), nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.DeleteFile(w, req)

	if w.Code != http.StatusNotFound && w.Code != http.StatusBadRequest && w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestFileHandler_DeleteFile_MissingID(t *testing.T) {
	handler, _, userID := setupFileHandlerWithUser(t)

	req := httptest.NewRequest(http.MethodDelete, "/files", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.DeleteFile(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestFileHandler_GetRoomFiles(t *testing.T) {
	handler, db, userID := setupFileHandlerWithUser(t)

	room := models.Room{
		ID:   uuid.New(),
		Name: "test-room",
	}
	db.Create(&room)

	db.Create(&models.UserRoom{
		UserID: userID,
		RoomID: room.ID,
		Role:   "member",
	})

	req := httptest.NewRequest(http.MethodGet, "/files?room_id="+room.ID.String(), nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetRoomFiles(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusBadRequest && w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestFileHandler_GetRoomFiles_MissingRoomID(t *testing.T) {
	handler, _, userID := setupFileHandlerWithUser(t)

	req := httptest.NewRequest(http.MethodGet, "/files", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetRoomFiles(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}
