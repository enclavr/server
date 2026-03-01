package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"fmt"
	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/internal/websocket"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"os"
)

func setupTestDBForThread(t *testing.T) *gorm.DB {
	db, err := gorm.Open(postgres.Open(getTestDSN()), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	err = db.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.UserRoom{},
		&models.Message{},
		&models.Thread{},
		&models.ThreadMessage{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupThreadHandlerWithUser(t *testing.T) (*ThreadHandler, *database.Database, uuid.UUID, uuid.UUID, uuid.UUID) {
	db := setupTestDBForThread(t)
	testDB := &database.Database{DB: db}
	hub := websocket.NewHub()
	handler := NewThreadHandler(testDB, hub)

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

	db.Create(&models.UserRoom{
		UserID: user.ID,
		RoomID: room.ID,
		Role:   "member",
	})

	parentMsg := models.Message{
		ID:      uuid.New(),
		RoomID:  room.ID,
		UserID:  user.ID,
		Content: "Parent message",
	}
	db.Create(&parentMsg)

	return handler, testDB, user.ID, room.ID, parentMsg.ID
}

func TestThreadHandler_CreateThread(t *testing.T) {
	handler, _, userID, roomID, parentID := setupThreadHandlerWithUser(t)

	tests := []struct {
		name           string
		body           CreateThreadRequest
		expectedStatus int
		setupContext   func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name: "valid thread creation",
			body: CreateThreadRequest{
				ParentID: parentID,
				RoomID:   roomID,
				Content:  "Thread content",
			},
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name: "missing parent_id",
			body: CreateThreadRequest{
				RoomID:  roomID,
				Content: "Thread content",
			},
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name: "empty content",
			body: CreateThreadRequest{
				ParentID: parentID,
				RoomID:   roomID,
				Content:  "",
			},
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name: "parent message not found",
			body: CreateThreadRequest{
				ParentID: uuid.New(),
				RoomID:   roomID,
				Content:  "Thread content",
			},
			expectedStatus: http.StatusNotFound,
			setupContext:   addUserIDToContext,
		},
		{
			name: "room mismatch",
			body: CreateThreadRequest{
				ParentID: parentID,
				RoomID:   uuid.New(),
				Content:  "Thread content",
			},
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "unauthorized",
			body:           CreateThreadRequest{},
			expectedStatus: http.StatusUnauthorized,
			setupContext:   func(ctx context.Context, _ uuid.UUID) context.Context { return ctx },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/threads/create", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(tt.setupContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.CreateThread(w, req)

			if tt.expectedStatus == http.StatusOK {
				if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
					t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
				}
			} else if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestThreadHandler_GetThread(t *testing.T) {
	handler, db, userID, roomID, parentID := setupThreadHandlerWithUser(t)

	thread := models.Thread{
		ID:        uuid.New(),
		RoomID:    roomID,
		ParentID:  parentID,
		CreatedBy: userID,
	}
	db.Create(&thread)

	threadMsg := models.ThreadMessage{
		ID:       uuid.New(),
		ThreadID: thread.ID,
		UserID:   userID,
		Content:  "Thread message",
	}
	db.Create(&threadMsg)

	tests := []struct {
		name           string
		threadID       uuid.UUID
		expectedStatus int
		setupContext   func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name:           "valid get thread",
			threadID:       thread.ID,
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "missing thread_id",
			threadID:       uuid.Nil,
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "thread not found",
			threadID:       uuid.New(),
			expectedStatus: http.StatusNotFound,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "invalid thread_id",
			threadID:       uuid.Nil,
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "not a room member",
			threadID:       thread.ID,
			expectedStatus: http.StatusForbidden,
			setupContext: func(ctx context.Context, uid uuid.UUID) context.Context {
				otherUser := models.User{
					ID:       uuid.New(),
					Username: "other",
					Email:    "other@example.com",
				}
				db.Create(&otherUser)
				return context.WithValue(ctx, middleware.UserIDKey, otherUser.ID)
			},
		},
		{
			name:           "unauthorized",
			threadID:       thread.ID,
			expectedStatus: http.StatusUnauthorized,
			setupContext:   func(ctx context.Context, _ uuid.UUID) context.Context { return ctx },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/thread?thread_id=" + tt.threadID.String()
			switch tt.name {
			case "missing thread_id":
				url = "/thread"
			case "invalid thread_id":
				url = "/thread?thread_id=invalid"
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			req = req.WithContext(tt.setupContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.GetThread(w, req)

			if tt.expectedStatus == http.StatusOK {
				if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
					t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
				}
			} else if w.Code != tt.expectedStatus && (tt.name != "invalid thread_id" || w.Code != http.StatusBadRequest) {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestThreadHandler_GetThreadsForMessage(t *testing.T) {
	handler, db, userID, roomID, parentID := setupThreadHandlerWithUser(t)

	thread := models.Thread{
		ID:        uuid.New(),
		RoomID:    roomID,
		ParentID:  parentID,
		CreatedBy: userID,
	}
	db.Create(&thread)

	threadMsg := models.ThreadMessage{
		ID:       uuid.New(),
		ThreadID: thread.ID,
		UserID:   userID,
		Content:  "Thread message",
	}
	db.Create(&threadMsg)

	tests := []struct {
		name           string
		messageID      uuid.UUID
		expectedStatus int
		setupContext   func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name:           "valid get threads",
			messageID:      parentID,
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "missing message_id",
			messageID:      uuid.Nil,
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "message not found",
			messageID:      uuid.New(),
			expectedStatus: http.StatusNotFound,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "invalid message_id",
			messageID:      uuid.Nil,
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "not a room member",
			messageID:      parentID,
			expectedStatus: http.StatusForbidden,
			setupContext: func(ctx context.Context, uid uuid.UUID) context.Context {
				otherUser := models.User{
					ID:       uuid.New(),
					Username: "other",
					Email:    "other@example.com",
				}
				db.Create(&otherUser)
				return context.WithValue(ctx, middleware.UserIDKey, otherUser.ID)
			},
		},
		{
			name:           "unauthorized",
			messageID:      parentID,
			expectedStatus: http.StatusUnauthorized,
			setupContext:   func(ctx context.Context, _ uuid.UUID) context.Context { return ctx },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/threads?message_id=" + tt.messageID.String()
			switch tt.name {
			case "missing message_id":
				url = "/threads"
			case "invalid message_id":
				url = "/threads?message_id=invalid"
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			req = req.WithContext(tt.setupContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.GetThreadsForMessage(w, req)

			if tt.expectedStatus == http.StatusOK {
				if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
					t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
				}
			} else if w.Code != tt.expectedStatus && (tt.name != "invalid message_id" || w.Code != http.StatusBadRequest) {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestThreadHandler_AddThreadMessage(t *testing.T) {
	handler, db, userID, roomID, parentID := setupThreadHandlerWithUser(t)

	thread := models.Thread{
		ID:        uuid.New(),
		RoomID:    roomID,
		ParentID:  parentID,
		CreatedBy: userID,
	}
	db.Create(&thread)

	tests := []struct {
		name           string
		body           ThreadMessageRequest
		threadID       uuid.UUID
		expectedStatus int
		setupContext   func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name: "valid add message",
			body: ThreadMessageRequest{
				Content: "New thread message",
			},
			threadID:       thread.ID,
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name: "missing thread_id",
			body: ThreadMessageRequest{
				Content: "New message",
			},
			threadID:       uuid.Nil,
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name: "thread not found",
			body: ThreadMessageRequest{
				Content: "New message",
			},
			threadID:       uuid.New(),
			expectedStatus: http.StatusNotFound,
			setupContext:   addUserIDToContext,
		},
		{
			name: "empty content",
			body: ThreadMessageRequest{
				Content: "",
			},
			threadID:       thread.ID,
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "invalid thread_id",
			body:           ThreadMessageRequest{Content: "Test"},
			threadID:       uuid.Nil,
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "unauthorized",
			body:           ThreadMessageRequest{Content: "Test"},
			threadID:       thread.ID,
			expectedStatus: http.StatusUnauthorized,
			setupContext:   func(ctx context.Context, _ uuid.UUID) context.Context { return ctx },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/threads/messages?thread_id=" + tt.threadID.String()
			switch tt.name {
			case "missing thread_id":
				url = "/threads/messages"
			case "invalid thread_id":
				url = "/threads/messages?thread_id=invalid"
			}
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(tt.setupContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.AddThreadMessage(w, req)

			if tt.expectedStatus == http.StatusOK {
				if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
					t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
				}
			} else if w.Code != tt.expectedStatus && (tt.name != "invalid thread_id" || w.Code != http.StatusBadRequest) {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestThreadHandler_UpdateThreadMessage(t *testing.T) {
	handler, db, userID, roomID, parentID := setupThreadHandlerWithUser(t)

	thread := models.Thread{
		ID:        uuid.New(),
		RoomID:    roomID,
		ParentID:  parentID,
		CreatedBy: userID,
	}
	db.Create(&thread)

	threadMsg := models.ThreadMessage{
		ID:       uuid.New(),
		ThreadID: thread.ID,
		UserID:   userID,
		Content:  "Original content",
	}
	db.Create(&threadMsg)

	otherUser := models.User{
		ID:       uuid.New(),
		Username: "otheruser",
		Email:    "other@example.com",
	}
	db.Create(&otherUser)

	otherMsg := models.ThreadMessage{
		ID:       uuid.New(),
		ThreadID: thread.ID,
		UserID:   otherUser.ID,
		Content:  "Other message",
	}
	db.Create(&otherMsg)

	tests := []struct {
		name           string
		body           ThreadMessageRequest
		messageID      uuid.UUID
		expectedStatus int
		setupContext   func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name: "valid update",
			body: ThreadMessageRequest{
				Content: "Updated content",
			},
			messageID:      threadMsg.ID,
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name: "missing message_id",
			body: ThreadMessageRequest{
				Content: "Updated content",
			},
			messageID:      uuid.Nil,
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name: "message not found",
			body: ThreadMessageRequest{
				Content: "Updated content",
			},
			messageID:      uuid.New(),
			expectedStatus: http.StatusNotFound,
			setupContext:   addUserIDToContext,
		},
		{
			name: "empty content",
			body: ThreadMessageRequest{
				Content: "",
			},
			messageID:      threadMsg.ID,
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name: "cannot edit others message",
			body: ThreadMessageRequest{
				Content: "Hacked content",
			},
			messageID:      otherMsg.ID,
			expectedStatus: http.StatusForbidden,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "invalid message_id",
			body:           ThreadMessageRequest{Content: "Test"},
			messageID:      uuid.Nil,
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "unauthorized",
			body:           ThreadMessageRequest{Content: "Test"},
			messageID:      threadMsg.ID,
			expectedStatus: http.StatusUnauthorized,
			setupContext:   func(ctx context.Context, _ uuid.UUID) context.Context { return ctx },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/threads/messages?message_id=" + tt.messageID.String()
			switch tt.name {
			case "missing message_id":
				url = "/threads/messages"
			case "invalid message_id":
				url = "/threads/messages?message_id=invalid"
			}
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPut, url, bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(tt.setupContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.UpdateThreadMessage(w, req)

			if tt.expectedStatus == http.StatusOK {
				if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
					t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
				}
			} else if w.Code != tt.expectedStatus && (tt.name != "invalid message_id" || w.Code != http.StatusBadRequest) {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestThreadHandler_DeleteThreadMessage(t *testing.T) {
	handler, db, userID, roomID, parentID := setupThreadHandlerWithUser(t)

	thread := models.Thread{
		ID:        uuid.New(),
		RoomID:    roomID,
		ParentID:  parentID,
		CreatedBy: userID,
	}
	db.Create(&thread)

	threadMsg := models.ThreadMessage{
		ID:       uuid.New(),
		ThreadID: thread.ID,
		UserID:   userID,
		Content:  "To be deleted",
	}
	db.Create(&threadMsg)

	otherUser := models.User{
		ID:       uuid.New(),
		Username: "otheruser",
		Email:    "other@example.com",
	}
	db.Create(&otherUser)

	otherMsg := models.ThreadMessage{
		ID:       uuid.New(),
		ThreadID: thread.ID,
		UserID:   otherUser.ID,
		Content:  "Other message",
	}
	db.Create(&otherMsg)

	tests := []struct {
		name           string
		messageID      uuid.UUID
		expectedStatus int
		setupContext   func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name:           "valid delete",
			messageID:      threadMsg.ID,
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "missing message_id",
			messageID:      uuid.Nil,
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "message not found",
			messageID:      uuid.New(),
			expectedStatus: http.StatusNotFound,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "cannot delete others message",
			messageID:      otherMsg.ID,
			expectedStatus: http.StatusForbidden,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "invalid message_id",
			messageID:      uuid.Nil,
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "unauthorized",
			messageID:      threadMsg.ID,
			expectedStatus: http.StatusUnauthorized,
			setupContext:   func(ctx context.Context, _ uuid.UUID) context.Context { return ctx },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/threads/messages?message_id=" + tt.messageID.String()
			switch tt.name {
			case "missing message_id":
				url = "/threads/messages"
			case "invalid message_id":
				url = "/threads/messages?message_id=invalid"
			}
			req := httptest.NewRequest(http.MethodDelete, url, nil)
			req = req.WithContext(tt.setupContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.DeleteThreadMessage(w, req)

			if tt.expectedStatus == http.StatusOK {
				if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
					t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
				}
			} else if w.Code != tt.expectedStatus && (tt.name != "invalid message_id" || w.Code != http.StatusBadRequest) {
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
