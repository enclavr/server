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

func setupTestDBForConnectionHandler(t *testing.T) *gorm.DB {
	db := openTestDB(t)

	err := db.AutoMigrate(
		&models.User{},
		&models.UserConnection{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupConnectionHandler(t *testing.T) (*ConnectionHandler, *database.Database, uuid.UUID) {
	db := setupTestDBForConnectionHandler(t)
	testDB := &database.Database{DB: db}
	handler := NewConnectionHandler(testDB)

	user := models.User{
		ID:       uuid.New(),
		Username: "testuser",
		Email:    "test@example.com",
	}
	db.Create(&user)

	return handler, testDB, user.ID
}

func TestConnectionHandler_SendRequest(t *testing.T) {
	handler, db, userID := setupConnectionHandler(t)

	targetUser := models.User{
		ID:       uuid.New(),
		Username: "targetuser",
		Email:    "target@example.com",
	}
	db.Create(&targetUser)

	tests := []struct {
		name           string
		body           ConnectionRequest
		expectedStatus int
		setupContext   func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name: "valid send request",
			body: ConnectionRequest{
				ConnectedUserID: targetUser.ID,
			},
			expectedStatus: http.StatusCreated,
			setupContext:   addUserIDToContext,
		},
		{
			name: "send request to self",
			body: ConnectionRequest{
				ConnectedUserID: userID,
			},
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name: "send request with nil UUID",
			body: ConnectionRequest{
				ConnectedUserID: uuid.Nil,
			},
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name: "send request to non-existent user",
			body: ConnectionRequest{
				ConnectedUserID: uuid.New(),
			},
			expectedStatus: http.StatusNotFound,
			setupContext:   addUserIDToContext,
		},
		{
			name: "unauthorized",
			body: ConnectionRequest{
				ConnectedUserID: targetUser.ID,
			},
			expectedStatus: http.StatusUnauthorized,
			setupContext:   func(ctx context.Context, _ uuid.UUID) context.Context { return ctx },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/connections/request", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(tt.setupContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.SendRequest(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestConnectionHandler_SendRequest_AlreadyPending(t *testing.T) {
	handler, db, userID := setupConnectionHandler(t)

	targetUser := models.User{
		ID:       uuid.New(),
		Username: "targetuser2",
		Email:    "target2@example.com",
	}
	db.Create(&targetUser)

	existing := &models.UserConnection{
		UserID:          userID,
		ConnectedUserID: targetUser.ID,
		Status:          models.ConnectionStatusPending,
		Direction:       models.ConnectionDirectionOneway,
	}
	db.Create(existing)

	body, _ := json.Marshal(ConnectionRequest{ConnectedUserID: targetUser.ID})
	req := httptest.NewRequest(http.MethodPost, "/connections/request", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.SendRequest(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected status %d, got %d", http.StatusConflict, w.Code)
	}
}

func TestConnectionHandler_AcceptRequest(t *testing.T) {
	handler, db, userID := setupConnectionHandler(t)

	requester := models.User{
		ID:       uuid.New(),
		Username: "requester",
		Email:    "requester@example.com",
	}
	db.Create(&requester)

	pending := &models.UserConnection{
		UserID:          requester.ID,
		ConnectedUserID: userID,
		Status:          models.ConnectionStatusPending,
		Direction:       models.ConnectionDirectionOneway,
	}
	db.Create(pending)

	tests := []struct {
		name           string
		body           ConnectionRequest
		expectedStatus int
		setupContext   func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name: "valid accept request",
			body: ConnectionRequest{
				ConnectedUserID: requester.ID,
			},
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name: "accept non-existent request",
			body: ConnectionRequest{
				ConnectedUserID: uuid.New(),
			},
			expectedStatus: http.StatusNotFound,
			setupContext:   addUserIDToContext,
		},
		{
			name: "unauthorized",
			body: ConnectionRequest{
				ConnectedUserID: requester.ID,
			},
			expectedStatus: http.StatusUnauthorized,
			setupContext:   func(ctx context.Context, _ uuid.UUID) context.Context { return ctx },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "valid accept request" {
				newPending := &models.UserConnection{
					UserID:          requester.ID,
					ConnectedUserID: userID,
					Status:          models.ConnectionStatusPending,
					Direction:       models.ConnectionDirectionOneway,
				}
				db.Create(newPending)
			}

			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/connections/accept", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(tt.setupContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.AcceptRequest(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestConnectionHandler_RejectRequest(t *testing.T) {
	handler, db, userID := setupConnectionHandler(t)

	requester := models.User{
		ID:       uuid.New(),
		Username: "requester",
		Email:    "requester@example.com",
	}
	db.Create(&requester)

	pending := &models.UserConnection{
		UserID:          requester.ID,
		ConnectedUserID: userID,
		Status:          models.ConnectionStatusPending,
		Direction:       models.ConnectionDirectionOneway,
	}
	db.Create(pending)

	tests := []struct {
		name           string
		body           ConnectionRequest
		expectedStatus int
		setupContext   func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name: "valid reject request",
			body: ConnectionRequest{
				ConnectedUserID: requester.ID,
			},
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name: "reject non-existent request",
			body: ConnectionRequest{
				ConnectedUserID: uuid.New(),
			},
			expectedStatus: http.StatusNotFound,
			setupContext:   addUserIDToContext,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "valid reject request" {
				newPending := &models.UserConnection{
					UserID:          requester.ID,
					ConnectedUserID: userID,
					Status:          models.ConnectionStatusPending,
					Direction:       models.ConnectionDirectionOneway,
				}
				db.Create(newPending)
			}

			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/connections/reject", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(tt.setupContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.RejectRequest(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestConnectionHandler_RemoveConnection(t *testing.T) {
	handler, db, userID := setupConnectionHandler(t)

	connectedUser := models.User{
		ID:       uuid.New(),
		Username: "connected",
		Email:    "connected@example.com",
	}
	db.Create(&connectedUser)

	conn := &models.UserConnection{
		UserID:          userID,
		ConnectedUserID: connectedUser.ID,
		Status:          models.ConnectionStatusAccepted,
		Direction:       models.ConnectionDirectionMutual,
	}
	db.Create(conn)

	tests := []struct {
		name           string
		body           ConnectionRequest
		expectedStatus int
		setupContext   func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name: "valid remove connection",
			body: ConnectionRequest{
				ConnectedUserID: connectedUser.ID,
			},
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name: "remove non-existent connection",
			body: ConnectionRequest{
				ConnectedUserID: uuid.New(),
			},
			expectedStatus: http.StatusNotFound,
			setupContext:   addUserIDToContext,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "valid remove connection" {
				newConn := &models.UserConnection{
					UserID:          userID,
					ConnectedUserID: connectedUser.ID,
					Status:          models.ConnectionStatusAccepted,
					Direction:       models.ConnectionDirectionMutual,
				}
				db.Create(newConn)
			}

			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/connections/remove", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(tt.setupContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.RemoveConnection(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestConnectionHandler_GetConnections(t *testing.T) {
	handler, db, userID := setupConnectionHandler(t)

	user1 := models.User{ID: uuid.New(), Username: "conn1", Email: "conn1@example.com"}
	user2 := models.User{ID: uuid.New(), Username: "conn2", Email: "conn2@example.com"}
	db.Create(&user1)
	db.Create(&user2)

	db.Create(&models.UserConnection{
		UserID:          userID,
		ConnectedUserID: user1.ID,
		Status:          models.ConnectionStatusAccepted,
		Direction:       models.ConnectionDirectionMutual,
	})
	db.Create(&models.UserConnection{
		UserID:          userID,
		ConnectedUserID: user2.ID,
		Status:          models.ConnectionStatusAccepted,
		Direction:       models.ConnectionDirectionMutual,
	})

	tests := []struct {
		name           string
		queryParams    string
		expectedStatus int
		setupContext   func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name:           "get all accepted connections",
			queryParams:    "",
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "filter by status",
			queryParams:    "?status=pending",
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "unauthorized",
			queryParams:    "",
			expectedStatus: http.StatusUnauthorized,
			setupContext:   func(ctx context.Context, _ uuid.UUID) context.Context { return ctx },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/connections"+tt.queryParams, nil)
			req = req.WithContext(tt.setupContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.GetConnections(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedStatus == http.StatusOK {
				var response ConnectionListResponse
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Logf("Response body: %s", w.Body.String())
				}
			}
		})
	}
}

func TestConnectionHandler_GetPendingRequests(t *testing.T) {
	handler, db, userID := setupConnectionHandler(t)

	requester := models.User{ID: uuid.New(), Username: "requester", Email: "req@example.com"}
	db.Create(&requester)

	db.Create(&models.UserConnection{
		UserID:          requester.ID,
		ConnectedUserID: userID,
		Status:          models.ConnectionStatusPending,
		Direction:       models.ConnectionDirectionOneway,
	})

	req := httptest.NewRequest(http.MethodGet, "/connections/pending", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetPendingRequests(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response ConnectionListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Logf("Response body: %s", w.Body.String())
	}
	if response.Total != 1 {
		t.Errorf("expected 1 pending request, got %d", response.Total)
	}
}

func TestConnectionHandler_GetSentRequests(t *testing.T) {
	handler, db, userID := setupConnectionHandler(t)

	target := models.User{ID: uuid.New(), Username: "target", Email: "target@example.com"}
	db.Create(&target)

	db.Create(&models.UserConnection{
		UserID:          userID,
		ConnectedUserID: target.ID,
		Status:          models.ConnectionStatusPending,
		Direction:       models.ConnectionDirectionOneway,
	})

	req := httptest.NewRequest(http.MethodGet, "/connections/sent", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetSentRequests(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response ConnectionListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Logf("Response body: %s", w.Body.String())
	}
	if response.Total != 1 {
		t.Errorf("expected 1 sent request, got %d", response.Total)
	}
}

func TestConnectionHandler_GetStatus(t *testing.T) {
	handler, db, userID := setupConnectionHandler(t)

	connectedUser := models.User{ID: uuid.New(), Username: "connected", Email: "c@example.com"}
	db.Create(&connectedUser)

	db.Create(&models.UserConnection{
		UserID:          userID,
		ConnectedUserID: connectedUser.ID,
		Status:          models.ConnectionStatusAccepted,
		Direction:       models.ConnectionDirectionMutual,
	})

	tests := []struct {
		name           string
		targetID       uuid.UUID
		expectedStatus int
		setupContext   func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name:           "existing connection",
			targetID:       connectedUser.ID,
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "no connection",
			targetID:       uuid.New(),
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "missing user_id",
			targetID:       uuid.Nil,
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "unauthorized",
			targetID:       connectedUser.ID,
			expectedStatus: http.StatusUnauthorized,
			setupContext:   func(ctx context.Context, _ uuid.UUID) context.Context { return ctx },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/connections/status"
			if tt.targetID != uuid.Nil {
				url += "?user_id=" + tt.targetID.String()
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			req = req.WithContext(tt.setupContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.GetStatus(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestConnectionHandler_BlockConnection(t *testing.T) {
	handler, db, userID := setupConnectionHandler(t)

	targetUser := models.User{ID: uuid.New(), Username: "target", Email: "target@example.com"}
	db.Create(&targetUser)

	tests := []struct {
		name           string
		body           ConnectionRequest
		expectedStatus int
		setupContext   func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name: "valid block",
			body: ConnectionRequest{
				ConnectedUserID: targetUser.ID,
			},
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name: "block self",
			body: ConnectionRequest{
				ConnectedUserID: userID,
			},
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name: "block with nil UUID",
			body: ConnectionRequest{
				ConnectedUserID: uuid.Nil,
			},
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name: "unauthorized",
			body: ConnectionRequest{
				ConnectedUserID: targetUser.ID,
			},
			expectedStatus: http.StatusUnauthorized,
			setupContext:   func(ctx context.Context, _ uuid.UUID) context.Context { return ctx },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/connections/block", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(tt.setupContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.BlockConnection(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestConnectionHandler_GetConnections_Empty(t *testing.T) {
	handler, _, userID := setupConnectionHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/connections", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetConnections(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response ConnectionListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Logf("Response body: %s", w.Body.String())
	}
	if response.Total != 0 {
		t.Errorf("expected 0 connections, got %d", response.Total)
	}
}
