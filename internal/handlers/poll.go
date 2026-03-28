package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/internal/websocket"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
)

type PollHandler struct {
	db  *database.Database
	hub *websocket.Hub
}

func NewPollHandler(db *database.Database, hub *websocket.Hub) *PollHandler {
	return &PollHandler{db: db, hub: hub}
}

type CreatePollRequest struct {
	RoomID     uuid.UUID  `json:"room_id"`
	Question   string     `json:"question"`
	Options    []string   `json:"options"`
	IsMultiple bool       `json:"is_multiple"`
	ExpiresAt  *time.Time `json:"expires_at"`
}

type VotePollRequest struct {
	PollID   uuid.UUID `json:"poll_id"`
	OptionID uuid.UUID `json:"option_id"`
}

type PollOptionResponse struct {
	ID       uuid.UUID `json:"id"`
	Text     string    `json:"text"`
	Position int       `json:"position"`
	Votes    int       `json:"votes"`
	HasVoted bool      `json:"has_voted"`
}

type PollResponse struct {
	ID         uuid.UUID            `json:"id"`
	RoomID     uuid.UUID            `json:"room_id"`
	Question   string               `json:"question"`
	CreatedBy  uuid.UUID            `json:"created_by"`
	Username   string               `json:"username"`
	IsMultiple bool                 `json:"is_multiple"`
	ExpiresAt  *time.Time           `json:"expires_at"`
	Options    []PollOptionResponse `json:"options"`
	TotalVotes int                  `json:"total_votes"`
	CreatedAt  time.Time            `json:"created_at"`
	HasVoted   bool                 `json:"has_voted"`
	CanVote    bool                 `json:"can_vote"`
}

func (h *PollHandler) CreatePoll(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req CreatePollRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Question == "" {
		http.Error(w, "Question is required", http.StatusBadRequest)
		return
	}

	if len(req.Options) < 2 {
		http.Error(w, "At least 2 options are required", http.StatusBadRequest)
		return
	}

	if len(req.Options) > 10 {
		http.Error(w, "Maximum 10 options allowed", http.StatusBadRequest)
		return
	}

	var userRoom models.UserRoom
	if err := h.db.First(&userRoom, "user_id = ? AND room_id = ?", userID, req.RoomID).Error; err != nil {
		http.Error(w, "You are not a member of this room", http.StatusForbidden)
		return
	}

	poll := &models.Poll{
		RoomID:     req.RoomID,
		Question:   req.Question,
		CreatedBy:  userID,
		IsMultiple: req.IsMultiple,
		ExpiresAt:  req.ExpiresAt,
	}

	if err := h.db.Create(poll).Error; err != nil {
		log.Printf("Error creating poll: %v", err)
		http.Error(w, "Failed to create poll", http.StatusInternalServerError)
		return
	}

	var options []PollOptionResponse
	for i, optText := range req.Options {
		opt := &models.PollOption{
			PollID:   poll.ID,
			Text:     optText,
			Position: i,
		}
		if err := h.db.Create(opt).Error; err != nil {
			log.Printf("Error creating poll option: %v", err)
			http.Error(w, "Failed to create poll option", http.StatusInternalServerError)
			return
		}

		options = append(options, PollOptionResponse{
			ID:       opt.ID,
			Text:     opt.Text,
			Position: opt.Position,
			Votes:    0,
		})
	}

	var user models.User
	h.db.First(&user, "id = ?", userID)

	response := PollResponse{
		ID:         poll.ID,
		RoomID:     poll.RoomID,
		Question:   poll.Question,
		CreatedBy:  poll.CreatedBy,
		Username:   user.Username,
		IsMultiple: poll.IsMultiple,
		ExpiresAt:  poll.ExpiresAt,
		Options:    options,
		TotalVotes: 0,
		CreatedAt:  poll.CreatedAt,
		HasVoted:   false,
		CanVote:    true,
	}

	wsMsg := &websocket.Message{
		Type:      "poll-created",
		RoomID:    req.RoomID,
		UserID:    userID,
		Timestamp: time.Now(),
	}
	wsPayload, _ := json.Marshal(response)
	wsMsg.Payload = wsPayload
	h.hub.BroadcastToRoom(req.RoomID, wsMsg, uuid.Nil)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *PollHandler) GetPolls(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	roomIDStr := r.URL.Query().Get("room_id")
	if roomIDStr == "" {
		http.Error(w, "room_id is required", http.StatusBadRequest)
		return
	}

	roomID, err := uuid.Parse(roomIDStr)
	if err != nil {
		http.Error(w, "Invalid room_id", http.StatusBadRequest)
		return
	}

	var userRoom models.UserRoom
	if err := h.db.First(&userRoom, "user_id = ? AND room_id = ?", userID, roomID).Error; err != nil {
		http.Error(w, "You are not a member of this room", http.StatusForbidden)
		return
	}

	var polls []models.Poll
	if err := h.db.Where("room_id = ?", roomID).Order("created_at DESC").Limit(20).Find(&polls).Error; err != nil {
		http.Error(w, "Failed to fetch polls", http.StatusInternalServerError)
		return
	}

	var responses []PollResponse
	for _, poll := range polls {
		var user models.User
		h.db.First(&user, "id = ?", poll.CreatedBy)

		options, totalVotes, hasVoted := h.getPollOptions(poll.ID, userID)
		canVote := h.canUserVote(&poll, userID)

		responses = append(responses, PollResponse{
			ID:         poll.ID,
			RoomID:     poll.RoomID,
			Question:   poll.Question,
			CreatedBy:  poll.CreatedBy,
			Username:   user.Username,
			IsMultiple: poll.IsMultiple,
			ExpiresAt:  poll.ExpiresAt,
			Options:    options,
			TotalVotes: totalVotes,
			CreatedAt:  poll.CreatedAt,
			HasVoted:   hasVoted,
			CanVote:    canVote,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(responses); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *PollHandler) GetPoll(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	pollIDStr := r.URL.Query().Get("poll_id")
	if pollIDStr == "" {
		http.Error(w, "poll_id is required", http.StatusBadRequest)
		return
	}

	pollID, err := uuid.Parse(pollIDStr)
	if err != nil {
		http.Error(w, "Invalid poll_id", http.StatusBadRequest)
		return
	}

	var poll models.Poll
	if err := h.db.First(&poll, "id = ?", pollID).Error; err != nil {
		http.Error(w, "Poll not found", http.StatusNotFound)
		return
	}

	var userRoom models.UserRoom
	if err := h.db.First(&userRoom, "user_id = ? AND room_id = ?", userID, poll.RoomID).Error; err != nil {
		http.Error(w, "You are not a member of this room", http.StatusForbidden)
		return
	}

	var user models.User
	h.db.First(&user, "id = ?", poll.CreatedBy)

	options, totalVotes, hasVoted := h.getPollOptions(poll.ID, userID)
	canVote := h.canUserVote(&poll, userID)

	response := PollResponse{
		ID:         poll.ID,
		RoomID:     poll.RoomID,
		Question:   poll.Question,
		CreatedBy:  poll.CreatedBy,
		Username:   user.Username,
		IsMultiple: poll.IsMultiple,
		ExpiresAt:  poll.ExpiresAt,
		Options:    options,
		TotalVotes: totalVotes,
		CreatedAt:  poll.CreatedAt,
		HasVoted:   hasVoted,
		CanVote:    canVote,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *PollHandler) getPollOptions(pollID uuid.UUID, userID uuid.UUID) ([]PollOptionResponse, int, bool) {
	var pollOptions []models.PollOption
	h.db.Where("poll_id = ?", pollID).Order("position ASC").Find(&pollOptions)

	var votes []models.PollVote
	h.db.Where("poll_id = ? AND user_id = ?", pollID, userID).Find(&votes)

	userVoteMap := make(map[uuid.UUID]bool)
	for _, vote := range votes {
		userVoteMap[vote.OptionID] = true
	}

	var options []PollOptionResponse
	var totalVotes int

	for _, opt := range pollOptions {
		var voteCount int64
		h.db.Model(&models.PollVote{}).Where("option_id = ?", opt.ID).Count(&voteCount)

		options = append(options, PollOptionResponse{
			ID:       opt.ID,
			Text:     opt.Text,
			Position: opt.Position,
			Votes:    int(voteCount),
			HasVoted: userVoteMap[opt.ID],
		})
		totalVotes += int(voteCount)
	}

	hasVoted := len(votes) > 0
	return options, totalVotes, hasVoted
}

func (h *PollHandler) canUserVote(poll *models.Poll, userID uuid.UUID) bool {
	if poll.ExpiresAt != nil && time.Now().After(*poll.ExpiresAt) {
		return false
	}

	var existingVotes []models.PollVote
	h.db.Where("poll_id = ? AND user_id = ?", poll.ID, userID).Find(&existingVotes)

	if poll.IsMultiple {
		return true
	}

	return len(existingVotes) == 0
}

func (h *PollHandler) Vote(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req VotePollRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var poll models.Poll
	if err := h.db.First(&poll, "id = ?", req.PollID).Error; err != nil {
		http.Error(w, "Poll not found", http.StatusNotFound)
		return
	}

	var pollOption models.PollOption
	if err := h.db.First(&pollOption, "id = ? AND poll_id = ?", req.OptionID, req.PollID).Error; err != nil {
		http.Error(w, "Poll option not found", http.StatusNotFound)
		return
	}

	tx := h.db.Begin()

	if !poll.IsMultiple {
		var existingCount int64
		tx.Model(&models.PollVote{}).Where("poll_id = ? AND user_id = ?", req.PollID, userID).Count(&existingCount)
		if existingCount > 0 {
			tx.Rollback()
			http.Error(w, "You have already voted on this poll", http.StatusForbidden)
			return
		}
	}

	if poll.ExpiresAt != nil && time.Now().After(*poll.ExpiresAt) {
		tx.Rollback()
		http.Error(w, "This poll has expired", http.StatusForbidden)
		return
	}

	vote := &models.PollVote{
		PollID:   req.PollID,
		OptionID: req.OptionID,
		UserID:   userID,
	}

	if err := tx.Create(vote).Error; err != nil {
		tx.Rollback()
		log.Printf("Error creating vote: %v", err)
		http.Error(w, "Failed to record vote", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit().Error; err != nil {
		log.Printf("Error committing vote: %v", err)
		http.Error(w, "Failed to record vote", http.StatusInternalServerError)
		return
	}

	options, totalVotes, hasVoted := h.getPollOptions(poll.ID, userID)

	response := PollResponse{
		ID:         poll.ID,
		RoomID:     poll.RoomID,
		Question:   poll.Question,
		CreatedBy:  poll.CreatedBy,
		IsMultiple: poll.IsMultiple,
		ExpiresAt:  poll.ExpiresAt,
		Options:    options,
		TotalVotes: totalVotes,
		CreatedAt:  poll.CreatedAt,
		HasVoted:   hasVoted,
		CanVote:    h.canUserVote(&poll, userID),
	}

	wsMsg := &websocket.Message{
		Type:      "poll-vote",
		RoomID:    poll.RoomID,
		UserID:    userID,
		Timestamp: time.Now(),
	}
	wsPayload, _ := json.Marshal(response)
	wsMsg.Payload = wsPayload
	h.hub.BroadcastToRoom(poll.RoomID, wsMsg, uuid.Nil)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *PollHandler) DeletePoll(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	pollIDStr := r.URL.Query().Get("poll_id")
	if pollIDStr == "" {
		http.Error(w, "poll_id is required", http.StatusBadRequest)
		return
	}

	pollID, err := uuid.Parse(pollIDStr)
	if err != nil {
		http.Error(w, "Invalid poll_id", http.StatusBadRequest)
		return
	}

	var poll models.Poll
	if err := h.db.First(&poll, "id = ?", pollID).Error; err != nil {
		http.Error(w, "Poll not found", http.StatusNotFound)
		return
	}

	if poll.CreatedBy != userID {
		http.Error(w, "You can only delete your own polls", http.StatusForbidden)
		return
	}

	h.db.Where("poll_id = ?", pollID).Delete(&models.PollVote{})
	h.db.Where("poll_id = ?", pollID).Delete(&models.PollOption{})
	h.db.Delete(&poll)

	wsMsg := &websocket.Message{
		Type:      "poll-deleted",
		RoomID:    poll.RoomID,
		UserID:    userID,
		Timestamp: time.Now(),
	}
	wsPayload, _ := json.Marshal(map[string]string{"id": pollID.String()})
	wsMsg.Payload = wsPayload
	h.hub.BroadcastToRoom(poll.RoomID, wsMsg, uuid.Nil)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "deleted"}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}
