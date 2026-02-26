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
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDBForInvite(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	err = db.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.UserRoom{},
		&models.Invite{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupInviteHandler(t *testing.T) (*InviteHandler, *database.Database, uuid.UUID, uuid.UUID) {
	db := setupTestDBForInvite(t)
	testDB := &database.Database{DB: db}
	handler := NewInviteHandler(testDB)

	user := models.User{
		ID:       uuid.New(),
		Username: "testuser",
		Email:    "test@example.com",
	}
	room := models.Room{
		ID:       uuid.New(),
		Name:     "Test Room",
		MaxUsers: 50,
	}
	db.Create(&user)
	db.Create(&room)
	db.Create(&models.UserRoom{UserID: user.ID, RoomID: room.ID, Role: "owner"})

	return handler, testDB, user.ID, room.ID
}

func TestInviteHandler_CreateInvite(t *testing.T) {
	handler, _, userID, roomID := setupInviteHandler(t)

	tests := []struct {
		name           string
		body           CreateInviteRequest
		expectedStatus int
		userID         uuid.UUID
	}{
		{
			name: "valid invite creation",
			body: CreateInviteRequest{
				RoomID:    &roomID,
				MaxUses:   10,
				ExpiresIn: 24,
			},
			expectedStatus: http.StatusOK,
			userID:         userID,
		},
		{
			name: "missing room ID",
			body: CreateInviteRequest{
				MaxUses:   10,
				ExpiresIn: 24,
			},
			expectedStatus: http.StatusBadRequest,
			userID:         userID,
		},
		{
			name: "room not found",
			body: CreateInviteRequest{
				RoomID:    func() *uuid.UUID { id := uuid.New(); return &id }(),
				MaxUses:   10,
				ExpiresIn: 24,
			},
			expectedStatus: http.StatusNotFound,
			userID:         userID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/invite/create", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(addUserIDToContext(req.Context(), tt.userID))
			w := httptest.NewRecorder()

			handler.CreateInvite(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d, body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestInviteHandler_GetInvites(t *testing.T) {
	handler, db, userID, roomID := setupInviteHandler(t)

	invite := models.Invite{
		ID:        uuid.New(),
		RoomID:    roomID,
		CreatedBy: userID,
		MaxUses:   10,
		Code:      "testcode",
	}
	db.Create(&invite)

	req := httptest.NewRequest(http.MethodGet, "/invites?room_id="+roomID.String(), nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetInvites(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestInviteHandler_GetInvites_MissingRoomID(t *testing.T) {
	handler, _, userID, _ := setupInviteHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/invites", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetInvites(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestInviteHandler_GetInvites_InvalidRoomID(t *testing.T) {
	handler, _, userID, _ := setupInviteHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/invites?room_id=invalid", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetInvites(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestInviteHandler_UseInvite(t *testing.T) {
	handler, db, userID, roomID := setupInviteHandler(t)

	joiner := models.User{
		ID:       uuid.New(),
		Username: "joiner",
		Email:    "joiner@example.com",
	}
	db.Create(&joiner)

	t.Run("valid invite use", func(t *testing.T) {
		invite := models.Invite{
			ID:        uuid.New(),
			RoomID:    roomID,
			CreatedBy: userID,
			MaxUses:   10,
			Code:      "testcode",
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}
		db.Create(&invite)

		body, _ := json.Marshal(map[string]string{"code": "testcode"})
		req := httptest.NewRequest(http.MethodPost, "/invite/use", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(addUserIDToContext(req.Context(), joiner.ID))
		w := httptest.NewRecorder()

		handler.UseInvite(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d, body: %s", http.StatusOK, w.Code, w.Body.String())
		}
	})

	t.Run("missing code", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"code": ""})
		req := httptest.NewRequest(http.MethodPost, "/invite/use", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(addUserIDToContext(req.Context(), joiner.ID))
		w := httptest.NewRecorder()

		handler.UseInvite(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("invalid code", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"code": "invalid"})
		req := httptest.NewRequest(http.MethodPost, "/invite/use", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(addUserIDToContext(req.Context(), joiner.ID))
		w := httptest.NewRecorder()

		handler.UseInvite(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
		}
	})

	t.Run("revoked invite", func(t *testing.T) {
		invite := models.Invite{
			ID:        uuid.New(),
			RoomID:    roomID,
			CreatedBy: userID,
			MaxUses:   10,
			Code:      "revokedcode",
			ExpiresAt: time.Now().Add(24 * time.Hour),
			IsRevoked: true,
		}
		db.Create(&invite)

		body, _ := json.Marshal(map[string]string{"code": "revokedcode"})
		req := httptest.NewRequest(http.MethodPost, "/invite/use", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(addUserIDToContext(req.Context(), joiner.ID))
		w := httptest.NewRecorder()

		handler.UseInvite(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
		}
	})
}

func TestInviteHandler_RevokeInvite(t *testing.T) {
	handler, db, userID, roomID := setupInviteHandler(t)

	invite := models.Invite{
		ID:        uuid.New(),
		RoomID:    roomID,
		CreatedBy: userID,
		MaxUses:   10,
		Code:      "testcode",
	}
	db.Create(&invite)

	type RevokeRequest struct {
		InviteID uuid.UUID `json:"invite_id"`
	}

	tests := []struct {
		name           string
		body           RevokeRequest
		expectedStatus int
		userID         uuid.UUID
	}{
		{
			name: "valid revoke",
			body: RevokeRequest{
				InviteID: invite.ID,
			},
			expectedStatus: http.StatusOK,
			userID:         userID,
		},
		{
			name: "invite not found",
			body: RevokeRequest{
				InviteID: uuid.New(),
			},
			expectedStatus: http.StatusNotFound,
			userID:         userID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/invite/revoke", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(addUserIDToContext(req.Context(), tt.userID))
			w := httptest.NewRecorder()

			handler.RevokeInvite(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d, body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestInviteHandler_UseInvite_Expired(t *testing.T) {
	handler, db, userID, roomID := setupInviteHandler(t)

	invite := models.Invite{
		ID:        uuid.New(),
		RoomID:    roomID,
		CreatedBy: userID,
		MaxUses:   10,
		Code:      "expiredcode",
		ExpiresAt: time.Now().Add(-24 * time.Hour),
	}
	db.Create(&invite)

	joiner := models.User{
		ID:       uuid.New(),
		Username: "joiner2",
		Email:    "joiner2@example.com",
	}
	db.Create(&joiner)

	type UseInviteRequest struct {
		Code string `json:"code"`
	}

	body, _ := json.Marshal(UseInviteRequest{Code: "expiredcode"})
	req := httptest.NewRequest(http.MethodPost, "/invite/use", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(addUserIDToContext(req.Context(), joiner.ID))
	w := httptest.NewRecorder()

	handler.UseInvite(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}

func TestInviteHandler_UseInvite_MaxUses(t *testing.T) {
	handler, db, userID, roomID := setupInviteHandler(t)

	invite := models.Invite{
		ID:        uuid.New(),
		RoomID:    roomID,
		CreatedBy: userID,
		MaxUses:   1,
		Uses:      1,
		Code:      "maxusescode",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	db.Create(&invite)

	joiner := models.User{
		ID:       uuid.New(),
		Username: "joiner3",
		Email:    "joiner3@example.com",
	}
	db.Create(&joiner)

	type UseInviteRequest struct {
		Code string `json:"code"`
	}

	body, _ := json.Marshal(UseInviteRequest{Code: "maxusescode"})
	req := httptest.NewRequest(http.MethodPost, "/invite/use", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(addUserIDToContext(req.Context(), joiner.ID))
	w := httptest.NewRecorder()

	handler.UseInvite(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}
