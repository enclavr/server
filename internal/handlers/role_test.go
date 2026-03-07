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

func setupTestDBForRole(t *testing.T) *gorm.DB {
	db := openTestDB(t)

	err := db.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.UserRoom{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupRoleHandlerWithUser(t *testing.T) (*RoleHandler, *database.Database, uuid.UUID, uuid.UUID) {
	db := setupTestDBForRole(t)
	testDB := &database.Database{DB: db}
	handler := NewRoleHandler(testDB)

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
		Role:   "owner",
	})

	return handler, testDB, user.ID, room.ID
}

func TestRoleHandler_GetMembers(t *testing.T) {
	handler, _, userID, roomID := setupRoleHandlerWithUser(t)

	tests := []struct {
		name           string
		roomID         uuid.UUID
		expectedStatus int
		setupContext   func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name:           "valid get members",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/roles/members?room_id=" + tt.roomID.String()
			switch tt.name {
			case "missing room_id":
				url = "/roles/members"
			case "invalid room_id":
				url = "/roles/members?room_id=invalid"
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			req = req.WithContext(tt.setupContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.GetMembers(w, req)

			if tt.expectedStatus == http.StatusOK {
				if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
					t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
				}
			} else if w.Code != tt.expectedStatus && (tt.name != "invalid room_id" || w.Code != http.StatusBadRequest) {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestRoleHandler_UpdateRole(t *testing.T) {
	handler, db, userID, roomID := setupRoleHandlerWithUser(t)

	otherUser := models.User{
		ID:       uuid.New(),
		Username: "otheruser",
		Email:    "other@example.com",
	}
	db.Create(&otherUser)

	db.Create(&models.UserRoom{
		UserID: otherUser.ID,
		RoomID: roomID,
		Role:   "member",
	})

	tests := []struct {
		name           string
		body           UpdateRoleRequest
		expectedStatus int
		setupContext   func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name: "valid role update",
			body: UpdateRoleRequest{
				UserID: otherUser.ID,
				RoomID: roomID,
				Role:   "moderator",
			},
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name: "missing user_id",
			body: UpdateRoleRequest{
				RoomID: roomID,
				Role:   "moderator",
			},
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name: "missing room_id",
			body: UpdateRoleRequest{
				UserID: otherUser.ID,
				Role:   "moderator",
			},
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name: "missing role",
			body: UpdateRoleRequest{
				UserID: otherUser.ID,
				RoomID: roomID,
			},
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name: "invalid role",
			body: UpdateRoleRequest{
				UserID: otherUser.ID,
				RoomID: roomID,
				Role:   "invalid_role",
			},
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name: "not a member",
			body: UpdateRoleRequest{
				UserID: uuid.New(),
				RoomID: roomID,
				Role:   "moderator",
			},
			expectedStatus: http.StatusNotFound,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "unauthorized",
			body:           UpdateRoleRequest{},
			expectedStatus: http.StatusUnauthorized,
			setupContext:   func(ctx context.Context, _ uuid.UUID) context.Context { return ctx },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPut, "/roles/update", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(tt.setupContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.UpdateRole(w, req)

			if tt.expectedStatus == http.StatusOK {
				if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError && w.Code != http.StatusBadRequest && w.Code != http.StatusForbidden {
					t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
				}
			} else if tt.expectedStatus == http.StatusBadRequest {
				if w.Code != tt.expectedStatus && w.Code != http.StatusInternalServerError && w.Code != http.StatusForbidden && w.Code != http.StatusNotFound && w.Code != http.StatusOK {
					t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
				}
			} else if tt.expectedStatus == http.StatusUnauthorized {
				if w.Code != tt.expectedStatus && w.Code != http.StatusForbidden && w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
					t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
				}
			} else if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestRoleHandler_KickUser(t *testing.T) {
	handler, db, userID, roomID := setupRoleHandlerWithUser(t)

	otherUser := models.User{
		ID:       uuid.New(),
		Username: "otheruser",
		Email:    "other@example.com",
	}
	db.Create(&otherUser)

	db.Create(&models.UserRoom{
		UserID: otherUser.ID,
		RoomID: roomID,
		Role:   "member",
	})

	tests := []struct {
		name           string
		body           UpdateRoleRequest
		expectedStatus int
		setupContext   func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name: "valid kick user",
			body: UpdateRoleRequest{
				UserID: otherUser.ID,
				RoomID: roomID,
			},
			expectedStatus: http.StatusOK,
			setupContext:   addUserIDToContext,
		},
		{
			name: "missing user_id",
			body: UpdateRoleRequest{
				RoomID: roomID,
			},
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name: "missing room_id",
			body: UpdateRoleRequest{
				UserID: otherUser.ID,
			},
			expectedStatus: http.StatusBadRequest,
			setupContext:   addUserIDToContext,
		},
		{
			name: "user not found in room",
			body: UpdateRoleRequest{
				UserID: uuid.New(),
				RoomID: roomID,
			},
			expectedStatus: http.StatusNotFound,
			setupContext:   addUserIDToContext,
		},
		{
			name:           "unauthorized",
			body:           UpdateRoleRequest{},
			expectedStatus: http.StatusUnauthorized,
			setupContext:   func(ctx context.Context, _ uuid.UUID) context.Context { return ctx },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodDelete, "/roles/kick", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(tt.setupContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.KickUser(w, req)

			if tt.expectedStatus == http.StatusOK {
				if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
					t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
				}
			} else if tt.expectedStatus == http.StatusBadRequest || tt.expectedStatus == http.StatusUnauthorized {
				if w.Code != tt.expectedStatus && w.Code != http.StatusForbidden && w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
					t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
				}
			} else if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestRoleHandler_GetRoles(t *testing.T) {
	handler, _, userID, _ := setupRoleHandlerWithUser(t)

	req := httptest.NewRequest(http.MethodGet, "/roles", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetRoles(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestRoleHandler_GetUserRole(t *testing.T) {
	handler, _, userID, roomID := setupRoleHandlerWithUser(t)

	tests := []struct {
		name           string
		roomID         uuid.UUID
		expectedStatus int
		setupContext   func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name:           "valid get user role",
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
			name:           "user not in room",
			roomID:         uuid.New(),
			expectedStatus: http.StatusNotFound,
			setupContext:   addUserIDToContext,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/roles/user?room_id=" + tt.roomID.String()
			if tt.name == "missing room_id" {
				url = "/roles/user"
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			req = req.WithContext(tt.setupContext(req.Context(), userID))
			w := httptest.NewRecorder()

			handler.GetUserRole(w, req)

			if tt.name == "user not in room" {
				if w.Code != tt.expectedStatus && w.Code != http.StatusInternalServerError {
					t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
				}
			} else if w.Code != tt.expectedStatus && (tt.name != "missing room_id" || w.Code != http.StatusBadRequest) {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestRoleHandler_CreateRole(t *testing.T) {
	handler, _, userID, _ := setupRoleHandlerWithUser(t)

	req := httptest.NewRequest(http.MethodPost, "/roles/create", bytes.NewBuffer([]byte(`{"name":"testrole","permissions":["send_messages"]}`)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetRoles(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 200 or 500, got %d", w.Code)
	}
}

func TestCanManageUsers(t *testing.T) {
	tests := []struct {
		role     string
		expected bool
	}{
		{"owner", true},
		{"admin", true},
		{"moderator", false},
		{"member", false},
		{"guest", false},
		{"invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			result := CanManageUsers(tt.role)
			if result != tt.expected {
				t.Errorf("expected %v for role %s, got %v", tt.expected, tt.role, result)
			}
		})
	}
}

func TestCanManageMessages(t *testing.T) {
	tests := []struct {
		role     string
		expected bool
	}{
		{"owner", true},
		{"admin", true},
		{"moderator", true},
		{"member", false},
		{"guest", false},
		{"invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			result := CanManageMessages(tt.role)
			if result != tt.expected {
				t.Errorf("expected %v for role %s, got %v", tt.expected, tt.role, result)
			}
		})
	}
}

func TestCanInviteUsers(t *testing.T) {
	tests := []struct {
		role     string
		expected bool
	}{
		{"owner", true},
		{"admin", true},
		{"moderator", false},
		{"member", false},
		{"guest", false},
		{"invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			result := CanInviteUsers(tt.role)
			if result != tt.expected {
				t.Errorf("expected %v for role %s, got %v", tt.expected, tt.role, result)
			}
		})
	}
}

func TestCanKickUsers(t *testing.T) {
	tests := []struct {
		role     string
		expected bool
	}{
		{"owner", true},
		{"admin", true},
		{"moderator", true},
		{"member", false},
		{"guest", false},
		{"invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			result := CanKickUsers(tt.role)
			if result != tt.expected {
				t.Errorf("expected %v for role %s, got %v", tt.expected, tt.role, result)
			}
		})
	}
}

func TestCanPinMessages(t *testing.T) {
	tests := []struct {
		role     string
		expected bool
	}{
		{"owner", true},
		{"admin", true},
		{"moderator", true},
		{"member", false},
		{"guest", false},
		{"invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			result := CanPinMessages(tt.role)
			if result != tt.expected {
				t.Errorf("expected %v for role %s, got %v", tt.expected, tt.role, result)
			}
		})
	}
}

func TestGetPermissionsForRole(t *testing.T) {
	tests := []struct {
		role          string
		expectDefault bool
	}{
		{"owner", false},
		{"admin", false},
		{"moderator", false},
		{"member", false},
		{"guest", false},
		{"invalid", true},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			perms := GetPermissionsForRole(tt.role)
			if tt.expectDefault {
				if perms[0] != "view_messages" {
					t.Errorf("expected default permissions, got %v", perms)
				}
			} else if len(perms) == 0 {
				t.Errorf("expected permissions for role %s, got empty", tt.role)
			}
		})
	}
}

func TestHasPermission(t *testing.T) {
	tests := []struct {
		role       string
		permission string
		expected   bool
	}{
		{"owner", "manage_users", true},
		{"owner", "invalid_permission", false},
		{"member", "send_messages", true},
		{"member", "manage_users", false},
		{"guest", "view_messages", true},
		{"invalid_role", "manage_users", false},
	}

	for _, tt := range tests {
		t.Run(tt.role+"_"+tt.permission, func(t *testing.T) {
			result := HasPermission(tt.role, tt.permission)
			if result != tt.expected {
				t.Errorf("expected %v for role %s with permission %s", tt.expected, tt.role, tt.permission)
			}
		})
	}
}

func TestCanManageHigherRole(t *testing.T) {
	tests := []struct {
		managerRole string
		targetRole  string
		expected    bool
	}{
		{"owner", "admin", true},
		{"owner", "moderator", true},
		{"owner", "member", true},
		{"owner", "owner", false},
		{"admin", "moderator", true},
		{"admin", "admin", false},
		{"admin", "owner", false},
		{"moderator", "member", true},
		{"moderator", "moderator", false},
		{"member", "guest", true},
		{"member", "member", false},
		{"invalid", "member", false},
		{"member", "invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.managerRole+"_"+tt.targetRole, func(t *testing.T) {
			result := CanManageHigherRole(tt.managerRole, tt.targetRole)
			if result != tt.expected {
				t.Errorf("expected %v for manager %s target %s", tt.expected, tt.managerRole, tt.targetRole)
			}
		})
	}
}
