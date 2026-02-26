package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupEmojiHandlerTest(t *testing.T) (*EmojiHandler, *database.Database, uuid.UUID) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	err = db.AutoMigrate(&models.ServerEmoji{})
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	testDB := &database.Database{DB: db}
	handler := NewEmojiHandler(testDB)

	userID := uuid.New()
	user := models.User{ID: userID, Username: "testuser", Email: "test@test.com"}
	db.Create(&user)

	return handler, testDB, userID
}

func TestEmojiHandler_CreateEmoji(t *testing.T) {
	handler, _, userID := setupEmojiHandlerTest(t)

	tests := []struct {
		name           string
		body           CreateEmojiRequest
		expectedStatus int
	}{
		{
			name: "valid emoji creation",
			body: CreateEmojiRequest{
				Name:     "smile",
				ImageURL: "https://example.com/emoji.png",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "missing name",
			body: CreateEmojiRequest{
				ImageURL: "https://example.com/emoji.png",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "missing image URL",
			body: CreateEmojiRequest{
				Name: "smile",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "name too long",
			body: CreateEmojiRequest{
				Name:     string(bytes.Repeat([]byte("a"), 51)),
				ImageURL: "https://example.com/emoji.png",
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/emoji/create", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(addUserIDToContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.CreateEmoji(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d, body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestEmojiHandler_GetEmojis(t *testing.T) {
	handler, db, userID := setupEmojiHandlerTest(t)

	emoji := models.ServerEmoji{
		ID:        uuid.New(),
		Name:      "smile",
		ImageURL:  "https://example.com/emoji.png",
		CreatedBy: userID,
	}
	db.Create(&emoji)

	req := httptest.NewRequest(http.MethodGet, "/emojis", nil)
	w := httptest.NewRecorder()

	handler.GetEmojis(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestEmojiHandler_DeleteEmoji(t *testing.T) {
	handler, db, userID := setupEmojiHandlerTest(t)

	emoji := models.ServerEmoji{
		ID:        uuid.New(),
		Name:      "smile",
		ImageURL:  "https://example.com/emoji.png",
		CreatedBy: userID,
	}
	db.Create(&emoji)

	req := httptest.NewRequest(http.MethodDelete, "/emoji/delete?emoji_id="+emoji.ID.String(), nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.DeleteEmoji(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestEmojiHandler_DeleteEmoji_NotFound(t *testing.T) {
	handler, _, userID := setupEmojiHandlerTest(t)

	req := httptest.NewRequest(http.MethodDelete, "/emoji/delete?emoji_id="+uuid.New().String(), nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.DeleteEmoji(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestEmojiHandler_DeleteEmoji_MissingID(t *testing.T) {
	handler, _, userID := setupEmojiHandlerTest(t)

	req := httptest.NewRequest(http.MethodDelete, "/emoji/delete", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.DeleteEmoji(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestEmojiHandler_DeleteEmoji_InvalidID(t *testing.T) {
	handler, _, userID := setupEmojiHandlerTest(t)

	req := httptest.NewRequest(http.MethodDelete, "/emoji/delete?emoji_id=invalid", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.DeleteEmoji(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestEmojiHandler_DeleteEmoji_Forbidden(t *testing.T) {
	handler, db, _ := setupEmojiHandlerTest(t)

	otherUserID := uuid.New()
	emoji := models.ServerEmoji{
		ID:        uuid.New(),
		Name:      "smile",
		ImageURL:  "https://example.com/emoji.png",
		CreatedBy: otherUserID,
	}
	db.Create(&emoji)

	userID := uuid.New()
	req := httptest.NewRequest(http.MethodDelete, "/emoji/delete?emoji_id="+emoji.ID.String(), nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.DeleteEmoji(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}
