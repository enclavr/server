package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"fmt"
	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"os"
)

func setupTestDBForUserHandler(t *testing.T) *gorm.DB {
	db, err := gorm.Open(postgres.Open(getTestDSN()), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	err = db.AutoMigrate(&models.User{})
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupUserHandler(t *testing.T) *UserHandler {
	db := setupTestDBForUserHandler(t)
	testDB := &database.Database{DB: db}
	handler := NewUserHandler(testDB)
	return handler
}

func TestUserHandler_SearchUsers(t *testing.T) {
	handler := setupUserHandler(t)

	testUser := models.User{
		Username:    "testuser",
		DisplayName: "Test User",
	}
	handler.db.Create(&testUser)

	tests := []struct {
		name           string
		query          string
		expectedStatus int
		expectedCount  int
	}{
		{
			name:           "valid search with results",
			query:          "test",
			expectedStatus: http.StatusOK,
			expectedCount:  1,
		},
		{
			name:           "search with no results",
			query:          "nonexistent",
			expectedStatus: http.StatusOK,
			expectedCount:  0,
		},
		{
			name:           "missing query parameter",
			query:          "",
			expectedStatus: http.StatusBadRequest,
			expectedCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/users/search?username="+tt.query, nil)
			w := httptest.NewRecorder()

			handler.SearchUsers(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedStatus == http.StatusOK {
				var results []UserSearchResult
				if err := json.Unmarshal(w.Body.Bytes(), &results); err != nil {
					t.Fatalf("failed to unmarshal response: %v", err)
				}
				if len(results) != tt.expectedCount {
					t.Errorf("expected %d results, got %d", tt.expectedCount, len(results))
				}
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
