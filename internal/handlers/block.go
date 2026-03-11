package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
)

type BlockHandler struct {
	db *database.Database
}

func NewBlockHandler(db *database.Database) *BlockHandler {
	return &BlockHandler{db: db}
}

type BlockUserRequest struct {
	BlockedID uuid.UUID `json:"blocked_id"`
	Reason    string    `json:"reason"`
}

type BlockResponse struct {
	ID        uuid.UUID `json:"id"`
	BlockerID uuid.UUID `json:"blocker_id"`
	BlockedID uuid.UUID `json:"blocked_id"`
	Reason    string    `json:"reason"`
	CreatedAt string    `json:"created_at"`
}

func (h *BlockHandler) BlockUser(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req BlockUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.BlockedID == uuid.Nil {
		http.Error(w, "blocked_id is required", http.StatusBadRequest)
		return
	}

	if req.BlockedID == userID {
		http.Error(w, "You cannot block yourself", http.StatusBadRequest)
		return
	}

	var existingBlock models.Block
	err := h.db.First(&existingBlock, "blocker_id = ? AND blocked_id = ?", userID, req.BlockedID).Error
	if err == nil {
		http.Error(w, "User is already blocked", http.StatusConflict)
		return
	}

	block := &models.Block{
		BlockerID: userID,
		BlockedID: req.BlockedID,
		Reason:    req.Reason,
	}

	if err := h.db.Create(block).Error; err != nil {
		log.Printf("Error blocking user: %v", err)
		http.Error(w, "Failed to block user", http.StatusInternalServerError)
		return
	}

	response := BlockResponse{
		ID:        block.ID,
		BlockerID: block.BlockerID,
		BlockedID: block.BlockedID,
		Reason:    block.Reason,
		CreatedAt: block.CreatedAt.String(),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *BlockHandler) UnblockUser(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	blockedIDStr := r.URL.Query().Get("blocked_id")
	if blockedIDStr == "" {
		http.Error(w, "blocked_id is required", http.StatusBadRequest)
		return
	}

	blockedID, err := uuid.Parse(blockedIDStr)
	if err != nil {
		http.Error(w, "Invalid blocked_id", http.StatusBadRequest)
		return
	}

	result := h.db.Where("blocker_id = ? AND blocked_id = ?", userID, blockedID).Delete(&models.Block{})
	if result.Error != nil {
		log.Printf("Error unblocking user: %v", result.Error)
		http.Error(w, "Failed to unblock user", http.StatusInternalServerError)
		return
	}

	if result.RowsAffected == 0 {
		http.Error(w, "User is not blocked", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "unblocked"}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *BlockHandler) GetBlockedUsers(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var blocks []models.Block
	if err := h.db.Where("blocker_id = ?", userID).Find(&blocks).Error; err != nil {
		log.Printf("Error fetching blocked users: %v", err)
		http.Error(w, "Failed to fetch blocked users", http.StatusInternalServerError)
		return
	}

	type BlockedUserResponse struct {
		ID        uuid.UUID `json:"id"`
		BlockedID uuid.UUID `json:"blocked_id"`
		Username  string    `json:"username"`
		Reason    string    `json:"reason"`
		CreatedAt string    `json:"created_at"`
	}

	var response []BlockedUserResponse
	for _, block := range blocks {
		var user models.User
		h.db.First(&user, "id = ?", block.BlockedID)

		response = append(response, BlockedUserResponse{
			ID:        block.ID,
			BlockedID: block.BlockedID,
			Username:  user.Username,
			Reason:    block.Reason,
			CreatedAt: block.CreatedAt.String(),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *BlockHandler) IsBlocked(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	targetIDStr := r.URL.Query().Get("user_id")
	if targetIDStr == "" {
		http.Error(w, "user_id is required", http.StatusBadRequest)
		return
	}

	targetID, err := uuid.Parse(targetIDStr)
	if err != nil {
		http.Error(w, "Invalid user_id", http.StatusBadRequest)
		return
	}

	var block models.Block
	err = h.db.First(&block, "blocker_id = ? AND blocked_id = ?", userID, targetID).Error
	isBlocked := err == nil

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]bool{"is_blocked": isBlocked}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}
