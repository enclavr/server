package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func setupVoiceSessionDB(t *testing.T) *gorm.DB {
	db := openTestDB(t)

	err := db.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.UserRoom{},
		&models.VoiceSession{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupVoiceSessionTest(t *testing.T) (*VoiceSessionHandler, *database.Database, uuid.UUID, uuid.UUID) {
	db := setupVoiceSessionDB(t)
	testDB := &database.Database{DB: db}
	handler := NewVoiceSessionHandler(testDB)

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

	userRoom := models.UserRoom{
		UserID: user.ID,
		RoomID: room.ID,
		Role:   "member",
	}
	db.Create(&userRoom)

	return handler, testDB, user.ID, room.ID
}

func TestVoiceSessionHandler_CreateVoiceSession(t *testing.T) {
	handler, _, userID, roomID := setupVoiceSessionTest(t)

	body, _ := json.Marshal(struct {
		RoomID uuid.UUID `json:"room_id"`
	}{RoomID: roomID})
	req := httptest.NewRequest(http.MethodPost, "/voice-session/create", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.CreateVoiceSession(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
	}

	var resp VoiceSessionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if !resp.IsActive {
		t.Error("expected session to be active")
	}

	if resp.UserID != userID {
		t.Errorf("expected user ID %s, got %s", userID, resp.UserID)
	}
}

func TestVoiceSessionHandler_CreateVoiceSession_MissingRoomID(t *testing.T) {
	handler, _, userID, _ := setupVoiceSessionTest(t)

	body, _ := json.Marshal(struct {
		RoomID uuid.UUID `json:"room_id"`
	}{RoomID: uuid.Nil})
	req := httptest.NewRequest(http.MethodPost, "/voice-session/create", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.CreateVoiceSession(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestVoiceSessionHandler_CreateVoiceSession_NotMember(t *testing.T) {
	handler, db, _, roomID := setupVoiceSessionTest(t)

	nonMember := models.User{
		ID:       uuid.New(),
		Username: "nonmember",
		Email:    "non@example.com",
	}
	db.Create(&nonMember)

	body, _ := json.Marshal(struct {
		RoomID uuid.UUID `json:"room_id"`
	}{RoomID: roomID})
	req := httptest.NewRequest(http.MethodPost, "/voice-session/create", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(addUserIDToContext(req.Context(), nonMember.ID))
	w := httptest.NewRecorder()

	handler.CreateVoiceSession(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}

func TestVoiceSessionHandler_EndVoiceSession(t *testing.T) {
	handler, db, userID, roomID := setupVoiceSessionTest(t)

	session := models.VoiceSession{
		RoomID:    roomID,
		UserID:    userID,
		StartedAt: time.Now().Add(-10 * time.Minute),
	}
	db.Create(&session)

	req := httptest.NewRequest(http.MethodPost, "/voice-session/end?id="+session.ID.String(), nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.EndVoiceSession(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp VoiceSessionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.IsActive {
		t.Error("expected session to not be active")
	}

	if resp.Duration <= 0 {
		t.Errorf("expected positive duration, got %d", resp.Duration)
	}
}

func TestVoiceSessionHandler_EndVoiceSession_AlreadyEnded(t *testing.T) {
	handler, db, userID, roomID := setupVoiceSessionTest(t)

	endedAt := time.Now().Add(-5 * time.Minute)
	session := models.VoiceSession{
		RoomID:    roomID,
		UserID:    userID,
		StartedAt: time.Now().Add(-10 * time.Minute),
		EndedAt:   &endedAt,
	}
	db.Create(&session)

	req := httptest.NewRequest(http.MethodPost, "/voice-session/end?id="+session.ID.String(), nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.EndVoiceSession(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestVoiceSessionHandler_EndVoiceSession_NotFound(t *testing.T) {
	handler, _, userID, _ := setupVoiceSessionTest(t)

	req := httptest.NewRequest(http.MethodPost, "/voice-session/end?id="+uuid.New().String(), nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.EndVoiceSession(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestVoiceSessionHandler_GetUserVoiceSessions(t *testing.T) {
	handler, db, userID, roomID := setupVoiceSessionTest(t)

	endedAt := time.Now().Add(-5 * time.Minute)
	session1 := models.VoiceSession{
		RoomID:    roomID,
		UserID:    userID,
		StartedAt: time.Now().Add(-30 * time.Minute),
		EndedAt:   &endedAt,
	}
	session2 := models.VoiceSession{
		RoomID:    roomID,
		UserID:    userID,
		StartedAt: time.Now().Add(-5 * time.Minute),
	}
	db.Create(&session1)
	db.Create(&session2)

	req := httptest.NewRequest(http.MethodGet, "/voice-sessions", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetUserVoiceSessions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var sessions []VoiceSessionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &sessions); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestVoiceSessionHandler_GetUserVoiceSessions_ActiveOnly(t *testing.T) {
	handler, db, userID, roomID := setupVoiceSessionTest(t)

	endedAt := time.Now().Add(-5 * time.Minute)
	db.Create(&models.VoiceSession{
		RoomID:    roomID,
		UserID:    userID,
		StartedAt: time.Now().Add(-30 * time.Minute),
		EndedAt:   &endedAt,
	})
	db.Create(&models.VoiceSession{
		RoomID:    roomID,
		UserID:    userID,
		StartedAt: time.Now().Add(-5 * time.Minute),
	})

	req := httptest.NewRequest(http.MethodGet, "/voice-sessions?active=true", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetUserVoiceSessions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var sessions []VoiceSessionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &sessions); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(sessions) != 1 {
		t.Errorf("expected 1 active session, got %d", len(sessions))
	}
}

func TestVoiceSessionHandler_GetRoomVoiceSessions(t *testing.T) {
	handler, db, userID, roomID := setupVoiceSessionTest(t)

	endedAt := time.Now().Add(-5 * time.Minute)
	db.Create(&models.VoiceSession{
		RoomID:    roomID,
		UserID:    userID,
		StartedAt: time.Now().Add(-30 * time.Minute),
		EndedAt:   &endedAt,
	})

	req := httptest.NewRequest(http.MethodGet, "/voice-sessions/room?room_id="+roomID.String(), nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetRoomVoiceSessions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var sessions []VoiceSessionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &sessions); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(sessions))
	}
}

func TestVoiceSessionHandler_GetRoomVoiceSessions_MissingRoomID(t *testing.T) {
	handler, _, userID, _ := setupVoiceSessionTest(t)

	req := httptest.NewRequest(http.MethodGet, "/voice-sessions/room", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetRoomVoiceSessions(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestVoiceSessionHandler_GetRoomVoiceSessions_NotMember(t *testing.T) {
	handler, db, _, roomID := setupVoiceSessionTest(t)

	nonMember := models.User{
		ID:       uuid.New(),
		Username: "nonmember",
		Email:    "non@example.com",
	}
	db.Create(&nonMember)

	req := httptest.NewRequest(http.MethodGet, "/voice-sessions/room?room_id="+roomID.String(), nil)
	req = req.WithContext(addUserIDToContext(req.Context(), nonMember.ID))
	w := httptest.NewRecorder()

	handler.GetRoomVoiceSessions(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}

func TestVoiceSessionHandler_GetVoiceSessionStats(t *testing.T) {
	handler, db, userID, roomID := setupVoiceSessionTest(t)

	endedAt := time.Now().Add(-5 * time.Minute)
	db.Create(&models.VoiceSession{
		RoomID:    roomID,
		UserID:    userID,
		StartedAt: time.Now().Add(-30 * time.Minute),
		EndedAt:   &endedAt,
	})
	db.Create(&models.VoiceSession{
		RoomID:    roomID,
		UserID:    userID,
		StartedAt: time.Now().Add(-5 * time.Minute),
	})

	req := httptest.NewRequest(http.MethodGet, "/voice-sessions/stats", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetVoiceSessionStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var stats VoiceSessionStats
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if stats.TotalSessions != 2 {
		t.Errorf("expected 2 total sessions, got %d", stats.TotalSessions)
	}

	if stats.ActiveSessions != 1 {
		t.Errorf("expected 1 active session, got %d", stats.ActiveSessions)
	}
}

func TestVoiceSessionHandler_NewVoiceSessionHandler(t *testing.T) {
	db := openTestDB(t)
	testDB := &database.Database{DB: db}
	handler := NewVoiceSessionHandler(testDB)

	if handler == nil {
		t.Fatal("expected handler to be created")
	}
	if handler.db == nil {
		t.Error("expected db to be set")
	}
}
