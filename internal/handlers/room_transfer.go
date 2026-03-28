package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
)

type RoomTransferHandler struct {
	db *database.Database
}

func NewRoomTransferHandler(db *database.Database) *RoomTransferHandler {
	return &RoomTransferHandler{db: db}
}

type TransferOwnershipRequest struct {
	RoomID     uuid.UUID `json:"room_id"`
	NewOwnerID uuid.UUID `json:"new_owner_id"`
}

type TransferOwnershipResponse struct {
	RoomID     uuid.UUID `json:"room_id"`
	OldOwnerID uuid.UUID `json:"old_owner_id"`
	NewOwnerID uuid.UUID `json:"new_owner_id"`
	Status     string    `json:"status"`
}

func (h *RoomTransferHandler) TransferOwnership(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req TransferOwnershipRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.RoomID == uuid.Nil {
		http.Error(w, "room_id is required", http.StatusBadRequest)
		return
	}

	if req.NewOwnerID == uuid.Nil {
		http.Error(w, "new_owner_id is required", http.StatusBadRequest)
		return
	}

	if req.NewOwnerID == userID {
		http.Error(w, "Cannot transfer ownership to yourself", http.StatusBadRequest)
		return
	}

	var room models.Room
	if err := h.db.First(&room, "id = ?", req.RoomID).Error; err != nil {
		http.Error(w, "Room not found", http.StatusNotFound)
		return
	}

	if room.CreatedBy != userID {
		http.Error(w, "Only the room owner can transfer ownership", http.StatusForbidden)
		return
	}

	var newOwner models.User
	if err := h.db.First(&newOwner, "id = ?", req.NewOwnerID).Error; err != nil {
		http.Error(w, "New owner not found", http.StatusNotFound)
		return
	}

	var newOwnerMembership models.UserRoom
	if err := h.db.Where("user_id = ? AND room_id = ?", req.NewOwnerID, req.RoomID).First(&newOwnerMembership).Error; err != nil {
		http.Error(w, "New owner must be a member of the room", http.StatusBadRequest)
		return
	}

	tx := h.db.Begin()

	room.CreatedBy = req.NewOwnerID
	if err := tx.Save(&room).Error; err != nil {
		tx.Rollback()
		http.Error(w, "Failed to update room ownership", http.StatusInternalServerError)
		return
	}

	if err := tx.Model(&models.UserRoom{}).
		Where("user_id = ? AND room_id = ?", req.NewOwnerID, req.RoomID).
		Update("role", "owner").Error; err != nil {
		tx.Rollback()
		http.Error(w, "Failed to update new owner role", http.StatusInternalServerError)
		return
	}

	if err := tx.Model(&models.UserRoom{}).
		Where("user_id = ? AND room_id = ?", userID, req.RoomID).
		Update("role", "member").Error; err != nil {
		tx.Rollback()
		http.Error(w, "Failed to update old owner role", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit().Error; err != nil {
		http.Error(w, "Failed to complete ownership transfer", http.StatusInternalServerError)
		return
	}

	response := TransferOwnershipResponse{
		RoomID:     req.RoomID,
		OldOwnerID: userID,
		NewOwnerID: req.NewOwnerID,
		Status:     "transferred",
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}
