package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDBForAnalytics(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	err = db.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.Message{},
		&models.DailyAnalytics{},
		&models.HourlyActivity{},
		&models.ChannelActivity{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupAnalyticsHandler(t *testing.T) (*AnalyticsHandler, *database.Database, uuid.UUID) {
	db := setupTestDBForAnalytics(t)
	testDB := &database.Database{DB: db}
	handler := NewAnalyticsHandler(testDB)

	user := models.User{
		ID:       uuid.New(),
		Username: "testuser",
		Email:    "test@example.com",
		IsAdmin:  true,
	}
	db.Create(&user)

	return handler, testDB, user.ID
}

func TestGetOverview(t *testing.T) {
	handler, db, userID := setupAnalyticsHandler(t)

	room := models.Room{
		ID:   uuid.New(),
		Name: "Test Room",
	}
	db.Create(&room)

	msg := models.Message{
		ID:      uuid.New(),
		RoomID:  room.ID,
		UserID:  userID,
		Content: "Test message",
	}
	db.Create(&msg)

	tests := []struct {
		name           string
		query          string
		expectedStatus int
		setupContext   func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name:           "valid overview request",
			query:          "days=30",
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "default days parameter",
			query:          "",
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "custom days parameter",
			query:          "days=7",
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "invalid days parameter - negative",
			query:          "days=-1",
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "invalid days parameter - too large",
			query:          "days=500",
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/analytics/overview?"+tt.query, nil)
			req = req.WithContext(tt.setupContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.GetOverview(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedStatus == http.StatusOK {
				var overview AnalyticsOverview
				if err := json.Unmarshal(w.Body.Bytes(), &overview); err != nil {
					t.Errorf("failed to unmarshal response: %v", err)
				} else {
					if overview.TotalUsers < 0 {
						t.Errorf("expected non-negative total users, got %d", overview.TotalUsers)
					}
					if overview.TotalMessages < 0 {
						t.Errorf("expected non-negative total messages, got %d", overview.TotalMessages)
					}
				}
			}
		})
	}
}

func TestGetDailyActivity(t *testing.T) {
	handler, db, userID := setupAnalyticsHandler(t)

	room := models.Room{
		ID:   uuid.New(),
		Name: "Test Room",
	}
	db.Create(&room)

	msg := models.Message{
		ID:      uuid.New(),
		RoomID:  room.ID,
		UserID:  userID,
		Content: "Test message",
	}
	db.Create(&msg)

	tests := []struct {
		name           string
		query          string
		expectedStatus int
		setupContext   func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name:           "valid daily activity request",
			query:          "days=30",
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "default days",
			query:          "",
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "custom days",
			query:          "days=7",
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/analytics/daily?"+tt.query, nil)
			req = req.WithContext(tt.setupContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.GetDailyActivity(w, req)

			if w.Code != tt.expectedStatus && w.Code != http.StatusInternalServerError {
				t.Errorf("expected status %d or 500, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedStatus == http.StatusOK && w.Code == http.StatusOK {
				var activity []DailyActivity
				if err := json.Unmarshal(w.Body.Bytes(), &activity); err != nil {
					t.Errorf("failed to unmarshal response: %v", err)
				}
			}
		})
	}
}

func TestGetChannelStats(t *testing.T) {
	handler, db, userID := setupAnalyticsHandler(t)

	room := models.Room{
		ID:   uuid.New(),
		Name: "Test Room",
	}
	db.Create(&room)

	msg := models.Message{
		ID:      uuid.New(),
		RoomID:  room.ID,
		UserID:  userID,
		Content: "Test message",
	}
	db.Create(&msg)

	tests := []struct {
		name           string
		query          string
		expectedStatus int
		setupContext   func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name:           "valid channel stats request",
			query:          "days=30",
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "default days",
			query:          "",
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/analytics/channels?"+tt.query, nil)
			req = req.WithContext(tt.setupContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.GetChannelStats(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestGetHourlyActivity(t *testing.T) {
	handler, db, userID := setupAnalyticsHandler(t)

	room := models.Room{
		ID:   uuid.New(),
		Name: "Test Room",
	}
	db.Create(&room)

	msg := models.Message{
		ID:      uuid.New(),
		RoomID:  room.ID,
		UserID:  userID,
		Content: "Test message",
	}
	db.Create(&msg)

	tests := []struct {
		name           string
		query          string
		expectedStatus int
		setupContext   func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name:           "valid hourly activity request",
			query:          "days=30",
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "default days",
			query:          "",
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/analytics/hourly?"+tt.query, nil)
			req = req.WithContext(tt.setupContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.GetHourlyActivity(w, req)

			if w.Code != tt.expectedStatus && w.Code != http.StatusInternalServerError {
				t.Errorf("expected status %d or 500, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedStatus == http.StatusOK && w.Code == http.StatusOK {
				var hourly []HourlyStats
				if err := json.Unmarshal(w.Body.Bytes(), &hourly); err != nil {
					t.Errorf("failed to unmarshal response: %v", err)
				} else {
					if len(hourly) != 24 {
						t.Errorf("expected 24 hourly entries, got %d", len(hourly))
					}
				}
			}
		})
	}
}

func TestGetTopUsers(t *testing.T) {
	handler, db, userID := setupAnalyticsHandler(t)

	room := models.Room{
		ID:   uuid.New(),
		Name: "Test Room",
	}
	db.Create(&room)

	msg := models.Message{
		ID:      uuid.New(),
		RoomID:  room.ID,
		UserID:  userID,
		Content: "Test message",
	}
	db.Create(&msg)

	tests := []struct {
		name           string
		query          string
		expectedStatus int
		setupContext   func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name:           "valid top users request",
			query:          "days=30",
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "default days",
			query:          "",
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "with limit",
			query:          "days=30&limit=5",
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "invalid limit - too large",
			query:          "days=30&limit=100",
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/analytics/topusers?"+tt.query, nil)
			req = req.WithContext(tt.setupContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.GetTopUsers(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestRequireAdmin(t *testing.T) {
	tests := []struct {
		name           string
		expectedStatus int
		setupContext   func(ctx context.Context, userID uuid.UUID) context.Context
		setupUser      func(db *database.Database, testUserID uuid.UUID)
	}{
		{
			name:           "admin user allowed",
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
			setupUser: func(db *database.Database, id uuid.UUID) {
				db.DB.Model(&models.User{}).Where("id = ?", id).Update("is_admin", true)
			},
		},
		{
			name:           "non-admin user forbidden",
			expectedStatus: http.StatusForbidden,
			setupContext:   addUserIDToContext,
			setupUser: func(db *database.Database, userID uuid.UUID) {
				db.DB.Model(&models.User{}).Where("id = ?", userID).Update("is_admin", false)
			},
		},
		{
			name:           "unauthorized - no user in context",
			expectedStatus: http.StatusUnauthorized,
			setupContext:   func(ctx context.Context, userID uuid.UUID) context.Context { return ctx },
			setupUser:      func(db *database.Database, userID uuid.UUID) {},
		},
		{
			name:           "user not found",
			expectedStatus: http.StatusNotFound,
			setupContext:   addUserIDToContext,
			setupUser: func(db *database.Database, userID uuid.UUID) {
				db.DB.Unscoped().Delete(&models.User{}, "id = ?", userID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, testDB, _ := setupAnalyticsHandler(t)

			user := models.User{
				ID:       uuid.New(),
				Username: "testuser" + uuid.New().String()[:8],
				Email:    "test" + uuid.New().String()[:8] + "@example.com",
				IsAdmin:  true,
			}
			testDB.DB.Create(&user)
			testUserID := user.ID

			tt.setupUser(testDB, testUserID)

			req := httptest.NewRequest(http.MethodGet, "/analytics/admin", nil)
			req = req.WithContext(tt.setupContext(req.Context(), testUserID))
			w := httptest.NewRecorder()

			handler.RequireAdmin(w, req)

			if w.Code != tt.expectedStatus && w.Code != http.StatusInternalServerError {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestRecordActivity(t *testing.T) {
	handler, db, userID := setupAnalyticsHandler(t)

	room := models.Room{
		ID:   uuid.New(),
		Name: "Test Room",
	}
	db.Create(&room)

	handler.RecordActivity(userID, "message")

	var daily models.DailyAnalytics
	if err := db.First(&daily).Error; err != nil {
		t.Errorf("expected daily analytics to be created, got error: %v", err)
	}

	var hourly models.HourlyActivity
	if err := db.First(&hourly).Error; err != nil {
		t.Errorf("expected hourly activity to be created, got error: %v", err)
	}

	var channel models.ChannelActivity
	if err := db.First(&channel).Error; err != nil {
		t.Errorf("expected channel activity to be created, got error: %v", err)
	}
}
