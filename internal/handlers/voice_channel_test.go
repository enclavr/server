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
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func setupVoiceChannelDB(t *testing.T) *gorm.DB {
	db := openTestDB(t)

	err := db.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.UserRoom{},
		&models.VoiceChannel{},
		&models.VoiceChannelParticipant{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupVoiceChannelTest(t *testing.T) (*VoiceChannelHandler, *database.Database, uuid.UUID, uuid.UUID) {
	db := setupVoiceChannelDB(t)
	testDB := &database.Database{DB: db}
	handler := NewVoiceChannelHandler(testDB)

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

func voiceChannelContextWithUserID(ctx context.Context, userID uuid.UUID) context.Context {
	return context.WithValue(ctx, middleware.UserIDKey, userID)
}

func TestVoiceChannelHandler_CreateChannel(t *testing.T) {
	handler, _, userID, roomID := setupVoiceChannelTest(t)

	tests := []struct {
		name           string
		body           CreateVoiceChannelRequest
		expectedStatus int
	}{
		{
			name: "valid channel creation",
			body: CreateVoiceChannelRequest{
				RoomID:          roomID,
				Name:            "General Voice",
				Description:     "Main voice channel",
				MaxParticipants: 10,
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name: "channel with default max participants",
			body: CreateVoiceChannelRequest{
				RoomID: roomID,
				Name:   "Default Channel",
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name: "private channel",
			body: CreateVoiceChannelRequest{
				RoomID:    roomID,
				Name:      "Private Voice",
				IsPrivate: true,
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name: "missing channel name",
			body: CreateVoiceChannelRequest{
				RoomID: roomID,
				Name:   "",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "missing room ID",
			body: CreateVoiceChannelRequest{
				RoomID: uuid.Nil,
				Name:   "No Room",
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/voice-channel/create", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(voiceChannelContextWithUserID(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.CreateChannel(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestVoiceChannelHandler_CreateChannel_InvalidJSON(t *testing.T) {
	handler, _, userID, _ := setupVoiceChannelTest(t)

	req := httptest.NewRequest(http.MethodPost, "/voice-channel/create", bytes.NewBuffer([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(voiceChannelContextWithUserID(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.CreateChannel(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestVoiceChannelHandler_CreateChannel_NotRoomMember(t *testing.T) {
	handler, db, _, roomID := setupVoiceChannelTest(t)

	nonMember := models.User{
		ID:       uuid.New(),
		Username: "nonmember",
		Email:    "non@example.com",
	}
	db.Create(&nonMember)

	body, _ := json.Marshal(CreateVoiceChannelRequest{
		RoomID: roomID,
		Name:   "Test Channel",
	})
	req := httptest.NewRequest(http.MethodPost, "/voice-channel/create", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(voiceChannelContextWithUserID(req.Context(), nonMember.ID))
	w := httptest.NewRecorder()

	handler.CreateChannel(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}

func TestVoiceChannelHandler_GetRoomChannels(t *testing.T) {
	handler, db, userID, roomID := setupVoiceChannelTest(t)

	ch1 := models.VoiceChannel{
		ID:              uuid.New(),
		RoomID:          roomID,
		Name:            "Channel 1",
		MaxParticipants: 10,
		CreatedBy:       userID,
	}
	ch2 := models.VoiceChannel{
		ID:              uuid.New(),
		RoomID:          roomID,
		Name:            "Channel 2",
		MaxParticipants: 5,
		CreatedBy:       userID,
	}
	db.Create(&ch1)
	db.Create(&ch2)

	req := httptest.NewRequest(http.MethodGet, "/voice-channels?room_id="+roomID.String(), nil)
	req = req.WithContext(voiceChannelContextWithUserID(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetRoomChannels(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var channels []VoiceChannelResponse
	if err := json.Unmarshal(w.Body.Bytes(), &channels); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(channels) != 2 {
		t.Errorf("expected 2 channels, got %d", len(channels))
	}
}

func TestVoiceChannelHandler_GetRoomChannels_MissingRoomID(t *testing.T) {
	handler, _, userID, _ := setupVoiceChannelTest(t)

	req := httptest.NewRequest(http.MethodGet, "/voice-channels", nil)
	req = req.WithContext(voiceChannelContextWithUserID(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetRoomChannels(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestVoiceChannelHandler_GetChannel(t *testing.T) {
	handler, db, userID, roomID := setupVoiceChannelTest(t)

	channel := models.VoiceChannel{
		ID:              uuid.New(),
		RoomID:          roomID,
		Name:            "Test Channel",
		MaxParticipants: 10,
		CreatedBy:       userID,
	}
	db.Create(&channel)

	req := httptest.NewRequest(http.MethodGet, "/voice-channel?id="+channel.ID.String(), nil)
	req = req.WithContext(voiceChannelContextWithUserID(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetChannel(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp VoiceChannelResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Name != "Test Channel" {
		t.Errorf("expected channel name 'Test Channel', got '%s'", resp.Name)
	}
}

func TestVoiceChannelHandler_GetChannel_NotFound(t *testing.T) {
	handler, _, userID, _ := setupVoiceChannelTest(t)

	nonexistentID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/voice-channel?id="+nonexistentID.String(), nil)
	req = req.WithContext(voiceChannelContextWithUserID(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetChannel(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestVoiceChannelHandler_UpdateChannel(t *testing.T) {
	handler, db, userID, roomID := setupVoiceChannelTest(t)

	channel := models.VoiceChannel{
		ID:              uuid.New(),
		RoomID:          roomID,
		Name:            "Original Name",
		MaxParticipants: 10,
		CreatedBy:       userID,
	}
	db.Create(&channel)

	newName := "Updated Name"
	body, _ := json.Marshal(UpdateVoiceChannelRequest{
		Name: &newName,
	})
	req := httptest.NewRequest(http.MethodPut, "/voice-channel/update?id="+channel.ID.String(), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(voiceChannelContextWithUserID(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.UpdateChannel(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp VoiceChannelResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Name != "Updated Name" {
		t.Errorf("expected name 'Updated Name', got '%s'", resp.Name)
	}
}

func TestVoiceChannelHandler_UpdateChannel_Forbidden(t *testing.T) {
	handler, db, _, roomID := setupVoiceChannelTest(t)

	owner := models.User{
		ID:       uuid.New(),
		Username: "owner",
		Email:    "owner@example.com",
	}
	db.Create(&owner)

	channel := models.VoiceChannel{
		ID:              uuid.New(),
		RoomID:          roomID,
		Name:            "Owner Channel",
		MaxParticipants: 10,
		CreatedBy:       owner.ID,
	}
	db.Create(&channel)

	nonOwner := models.User{
		ID:       uuid.New(),
		Username: "nonowner",
		Email:    "nonowner@example.com",
	}
	db.Create(&nonOwner)

	db.Create(&models.UserRoom{UserID: nonOwner.ID, RoomID: roomID, Role: "member"})

	newName := "Hacked"
	body, _ := json.Marshal(UpdateVoiceChannelRequest{
		Name: &newName,
	})
	req := httptest.NewRequest(http.MethodPut, "/voice-channel/update?id="+channel.ID.String(), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(voiceChannelContextWithUserID(req.Context(), nonOwner.ID))
	w := httptest.NewRecorder()

	handler.UpdateChannel(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}

func TestVoiceChannelHandler_DeleteChannel(t *testing.T) {
	handler, db, userID, roomID := setupVoiceChannelTest(t)

	channel := models.VoiceChannel{
		ID:              uuid.New(),
		RoomID:          roomID,
		Name:            "To Delete",
		MaxParticipants: 10,
		CreatedBy:       userID,
	}
	db.Create(&channel)

	req := httptest.NewRequest(http.MethodDelete, "/voice-channel/delete?id="+channel.ID.String(), nil)
	req = req.WithContext(voiceChannelContextWithUserID(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.DeleteChannel(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var deletedChannel models.VoiceChannel
	result := db.Unscoped().Where("id = ?", channel.ID).First(&deletedChannel)
	if result.Error == nil && deletedChannel.DeletedAt.Valid == false {
		t.Error("expected channel to be soft-deleted")
	}
}

func TestVoiceChannelHandler_JoinChannel(t *testing.T) {
	handler, db, userID, roomID := setupVoiceChannelTest(t)

	channel := models.VoiceChannel{
		ID:              uuid.New(),
		RoomID:          roomID,
		Name:            "Joinable",
		MaxParticipants: 10,
		CreatedBy:       userID,
	}
	db.Create(&channel)

	body, _ := json.Marshal(struct {
		ChannelID uuid.UUID `json:"channel_id"`
	}{ChannelID: channel.ID})
	req := httptest.NewRequest(http.MethodPost, "/voice-channel/join", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(voiceChannelContextWithUserID(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.JoinChannel(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
	}
}

func TestVoiceChannelHandler_JoinChannel_AlreadyJoined(t *testing.T) {
	handler, db, userID, roomID := setupVoiceChannelTest(t)

	channel := models.VoiceChannel{
		ID:              uuid.New(),
		RoomID:          roomID,
		Name:            "Already Joined",
		MaxParticipants: 10,
		CreatedBy:       userID,
	}
	db.Create(&channel)

	db.Create(&models.VoiceChannelParticipant{
		ChannelID: channel.ID,
		UserID:    userID,
	})

	body, _ := json.Marshal(struct {
		ChannelID uuid.UUID `json:"channel_id"`
	}{ChannelID: channel.ID})
	req := httptest.NewRequest(http.MethodPost, "/voice-channel/join", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(voiceChannelContextWithUserID(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.JoinChannel(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestVoiceChannelHandler_JoinChannel_Full(t *testing.T) {
	handler, db, userID, roomID := setupVoiceChannelTest(t)

	channel := models.VoiceChannel{
		ID:              uuid.New(),
		RoomID:          roomID,
		Name:            "Full Channel",
		MaxParticipants: 1,
		CreatedBy:       userID,
	}
	db.Create(&channel)

	otherUser := models.User{
		ID:       uuid.New(),
		Username: "other",
		Email:    "other@example.com",
	}
	db.Create(&otherUser)
	db.Create(&models.UserRoom{UserID: otherUser.ID, RoomID: roomID, Role: "member"})
	db.Create(&models.VoiceChannelParticipant{
		ChannelID: channel.ID,
		UserID:    otherUser.ID,
	})

	body, _ := json.Marshal(struct {
		ChannelID uuid.UUID `json:"channel_id"`
	}{ChannelID: channel.ID})
	req := httptest.NewRequest(http.MethodPost, "/voice-channel/join", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(voiceChannelContextWithUserID(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.JoinChannel(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}

func TestVoiceChannelHandler_LeaveChannel(t *testing.T) {
	handler, db, userID, roomID := setupVoiceChannelTest(t)

	channel := models.VoiceChannel{
		ID:              uuid.New(),
		RoomID:          roomID,
		Name:            "Leavable",
		MaxParticipants: 10,
		CreatedBy:       userID,
	}
	db.Create(&channel)

	db.Create(&models.VoiceChannelParticipant{
		ChannelID: channel.ID,
		UserID:    userID,
	})

	req := httptest.NewRequest(http.MethodPost, "/voice-channel/leave?channel_id="+channel.ID.String(), nil)
	req = req.WithContext(voiceChannelContextWithUserID(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.LeaveChannel(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestVoiceChannelHandler_LeaveChannel_NotInChannel(t *testing.T) {
	handler, db, userID, roomID := setupVoiceChannelTest(t)

	channel := models.VoiceChannel{
		ID:              uuid.New(),
		RoomID:          roomID,
		Name:            "Not In",
		MaxParticipants: 10,
		CreatedBy:       userID,
	}
	db.Create(&channel)

	req := httptest.NewRequest(http.MethodPost, "/voice-channel/leave?channel_id="+channel.ID.String(), nil)
	req = req.WithContext(voiceChannelContextWithUserID(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.LeaveChannel(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestVoiceChannelHandler_UpdateParticipant(t *testing.T) {
	handler, db, userID, roomID := setupVoiceChannelTest(t)

	channel := models.VoiceChannel{
		ID:              uuid.New(),
		RoomID:          roomID,
		Name:            "Update Participant",
		MaxParticipants: 10,
		CreatedBy:       userID,
	}
	db.Create(&channel)

	db.Create(&models.VoiceChannelParticipant{
		ChannelID: channel.ID,
		UserID:    userID,
	})

	isMuted := true
	body, _ := json.Marshal(struct {
		ChannelID uuid.UUID `json:"channel_id"`
		IsMuted   *bool     `json:"is_muted,omitempty"`
	}{ChannelID: channel.ID, IsMuted: &isMuted})
	req := httptest.NewRequest(http.MethodPut, "/voice-channel/participant/update", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(voiceChannelContextWithUserID(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.UpdateParticipant(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp VoiceParticipantResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if !resp.IsMuted {
		t.Error("expected participant to be muted")
	}
}

func TestVoiceChannelHandler_GetParticipants(t *testing.T) {
	handler, db, userID, roomID := setupVoiceChannelTest(t)

	channel := models.VoiceChannel{
		ID:              uuid.New(),
		RoomID:          roomID,
		Name:            "With Participants",
		MaxParticipants: 10,
		CreatedBy:       userID,
	}
	db.Create(&channel)

	db.Create(&models.VoiceChannelParticipant{
		ChannelID: channel.ID,
		UserID:    userID,
	})

	otherUser := models.User{
		ID:       uuid.New(),
		Username: "otheruser",
		Email:    "other@example.com",
	}
	db.Create(&otherUser)
	db.Create(&models.VoiceChannelParticipant{
		ChannelID: channel.ID,
		UserID:    otherUser.ID,
	})

	req := httptest.NewRequest(http.MethodGet, "/voice-channel/participants?channel_id="+channel.ID.String(), nil)
	req = req.WithContext(voiceChannelContextWithUserID(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetParticipants(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var participants []VoiceParticipantResponse
	if err := json.Unmarshal(w.Body.Bytes(), &participants); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(participants) != 2 {
		t.Errorf("expected 2 participants, got %d", len(participants))
	}
}

func TestVoiceChannelHandler_NewVoiceChannelHandler(t *testing.T) {
	db := openTestDB(t)
	testDB := &database.Database{DB: db}
	handler := NewVoiceChannelHandler(testDB)

	if handler == nil {
		t.Fatal("expected handler to be created")
	}
	if handler.db == nil {
		t.Error("expected db to be set")
	}
}
