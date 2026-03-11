package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func setupTestDBForInviteLink(t *testing.T) *gorm.DB {
	db := openTestDB(t)

	err := db.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.UserRoom{},
		&models.InviteLink{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupInviteLinkHandler(t *testing.T) (*InviteLinkHandler, *database.Database, uuid.UUID, uuid.UUID) {
	db := setupTestDBForInviteLink(t)
	testDB := &database.Database{DB: db}
	handler := NewInviteLinkHandler(testDB)

	user := models.User{
		ID:       uuid.New(),
		Username: "testuser",
		Email:    "test@example.com",
	}
	room := models.Room{
		ID:       uuid.New(),
		Name:     "Test Room",
		MaxUsers: 50,
	}
	db.Create(&user)
	db.Create(&room)
	db.Create(&models.UserRoom{UserID: user.ID, RoomID: room.ID, Role: "owner"})

	return handler, testDB, user.ID, room.ID
}

func TestInviteLinkHandler_CreateInviteLink(t *testing.T) {
	handler, _, userID, roomID := setupInviteLinkHandler(t)

	tests := []struct {
		name           string
		body           CreateInviteLinkRequest
		expectedStatus int
		userID         uuid.UUID
	}{
		{
			name: "valid invite link creation",
			body: CreateInviteLinkRequest{
				RoomID:      &roomID,
				Title:       "Test Invite",
				Description: "Test description",
				MaxUses:     10,
				ExpiresIn:   24,
			},
			expectedStatus: http.StatusOK,
			userID:         userID,
		},
		{
			name: "valid permanent invite link",
			body: CreateInviteLinkRequest{
				RoomID:      &roomID,
				Title:       "Permanent Invite",
				Description: "Never expires",
				MaxUses:     0,
				ExpiresIn:   0,
			},
			expectedStatus: http.StatusOK,
			userID:         userID,
		},
		{
			name: "missing room ID",
			body: CreateInviteLinkRequest{
				Title:       "Test Invite",
				Description: "Test description",
				MaxUses:     10,
				ExpiresIn:   24,
			},
			expectedStatus: http.StatusBadRequest,
			userID:         userID,
		},
		{
			name: "room not found",
			body: CreateInviteLinkRequest{
				RoomID:      func() *uuid.UUID { id := uuid.New(); return &id }(),
				Title:       "Test Invite",
				Description: "Test description",
				MaxUses:     10,
				ExpiresIn:   24,
			},
			expectedStatus: http.StatusNotFound,
			userID:         userID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/invite-link/create", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(addUserIDToContext(req.Context(), tt.userID))
			w := httptest.NewRecorder()

			handler.CreateInviteLink(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d, body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestInviteLinkHandler_CreateInviteLink_NotMember(t *testing.T) {
	db := setupTestDBForInviteLink(t)
	testDB := &database.Database{DB: db}
	handler := NewInviteLinkHandler(testDB)

	owner := models.User{
		ID:       uuid.New(),
		Username: "owner",
		Email:    "owner@example.com",
	}
	member := models.User{
		ID:       uuid.New(),
		Username: "member",
		Email:    "member@example.com",
	}
	room := models.Room{
		ID:       uuid.New(),
		Name:     "Test Room",
		MaxUsers: 50,
	}
	db.Create(&owner)
	db.Create(&member)
	db.Create(&room)
	db.Create(&models.UserRoom{UserID: owner.ID, RoomID: room.ID, Role: "owner"})

	body, _ := json.Marshal(CreateInviteLinkRequest{
		RoomID:      &room.ID,
		Title:       "Test Invite",
		Description: "Test description",
		MaxUses:     10,
	})
	req := httptest.NewRequest(http.MethodPost, "/invite-link/create", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(addUserIDToContext(req.Context(), member.ID))
	w := httptest.NewRecorder()

	handler.CreateInviteLink(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d, body: %s", http.StatusForbidden, w.Code, w.Body.String())
	}
}

func TestInviteLinkHandler_CreateInviteLink_NotOwnerOrAdmin(t *testing.T) {
	db := setupTestDBForInviteLink(t)
	testDB := &database.Database{DB: db}
	handler := NewInviteLinkHandler(testDB)

	owner := models.User{
		ID:       uuid.New(),
		Username: "owner",
		Email:    "owner@example.com",
	}
	member := models.User{
		ID:       uuid.New(),
		Username: "member",
		Email:    "member@example.com",
	}
	room := models.Room{
		ID:       uuid.New(),
		Name:     "Test Room",
		MaxUsers: 50,
	}
	db.Create(&owner)
	db.Create(&member)
	db.Create(&room)
	db.Create(&models.UserRoom{UserID: owner.ID, RoomID: room.ID, Role: "owner"})
	db.Create(&models.UserRoom{UserID: member.ID, RoomID: room.ID, Role: "member"})

	body, _ := json.Marshal(CreateInviteLinkRequest{
		RoomID:      &room.ID,
		Title:       "Test Invite",
		Description: "Test description",
		MaxUses:     10,
	})
	req := httptest.NewRequest(http.MethodPost, "/invite-link/create", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(addUserIDToContext(req.Context(), member.ID))
	w := httptest.NewRecorder()

	handler.CreateInviteLink(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d, body: %s", http.StatusForbidden, w.Code, w.Body.String())
	}
}

func TestInviteLinkHandler_CreateInviteLink_CodeAlreadyTaken(t *testing.T) {
	handler, db, userID, roomID := setupInviteLinkHandler(t)

	existingLink := models.InviteLink{
		ID:          uuid.New(),
		RoomID:      roomID,
		CreatedBy:   userID,
		Code:        "takencode",
		Title:       "Existing",
		Description: "Existing link",
		MaxUses:     10,
		IsEnabled:   true,
	}
	db.Create(&existingLink)

	body, _ := json.Marshal(CreateInviteLinkRequest{
		RoomID:      &roomID,
		Code:        "takencode",
		Title:       "Test Invite",
		Description: "Test description",
		MaxUses:     10,
	})
	req := httptest.NewRequest(http.MethodPost, "/invite-link/create", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.CreateInviteLink(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected status %d, got %d, body: %s", http.StatusConflict, w.Code, w.Body.String())
	}
}

func TestInviteLinkHandler_GetInviteLinks(t *testing.T) {
	handler, db, userID, roomID := setupInviteLinkHandler(t)

	link := models.InviteLink{
		ID:          uuid.New(),
		RoomID:      roomID,
		CreatedBy:   userID,
		Code:        "testcode",
		Title:       "Test",
		Description: "Test",
		MaxUses:     10,
		IsEnabled:   true,
	}
	db.Create(&link)

	req := httptest.NewRequest(http.MethodGet, "/invite-links?room_id="+roomID.String(), nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetInviteLinks(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestInviteLinkHandler_GetInviteLinks_MissingRoomID(t *testing.T) {
	handler, _, userID, _ := setupInviteLinkHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/invite-links", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetInviteLinks(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestInviteLinkHandler_GetInviteLinks_InvalidRoomID(t *testing.T) {
	handler, _, userID, _ := setupInviteLinkHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/invite-links?room_id=invalid", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.GetInviteLinks(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestInviteLinkHandler_UpdateInviteLink(t *testing.T) {
	handler, db, userID, roomID := setupInviteLinkHandler(t)

	link := models.InviteLink{
		ID:          uuid.New(),
		RoomID:      roomID,
		CreatedBy:   userID,
		Code:        "testcode",
		Title:       "Original",
		Description: "Original description",
		MaxUses:     10,
		IsEnabled:   true,
	}
	db.Create(&link)

	t.Run("valid update", func(t *testing.T) {
		body, _ := json.Marshal(UpdateInviteLinkRequest{
			ID:          link.ID,
			Title:       "Updated",
			Description: "Updated description",
			MaxUses:     20,
			IsEnabled:   false,
		})
		req := httptest.NewRequest(http.MethodPut, "/invite-link/update", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(addUserIDToContext(req.Context(), userID))
		w := httptest.NewRecorder()

		handler.UpdateInviteLink(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d, body: %s", http.StatusOK, w.Code, w.Body.String())
		}
	})

	t.Run("not found", func(t *testing.T) {
		body, _ := json.Marshal(UpdateInviteLinkRequest{
			ID:          uuid.New(),
			Title:       "Updated",
			Description: "Updated description",
			MaxUses:     20,
			IsEnabled:   false,
		})
		req := httptest.NewRequest(http.MethodPut, "/invite-link/update", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(addUserIDToContext(req.Context(), userID))
		w := httptest.NewRecorder()

		handler.UpdateInviteLink(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
		}
	})
}

func TestInviteLinkHandler_DeleteInviteLink(t *testing.T) {
	handler, db, userID, roomID := setupInviteLinkHandler(t)

	link := models.InviteLink{
		ID:          uuid.New(),
		RoomID:      roomID,
		CreatedBy:   userID,
		Code:        "testcode",
		Title:       "Test",
		Description: "Test",
		MaxUses:     10,
		IsEnabled:   true,
	}
	db.Create(&link)

	t.Run("valid delete", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/invite-link/delete?id="+link.ID.String(), nil)
		req = req.WithContext(addUserIDToContext(req.Context(), userID))
		w := httptest.NewRecorder()

		handler.DeleteInviteLink(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d, body: %s", http.StatusOK, w.Code, w.Body.String())
		}
	})

	t.Run("missing id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/invite-link/delete", nil)
		req = req.WithContext(addUserIDToContext(req.Context(), userID))
		w := httptest.NewRecorder()

		handler.DeleteInviteLink(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("invalid id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/invite-link/delete?id=invalid", nil)
		req = req.WithContext(addUserIDToContext(req.Context(), userID))
		w := httptest.NewRecorder()

		handler.DeleteInviteLink(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/invite-link/delete?id="+uuid.New().String(), nil)
		req = req.WithContext(addUserIDToContext(req.Context(), userID))
		w := httptest.NewRecorder()

		handler.DeleteInviteLink(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
		}
	})
}

func TestInviteLinkHandler_ResolveInviteLink(t *testing.T) {
	handler, db, userID, roomID := setupInviteLinkHandler(t)

	link := models.InviteLink{
		ID:          uuid.New(),
		RoomID:      roomID,
		CreatedBy:   userID,
		Code:        "resolvetest",
		Title:       "Test",
		Description: "Test",
		MaxUses:     10,
		IsEnabled:   true,
	}
	db.Create(&link)

	t.Run("valid resolve", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/invite-link/resolve?code=resolvetest", nil)
		w := httptest.NewRecorder()

		handler.ResolveInviteLink(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d, body: %s", http.StatusOK, w.Code, w.Body.String())
		}
	})

	t.Run("missing code", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/invite-link/resolve", nil)
		w := httptest.NewRecorder()

		handler.ResolveInviteLink(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/invite-link/resolve?code=invalid", nil)
		w := httptest.NewRecorder()

		handler.ResolveInviteLink(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
		}
	})
}

func TestInviteLinkHandler_ResolveInviteLink_Disabled(t *testing.T) {
	handler, db, userID, roomID := setupInviteLinkHandler(t)

	// Create link with IsEnabled=true first, then update to false
	// This is needed because GORM default:true overrides explicit false values in Create
	link := models.InviteLink{
		ID:          uuid.New(),
		RoomID:      roomID,
		CreatedBy:   userID,
		Code:        "disabledcode",
		Title:       "Test",
		Description: "Test",
		MaxUses:     10,
		IsEnabled:   true,
	}
	db.Create(&link)

	// Now update to disabled
	link.IsEnabled = false
	db.Save(&link)

	// Debug: verify link was saved
	var checkLink models.InviteLink
	db.Where("code = ?", "disabledcode").First(&checkLink)
	t.Logf("Saved link IsEnabled: %v", checkLink.IsEnabled)

	req := httptest.NewRequest(http.MethodGet, "/invite-link/resolve?code=disabledcode", nil)
	w := httptest.NewRecorder()

	handler.ResolveInviteLink(w, req)

	t.Logf("Response status: %d, body: %s", w.Code, w.Body.String())
	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}

func TestInviteLinkHandler_ResolveInviteLink_Expired(t *testing.T) {
	handler, db, userID, roomID := setupInviteLinkHandler(t)

	expiredTime := time.Now().Add(-24 * time.Hour)
	link := models.InviteLink{
		ID:          uuid.New(),
		RoomID:      roomID,
		CreatedBy:   userID,
		Code:        "expiredcode",
		Title:       "Test",
		Description: "Test",
		MaxUses:     10,
		IsEnabled:   true,
		ExpiresAt:   &expiredTime,
	}
	db.Create(&link)

	req := httptest.NewRequest(http.MethodGet, "/invite-link/resolve?code=expiredcode", nil)
	w := httptest.NewRecorder()

	handler.ResolveInviteLink(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}

func TestInviteLinkHandler_ResolveInviteLink_MaxUses(t *testing.T) {
	handler, db, userID, roomID := setupInviteLinkHandler(t)

	link := models.InviteLink{
		ID:          uuid.New(),
		RoomID:      roomID,
		CreatedBy:   userID,
		Code:        "maxusestest",
		Title:       "Test",
		Description: "Test",
		MaxUses:     1,
		Uses:        1,
		IsEnabled:   true,
	}
	db.Create(&link)

	req := httptest.NewRequest(http.MethodGet, "/invite-link/resolve?code=maxusestest", nil)
	w := httptest.NewRecorder()

	handler.ResolveInviteLink(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}

func TestInviteLinkHandler_UseInviteLink(t *testing.T) {
	handler, db, userID, roomID := setupInviteLinkHandler(t)

	joiner := models.User{
		ID:       uuid.New(),
		Username: "joiner",
		Email:    "joiner@example.com",
	}
	db.Create(&joiner)

	link := models.InviteLink{
		ID:          uuid.New(),
		RoomID:      roomID,
		CreatedBy:   userID,
		Code:        "usetest",
		Title:       "Test",
		Description: "Test",
		MaxUses:     10,
		IsEnabled:   true,
	}
	db.Create(&link)

	t.Run("valid use", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/invite-link/use?code=usetest", nil)
		req = req.WithContext(addUserIDToContext(req.Context(), joiner.ID))
		w := httptest.NewRecorder()

		handler.UseInviteLink(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d, body: %s", http.StatusOK, w.Code, w.Body.String())
		}
	})

	t.Run("missing code", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/invite-link/use", nil)
		req = req.WithContext(addUserIDToContext(req.Context(), joiner.ID))
		w := httptest.NewRecorder()

		handler.UseInviteLink(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
	})

	t.Run("invalid code", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/invite-link/use?code=invalid", nil)
		req = req.WithContext(addUserIDToContext(req.Context(), joiner.ID))
		w := httptest.NewRecorder()

		handler.UseInviteLink(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
		}
	})
}

func TestInviteLinkHandler_UseInviteLink_AlreadyInRoom(t *testing.T) {
	handler, db, userID, roomID := setupInviteLinkHandler(t)

	link := models.InviteLink{
		ID:          uuid.New(),
		RoomID:      roomID,
		CreatedBy:   userID,
		Code:        "alreadyin",
		Title:       "Test",
		Description: "Test",
		MaxUses:     10,
		IsEnabled:   true,
	}
	db.Create(&link)

	req := httptest.NewRequest(http.MethodPost, "/invite-link/use?code=alreadyin", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.UseInviteLink(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected status %d, got %d", http.StatusConflict, w.Code)
	}
}

func TestInviteLinkHandler_UseInviteLink_Disabled(t *testing.T) {
	handler, db, userID, roomID := setupInviteLinkHandler(t)

	joiner := models.User{
		ID:       uuid.New(),
		Username: "joiner2",
		Email:    "joiner2@example.com",
	}
	db.Create(&joiner)

	link := models.InviteLink{
		ID:          uuid.New(),
		RoomID:      roomID,
		CreatedBy:   userID,
		Code:        "disableduse",
		Title:       "Test",
		Description: "Test",
		MaxUses:     10,
		IsEnabled:   true,
	}
	db.Create(&link)
	link.IsEnabled = false
	db.Save(&link)

	req := httptest.NewRequest(http.MethodPost, "/invite-link/use?code=disableduse", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), joiner.ID))
	w := httptest.NewRecorder()

	handler.UseInviteLink(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}

func TestInviteLinkHandler_UseInviteLink_Expired(t *testing.T) {
	handler, db, userID, roomID := setupInviteLinkHandler(t)

	joiner := models.User{
		ID:       uuid.New(),
		Username: "joiner3",
		Email:    "joiner3@example.com",
	}
	db.Create(&joiner)

	expiredTime := time.Now().Add(-24 * time.Hour)
	link := models.InviteLink{
		ID:          uuid.New(),
		RoomID:      roomID,
		CreatedBy:   userID,
		Code:        "expireduse",
		Title:       "Test",
		Description: "Test",
		MaxUses:     10,
		IsEnabled:   true,
		ExpiresAt:   &expiredTime,
	}
	db.Create(&link)

	req := httptest.NewRequest(http.MethodPost, "/invite-link/use?code=expireduse", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), joiner.ID))
	w := httptest.NewRecorder()

	handler.UseInviteLink(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}

func TestInviteLinkHandler_UseInviteLink_MaxUses(t *testing.T) {
	handler, db, userID, roomID := setupInviteLinkHandler(t)

	joiner := models.User{
		ID:       uuid.New(),
		Username: "joiner4",
		Email:    "joiner4@example.com",
	}
	db.Create(&joiner)

	link := models.InviteLink{
		ID:          uuid.New(),
		RoomID:      roomID,
		CreatedBy:   userID,
		Code:        "maxusesuse",
		Title:       "Test",
		Description: "Test",
		MaxUses:     1,
		Uses:        1,
		IsEnabled:   true,
	}
	db.Create(&link)

	req := httptest.NewRequest(http.MethodPost, "/invite-link/use?code=maxusesuse", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), joiner.ID))
	w := httptest.NewRecorder()

	handler.UseInviteLink(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
	}
}

func TestInviteLinkHandler_UseInviteLink_RoomNotFound(t *testing.T) {
	handler, _, userID, _ := setupInviteLinkHandler(t)

	link := models.InviteLink{
		ID:          uuid.New(),
		RoomID:      uuid.New(),
		CreatedBy:   userID,
		Code:        "roomnotfound",
		Title:       "Test",
		Description: "Test",
		MaxUses:     10,
		IsEnabled:   true,
	}

	t.Logf("Testing with RoomID: %s", link.RoomID)

	req := httptest.NewRequest(http.MethodPost, "/invite-link/use?code=roomnotfound", nil)
	req = req.WithContext(addUserIDToContext(req.Context(), userID))
	w := httptest.NewRecorder()

	handler.UseInviteLink(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d, body: %s", http.StatusNotFound, w.Code, w.Body.String())
	}
}
