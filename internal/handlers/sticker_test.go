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
)

func setupStickerHandlerTest(t *testing.T) (*StickerHandler, *database.Database, uuid.UUID) {
	db := openTestDB(t)

	err := db.AutoMigrate(&models.ServerSticker{})
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	testDB := &database.Database{DB: db}
	handler := NewStickerHandler(testDB)

	userID := uuid.New()
	user := models.User{ID: userID, Username: "testuser", Email: "test@test.com"}
	db.Create(&user)

	return handler, testDB, userID
}

func TestStickerHandler_CreateSticker(t *testing.T) {
	handler, _, userID := setupStickerHandlerTest(t)

	tests := []struct {
		name           string
		body           CreateStickerRequest
		expectedStatus int
	}{
		{
			name: "valid sticker creation",
			body: CreateStickerRequest{
				Name:     "test-sticker",
				ImageURL: "https://example.com/sticker.png",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "missing name",
			body: CreateStickerRequest{
				ImageURL: "https://example.com/sticker.png",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "missing image URL",
			body: CreateStickerRequest{
				Name: "test-sticker",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "name too long",
			body: CreateStickerRequest{
				Name:     string(bytes.Repeat([]byte("a"), 51)),
				ImageURL: "https://example.com/sticker.png",
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/sticker/create", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(addUserIDToContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.CreateSticker(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d, body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestStickerHandler_GetStickers(t *testing.T) {
	handler, db, userID := setupStickerHandlerTest(t)

	sticker := models.ServerSticker{
		ID:        uuid.New(),
		Name:      "test-sticker",
		ImageURL:  "https://example.com/sticker.png",
		CreatedBy: userID,
	}
	db.Create(&sticker)

	req := httptest.NewRequest(http.MethodGet, "/stickers", nil)
	w := httptest.NewRecorder()

	handler.GetStickers(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestStickerHandler_DeleteSticker(t *testing.T) {
	handler, db, userID := setupStickerHandlerTest(t)

	sticker := models.ServerSticker{
		ID:        uuid.New(),
		Name:      "test-sticker",
		ImageURL:  "https://example.com/sticker.png",
		CreatedBy: userID,
	}
	db.Create(&sticker)

	req := httptest.NewRequest(http.MethodDelete, "/sticker/delete?sticker_id="+sticker.ID.String(), nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.DeleteSticker(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestStickerHandler_DeleteSticker_NotFound(t *testing.T) {
	handler, _, userID := setupStickerHandlerTest(t)

	req := httptest.NewRequest(http.MethodDelete, "/sticker/delete?sticker_id="+uuid.New().String(), nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.DeleteSticker(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestStickerHandler_DeleteSticker_MissingID(t *testing.T) {
	handler, _, userID := setupStickerHandlerTest(t)

	req := httptest.NewRequest(http.MethodDelete, "/sticker/delete", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.DeleteSticker(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestStickerHandler_DeleteSticker_InvalidID(t *testing.T) {
	handler, _, userID := setupStickerHandlerTest(t)

	req := httptest.NewRequest(http.MethodDelete, "/sticker/delete?sticker_id=invalid", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.DeleteSticker(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestStickerHandler_DeleteSticker_Forbidden(t *testing.T) {
	handler, db, _ := setupStickerHandlerTest(t)

	otherUserID := uuid.New()
	sticker := models.ServerSticker{
		ID:        uuid.New(),
		Name:      "test-sticker",
		ImageURL:  "https://example.com/sticker.png",
		CreatedBy: otherUserID,
	}
	db.Create(&sticker)

	userID := uuid.New()
	req := httptest.NewRequest(http.MethodDelete, "/sticker/delete?sticker_id="+sticker.ID.String(), nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.DeleteSticker(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}
