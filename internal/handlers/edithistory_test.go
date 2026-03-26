package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func setupEditHistoryDB(t *testing.T) *gorm.DB {
	db := openTestDB(t)

	err := db.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.Message{},
		&models.MessageEditHistory{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupEditHistoryTest(t *testing.T) (*EditHistoryHandler, *database.Database, uuid.UUID, uuid.UUID) {
	db := setupEditHistoryDB(t)
	testDB := &database.Database{DB: db}
	handler := NewEditHistoryHandler(testDB)

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

	message := models.Message{
		ID:        uuid.New(),
		RoomID:    room.ID,
		UserID:    user.ID,
		Content:   "Original content",
		CreatedAt: time.Now(),
	}
	db.Create(&message)

	return handler, testDB, user.ID, message.ID
}

func TestEditHistoryHandler_GetMessageEditHistory(t *testing.T) {
	handler, db, userID, messageID := setupEditHistoryTest(t)

	history1 := models.MessageEditHistory{
		ID:         uuid.New(),
		MessageID:  messageID,
		UserID:     userID,
		OldContent: "Original",
		NewContent: "First edit",
		CreatedAt:  time.Now().Add(-1 * time.Hour),
	}
	history2 := models.MessageEditHistory{
		ID:         uuid.New(),
		MessageID:  messageID,
		UserID:     userID,
		OldContent: "First edit",
		NewContent: "Second edit",
		CreatedAt:  time.Now(),
	}
	db.Create(&history1)
	db.Create(&history2)

	req := httptest.NewRequest(http.MethodGet, "/api/message/edit-history?message_id="+messageID.String(), nil)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.GetMessageEditHistory(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var results []EditHistoryResponse
	if err := json.Unmarshal(w.Body.Bytes(), &results); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 edit history entries, got %d", len(results))
	}

	if results[0].OldContent != "Original" {
		t.Errorf("expected first entry old content 'Original', got '%s'", results[0].OldContent)
	}

	if results[1].NewContent != "Second edit" {
		t.Errorf("expected second entry new content 'Second edit', got '%s'", results[1].NewContent)
	}
}

func TestEditHistoryHandler_GetMessageEditHistory_NoMessageID(t *testing.T) {
	handler, _, userID, _ := setupEditHistoryTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/message/edit-history", nil)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.GetMessageEditHistory(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestEditHistoryHandler_GetMessageEditHistory_Unauthorized(t *testing.T) {
	handler, _, _, messageID := setupEditHistoryTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/message/edit-history?message_id="+messageID.String(), nil)

	w := httptest.NewRecorder()
	handler.GetMessageEditHistory(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}
