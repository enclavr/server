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

func setupGroupDMDB(t *testing.T) *gorm.DB {
	db := openTestDB(t)

	err := db.AutoMigrate(
		&models.User{},
		&models.GroupDM{},
		&models.GroupDMMember{},
		&models.GroupDMMessage{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func setupGroupDMTest(t *testing.T) (*GroupDMHandler, *database.Database, uuid.UUID, uuid.UUID) {
	db := setupGroupDMDB(t)
	testDB := &database.Database{DB: db}
	handler := NewGroupDMHandler(testDB)

	user1 := models.User{
		ID:       uuid.New(),
		Username: "user1",
		Email:    "user1@example.com",
	}
	db.Create(&user1)

	user2 := models.User{
		ID:       uuid.New(),
		Username: "user2",
		Email:    "user2@example.com",
	}
	db.Create(&user2)

	return handler, testDB, user1.ID, user2.ID
}

func groupDMContextWithUserID(ctx context.Context, userID uuid.UUID) context.Context {
	return context.WithValue(ctx, middleware.UserIDKey, userID)
}

func TestGroupDMHandler_CreateGroupDM(t *testing.T) {
	handler, _, user1ID, user2ID := setupGroupDMTest(t)

	tests := []struct {
		name           string
		body           CreateGroupDMRequest
		expectedStatus int
		setupCtx       func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name: "valid group DM",
			body: CreateGroupDMRequest{
				Name:      "Test Group",
				MemberIDs: []uuid.UUID{user2ID},
			},
			expectedStatus: http.StatusCreated,
			setupCtx:       groupDMContextWithUserID,
		},
		{
			name: "empty members",
			body: CreateGroupDMRequest{
				Name:      "Empty Group",
				MemberIDs: []uuid.UUID{},
			},
			expectedStatus: http.StatusBadRequest,
			setupCtx:       groupDMContextWithUserID,
		},
		{
			name: "unauthorized",
			body: CreateGroupDMRequest{
				Name:      "Test Group",
				MemberIDs: []uuid.UUID{user2ID},
			},
			expectedStatus: http.StatusUnauthorized,
			setupCtx:       func(ctx context.Context, _ uuid.UUID) context.Context { return ctx },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/group-dm/create", bytes.NewReader(body))
			req = req.WithContext(tt.setupCtx(req.Context(), user1ID))
			w := httptest.NewRecorder()

			handler.CreateGroupDM(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}

	t.Run("created group has correct members", func(t *testing.T) {
		body, _ := json.Marshal(CreateGroupDMRequest{
			Name:      "Verify Group",
			MemberIDs: []uuid.UUID{user2ID},
		})
		req := httptest.NewRequest(http.MethodPost, "/api/group-dm/create", bytes.NewReader(body))
		req = req.WithContext(groupDMContextWithUserID(req.Context(), user1ID))
		w := httptest.NewRecorder()

		handler.CreateGroupDM(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected status %d, got %d", http.StatusCreated, w.Code)
		}

		var response GroupDMResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if len(response.Members) != 2 {
			t.Errorf("expected 2 members, got %d", len(response.Members))
		}

		if response.Name != "Verify Group" {
			t.Errorf("expected name 'Verify Group', got %q", response.Name)
		}
	})
}

func TestGroupDMHandler_GetGroupDMs(t *testing.T) {
	handler, db, user1ID, user2ID := setupGroupDMTest(t)

	groupDM := models.GroupDM{
		Name:      "Existing Group",
		CreatedBy: user1ID,
	}
	db.Create(&groupDM)

	db.Create(&models.GroupDMMember{GroupDMID: groupDM.ID, UserID: user1ID, Role: "owner"})
	db.Create(&models.GroupDMMember{GroupDMID: groupDM.ID, UserID: user2ID, Role: "member"})

	t.Run("get group DMs for member", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/group-dms", nil)
		req = req.WithContext(groupDMContextWithUserID(req.Context(), user1ID))
		w := httptest.NewRecorder()

		handler.GetGroupDMs(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
		}

		var response []GroupDMResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if len(response) != 1 {
			t.Errorf("expected 1 group DM, got %d", len(response))
		}
	})

	t.Run("get group DMs for non-member", func(t *testing.T) {
		nonMemberID := uuid.New()
		db.Create(&models.User{ID: nonMemberID, Username: "nonmember", Email: "non@test.com"})

		req := httptest.NewRequest(http.MethodGet, "/api/group-dms", nil)
		req = req.WithContext(groupDMContextWithUserID(req.Context(), nonMemberID))
		w := httptest.NewRecorder()

		handler.GetGroupDMs(w, req)

		var response []GroupDMResponse
		json.NewDecoder(w.Body).Decode(&response)

		if len(response) != 0 {
			t.Errorf("expected 0 group DMs, got %d", len(response))
		}
	})
}

func TestGroupDMHandler_SendGroupDMMessage(t *testing.T) {
	handler, db, user1ID, user2ID := setupGroupDMTest(t)

	groupDM := models.GroupDM{
		Name:      "Message Group",
		CreatedBy: user1ID,
	}
	db.Create(&groupDM)

	db.Create(&models.GroupDMMember{GroupDMID: groupDM.ID, UserID: user1ID, Role: "owner"})
	db.Create(&models.GroupDMMember{GroupDMID: groupDM.ID, UserID: user2ID, Role: "member"})

	tests := []struct {
		name           string
		body           SendGroupDMRequest
		expectedStatus int
		setupCtx       func(ctx context.Context, userID uuid.UUID) context.Context
	}{
		{
			name: "valid message",
			body: SendGroupDMRequest{
				GroupDMID: groupDM.ID,
				Content:   "Hello group!",
			},
			expectedStatus: http.StatusCreated,
			setupCtx:       groupDMContextWithUserID,
		},
		{
			name: "empty content",
			body: SendGroupDMRequest{
				GroupDMID: groupDM.ID,
				Content:   "",
			},
			expectedStatus: http.StatusBadRequest,
			setupCtx:       groupDMContextWithUserID,
		},
		{
			name: "non-member",
			body: SendGroupDMRequest{
				GroupDMID: groupDM.ID,
				Content:   "Hello",
			},
			expectedStatus: http.StatusForbidden,
			setupCtx: func(ctx context.Context, _ uuid.UUID) context.Context {
				nonMemberID := uuid.New()
				db.Create(&models.User{ID: nonMemberID, Username: "nonmember", Email: "non@test.com"})
				return context.WithValue(ctx, middleware.UserIDKey, nonMemberID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/group-dm/message/send", bytes.NewReader(body))
			req = req.WithContext(tt.setupCtx(req.Context(), user1ID))
			w := httptest.NewRecorder()

			handler.SendGroupDMMessage(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestGroupDMHandler_GetGroupDMMessages(t *testing.T) {
	handler, db, user1ID, user2ID := setupGroupDMTest(t)

	groupDM := models.GroupDM{
		Name:      "Messages Group",
		CreatedBy: user1ID,
	}
	db.Create(&groupDM)

	db.Create(&models.GroupDMMember{GroupDMID: groupDM.ID, UserID: user1ID, Role: "owner"})
	db.Create(&models.GroupDMMember{GroupDMID: groupDM.ID, UserID: user2ID, Role: "member"})

	msg := models.GroupDMMessage{
		GroupDMID: groupDM.ID,
		SenderID:  user1ID,
		Content:   "Test message",
	}
	db.Create(&msg)

	t.Run("get messages as member", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/group-dm/messages?group_dm_id="+groupDM.ID.String(), nil)
		req = req.WithContext(groupDMContextWithUserID(req.Context(), user1ID))
		w := httptest.NewRecorder()

		handler.GetGroupDMMessages(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
		}

		var response []GroupDMMessageResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if len(response) != 1 {
			t.Errorf("expected 1 message, got %d", len(response))
		}
	})

	t.Run("get messages as non-member", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/group-dm/messages?group_dm_id="+groupDM.ID.String(), nil)
		req = req.WithContext(groupDMContextWithUserID(req.Context(), uuid.New()))
		w := httptest.NewRecorder()

		handler.GetGroupDMMessages(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
		}
	})
}

func TestGroupDMHandler_AddRemoveMember(t *testing.T) {
	handler, db, user1ID, user2ID := setupGroupDMTest(t)

	user3 := models.User{
		ID:       uuid.New(),
		Username: "user3",
		Email:    "user3@example.com",
	}
	db.Create(&user3)

	groupDM := models.GroupDM{
		Name:      "Membership Group",
		CreatedBy: user1ID,
	}
	db.Create(&groupDM)

	db.Create(&models.GroupDMMember{GroupDMID: groupDM.ID, UserID: user1ID, Role: "owner"})
	db.Create(&models.GroupDMMember{GroupDMID: groupDM.ID, UserID: user2ID, Role: "member"})

	t.Run("owner can add member", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"group_dm_id": groupDM.ID.String(),
			"user_id":     user3.ID.String(),
		})
		req := httptest.NewRequest(http.MethodPost, "/api/group-dm/member/add", bytes.NewReader(body))
		req = req.WithContext(groupDMContextWithUserID(req.Context(), user1ID))
		w := httptest.NewRecorder()

		handler.AddMember(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("expected status %d, got %d", http.StatusCreated, w.Code)
		}
	})

	t.Run("non-owner cannot add member", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"group_dm_id": groupDM.ID.String(),
			"user_id":     user3.ID.String(),
		})
		req := httptest.NewRequest(http.MethodPost, "/api/group-dm/member/add", bytes.NewReader(body))
		req = req.WithContext(groupDMContextWithUserID(req.Context(), user2ID))
		w := httptest.NewRecorder()

		handler.AddMember(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
		}
	})

	t.Run("owner can remove member", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"group_dm_id": groupDM.ID.String(),
			"user_id":     user2ID.String(),
		})
		req := httptest.NewRequest(http.MethodPost, "/api/group-dm/member/remove", bytes.NewReader(body))
		req = req.WithContext(groupDMContextWithUserID(req.Context(), user1ID))
		w := httptest.NewRecorder()

		handler.RemoveMember(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
		}
	})
}

func TestGroupDMHandler_LeaveGroupDM(t *testing.T) {
	handler, db, user1ID, user2ID := setupGroupDMTest(t)

	groupDM := models.GroupDM{
		Name:      "Leave Group",
		CreatedBy: user1ID,
	}
	db.Create(&groupDM)

	db.Create(&models.GroupDMMember{GroupDMID: groupDM.ID, UserID: user1ID, Role: "owner"})
	db.Create(&models.GroupDMMember{GroupDMID: groupDM.ID, UserID: user2ID, Role: "member"})

	t.Run("member can leave group", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/group-dm/leave?group_dm_id="+groupDM.ID.String(), nil)
		req = req.WithContext(groupDMContextWithUserID(req.Context(), user2ID))
		w := httptest.NewRecorder()

		handler.LeaveGroupDM(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
		}

		var count int64
		db.Model(&models.GroupDMMember{}).Where("group_dm_id = ? AND user_id = ?", groupDM.ID, user2ID).Count(&count)
		if count != 0 {
			t.Error("expected member to be removed")
		}
	})

	t.Run("non-member cannot leave", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/group-dm/leave?group_dm_id="+groupDM.ID.String(), nil)
		req = req.WithContext(groupDMContextWithUserID(req.Context(), uuid.New()))
		w := httptest.NewRecorder()

		handler.LeaveGroupDM(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("expected status %d, got %d", http.StatusForbidden, w.Code)
		}
	})
}
