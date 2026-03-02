package handlers

import (
	"bytes"
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
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDBForBan(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(getTestDSN()), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	err = db.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.Category{},
		&models.Ban{},
		&models.UserRoom{},
		&models.AuditLog{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupBanHandlerWithUser(t *testing.T) (*BanHandler, *database.Database, uuid.UUID) {
	db := setupTestDBForBan(t)
	testDB := &database.Database{DB: db}
	handler := NewBanHandler(testDB)

	user := models.User{
		ID:       uuid.New(),
		Username: "adminuser",
		Email:    "admin@example.com",
	}
	db.Create(&user)

	return handler, testDB, user.ID
}

func TestBanHandler_CreateBan(t *testing.T) {
	handler, db, adminID := setupBanHandlerWithUser(t)

	room := models.Room{
		ID:   uuid.New(),
		Name: "Test Room",
	}
	db.Create(&room)

	user := models.User{
		ID:       uuid.New(),
		Username: "banneduser",
		Email:    "banned@example.com",
	}
	db.Create(&user)

	tests := []struct {
		name           string
		body           CreateBanRequest
		expectedStatus int
		userID         uuid.UUID
	}{
		{
			name: "valid ban creation",
			body: CreateBanRequest{
				UserID: user.ID,
				RoomID: room.ID,
				Reason: "Test ban reason",
			},
			expectedStatus: http.StatusOK,
			userID:         adminID,
		},
		{
			name: "missing user_id",
			body: CreateBanRequest{
				RoomID: room.ID,
				Reason: "Test ban reason",
			},
			expectedStatus: http.StatusBadRequest,
			userID:         adminID,
		},
		{
			name: "missing room_id",
			body: CreateBanRequest{
				UserID: user.ID,
				Reason: "Test ban reason",
			},
			expectedStatus: http.StatusBadRequest,
			userID:         adminID,
		},
		{
			name: "room not found",
			body: CreateBanRequest{
				UserID: user.ID,
				RoomID: uuid.New(),
				Reason: "Test ban reason",
			},
			expectedStatus: http.StatusNotFound,
			userID:         adminID,
		},
		{
			name: "user not found",
			body: CreateBanRequest{
				UserID: uuid.New(),
				RoomID: room.ID,
				Reason: "Test ban reason",
			},
			expectedStatus: http.StatusNotFound,
			userID:         adminID,
		},
		{
			name: "already banned",
			body: CreateBanRequest{
				UserID: user.ID,
				RoomID: room.ID,
				Reason: "Test ban reason",
			},
			expectedStatus: http.StatusConflict,
			userID:         adminID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/ban/create", bytes.NewReader(body))
			req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, tt.userID))
			w := httptest.NewRecorder()

			handler.CreateBan(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestBanHandler_GetBans(t *testing.T) {
	handler, db, adminID := setupBanHandlerWithUser(t)

	room := models.Room{
		ID:   uuid.New(),
		Name: "Test Room",
	}
	db.Create(&room)

	user := models.User{
		ID:       uuid.New(),
		Username: "banneduser",
		Email:    "banned@example.com",
	}
	db.Create(&user)

	ban := models.Ban{
		UserID:   user.ID,
		RoomID:   room.ID,
		BannedBy: adminID,
		Reason:   "Test ban",
	}
	db.Create(&ban)

	tests := []struct {
		name           string
		roomID         string
		expectedStatus int
		expectedCount  int
		userID         uuid.UUID
	}{
		{
			name:           "get bans for room",
			roomID:         room.ID.String(),
			expectedStatus: http.StatusOK,
			expectedCount:  1,
			userID:         adminID,
		},
		{
			name:           "missing room_id",
			roomID:         "",
			expectedStatus: http.StatusBadRequest,
			expectedCount:  0,
			userID:         adminID,
		},
		{
			name:           "invalid room_id",
			roomID:         "invalid-uuid",
			expectedStatus: http.StatusBadRequest,
			expectedCount:  0,
			userID:         adminID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/ban?room_id="+tt.roomID, nil)
			req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, tt.userID))
			w := httptest.NewRecorder()

			handler.GetBans(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestBanHandler_GetBan(t *testing.T) {
	handler, db, adminID := setupBanHandlerWithUser(t)

	room := models.Room{
		ID:   uuid.New(),
		Name: "Test Room",
	}
	db.Create(&room)

	user := models.User{
		ID:       uuid.New(),
		Username: "banneduser",
		Email:    "banned@example.com",
	}
	db.Create(&user)

	ban := models.Ban{
		ID:       uuid.New(),
		UserID:   user.ID,
		RoomID:   room.ID,
		BannedBy: adminID,
		Reason:   "Test ban",
	}
	db.Create(&ban)

	tests := []struct {
		name           string
		banID          string
		expectedStatus int
		userID         uuid.UUID
	}{
		{
			name:           "get ban by id",
			banID:          ban.ID.String(),
			expectedStatus: http.StatusOK,
			userID:         adminID,
		},
		{
			name:           "missing ban_id",
			banID:          "",
			expectedStatus: http.StatusBadRequest,
			userID:         adminID,
		},
		{
			name:           "invalid ban_id",
			banID:          "invalid-uuid",
			expectedStatus: http.StatusBadRequest,
			userID:         adminID,
		},
		{
			name:           "ban not found",
			banID:          uuid.New().String(),
			expectedStatus: http.StatusNotFound,
			userID:         adminID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/ban?id="+tt.banID, nil)
			req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, tt.userID))
			w := httptest.NewRecorder()

			handler.GetBan(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestBanHandler_UpdateBan(t *testing.T) {
	handler, db, adminID := setupBanHandlerWithUser(t)

	room := models.Room{
		ID:   uuid.New(),
		Name: "Test Room",
	}
	db.Create(&room)

	user := models.User{
		ID:       uuid.New(),
		Username: "banneduser",
		Email:    "banned@example.com",
	}
	db.Create(&user)

	ban := models.Ban{
		ID:       uuid.New(),
		UserID:   user.ID,
		RoomID:   room.ID,
		BannedBy: adminID,
		Reason:   "Original reason",
	}
	db.Create(&ban)

	futureTime := time.Now().Add(24 * time.Hour)

	tests := []struct {
		name           string
		banID          string
		body           UpdateBanRequest
		expectedStatus int
		userID         uuid.UUID
		expectedReason string
	}{
		{
			name:           "update ban reason",
			banID:          ban.ID.String(),
			body:           UpdateBanRequest{Reason: "Updated reason"},
			expectedStatus: http.StatusOK,
			userID:         adminID,
			expectedReason: "Updated reason",
		},
		{
			name:           "update ban expires",
			banID:          ban.ID.String(),
			body:           UpdateBanRequest{ExpiresAt: &futureTime},
			expectedStatus: http.StatusOK,
			userID:         adminID,
			expectedReason: "Original reason",
		},
		{
			name:           "ban not found",
			banID:          uuid.New().String(),
			body:           UpdateBanRequest{Reason: "New reason"},
			expectedStatus: http.StatusNotFound,
			userID:         adminID,
			expectedReason: "",
		},
		{
			name:           "missing ban_id",
			banID:          "",
			body:           UpdateBanRequest{Reason: "New reason"},
			expectedStatus: http.StatusBadRequest,
			userID:         adminID,
			expectedReason: "",
		},
		{
			name:           "invalid ban_id",
			banID:          "invalid-uuid",
			body:           UpdateBanRequest{Reason: "New reason"},
			expectedStatus: http.StatusBadRequest,
			userID:         adminID,
			expectedReason: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPut, "/api/ban/update?id="+tt.banID, bytes.NewReader(body))
			req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, tt.userID))
			w := httptest.NewRecorder()

			handler.UpdateBan(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestBanHandler_DeleteBan(t *testing.T) {
	handler, db, adminID := setupBanHandlerWithUser(t)

	room := models.Room{
		ID:   uuid.New(),
		Name: "Test Room",
	}
	db.Create(&room)

	user := models.User{
		ID:       uuid.New(),
		Username: "banneduser",
		Email:    "banned@example.com",
	}
	db.Create(&user)

	ban := models.Ban{
		ID:       uuid.New(),
		UserID:   user.ID,
		RoomID:   room.ID,
		BannedBy: adminID,
		Reason:   "Test ban",
	}
	db.Create(&ban)

	tests := []struct {
		name           string
		banID          string
		expectedStatus int
		userID         uuid.UUID
	}{
		{
			name:           "delete ban",
			banID:          ban.ID.String(),
			expectedStatus: http.StatusOK,
			userID:         adminID,
		},
		{
			name:           "ban not found",
			banID:          uuid.New().String(),
			expectedStatus: http.StatusNotFound,
			userID:         adminID,
		},
		{
			name:           "missing ban_id",
			banID:          "",
			expectedStatus: http.StatusBadRequest,
			userID:         adminID,
		},
		{
			name:           "invalid ban_id",
			banID:          "invalid-uuid",
			expectedStatus: http.StatusBadRequest,
			userID:         adminID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodDelete, "/api/ban/delete?id="+tt.banID, nil)
			req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, tt.userID))
			w := httptest.NewRecorder()

			handler.DeleteBan(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestBanHandler_CheckUserBan(t *testing.T) {
	handler, db, adminID := setupBanHandlerWithUser(t)

	room := models.Room{
		ID:   uuid.New(),
		Name: "Test Room",
	}
	db.Create(&room)

	user := models.User{
		ID:       uuid.New(),
		Username: "banneduser",
		Email:    "banned@example.com",
	}
	db.Create(&user)

	ban := models.Ban{
		ID:       uuid.New(),
		UserID:   user.ID,
		RoomID:   room.ID,
		BannedBy: adminID,
		Reason:   "Test ban",
	}
	db.Create(&ban)

	pastTime := time.Now().Add(-24 * time.Hour)

	expiredUser := models.User{
		ID:       uuid.New(),
		Username: "expireduser",
		Email:    "expired@example.com",
	}
	db.Create(&expiredUser)

	expiredBan := models.Ban{
		ID:        uuid.New(),
		UserID:    expiredUser.ID,
		RoomID:    room.ID,
		BannedBy:  adminID,
		Reason:    "Expired ban",
		ExpiresAt: &pastTime,
	}
	db.Create(&expiredBan)

	tests := []struct {
		name           string
		userID         string
		roomID         string
		expectedStatus int
		expectedBanned bool
	}{
		{
			name:           "user is banned",
			userID:         user.ID.String(),
			roomID:         room.ID.String(),
			expectedStatus: http.StatusOK,
			expectedBanned: true,
		},
		{
			name:           "user is not banned",
			userID:         uuid.New().String(),
			roomID:         room.ID.String(),
			expectedStatus: http.StatusOK,
			expectedBanned: false,
		},
		{
			name:           "user banned but expired",
			userID:         expiredUser.ID.String(),
			roomID:         room.ID.String(),
			expectedStatus: http.StatusOK,
			expectedBanned: false,
		},
		{
			name:           "missing user_id",
			userID:         "",
			roomID:         room.ID.String(),
			expectedStatus: http.StatusBadRequest,
			expectedBanned: false,
		},
		{
			name:           "missing room_id",
			userID:         user.ID.String(),
			roomID:         "",
			expectedStatus: http.StatusBadRequest,
			expectedBanned: false,
		},
		{
			name:           "invalid user_id",
			userID:         "invalid-uuid",
			roomID:         room.ID.String(),
			expectedStatus: http.StatusBadRequest,
			expectedBanned: false,
		},
		{
			name:           "invalid room_id",
			userID:         user.ID.String(),
			roomID:         "invalid-uuid",
			expectedStatus: http.StatusBadRequest,
			expectedBanned: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/ban/check?user_id="+tt.userID+"&room_id="+tt.roomID, nil)
			w := httptest.NewRecorder()

			handler.CheckUserBan(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedStatus == http.StatusOK {
				var response map[string]interface{}
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Fatalf("failed to unmarshal response: %v", err)
				}
				banned, ok := response["banned"].(bool)
				if ok && banned != tt.expectedBanned {
					t.Errorf("expected banned=%v, got %v", tt.expectedBanned, banned)
				}
			}
		})
	}
}
