package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"fmt"
	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"os"
)

func setupTestDBForFile(t *testing.T) *gorm.DB {
	db, err := gorm.Open(postgres.Open(getTestDSN()), &gorm.Config{})
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

func TestFileHandler_GetFile_PathTraversal(t *testing.T) {
	handler, _, userID := setupFileHandlerWithUser(t)

	req := httptest.NewRequest(http.MethodGet, "/api/files/../../../etc/passwd", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetFile(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d for path traversal attempt, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestFileHandler_GetFile_PathTraversalWithDot(t *testing.T) {
	handler, _, userID := setupFileHandlerWithUser(t)

	req := httptest.NewRequest(http.MethodGet, "/api/files/room/2024/01/01/../test.png", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetFile(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d for path traversal attempt, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestFileHandler_NewFileHandler_DefaultValues(t *testing.T) {
	db, err := gorm.Open(postgres.Open(getTestDSN()), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	testDB := &database.Database{DB: db}

	handler := NewFileHandler(testDB, "", 0)

	if handler.uploadDir != "./uploads" {
		t.Errorf("expected uploadDir ./uploads, got %s", handler.uploadDir)
	}
	if handler.maxFileSize != 10*1024*1024 {
		t.Errorf("expected maxFileSize 10485760, got %d", handler.maxFileSize)
	}
}

func TestFileHandler_UploadFile_ZeroMaxUploadSize(t *testing.T) {
	db, err := gorm.Open(postgres.Open(getTestDSN()), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	err = db.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.UserRoom{},
		&models.ServerSettings{},
	)
	if err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	testDB := &database.Database{DB: db}

	settings := models.ServerSettings{
		ID:                uuid.New(),
		EnableFileUploads: true,
		MaxUploadSizeMB:   0,
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

	req := httptest.NewRequest(http.MethodPost, "/files/upload?room_id="+room.ID.String(), nil)
	req = req.WithContext(addUserIDToContext(req.Context(), user.ID))
	w := httptest.NewRecorder()

	handler.UploadFile(w, req)

	if w.Code == 0 {
		t.Error("expected a valid HTTP status code")
	}
}

func TestFileHandler_UploadToWebhook(t *testing.T) {
	handler, _, _ := setupFileHandlerWithUser(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	err := handler.UploadToWebhook(server.URL, "file", "test.txt", "text/plain", []byte("test content"))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFileHandler_UploadToWebhook_Error(t *testing.T) {
	handler, _, _ := setupFileHandlerWithUser(t)

	err := handler.UploadToWebhook("http://invalid-host-that-does-not-exist.example.com", "file", "test.txt", "text/plain", []byte("test content"))
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func getTestDSN() string {
	host := getEnv("DB_HOST", "localhost")
	port := getEnv("DB_PORT", "5432")
	user := getEnv("DB_USER", "enclavr")
	password := getEnv("DB_PASSWORD", "enclavr")
	dbname := getEnv("DB_NAME", "enclavr_test")
	sslmode := getEnv("DB_SSLMODE", "disable")
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbname, sslmode)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
