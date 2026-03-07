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
	"github.com/enclavr/server/internal/websocket"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func setupRoomHandlerDB(t *testing.T) *gorm.DB {
	db := openTestDB(t)

	err := db.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.UserRoom{},
		&models.Category{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupRoomHandlerTest(t *testing.T) (*RoomHandler, *database.Database, uuid.UUID) {
	db := setupRoomHandlerDB(t)
	testDB := &database.Database{DB: db}
	_ = websocket.NewHub()
	handler := NewRoomHandler(testDB)

	user := models.User{
		ID:       uuid.New(),
		Username: "testuser",
		Email:    "test@example.com",
	}
	db.Create(&user)

	return handler, testDB, user.ID
}

func roomContextWithUserID(ctx context.Context, userID uuid.UUID) context.Context {
	return context.WithValue(ctx, middleware.UserIDKey, userID)
}

func TestRoomHandler_CreateRoom(t *testing.T) {
	handler, _, userID := setupRoomHandlerTest(t)

	tests := []struct {
		name           string
		body           CreateRoomRequest
		expectedStatus int
		setupCtx       func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name: "valid room creation",
			body: CreateRoomRequest{
				Name:     "Test Room 1",
				MaxUsers: 50,
			},
			expectedStatus: http.StatusOK,
			setupCtx:       roomContextWithUserID,
		},
		{
			name: "room with description",
			body: CreateRoomRequest{
				Name:        "Test Room 2",
				Description: "A test room description",
				MaxUsers:    25,
			},
			expectedStatus: http.StatusOK,
			setupCtx:       roomContextWithUserID,
		},
		{
			name: "private room",
			body: CreateRoomRequest{
				Name:      "Test Room 3",
				IsPrivate: true,
				MaxUsers:  10,
			},
			expectedStatus: http.StatusOK,
			setupCtx:       roomContextWithUserID,
		},
		{
			name: "room with password",
			body: CreateRoomRequest{
				Name:     "Test Room 4",
				Password: "secret123",
				MaxUsers: 20,
			},
			expectedStatus: http.StatusOK,
			setupCtx:       roomContextWithUserID,
		},
		{
			name: "missing room name",
			body: CreateRoomRequest{
				Name:     "",
				MaxUsers: 50,
			},
			expectedStatus: http.StatusBadRequest,
			setupCtx:       roomContextWithUserID,
		},
		{
			name: "default max users",
			body: CreateRoomRequest{
				Name:     "Test Room 5",
				MaxUsers: 0,
			},
			expectedStatus: http.StatusOK,
			setupCtx:       roomContextWithUserID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/rooms", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")

			ctx := tt.setupCtx(req.Context(), userID)
			req = req.WithContext(ctx)

			w := httptest.NewRecorder()
			handler.CreateRoom(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestRoomHandler_CreateRoom_InvalidJSON(t *testing.T) {
	handler, _, userID := setupRoomHandlerTest(t)

	req := httptest.NewRequest(http.MethodPost, "/rooms", bytes.NewBuffer([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(roomContextWithUserID(req.Context(), userID))

	w := httptest.NewRecorder()
	handler.CreateRoom(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestRoomHandler_GetRooms(t *testing.T) {
	handler, db, userID := setupRoomHandlerTest(t)

	room1 := models.Room{
		ID:       uuid.New(),
		Name:     "Room 1",
		MaxUsers: 50,
	}
	room2 := models.Room{
		ID:       uuid.New(),
		Name:     "Room 2",
		MaxUsers: 25,
	}
	db.Create(&room1)
	db.Create(&room2)

	db.Create(&models.UserRoom{UserID: userID, RoomID: room1.ID, Role: "member"})
	db.Create(&models.UserRoom{UserID: userID, RoomID: room2.ID, Role: "member"})

	req := httptest.NewRequest(http.MethodGet, "/rooms", nil)
	w := httptest.NewRecorder()
	handler.GetRooms(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var rooms []RoomResponse
	if err := json.Unmarshal(w.Body.Bytes(), &rooms); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(rooms) != 2 {
		t.Errorf("expected 2 rooms, got %d", len(rooms))
	}
}

func TestRoomHandler_GetRoom(t *testing.T) {
	handler, db, userID := setupRoomHandlerTest(t)

	room := models.Room{
		ID:       uuid.New(),
		Name:     "Test Room",
		MaxUsers: 50,
	}
	db.Create(&room)
	db.Create(&models.UserRoom{UserID: userID, RoomID: room.ID, Role: "owner"})

	req := httptest.NewRequest(http.MethodGet, "/rooms?id="+room.ID.String(), nil)
	req = req.WithContext(roomContextWithUserID(req.Context(), userID))
	w := httptest.NewRecorder()
	handler.GetRoom(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var roomResp RoomResponse
	if err := json.Unmarshal(w.Body.Bytes(), &roomResp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if roomResp.Name != "Test Room" {
		t.Errorf("expected room name 'Test Room', got %s", roomResp.Name)
	}
}

func TestRoomHandler_GetRoom_NotFound(t *testing.T) {
	handler, _, userID := setupRoomHandlerTest(t)

	nonexistentID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/rooms?id="+nonexistentID.String(), nil)
	req = req.WithContext(roomContextWithUserID(req.Context(), userID))
	w := httptest.NewRecorder()
	handler.GetRoom(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestRoomHandler_GetRoom_InvalidID(t *testing.T) {
	handler, _, userID := setupRoomHandlerTest(t)

	req := httptest.NewRequest(http.MethodGet, "/rooms?id=invalid", nil)
	req = req.WithContext(roomContextWithUserID(req.Context(), userID))
	w := httptest.NewRecorder()
	handler.GetRoom(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestRoomHandler_JoinRoom(t *testing.T) {
	handler, db, userID := setupRoomHandlerTest(t)

	room := models.Room{
		ID:       uuid.New(),
		Name:     "Joinable Room",
		MaxUsers: 50,
	}
	db.Create(&room)

	reqBody := struct {
		RoomID   uuid.UUID `json:"room_id"`
		Password string    `json:"password"`
	}{RoomID: room.ID}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/rooms/join", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(roomContextWithUserID(req.Context(), userID))
	w := httptest.NewRecorder()
	handler.JoinRoom(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}
}

func TestRoomHandler_JoinRoom_InvalidRoomID(t *testing.T) {
	handler, _, userID := setupRoomHandlerTest(t)

	reqBody := struct {
		RoomID   uuid.UUID `json:"room_id"`
		Password string    `json:"password"`
	}{RoomID: uuid.Nil}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/rooms/join", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(roomContextWithUserID(req.Context(), userID))
	w := httptest.NewRecorder()
	handler.JoinRoom(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestRoomHandler_JoinRoom_RoomNotFound(t *testing.T) {
	handler, _, userID := setupRoomHandlerTest(t)

	nonexistentID := uuid.New()
	reqBody := struct {
		RoomID   uuid.UUID `json:"room_id"`
		Password string    `json:"password"`
	}{RoomID: nonexistentID}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/rooms/join", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(roomContextWithUserID(req.Context(), userID))
	w := httptest.NewRecorder()
	handler.JoinRoom(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestRoomHandler_JoinRoom_WithPassword(t *testing.T) {
	handler, db, userID := setupRoomHandlerTest(t)

	room := models.Room{
		ID:        uuid.New(),
		Name:      "Password Room",
		IsPrivate: true,
		Password:  "secret123",
		MaxUsers:  50,
	}
	db.Create(&room)

	reqBody := struct {
		RoomID   uuid.UUID `json:"room_id"`
		Password string    `json:"password"`
	}{RoomID: room.ID, Password: "wrongpassword"}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/rooms/join", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(roomContextWithUserID(req.Context(), userID))
	w := httptest.NewRecorder()
	handler.JoinRoom(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}

func TestRoomHandler_JoinRoom_AlreadyInRoom(t *testing.T) {
	handler, db, userID := setupRoomHandlerTest(t)

	room := models.Room{
		ID:       uuid.New(),
		Name:     "Already Joined Room",
		MaxUsers: 50,
	}
	db.Create(&room)
	db.Create(&models.UserRoom{UserID: userID, RoomID: room.ID, Role: "member"})

	reqBody := struct {
		RoomID   uuid.UUID `json:"room_id"`
		Password string    `json:"password"`
	}{RoomID: room.ID}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/rooms/join", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(roomContextWithUserID(req.Context(), userID))
	w := httptest.NewRecorder()
	handler.JoinRoom(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected status %d, got %d", http.StatusConflict, w.Code)
	}
}

func TestRoomHandler_JoinRoom_RoomFull(t *testing.T) {
	handler, db, userID := setupRoomHandlerTest(t)

	room := models.Room{
		ID:       uuid.New(),
		Name:     "Full Room",
		MaxUsers: 1,
	}
	db.Create(&room)

	db.Create(&models.UserRoom{UserID: uuid.New(), RoomID: room.ID, Role: "member"})

	reqBody := struct {
		RoomID   uuid.UUID `json:"room_id"`
		Password string    `json:"password"`
	}{RoomID: room.ID}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/rooms/join", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(roomContextWithUserID(req.Context(), userID))
	w := httptest.NewRecorder()
	handler.JoinRoom(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}

func TestRoomHandler_LeaveRoom(t *testing.T) {
	handler, db, userID := setupRoomHandlerTest(t)

	room := models.Room{
		ID:   uuid.New(),
		Name: "Leave Room",
	}
	db.Create(&room)
	db.Create(&models.UserRoom{UserID: userID, RoomID: room.ID, Role: "member"})

	reqBody := struct {
		RoomID uuid.UUID `json:"room_id"`
	}{RoomID: room.ID}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/rooms/leave", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(roomContextWithUserID(req.Context(), userID))
	w := httptest.NewRecorder()
	handler.LeaveRoom(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestRoomHandler_LeaveRoom_NotMember(t *testing.T) {
	handler, db, userID := setupRoomHandlerTest(t)

	room := models.Room{
		ID:   uuid.New(),
		Name: "Not Member Room",
	}
	db.Create(&room)

	reqBody := struct {
		RoomID uuid.UUID `json:"room_id"`
	}{RoomID: room.ID}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/rooms/leave", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(roomContextWithUserID(req.Context(), userID))
	w := httptest.NewRecorder()
	handler.LeaveRoom(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestRoomHandler_LeaveRoom_InvalidJSON(t *testing.T) {
	handler, _, userID := setupRoomHandlerTest(t)

	req := httptest.NewRequest(http.MethodPost, "/rooms/leave", bytes.NewBuffer([]byte("invalid")))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(roomContextWithUserID(req.Context(), userID))
	w := httptest.NewRecorder()
	handler.LeaveRoom(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestRoomHandler_roomToResponse(t *testing.T) {
	handler, db, _ := setupRoomHandlerTest(t)

	room := models.Room{
		ID:          uuid.New(),
		Name:        "Response Room",
		Description: "Test description",
		MaxUsers:    50,
	}
	db.Create(&room)

	response := handler.roomToResponse(&room, 5)

	if response.Name != "Response Room" {
		t.Errorf("expected room name 'Response Room', got %s", response.Name)
	}

	if response.UserCount != 5 {
		t.Errorf("expected user count 5, got %d", response.UserCount)
	}
}

func TestRoomHandler_sendRoomResponse(t *testing.T) {
	handler, db, _ := setupRoomHandlerTest(t)

	room := models.Room{
		ID:          uuid.New(),
		Name:        "Send Response Room",
		Description: "Test description",
		MaxUsers:    50,
	}
	db.Create(&room)

	w := httptest.NewRecorder()
	handler.sendRoomResponse(w, &room, 1)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var roomResp RoomResponse
	if err := json.Unmarshal(w.Body.Bytes(), &roomResp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if roomResp.Name != "Send Response Room" {
		t.Errorf("expected room name 'Send Response Room', got %s", roomResp.Name)
	}

	if roomResp.UserCount != 1 {
		t.Errorf("expected user count 1, got %d", roomResp.UserCount)
	}
}

func TestRoomHandler_NewRoomHandler(t *testing.T) {
	db := openTestDB(t)

	testDB := &database.Database{DB: db}
	handler := NewRoomHandler(testDB)

	if handler != nil && handler.db == nil {
		t.Error("expected db to be set")
	}
}
