package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/enclavr/server/internal/config"
	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/internal/websocket"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDBForVoice(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(getTestDSN()), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	err = db.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.UserRoom{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupVoiceHandler(t *testing.T) (*VoiceHandler, *database.Database) {
	db := setupTestDBForVoice(t)
	testDB := &database.Database{DB: db}
	cfg := &config.Config{
		Voice: config.VoiceConfig{
			STUNServer: "stun:stun.l.google.com:19302",
		},
		Server: config.ServerConfig{
			AllowedOrigins: []string{},
		},
	}
	hub := websocket.NewHub()
	handler := NewVoiceHandler(testDB, hub, cfg)

	user := models.User{
		ID:       uuid.New(),
		Username: "testuser",
		Email:    "test@example.com",
	}
	testDB.Create(&user)

	room := models.Room{
		ID:   uuid.New(),
		Name: "Test Room",
	}
	testDB.Create(&room)

	userRoom := models.UserRoom{
		UserID: user.ID,
		RoomID: room.ID,
	}
	testDB.Create(&userRoom)

	return handler, testDB
}

func TestVoiceHandler_GetICEConfig(t *testing.T) {
	handler, _ := setupVoiceHandler(t)

	tests := []struct {
		name           string
		expectedStatus int
		checkResponse  func(t *testing.T, response *http.Response)
	}{
		{
			name:           "returns ICE config successfully",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, response *http.Response) {
				var iceConfig ICEConfig
				err := json.NewDecoder(response.Body).Decode(&iceConfig)
				if err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if len(iceConfig.ICEServers) != 1 {
					t.Errorf("expected 1 ICE server, got %d", len(iceConfig.ICEServers))
				}
				if iceConfig.ICEServers[0].URLs[0] != "stun:stun.l.google.com:19302" {
					t.Errorf("unexpected STUN server URL: %s", iceConfig.ICEServers[0].URLs[0])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/ice-config", nil)
			w := httptest.NewRecorder()

			handler.GetICEConfig(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.checkResponse != nil {
				tt.checkResponse(t, w.Result())
			}
		})
	}
}

func TestVoiceHandler_GetICEConfig_WithTURN(t *testing.T) {
	db := setupTestDBForVoice(t)
	testDB := &database.Database{DB: db}
	cfg := &config.Config{
		Voice: config.VoiceConfig{
			STUNServer: "stun:stun.l.google.com:19302",
			TURNServer: "turn:turn.example.com:3478",
			TURNUser:   "testuser",
			TURNPass:   "testpass",
		},
		Server: config.ServerConfig{
			AllowedOrigins: []string{},
		},
	}
	hub := websocket.NewHub()
	handler := NewVoiceHandler(testDB, hub, cfg)

	req := httptest.NewRequest(http.MethodGet, "/ice-config", nil)
	w := httptest.NewRecorder()

	handler.GetICEConfig(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var iceConfig ICEConfig
	err := json.NewDecoder(w.Body).Decode(&iceConfig)
	if err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(iceConfig.ICEServers) != 2 {
		t.Errorf("expected 2 ICE servers, got %d", len(iceConfig.ICEServers))
	}

	if iceConfig.ICEServers[1].Username != "testuser" {
		t.Errorf("expected TURN username 'testuser', got '%s'", iceConfig.ICEServers[1].Username)
	}
	if iceConfig.ICEServers[1].Credential != "testpass" {
		t.Errorf("expected TURN credential 'testpass', got '%s'", iceConfig.ICEServers[1].Credential)
	}
}

func TestVoiceHandler_HandleWebSocket_MissingRoomID(t *testing.T) {
	handler, _ := setupVoiceHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	w := httptest.NewRecorder()

	handler.HandleWebSocket(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	if w.Body.String() != "Room ID is required\n" {
		t.Errorf("unexpected error message: %s", w.Body.String())
	}
}

func TestVoiceHandler_HandleWebSocket_InvalidRoomID(t *testing.T) {
	handler, _ := setupVoiceHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/ws?room_id=invalid", nil)
	w := httptest.NewRecorder()

	handler.HandleWebSocket(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	if w.Body.String() != "Invalid room ID\n" {
		t.Errorf("unexpected error message: %s", w.Body.String())
	}
}

func TestVoiceHandler_HandleWebSocket_UserNotInRoom(t *testing.T) {
	handler, testDB := setupVoiceHandler(t)

	user := models.User{
		ID:       uuid.New(),
		Username: "otheruser",
		Email:    "other@example.com",
	}
	testDB.Create(&user)

	req := httptest.NewRequest(http.MethodGet, "/ws?room_id="+uuid.New().String(), nil)
	w := httptest.NewRecorder()

	ctx := context.Background()
	req = req.WithContext(context.WithValue(ctx, middleware.UserIDKey, user.ID))

	handler.HandleWebSocket(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}

	if w.Body.String() != "Not in room\n" {
		t.Errorf("unexpected error message: %s", w.Body.String())
	}
}
