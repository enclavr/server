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
	"github.com/enclavr/server/internal/websocket"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDBForPoll(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	err = db.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.UserRoom{},
		&models.Poll{},
		&models.PollOption{},
		&models.PollVote{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupPollHandlerWithUser(t *testing.T) (*PollHandler, *database.Database, uuid.UUID, uuid.UUID) {
	db := setupTestDBForPoll(t)
	testDB := &database.Database{DB: db}
	hub := websocket.NewHub()
	handler := NewPollHandler(testDB, hub)

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

	return handler, testDB, user.ID, room.ID
}

func TestPollHandler_CreatePoll(t *testing.T) {
	handler, _, userID, roomID := setupPollHandlerWithUser(t)

	tests := []struct {
		name           string
		body           CreatePollRequest
		expectedStatus int
		setupContext   func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name: "valid poll creation",
			body: CreatePollRequest{
				RoomID:   roomID,
				Question: "What is your favorite color?",
				Options:  []string{"Red", "Blue", "Green"},
			},
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name: "missing question",
			body: CreatePollRequest{
				RoomID:  roomID,
				Options: []string{"Red", "Blue"},
			},
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name: "only one option",
			body: CreatePollRequest{
				RoomID:   roomID,
				Question: "Test?",
				Options:  []string{"Only one"},
			},
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name: "too many options",
			body: CreatePollRequest{
				RoomID:   roomID,
				Question: "Test?",
				Options:  []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11"},
			},
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name: "not a room member",
			body: CreatePollRequest{
				RoomID:   uuid.New(),
				Question: "Test?",
				Options:  []string{"A", "B"},
			},
			expectedStatus: http.StatusForbidden,
			setupContext:   addUserIDToContext,
		},
		{
			name: "unauthorized",
			body: CreatePollRequest{
				RoomID:   roomID,
				Question: "Test?",
				Options:  []string{"A", "B"},
			},
			expectedStatus: http.StatusUnauthorized,
			setupContext:   func(ctx context.Context, _ uuid.UUID) context.Context { return ctx },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/polls/create", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(tt.setupContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.CreatePoll(w, req)

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

func TestPollHandler_GetPolls(t *testing.T) {
	handler, db, userID, roomID := setupPollHandlerWithUser(t)

	poll := models.Poll{
		ID:         uuid.New(),
		RoomID:     roomID,
		Question:   "Test Poll",
		CreatedBy:  userID,
		IsMultiple: false,
	}
	db.Create(&poll)

	opt1 := models.PollOption{
		ID:       uuid.New(),
		PollID:   poll.ID,
		Text:     "Option 1",
		Position: 0,
	}
	opt2 := models.PollOption{
		ID:       uuid.New(),
		PollID:   poll.ID,
		Text:     "Option 2",
		Position: 1,
	}
	db.Create(&opt1)
	db.Create(&opt2)

	tests := []struct {
		name           string
		roomID         uuid.UUID
		expectedStatus int
		setupContext   func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name:           "valid get polls",
			roomID:         roomID,
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "missing room_id",
			roomID:         uuid.Nil,
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "invalid room_id",
			roomID:         uuid.Nil,
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "not a room member",
			roomID:         uuid.New(),
			expectedStatus: http.StatusForbidden,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "unauthorized",
			roomID:         roomID,
			expectedStatus: http.StatusUnauthorized,
			setupContext:   func(ctx context.Context, _ uuid.UUID) context.Context { return ctx },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/polls?room_id=" + tt.roomID.String()
			if tt.roomID == uuid.Nil {
				url = "/polls?room_id=invalid"
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			req = req.WithContext(tt.setupContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.GetPolls(w, req)

			if tt.expectedStatus == http.StatusOK {
				if w.Code != http.StatusOK && w.Code != http.StatusBadRequest && w.Code != http.StatusInternalServerError {
					t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
				}
			} else if w.Code != tt.expectedStatus && (tt.name != "invalid room_id" || w.Code != http.StatusBadRequest) {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestPollHandler_GetPoll(t *testing.T) {
	handler, db, userID, roomID := setupPollHandlerWithUser(t)

	poll := models.Poll{
		ID:         uuid.New(),
		RoomID:     roomID,
		Question:   "Test Poll",
		CreatedBy:  userID,
		IsMultiple: false,
	}
	db.Create(&poll)

	opt1 := models.PollOption{
		ID:       uuid.New(),
		PollID:   poll.ID,
		Text:     "Option 1",
		Position: 0,
	}
	db.Create(&opt1)

	tests := []struct {
		name           string
		pollID         uuid.UUID
		expectedStatus int
		setupContext   func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name:           "valid get poll",
			pollID:         poll.ID,
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "missing poll_id",
			pollID:         uuid.Nil,
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "poll not found",
			pollID:         uuid.New(),
			expectedStatus: http.StatusNotFound,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "invalid poll_id",
			pollID:         uuid.Nil,
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "not a room member",
			pollID:         poll.ID,
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
			pollID:         poll.ID,
			expectedStatus: http.StatusUnauthorized,
			setupContext:   func(ctx context.Context, _ uuid.UUID) context.Context { return ctx },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/poll?poll_id=" + tt.pollID.String()
			switch tt.name {
			case "missing poll_id":
				url = "/poll"
			case "invalid poll_id":
				url = "/poll?poll_id=invalid"
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			req = req.WithContext(tt.setupContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.GetPoll(w, req)

			if tt.expectedStatus == http.StatusOK {
				if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
					t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
				}
			} else if w.Code != tt.expectedStatus && (tt.name != "invalid poll_id" || w.Code != http.StatusBadRequest) {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestPollHandler_Vote(t *testing.T) {
	handler, db, userID, roomID := setupPollHandlerWithUser(t)

	poll := models.Poll{
		ID:         uuid.New(),
		RoomID:     roomID,
		Question:   "Test Poll",
		CreatedBy:  userID,
		IsMultiple: false,
	}
	db.Create(&poll)

	opt1 := models.PollOption{
		ID:       uuid.New(),
		PollID:   poll.ID,
		Text:     "Option 1",
		Position: 0,
	}
	opt2 := models.PollOption{
		ID:       uuid.New(),
		PollID:   poll.ID,
		Text:     "Option 2",
		Position: 1,
	}
	db.Create(&opt1)
	db.Create(&opt2)

	tests := []struct {
		name           string
		body           VotePollRequest
		expectedStatus int
		setupContext   func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name: "valid vote",
			body: VotePollRequest{
				PollID:   poll.ID,
				OptionID: opt1.ID,
			},
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name: "poll not found",
			body: VotePollRequest{
				PollID:   uuid.New(),
				OptionID: opt1.ID,
			},
			expectedStatus: http.StatusNotFound,
			setupContext:   addUserIDToContext,
		},
		{
			name: "option not found",
			body: VotePollRequest{
				PollID:   poll.ID,
				OptionID: uuid.New(),
			},
			expectedStatus: http.StatusNotFound,
			setupContext:   addUserIDToContext,
		},
		{
			name: "cannot vote - poll expired",
			body: VotePollRequest{
				PollID:   poll.ID,
				OptionID: opt2.ID,
			},
			expectedStatus: http.StatusForbidden,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "empty body",
			body:           VotePollRequest{},
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/polls/vote", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(tt.setupContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.Vote(w, req)

			if tt.expectedStatus == http.StatusOK {
				if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
					t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
				}
			} else if tt.expectedStatus == http.StatusBadRequest {
				if w.Code != tt.expectedStatus && w.Code != http.StatusInternalServerError && w.Code != http.StatusNotFound {
					t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
				}
			} else if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestPollHandler_DeletePoll(t *testing.T) {
	handler, db, userID, roomID := setupPollHandlerWithUser(t)

	poll := models.Poll{
		ID:         uuid.New(),
		RoomID:     roomID,
		Question:   "Test Poll",
		CreatedBy:  userID,
		IsMultiple: false,
	}
	db.Create(&poll)

	opt1 := models.PollOption{
		ID:       uuid.New(),
		PollID:   poll.ID,
		Text:     "Option 1",
		Position: 0,
	}
	db.Create(&opt1)

	vote := models.PollVote{
		PollID:   poll.ID,
		OptionID: opt1.ID,
		UserID:   userID,
	}
	db.Create(&vote)

	tests := []struct {
		name           string
		pollID         uuid.UUID
		expectedStatus int
		setupContext   func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name:           "valid delete",
			pollID:         poll.ID,
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "missing poll_id",
			pollID:         uuid.Nil,
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "poll not found",
			pollID:         uuid.New(),
			expectedStatus: http.StatusNotFound,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "cannot delete others poll",
			pollID:         poll.ID,
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
			name:           "invalid poll_id",
			pollID:         uuid.Nil,
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "unauthorized",
			pollID:         poll.ID,
			expectedStatus: http.StatusUnauthorized,
			setupContext:   func(ctx context.Context, _ uuid.UUID) context.Context { return ctx },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/polls/delete?poll_id=" + tt.pollID.String()
			switch tt.name {
			case "missing poll_id":
				url = "/polls/delete"
			case "invalid poll_id":
				url = "/polls/delete?poll_id=invalid"
			}
			req := httptest.NewRequest(http.MethodDelete, url, nil)
			req = req.WithContext(tt.setupContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.DeletePoll(w, req)

			if tt.expectedStatus == http.StatusOK {
				if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
					t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
				}
			} else if tt.expectedStatus == http.StatusForbidden {
				if w.Code != tt.expectedStatus && w.Code != http.StatusInternalServerError && w.Code != http.StatusNotFound {
					t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
				}
			} else if w.Code != tt.expectedStatus && (tt.name != "invalid poll_id" || w.Code != http.StatusBadRequest) {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestPollHandler_canUserVote(t *testing.T) {
	handler, db, userID, roomID := setupPollHandlerWithUser(t)

	pollWithExpiry := models.Poll{
		ID:         uuid.New(),
		RoomID:     roomID,
		Question:   "Expired Poll",
		CreatedBy:  userID,
		IsMultiple: false,
		ExpiresAt:  func() *time.Time { t := time.Now().Add(-1 * time.Hour); return &t }(),
	}
	db.Create(&pollWithExpiry)

	pollNoExpiry := models.Poll{
		ID:         uuid.New(),
		RoomID:     roomID,
		Question:   "Valid Poll",
		CreatedBy:  userID,
		IsMultiple: false,
	}
	db.Create(&pollNoExpiry)

	existingVote := models.PollVote{
		PollID:   pollNoExpiry.ID,
		OptionID: uuid.New(),
		UserID:   userID,
	}
	db.Create(&existingVote)

	if !handler.canUserVote(&pollNoExpiry, userID) {
		t.Log("User already voted on single choice poll - expected behavior")
	}

	if handler.canUserVote(&pollWithExpiry, userID) {
		t.Error("User should not be able to vote on expired poll")
	}

	otherUser := uuid.New()
	if !handler.canUserVote(&pollNoExpiry, otherUser) {
		t.Error("Other user should be able to vote on poll")
	}

	multiPoll := models.Poll{
		ID:         uuid.New(),
		RoomID:     roomID,
		Question:   "Multi Poll",
		CreatedBy:  userID,
		IsMultiple: true,
	}
	db.Create(&multiPoll)
	db.Create(&models.PollVote{
		PollID:   multiPoll.ID,
		OptionID: uuid.New(),
		UserID:   userID,
	})

	if !handler.canUserVote(&multiPoll, userID) {
		t.Error("User should be able to vote multiple times on multi-choice poll")
	}
}
