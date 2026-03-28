package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func setupRoomTransferTestDB(t *testing.T) *gorm.DB {
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

func setupRoomTransferTest(t *testing.T) (*RoomTransferHandler, *database.Database, uuid.UUID, uuid.UUID, uuid.UUID) {
	db := setupRoomTransferTestDB(t)
	testDB := &database.Database{DB: db}
	handler := NewRoomTransferHandler(testDB)

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
	db.Create(&owner)
	db.Create(&member)

	room := models.Room{
		ID:        uuid.New(),
		Name:      "test-room",
		CreatedBy: owner.ID,
	}
	db.Create(&room)

	db.Create(&models.UserRoom{UserID: owner.ID, RoomID: room.ID, Role: "owner"})
	db.Create(&models.UserRoom{UserID: member.ID, RoomID: room.ID, Role: "member"})

	return handler, testDB, owner.ID, member.ID, room.ID
}

func TestRoomTransferHandler_TransferOwnership(t *testing.T) {
	nonMember := models.User{
		ID:       uuid.New(),
		Username: "nonmember",
		Email:    "nonmember@example.com",
	}

	tests := []struct {
		name           string
		setupRoom      bool
		expectedStatus int
	}{
		{
			name:           "valid transfer",
			setupRoom:      true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "transfer to self",
			setupRoom:      true,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "non-owner cannot transfer",
			setupRoom:      true,
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "transfer to non-member",
			setupRoom:      true,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "room not found",
			setupRoom:      false,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "missing room_id",
			setupRoom:      false,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, db, ownerID, memberID, roomID := setupRoomTransferTest(t)
			db.Create(&nonMember)

			var reqRoomID uuid.UUID
			var reqNewOwnerID uuid.UUID
			var reqUserID uuid.UUID

			switch tt.name {
			case "valid transfer":
				reqRoomID = roomID
				reqNewOwnerID = memberID
				reqUserID = ownerID
			case "transfer to self":
				reqRoomID = roomID
				reqNewOwnerID = ownerID
				reqUserID = ownerID
			case "non-owner cannot transfer":
				reqRoomID = roomID
				reqNewOwnerID = ownerID
				reqUserID = memberID
			case "transfer to non-member":
				reqRoomID = roomID
				reqNewOwnerID = nonMember.ID
				reqUserID = ownerID
			case "room not found":
				reqRoomID = uuid.New()
				reqNewOwnerID = memberID
				reqUserID = ownerID
			case "missing room_id":
				reqRoomID = uuid.Nil
				reqNewOwnerID = memberID
				reqUserID = ownerID
			}

			reqBody := TransferOwnershipRequest{
				RoomID:     reqRoomID,
				NewOwnerID: reqNewOwnerID,
			}
			body, _ := json.Marshal(reqBody)
			req := httptest.NewRequest(http.MethodPost, "/api/room/transfer-ownership", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(addUserIDToContext(req.Context(), reqUserID))
			w := httptest.NewRecorder()

			handler.TransferOwnership(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d, body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}

			if tt.expectedStatus == http.StatusOK {
				var resp TransferOwnershipResponse
				if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
					t.Fatalf("failed to unmarshal response: %v", err)
				}
				if resp.Status != "transferred" {
					t.Errorf("expected status 'transferred', got '%s'", resp.Status)
				}

				var room models.Room
				db.First(&room, "id = ?", roomID)
				if room.CreatedBy != memberID {
					t.Errorf("expected room created_by to be %s, got %s", memberID, room.CreatedBy)
				}

				var newOwnerRoom models.UserRoom
				db.Where("user_id = ? AND room_id = ?", memberID, roomID).First(&newOwnerRoom)
				if newOwnerRoom.Role != "owner" {
					t.Errorf("expected new owner role 'owner', got '%s'", newOwnerRoom.Role)
				}

				var oldOwnerRoom models.UserRoom
				db.Where("user_id = ? AND room_id = ?", ownerID, roomID).First(&oldOwnerRoom)
				if oldOwnerRoom.Role != "member" {
					t.Errorf("expected old owner role 'member', got '%s'", oldOwnerRoom.Role)
				}
			}
		})
	}
}
