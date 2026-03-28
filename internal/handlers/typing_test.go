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

func setupTypingIndicatorDB(t *testing.T) *gorm.DB {
	db := openTestDB(t)

	err := db.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.UserRoom{},
		&models.TypingIndicator{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupTypingIndicatorTest(t *testing.T) (*TypingIndicatorHandler, *database.Database, uuid.UUID, uuid.UUID) {
	db := setupTypingIndicatorDB(t)
	testDB := &database.Database{DB: db}
	handler := NewTypingIndicatorHandler(testDB)

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

func TestTypingIndicatorHandler_StartTyping_Room(t *testing.T) {
	handler, _, userID, roomID := setupTypingIndicatorTest(t)

	body, _ := json.Marshal(StartTypingRequest{
		RoomID: &roomID,
	})
	req := httptest.NewRequest(http.MethodPost, "/typing/start", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.StartTyping(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}
}

func TestTypingIndicatorHandler_StartTyping_Refresh(t *testing.T) {
	handler, db, userID, roomID := setupTypingIndicatorTest(t)

	db.Create(&models.TypingIndicator{
		UserID:    userID,
		RoomID:    &roomID,
		StartedAt: time.Now().Add(-5 * time.Second),
		ExpiresAt: time.Now().Add(5 * time.Second),
	})

	body, _ := json.Marshal(StartTypingRequest{
		RoomID: &roomID,
	})
	req := httptest.NewRequest(http.MethodPost, "/typing/start", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.StartTyping(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}
}

func TestTypingIndicatorHandler_StartTyping_MissingBoth(t *testing.T) {
	handler, _, userID, _ := setupTypingIndicatorTest(t)

	body, _ := json.Marshal(StartTypingRequest{})
	req := httptest.NewRequest(http.MethodPost, "/typing/start", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.StartTyping(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestTypingIndicatorHandler_StartTyping_BothParams(t *testing.T) {
	handler, _, userID, roomID := setupTypingIndicatorTest(t)

	dmUserID := uuid.New()
	body, _ := json.Marshal(StartTypingRequest{
		RoomID:   &roomID,
		DMUserID: &dmUserID,
	})
	req := httptest.NewRequest(http.MethodPost, "/typing/start", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.StartTyping(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestTypingIndicatorHandler_StartTyping_NotRoomMember(t *testing.T) {
	handler, db, _, roomID := setupTypingIndicatorTest(t)

	nonMember := models.User{
		ID:       uuid.New(),
		Username: "nonmember",
		Email:    "non@example.com",
	}
	db.Create(&nonMember)

	body, _ := json.Marshal(StartTypingRequest{
		RoomID: &roomID,
	})
	req := httptest.NewRequest(http.MethodPost, "/typing/start", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(addUserIDToContext(req.Context(), nonMember.ID))
	w := httptest.NewRecorder()

	handler.StartTyping(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}

func TestTypingIndicatorHandler_StartTyping_InvalidJSON(t *testing.T) {
	handler, _, userID, _ := setupTypingIndicatorTest(t)

	req := httptest.NewRequest(http.MethodPost, "/typing/start", bytes.NewBuffer([]byte("invalid")))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.StartTyping(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestTypingIndicatorHandler_StopTyping_Room(t *testing.T) {
	handler, db, userID, roomID := setupTypingIndicatorTest(t)

	db.Create(&models.TypingIndicator{
		UserID:    userID,
		RoomID:    &roomID,
		StartedAt: time.Now(),
		ExpiresAt: time.Now().Add(10 * time.Second),
	})

	req := httptest.NewRequest(http.MethodPost, "/typing/stop?room_id="+roomID.String(), nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.StopTyping(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var count int64
	db.Model(&models.TypingIndicator{}).Where("user_id = ? AND room_id = ?", userID, roomID).Count(&count)
	if count != 0 {
		t.Errorf("expected typing indicator to be deleted, got count %d", count)
	}
}

func TestTypingIndicatorHandler_StopTyping_MissingParams(t *testing.T) {
	handler, _, userID, _ := setupTypingIndicatorTest(t)

	req := httptest.NewRequest(http.MethodPost, "/typing/stop", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.StopTyping(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestTypingIndicatorHandler_GetTypingUsers_Room(t *testing.T) {
	handler, db, userID, roomID := setupTypingIndicatorTest(t)

	otherUser := models.User{
		ID:       uuid.New(),
		Username: "otheruser",
		Email:    "other@example.com",
	}
	db.Create(&otherUser)
	db.Create(&models.UserRoom{UserID: otherUser.ID, RoomID: roomID, Role: "member"})

	db.Create(&models.TypingIndicator{
		UserID:    otherUser.ID,
		RoomID:    &roomID,
		StartedAt: time.Now(),
		ExpiresAt: time.Now().Add(10 * time.Second),
	})

	req := httptest.NewRequest(http.MethodGet, "/typing/users?room_id="+roomID.String(), nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetTypingUsers(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var users []TypingUserResponse
	if err := json.Unmarshal(w.Body.Bytes(), &users); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(users) != 1 {
		t.Errorf("expected 1 typing user, got %d", len(users))
	}

	if len(users) > 0 && users[0].Username != "otheruser" {
		t.Errorf("expected username 'otheruser', got '%s'", users[0].Username)
	}
}

func TestTypingIndicatorHandler_GetTypingUsers_MissingParams(t *testing.T) {
	handler, _, userID, _ := setupTypingIndicatorTest(t)

	req := httptest.NewRequest(http.MethodGet, "/typing/users", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetTypingUsers(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestTypingIndicatorHandler_GetTypingUsers_ExcludesExpired(t *testing.T) {
	handler, db, userID, roomID := setupTypingIndicatorTest(t)

	db.Create(&models.TypingIndicator{
		UserID:    userID,
		RoomID:    &roomID,
		StartedAt: time.Now().Add(-20 * time.Second),
		ExpiresAt: time.Now().Add(-5 * time.Second),
	})

	req := httptest.NewRequest(http.MethodGet, "/typing/users?room_id="+roomID.String(), nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetTypingUsers(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var users []TypingUserResponse
	if err := json.Unmarshal(w.Body.Bytes(), &users); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(users) != 0 {
		t.Errorf("expected 0 typing users (expired), got %d", len(users))
	}
}

func TestTypingIndicatorHandler_NewTypingIndicatorHandler(t *testing.T) {
	db := openTestDB(t)
	testDB := &database.Database{DB: db}
	handler := NewTypingIndicatorHandler(testDB)

	if handler == nil {
		t.Fatal("expected handler to be created")
	}
	if handler.db == nil {
		t.Error("expected db to be set")
	}
}

func TestTypingIndicator_IsExpired(t *testing.T) {
	expired := models.TypingIndicator{
		ExpiresAt: time.Now().Add(-1 * time.Second),
	}
	if !expired.IsExpired() {
		t.Error("expected indicator to be expired")
	}

	active := models.TypingIndicator{
		ExpiresAt: time.Now().Add(10 * time.Second),
	}
	if active.IsExpired() {
		t.Error("expected indicator to not be expired")
	}
}
