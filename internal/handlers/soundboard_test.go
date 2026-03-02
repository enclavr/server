package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/internal/websocket"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupSoundboardHandlerTest(t *testing.T) (*SoundboardHandler, *database.Database, *websocket.Hub, uuid.UUID) {
	db, err := gorm.Open(sqlite.Open(getTestDSN()), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	err = db.AutoMigrate(&models.SoundboardSound{})
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	testDB := &database.Database{DB: db}
	hub := websocket.NewHub()
	handler := NewSoundboardHandler(testDB, hub)

	userID := uuid.New()
	user := models.User{ID: userID, Username: "testuser", Email: "test@test.com"}
	db.Create(&user)

	return handler, testDB, hub, userID
}

func TestSoundboardHandler_CreateSound(t *testing.T) {
	handler, _, _, userID := setupSoundboardHandlerTest(t)

	tests := []struct {
		name           string
		body           CreateSoundRequest
		expectedStatus int
	}{
		{
			name: "valid sound creation",
			body: CreateSoundRequest{
				Name:     "test-sound",
				AudioURL: "https://example.com/sound.mp3",
				Hotkey:   "ctrl+1",
				Volume:   1.0,
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "missing name",
			body: CreateSoundRequest{
				AudioURL: "https://example.com/sound.mp3",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "missing audio URL",
			body: CreateSoundRequest{
				Name: "test-sound",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "name too long",
			body: CreateSoundRequest{
				Name:     string(bytes.Repeat([]byte("a"), 51)),
				AudioURL: "https://example.com/sound.mp3",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "volume too high",
			body: CreateSoundRequest{
				Name:     "test-sound",
				AudioURL: "https://example.com/sound.mp3",
				Volume:   3.0,
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/sound/create", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(addUserIDToContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.CreateSound(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d, body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestSoundboardHandler_GetSounds(t *testing.T) {
	handler, db, _, userID := setupSoundboardHandlerTest(t)

	sound := models.SoundboardSound{
		ID:        uuid.New(),
		Name:      "test-sound",
		AudioURL:  "https://example.com/sound.mp3",
		Hotkey:    "ctrl+1",
		Volume:    1.0,
		CreatedBy: userID,
	}
	db.Create(&sound)

	req := httptest.NewRequest(http.MethodGet, "/sounds", nil)
	w := httptest.NewRecorder()

	handler.GetSounds(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestSoundboardHandler_PlaySound(t *testing.T) {
	handler, db, _, userID := setupSoundboardHandlerTest(t)

	roomID := uuid.New()
	sound := models.SoundboardSound{
		ID:        uuid.New(),
		Name:      "test-sound",
		AudioURL:  "https://example.com/sound.mp3",
		Hotkey:    "ctrl+1",
		Volume:    1.0,
		CreatedBy: userID,
	}
	db.Create(&sound)

	room := models.Room{ID: roomID, Name: "Test Room"}
	db.Create(&room)

	db.Create(&models.UserRoom{UserID: userID, RoomID: roomID, Role: "member"})

	playReq := struct {
		SoundID uuid.UUID `json:"sound_id"`
		RoomID  uuid.UUID `json:"room_id"`
	}{
		SoundID: sound.ID,
		RoomID:  roomID,
	}
	body, _ := json.Marshal(playReq)
	req := httptest.NewRequest(http.MethodPost, "/sound/play", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.PlaySound(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestSoundboardHandler_PlaySound_MissingSoundID(t *testing.T) {
	handler, db, _, userID := setupSoundboardHandlerTest(t)

	roomID := uuid.New()
	room := models.Room{ID: roomID, Name: "Test Room"}
	db.Create(&room)

	playReq := struct {
		SoundID uuid.UUID `json:"sound_id"`
		RoomID  uuid.UUID `json:"room_id"`
	}{
		RoomID: roomID,
	}
	body, _ := json.Marshal(playReq)
	req := httptest.NewRequest(http.MethodPost, "/sound/play", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.PlaySound(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestSoundboardHandler_PlaySound_SoundNotFound(t *testing.T) {
	handler, db, _, userID := setupSoundboardHandlerTest(t)

	roomID := uuid.New()
	room := models.Room{ID: roomID, Name: "Test Room"}
	db.Create(&room)

	playReq := struct {
		SoundID uuid.UUID `json:"sound_id"`
		RoomID  uuid.UUID `json:"room_id"`
	}{
		SoundID: uuid.New(),
		RoomID:  roomID,
	}
	body, _ := json.Marshal(playReq)
	req := httptest.NewRequest(http.MethodPost, "/sound/play", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.PlaySound(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestSoundboardHandler_DeleteSound(t *testing.T) {
	handler, db, _, userID := setupSoundboardHandlerTest(t)

	sound := models.SoundboardSound{
		ID:        uuid.New(),
		Name:      "test-sound",
		AudioURL:  "https://example.com/sound.mp3",
		Hotkey:    "ctrl+1",
		Volume:    1.0,
		CreatedBy: userID,
	}
	db.Create(&sound)

	req := httptest.NewRequest(http.MethodDelete, "/sound/delete?sound_id="+sound.ID.String(), nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.DeleteSound(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestSoundboardHandler_DeleteSound_NotFound(t *testing.T) {
	handler, _, _, userID := setupSoundboardHandlerTest(t)

	req := httptest.NewRequest(http.MethodDelete, "/sound/delete?sound_id="+uuid.New().String(), nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.DeleteSound(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestSoundboardHandler_DeleteSound_MissingID(t *testing.T) {
	handler, _, _, userID := setupSoundboardHandlerTest(t)

	req := httptest.NewRequest(http.MethodDelete, "/sound/delete", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.DeleteSound(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestSoundboardHandler_DeleteSound_InvalidID(t *testing.T) {
	handler, _, _, userID := setupSoundboardHandlerTest(t)

	req := httptest.NewRequest(http.MethodDelete, "/sound/delete?sound_id=invalid", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.DeleteSound(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestSoundboardHandler_DeleteSound_Forbidden(t *testing.T) {
	handler, db, _, _ := setupSoundboardHandlerTest(t)

	otherUserID := uuid.New()
	sound := models.SoundboardSound{
		ID:        uuid.New(),
		Name:      "test-sound",
		AudioURL:  "https://example.com/sound.mp3",
		Hotkey:    "ctrl+1",
		Volume:    1.0,
		CreatedBy: otherUserID,
	}
	db.Create(&sound)

	userID := uuid.New()
	req := httptest.NewRequest(http.MethodDelete, "/sound/delete?sound_id="+sound.ID.String(), nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.DeleteSound(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}
