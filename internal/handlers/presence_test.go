package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"fmt"
	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"os"
)

func setupTestDBForPresence(t *testing.T) *gorm.DB {
	db, err := gorm.Open(postgres.Open(getTestDSN()), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	err = db.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.UserRoom{},
		&models.Presence{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupPresenceHandler(t *testing.T) (*PresenceHandler, *database.Database, uuid.UUID) {
	db := setupTestDBForPresence(t)
	testDB := &database.Database{DB: db}
	handler := NewPresenceHandler(testDB)

	user := models.User{
		ID:       uuid.New(),
		Username: "testuser",
		Email:    "test@example.com",
	}
	db.Create(&user)

	return handler, testDB, user.ID
}

func TestPresenceHandler_UpdatePresence(t *testing.T) {
	handler, _, userID := setupPresenceHandler(t)

	tests := []struct {
		name           string
		body           UpdatePresenceRequest
		expectedStatus int
		userID         uuid.UUID
	}{
		{
			name: "set online status",
			body: UpdatePresenceRequest{
				Status: "online",
			},
			expectedStatus: http.StatusOK,
			userID:         userID,
		},
		{
			name: "set away status",
			body: UpdatePresenceRequest{
				Status: "away",
			},
			expectedStatus: http.StatusOK,
			userID:         userID,
		},
		{
			name: "set busy status",
			body: UpdatePresenceRequest{
				Status: "busy",
			},
			expectedStatus: http.StatusOK,
			userID:         userID,
		},
		{
			name: "set offline status",
			body: UpdatePresenceRequest{
				Status: "offline",
			},
			expectedStatus: http.StatusOK,
			userID:         userID,
		},
		{
			name: "invalid status defaults to online",
			body: UpdatePresenceRequest{
				Status: "invalid",
			},
			expectedStatus: http.StatusOK,
			userID:         userID,
		},
		{
			name: "with room ID",
			body: UpdatePresenceRequest{
				Status: "online",
				RoomID: func() *uuid.UUID { id := uuid.New(); return &id }(),
			},
			expectedStatus: http.StatusOK,
			userID:         userID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/presence/update", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(addUserIDToContext(req.Context(), tt.userID))
			w := httptest.NewRecorder()

			handler.UpdatePresence(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d, body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestPresenceHandler_GetPresence(t *testing.T) {
	handler, db, userID := setupPresenceHandler(t)

	roomID := uuid.New()
	db.Create(&models.UserRoom{UserID: userID, RoomID: roomID, Role: "member"})

	presence := models.Presence{
		UserID:   userID,
		Status:   models.PresenceOnline,
		LastSeen: time.Now(),
	}
	db.Create(&presence)

	req := httptest.NewRequest(http.MethodGet, "/presence?room_id="+roomID.String(), nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetPresence(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestPresenceHandler_GetPresence_NotFound(t *testing.T) {
	handler, _, userID := setupPresenceHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/presence?room_id="+uuid.New().String(), nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetPresence(w, req)

	if w.Code != http.StatusForbidden && w.Code != http.StatusOK {
		t.Errorf("expected status %d or %d, got %d", http.StatusForbidden, http.StatusOK, w.Code)
	}
}

func TestPresenceHandler_GetUserPresence(t *testing.T) {
	handler, db, userID := setupPresenceHandler(t)

	presence := models.Presence{
		UserID:   userID,
		Status:   models.PresenceOnline,
		LastSeen: time.Now(),
	}
	db.Create(&presence)

	tests := []struct {
		name           string
		targetUserID   uuid.UUID
		expectedStatus int
	}{
		{
			name:           "valid user presence",
			targetUserID:   userID,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "user not found",
			targetUserID:   uuid.New(),
			expectedStatus: http.StatusOK,
		},
		{
			name:           "missing user_id",
			targetUserID:   uuid.Nil,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/presence/user?user_id=" + tt.targetUserID.String()
			if tt.name == "missing user_id" {
				url = "/presence/user"
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			w := httptest.NewRecorder()

			handler.GetUserPresence(w, req)

			if tt.name == "missing user_id" {
				if w.Code != tt.expectedStatus && w.Code != http.StatusBadRequest {
					t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
				}
			} else if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func getTestDSN() string {
	host := getEnv("DB_HOST", "localhost")
	port := getEnv("DB_PORT", "5432")
	user := getEnv("DB_USER", "enclavr")
	password := getEnv("DB_PASSWORD", "enclavr")
	dbname := getEnv("DB_NAME", "enclavr_test")
	sslmode := getEnv("DB_SSLMODE", "disable")
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbname, sslmode)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
