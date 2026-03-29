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
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func setupVoiceChannelPermissionDB(t *testing.T) *gorm.DB {
	db := openTestDB(t)

	err := db.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.UserRoom{},
		&models.VoiceChannel{},
		&models.VoiceChannelParticipant{},
		&models.VoiceChannelPermission{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupVoiceChannelPermissionTest(t *testing.T) (*VoiceChannelPermissionHandler, *database.Database, uuid.UUID, uuid.UUID, uuid.UUID) {
	db := setupVoiceChannelPermissionDB(t)
	testDB := &database.Database{DB: db}
	handler := NewVoiceChannelPermissionHandler(testDB)

	user := models.User{
		ID:       uuid.New(),
		Username: "admin",
		Email:    "admin@example.com",
	}
	db.Create(&user)

	targetUser := models.User{
		ID:       uuid.New(),
		Username: "target",
		Email:    "target@example.com",
	}
	db.Create(&targetUser)

	room := models.Room{
		ID:       uuid.New(),
		Name:     "Test Room",
		MaxUsers: 50,
	}
	db.Create(&room)

	db.Create(&models.UserRoom{UserID: user.ID, RoomID: room.ID, Role: "admin"})
	db.Create(&models.UserRoom{UserID: targetUser.ID, RoomID: room.ID, Role: "member"})

	channel := models.VoiceChannel{
		ID:              uuid.New(),
		RoomID:          room.ID,
		Name:            "Test Channel",
		MaxParticipants: 10,
		CreatedBy:       user.ID,
	}
	db.Create(&channel)

	return handler, testDB, user.ID, targetUser.ID, channel.ID
}

func permissionContextWithUserID(ctx context.Context, userID uuid.UUID) context.Context {
	return context.WithValue(ctx, middleware.UserIDKey, userID)
}

func TestVoiceChannelPermission_SetPermission(t *testing.T) {
	handler, _, adminID, targetID, channelID := setupVoiceChannelPermissionTest(t)

	canSpeak := false
	body, _ := json.Marshal(SetPermissionRequest{
		ChannelID: channelID,
		UserID:    targetID,
		CanSpeak:  &canSpeak,
	})
	req := httptest.NewRequest(http.MethodPost, "/voice-channel/permission/set", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(permissionContextWithUserID(req.Context(), adminID))
	w := httptest.NewRecorder()

	handler.SetPermission(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp PermissionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.CanSpeak {
		t.Error("expected can_speak to be false")
	}
	if !resp.CanJoin {
		t.Error("expected can_join to be true by default")
	}
}

func TestVoiceChannelPermission_SetPermission_Update(t *testing.T) {
	handler, db, adminID, targetID, channelID := setupVoiceChannelPermissionTest(t)

	db.Create(&models.VoiceChannelPermission{
		ChannelID: channelID,
		UserID:    targetID,
		CanJoin:   true,
		CanSpeak:  true,
		GrantedBy: adminID,
	})

	canJoin := false
	body, _ := json.Marshal(SetPermissionRequest{
		ChannelID: channelID,
		UserID:    targetID,
		CanJoin:   &canJoin,
	})
	req := httptest.NewRequest(http.MethodPost, "/voice-channel/permission/set", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(permissionContextWithUserID(req.Context(), adminID))
	w := httptest.NewRecorder()

	handler.SetPermission(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp PermissionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.CanJoin {
		t.Error("expected can_join to be false after update")
	}
}

func TestVoiceChannelPermission_SetPermission_Forbidden(t *testing.T) {
	handler, _, _, targetID, channelID := setupVoiceChannelPermissionTest(t)

	regularUser := uuid.New()
	body, _ := json.Marshal(SetPermissionRequest{
		ChannelID: channelID,
		UserID:    targetID,
	})
	req := httptest.NewRequest(http.MethodPost, "/voice-channel/permission/set", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(permissionContextWithUserID(req.Context(), regularUser))
	w := httptest.NewRecorder()

	handler.SetPermission(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}

func TestVoiceChannelPermission_SetPermission_MissingChannelID(t *testing.T) {
	handler, _, adminID, targetID, _ := setupVoiceChannelPermissionTest(t)

	body, _ := json.Marshal(SetPermissionRequest{
		UserID: targetID,
	})
	req := httptest.NewRequest(http.MethodPost, "/voice-channel/permission/set", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(permissionContextWithUserID(req.Context(), adminID))
	w := httptest.NewRecorder()

	handler.SetPermission(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestVoiceChannelPermission_GetPermissions(t *testing.T) {
	handler, db, adminID, targetID, channelID := setupVoiceChannelPermissionTest(t)

	db.Create(&models.VoiceChannelPermission{
		ChannelID: channelID,
		UserID:    targetID,
		CanJoin:   true,
		CanSpeak:  true,
		GrantedBy: adminID,
	})

	req := httptest.NewRequest(http.MethodGet, "/voice-channel/permissions?channel_id="+channelID.String(), nil)
	req = req.WithContext(permissionContextWithUserID(req.Context(), adminID))
	w := httptest.NewRecorder()

	handler.GetPermissions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var permissions []PermissionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &permissions); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(permissions) != 1 {
		t.Errorf("expected 1 permission, got %d", len(permissions))
	}
}

func TestVoiceChannelPermission_GetUserPermission(t *testing.T) {
	handler, db, adminID, targetID, channelID := setupVoiceChannelPermissionTest(t)

	db.Create(&models.VoiceChannelPermission{
		ChannelID:     channelID,
		UserID:        targetID,
		CanJoin:       true,
		CanSpeak:      false,
		CanMuteOthers: true,
		GrantedBy:     adminID,
	})

	req := httptest.NewRequest(http.MethodGet,
		"/voice-channel/permission?channel_id="+channelID.String()+"&user_id="+targetID.String(), nil)
	req = req.WithContext(permissionContextWithUserID(req.Context(), adminID))
	w := httptest.NewRecorder()

	handler.GetUserPermission(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var perm PermissionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &perm); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if !perm.CanMuteOthers {
		t.Error("expected can_mute_others to be true")
	}
	if perm.CanSpeak {
		t.Error("expected can_speak to be false")
	}
}

func TestVoiceChannelPermission_GetUserPermission_Default(t *testing.T) {
	handler, _, adminID, targetID, channelID := setupVoiceChannelPermissionTest(t)

	req := httptest.NewRequest(http.MethodGet,
		"/voice-channel/permission?channel_id="+channelID.String()+"&user_id="+targetID.String(), nil)
	req = req.WithContext(permissionContextWithUserID(req.Context(), adminID))
	w := httptest.NewRecorder()

	handler.GetUserPermission(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var perm PermissionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &perm); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if !perm.CanJoin {
		t.Error("expected default can_join to be true")
	}
	if !perm.CanSpeak {
		t.Error("expected default can_speak to be true")
	}
}

func TestVoiceChannelPermission_DeletePermission(t *testing.T) {
	handler, db, adminID, targetID, channelID := setupVoiceChannelPermissionTest(t)

	db.Create(&models.VoiceChannelPermission{
		ChannelID: channelID,
		UserID:    targetID,
		GrantedBy: adminID,
	})

	req := httptest.NewRequest(http.MethodDelete,
		"/voice-channel/permission/delete?channel_id="+channelID.String()+"&user_id="+targetID.String(), nil)
	req = req.WithContext(permissionContextWithUserID(req.Context(), adminID))
	w := httptest.NewRecorder()

	handler.DeletePermission(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var count int64
	db.Model(&models.VoiceChannelPermission{}).Where("channel_id = ? AND user_id = ?", channelID, targetID).Count(&count)
	if count != 0 {
		t.Error("expected permission to be deleted")
	}
}

func TestVoiceChannelPermission_DeletePermission_NotFound(t *testing.T) {
	handler, _, adminID, _, channelID := setupVoiceChannelPermissionTest(t)

	nonExistentUserID := uuid.New()
	req := httptest.NewRequest(http.MethodDelete,
		"/voice-channel/permission/delete?channel_id="+channelID.String()+"&user_id="+nonExistentUserID.String(), nil)
	req = req.WithContext(permissionContextWithUserID(req.Context(), adminID))
	w := httptest.NewRecorder()

	handler.DeletePermission(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestVoiceChannelPermission_CheckPermission_Join(t *testing.T) {
	handler, _, adminID, targetID, channelID := setupVoiceChannelPermissionTest(t)

	tests := []struct {
		name    string
		userID  uuid.UUID
		action  string
		allowed bool
	}{
		{
			name:    "admin can join",
			userID:  adminID,
			action:  "join",
			allowed: true,
		},
		{
			name:    "target can join open channel",
			userID:  targetID,
			action:  "join",
			allowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet,
				"/voice-channel/permission/check?channel_id="+channelID.String()+"&action="+tt.action, nil)
			req = req.WithContext(permissionContextWithUserID(req.Context(), tt.userID))
			w := httptest.NewRecorder()

			handler.CheckPermission(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
			}
		})
	}
}

func TestVoiceChannelPermission_CheckPermission_DeniedJoin(t *testing.T) {
	handler, db, adminID, targetID, channelID := setupVoiceChannelPermissionTest(t)

	canJoin := false
	db.Create(&models.VoiceChannelPermission{
		ChannelID: channelID,
		UserID:    targetID,
		CanJoin:   canJoin,
		CanSpeak:  true,
		GrantedBy: adminID,
	})

	req := httptest.NewRequest(http.MethodGet,
		"/voice-channel/permission/check?channel_id="+channelID.String()+"&action=join", nil)
	req = req.WithContext(permissionContextWithUserID(req.Context(), targetID))
	w := httptest.NewRecorder()

	handler.CheckPermission(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if result["allowed"].(bool) {
		t.Error("expected join to be denied")
	}
}

func TestVoiceChannelPermission_BulkSetPermissions(t *testing.T) {
	handler, _, adminID, targetID, channelID := setupVoiceChannelPermissionTest(t)

	canSpeak := false
	body, _ := json.Marshal(map[string]interface{}{
		"channel_id": channelID.String(),
		"permissions": []SetPermissionRequest{
			{
				ChannelID: channelID,
				UserID:    targetID,
				CanSpeak:  &canSpeak,
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/voice-channel/permissions/bulk", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(permissionContextWithUserID(req.Context(), adminID))
	w := httptest.NewRecorder()

	handler.BulkSetPermissions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var responses []PermissionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &responses); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(responses) != 1 {
		t.Errorf("expected 1 permission, got %d", len(responses))
	}

	if responses[0].CanSpeak {
		t.Error("expected can_speak to be false")
	}
}

func TestVoiceChannelPermission_BulkSetPermissions_Forbidden(t *testing.T) {
	handler, _, _, targetID, channelID := setupVoiceChannelPermissionTest(t)

	nonAdmin := uuid.New()
	body, _ := json.Marshal(map[string]interface{}{
		"channel_id": channelID.String(),
		"permissions": []SetPermissionRequest{
			{
				ChannelID: channelID,
				UserID:    targetID,
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/voice-channel/permissions/bulk", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(permissionContextWithUserID(req.Context(), nonAdmin))
	w := httptest.NewRecorder()

	handler.BulkSetPermissions(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}

func TestVoiceChannelPermission_NewVoiceChannelPermissionHandler(t *testing.T) {
	db := openTestDB(t)
	testDB := &database.Database{DB: db}
	handler := NewVoiceChannelPermissionHandler(testDB)

	if handler == nil {
		t.Fatal("expected handler to be created")
	}
	if handler.db == nil {
		t.Error("expected db to be set")
	}
}
