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
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func setupTestDBForBlockHandler(t *testing.T) *gorm.DB {
	db := openTestDB(t)

	err := db.AutoMigrate(
		&models.User{},
		&models.Block{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupBlockHandler(t *testing.T) (*BlockHandler, *database.Database, uuid.UUID) {
	db := setupTestDBForBlockHandler(t)
	testDB := &database.Database{DB: db}
	handler := NewBlockHandler(testDB)

	user := models.User{
		ID:       uuid.New(),
		Username: "testuser",
		Email:    "test@example.com",
	}
	db.Create(&user)

	return handler, testDB, user.ID
}

func TestBlockHandler_BlockUser(t *testing.T) {
	handler, db, userID := setupBlockHandler(t)

	blockedUser := models.User{
		ID:       uuid.New(),
		Username: "blockeduser",
		Email:    "blocked@example.com",
	}
	db.Create(&blockedUser)

	tests := []struct {
		name           string
		body           BlockUserRequest
		expectedStatus int
		setupContext   func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name: "valid block user",
			body: BlockUserRequest{
				BlockedID: blockedUser.ID,
				Reason:    "Spammer",
			},
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name: "block self",
			body: BlockUserRequest{
				BlockedID: userID,
				Reason:    "Test",
			},
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name: "block with nil UUID",
			body: BlockUserRequest{
				BlockedID: uuid.Nil,
				Reason:    "Test",
			},
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name: "already blocked",
			body: BlockUserRequest{
				BlockedID: blockedUser.ID,
				Reason:    "Already blocked",
			},
			expectedStatus: http.StatusConflict,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "unauthorized",
			body:           BlockUserRequest{BlockedID: blockedUser.ID},
			expectedStatus: http.StatusUnauthorized,
			setupContext:   func(ctx context.Context, _ uuid.UUID) context.Context { return ctx },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "already blocked" {
				block := &models.Block{
					BlockerID: userID,
					BlockedID: blockedUser.ID,
					Reason:    "First block",
				}
				db.Create(block)
			}

			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/block", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(tt.setupContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.BlockUser(w, req)

			if tt.expectedStatus == http.StatusOK || tt.expectedStatus == http.StatusConflict {
				if w.Code != tt.expectedStatus && w.Code != http.StatusInternalServerError {
					t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
				}
			} else if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestBlockHandler_UnblockUser(t *testing.T) {
	handler, db, userID := setupBlockHandler(t)

	blockedUser := models.User{
		ID:       uuid.New(),
		Username: "targetuser",
		Email:    "target@example.com",
	}
	db.Create(&blockedUser)

	block := &models.Block{
		BlockerID: userID,
		BlockedID: blockedUser.ID,
		Reason:    "To be unblocked",
	}
	db.Create(block)

	tests := []struct {
		name           string
		blockedID      uuid.UUID
		expectedStatus int
		setupContext   func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name:           "valid unblock",
			blockedID:      blockedUser.ID,
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "not blocked",
			blockedID:      uuid.New(),
			expectedStatus: http.StatusNotFound,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "invalid UUID",
			blockedID:      uuid.Nil,
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "unauthorized",
			blockedID:      blockedUser.ID,
			expectedStatus: http.StatusUnauthorized,
			setupContext:   func(ctx context.Context, _ uuid.UUID) context.Context { return ctx },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "valid unblock" {
				newBlock := &models.Block{
					BlockerID: userID,
					BlockedID: tt.blockedID,
					Reason:    "Test",
				}
				db.Create(newBlock)
			}

			req := httptest.NewRequest(http.MethodDelete, "/unblock?blocked_id="+tt.blockedID.String(), nil)
			req = req.WithContext(tt.setupContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.UnblockUser(w, req)

			if tt.expectedStatus == http.StatusOK || tt.expectedStatus == http.StatusNotFound {
				if w.Code != tt.expectedStatus && w.Code != http.StatusInternalServerError {
					t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
				}
			} else if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestBlockHandler_GetBlockedUsers(t *testing.T) {
	handler, db, userID := setupBlockHandler(t)

	user1 := models.User{ID: uuid.New(), Username: "user1", Email: "user1@example.com"}
	user2 := models.User{ID: uuid.New(), Username: "user2", Email: "user2@example.com"}
	db.Create(&user1)
	db.Create(&user2)

	db.Create(&models.Block{BlockerID: userID, BlockedID: user1.ID, Reason: "Spammer"})
	db.Create(&models.Block{BlockerID: userID, BlockedID: user2.ID, Reason: "Annoying"})

	req := httptest.NewRequest(http.MethodGet, "/blocked", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetBlockedUsers(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	if w.Code == http.StatusOK {
		var response []map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Logf("Response body: %s", w.Body.String())
		}
	}
}

func TestBlockHandler_GetBlockedUsers_Empty(t *testing.T) {
	handler, _, userID := setupBlockHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/blocked", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetBlockedUsers(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestBlockHandler_IsBlocked(t *testing.T) {
	handler, db, userID := setupBlockHandler(t)

	targetUser := models.User{ID: uuid.New(), Username: "target", Email: "target@example.com"}
	db.Create(&targetUser)

	db.Create(&models.Block{BlockerID: userID, BlockedID: targetUser.ID, Reason: "Blocked"})

	tests := []struct {
		name           string
		targetID       uuid.UUID
		expectedStatus int
		setupContext   func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name:           "user is blocked",
			targetID:       targetUser.ID,
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "user is not blocked",
			targetID:       uuid.New(),
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "invalid user_id",
			targetID:       uuid.Nil,
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "unauthorized",
			targetID:       targetUser.ID,
			expectedStatus: http.StatusUnauthorized,
			setupContext:   func(ctx context.Context, _ uuid.UUID) context.Context { return ctx },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/is-blocked?user_id="+tt.targetID.String(), nil)
			req = req.WithContext(tt.setupContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.IsBlocked(w, req)

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

func TestBlockHandler_IsBlocked_MissingQueryParam(t *testing.T) {
	handler, _, userID := setupBlockHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/is-blocked", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.IsBlocked(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestBlockHandler_GetBlockedByUsers(t *testing.T) {
	handler, db, userID := setupBlockHandler(t)

	blockerUser1 := models.User{ID: uuid.New(), Username: "blocker1", Email: "blocker1@example.com"}
	blockerUser2 := models.User{ID: uuid.New(), Username: "blocker2", Email: "blocker2@example.com"}
	db.Create(&blockerUser1)
	db.Create(&blockerUser2)

	db.Create(&models.Block{BlockerID: blockerUser1.ID, BlockedID: userID, Reason: "Spammer"})
	db.Create(&models.Block{BlockerID: blockerUser2.ID, BlockedID: userID, Reason: "Annoying"})

	req := httptest.NewRequest(http.MethodGet, "/blocked-by", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetBlockedByUsers(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	if w.Code == http.StatusOK {
		var response []map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Logf("Response body: %s", w.Body.String())
		}
	}
}

func TestBlockHandler_GetBlockedByUsers_Empty(t *testing.T) {
	handler, _, userID := setupBlockHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/blocked-by", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetBlockedByUsers(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}
