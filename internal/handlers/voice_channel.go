package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// VoiceChannelHandler handles voice channel CRUD and participant management.
type VoiceChannelHandler struct {
	db *database.Database
}

// NewVoiceChannelHandler creates a new VoiceChannelHandler instance.
func NewVoiceChannelHandler(db *database.Database) *VoiceChannelHandler {
	return &VoiceChannelHandler{db: db}
}

// CreateVoiceChannelRequest represents the request body for creating a voice channel.
type CreateVoiceChannelRequest struct {
	RoomID          uuid.UUID `json:"room_id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	MaxParticipants int       `json:"max_participants"`
	IsPrivate       bool      `json:"is_private"`
}

// UpdateVoiceChannelRequest represents the request body for updating a voice channel.
type UpdateVoiceChannelRequest struct {
	Name            *string `json:"name,omitempty"`
	Description     *string `json:"description,omitempty"`
	MaxParticipants *int    `json:"max_participants,omitempty"`
	IsPrivate       *bool   `json:"is_private,omitempty"`
}

// VoiceChannelResponse represents the JSON response for a voice channel.
type VoiceChannelResponse struct {
	ID               uuid.UUID                  `json:"id"`
	RoomID           uuid.UUID                  `json:"room_id"`
	Name             string                     `json:"name"`
	Description      string                     `json:"description"`
	MaxParticipants  int                        `json:"max_participants"`
	IsPrivate        bool                       `json:"is_private"`
	CreatedBy        uuid.UUID                  `json:"created_by"`
	ParticipantCount int                        `json:"participant_count"`
	Participants     []VoiceParticipantResponse `json:"participants,omitempty"`
	CreatedAt        string                     `json:"created_at"`
	UpdatedAt        string                     `json:"updated_at"`
}

// VoiceParticipantResponse represents a participant in a voice channel.
type VoiceParticipantResponse struct {
	ID         uuid.UUID `json:"id"`
	UserID     uuid.UUID `json:"user_id"`
	Username   string    `json:"username"`
	IsMuted    bool      `json:"is_muted"`
	IsDeafened bool      `json:"is_deafened"`
	IsSpeaking bool      `json:"is_speaking"`
	JoinedAt   string    `json:"joined_at"`
}

func (h *VoiceChannelHandler) channelToResponse(channel *models.VoiceChannel, participants []models.VoiceChannelParticipant) VoiceChannelResponse {
	resp := VoiceChannelResponse{
		ID:               channel.ID,
		RoomID:           channel.RoomID,
		Name:             channel.Name,
		Description:      channel.Description,
		MaxParticipants:  channel.MaxParticipants,
		IsPrivate:        channel.IsPrivate,
		CreatedBy:        channel.CreatedBy,
		ParticipantCount: len(participants),
		CreatedAt:        channel.CreatedAt.Format(time.RFC3339),
		UpdatedAt:        channel.UpdatedAt.Format(time.RFC3339),
	}

	if len(participants) > 0 {
		resp.Participants = make([]VoiceParticipantResponse, 0, len(participants))
		for _, p := range participants {
			resp.Participants = append(resp.Participants, VoiceParticipantResponse{
				ID:         p.ID,
				UserID:     p.UserID,
				IsMuted:    p.IsMuted,
				IsDeafened: p.IsDeafened,
				IsSpeaking: p.IsSpeaking,
				JoinedAt:   p.JoinedAt.Format(time.RFC3339),
			})
		}
	}

	return resp
}

func sendVoiceChannelJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error encoding voice channel response: %v", err)
	}
}

// CreateChannel creates a new voice channel in a room.
func (h *VoiceChannelHandler) CreateChannel(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req CreateVoiceChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "Channel name is required", http.StatusBadRequest)
		return
	}

	if req.RoomID == uuid.Nil {
		http.Error(w, "Room ID is required", http.StatusBadRequest)
		return
	}

	var userRoom models.UserRoom
	if err := h.db.Where("user_id = ? AND room_id = ?", userID, req.RoomID).First(&userRoom).Error; err != nil {
		http.Error(w, "You are not a member of this room", http.StatusForbidden)
		return
	}

	if req.MaxParticipants <= 0 {
		req.MaxParticipants = 10
	}
	if req.MaxParticipants > 99 {
		req.MaxParticipants = 99
	}

	channel := models.VoiceChannel{
		RoomID:          req.RoomID,
		Name:            req.Name,
		Description:     req.Description,
		MaxParticipants: req.MaxParticipants,
		IsPrivate:       req.IsPrivate,
		CreatedBy:       userID,
	}

	if err := h.db.Create(&channel).Error; err != nil {
		log.Printf("Error creating voice channel: %v", err)
		http.Error(w, "Failed to create voice channel", http.StatusInternalServerError)
		return
	}

	resp := h.channelToResponse(&channel, nil)
	sendVoiceChannelJSON(w, http.StatusCreated, resp)
}

// GetChannel returns a voice channel by ID with its current participants.
func (h *VoiceChannelHandler) GetChannel(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	channelIDStr := r.URL.Query().Get("id")
	if channelIDStr == "" {
		http.Error(w, "Channel ID is required", http.StatusBadRequest)
		return
	}

	channelID, err := uuid.Parse(channelIDStr)
	if err != nil {
		http.Error(w, "Invalid channel ID", http.StatusBadRequest)
		return
	}

	var channel models.VoiceChannel
	if err := h.db.Where("id = ?", channelID).First(&channel).Error; err != nil {
		http.Error(w, "Voice channel not found", http.StatusNotFound)
		return
	}

	var userRoom models.UserRoom
	if err := h.db.Where("user_id = ? AND room_id = ?", userID, channel.RoomID).First(&userRoom).Error; err != nil {
		http.Error(w, "You are not a member of this room", http.StatusForbidden)
		return
	}

	var participants []models.VoiceChannelParticipant
	h.db.Preload("User").Where("channel_id = ?", channelID).Find(&participants)

	participantResponses := make([]VoiceParticipantResponse, 0, len(participants))
	for _, p := range participants {
		participantResponses = append(participantResponses, VoiceParticipantResponse{
			ID:         p.ID,
			UserID:     p.UserID,
			Username:   p.User.Username,
			IsMuted:    p.IsMuted,
			IsDeafened: p.IsDeafened,
			IsSpeaking: p.IsSpeaking,
			JoinedAt:   p.JoinedAt.Format(time.RFC3339),
		})
	}

	resp := VoiceChannelResponse{
		ID:               channel.ID,
		RoomID:           channel.RoomID,
		Name:             channel.Name,
		Description:      channel.Description,
		MaxParticipants:  channel.MaxParticipants,
		IsPrivate:        channel.IsPrivate,
		CreatedBy:        channel.CreatedBy,
		ParticipantCount: len(participants),
		Participants:     participantResponses,
		CreatedAt:        channel.CreatedAt.Format(time.RFC3339),
		UpdatedAt:        channel.UpdatedAt.Format(time.RFC3339),
	}

	sendVoiceChannelJSON(w, http.StatusOK, resp)
}

// GetRoomChannels returns all voice channels for a given room.
func (h *VoiceChannelHandler) GetRoomChannels(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	roomIDStr := r.URL.Query().Get("room_id")
	if roomIDStr == "" {
		http.Error(w, "Room ID is required", http.StatusBadRequest)
		return
	}

	roomID, err := uuid.Parse(roomIDStr)
	if err != nil {
		http.Error(w, "Invalid room ID", http.StatusBadRequest)
		return
	}

	var userRoom models.UserRoom
	if err := h.db.Where("user_id = ? AND room_id = ?", userID, roomID).First(&userRoom).Error; err != nil {
		http.Error(w, "You are not a member of this room", http.StatusForbidden)
		return
	}

	var channels []models.VoiceChannel
	h.db.Where("room_id = ?", roomID).Find(&channels)

	responses := make([]VoiceChannelResponse, 0, len(channels))
	for _, ch := range channels {
		var count int64
		h.db.Model(&models.VoiceChannelParticipant{}).Where("channel_id = ?", ch.ID).Count(&count)

		responses = append(responses, VoiceChannelResponse{
			ID:               ch.ID,
			RoomID:           ch.RoomID,
			Name:             ch.Name,
			Description:      ch.Description,
			MaxParticipants:  ch.MaxParticipants,
			IsPrivate:        ch.IsPrivate,
			CreatedBy:        ch.CreatedBy,
			ParticipantCount: int(count),
			CreatedAt:        ch.CreatedAt.Format(time.RFC3339),
			UpdatedAt:        ch.UpdatedAt.Format(time.RFC3339),
		})
	}

	sendVoiceChannelJSON(w, http.StatusOK, responses)
}

// UpdateChannel updates a voice channel's properties.
func (h *VoiceChannelHandler) UpdateChannel(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	channelIDStr := r.URL.Query().Get("id")
	if channelIDStr == "" {
		http.Error(w, "Channel ID is required", http.StatusBadRequest)
		return
	}

	channelID, err := uuid.Parse(channelIDStr)
	if err != nil {
		http.Error(w, "Invalid channel ID", http.StatusBadRequest)
		return
	}

	var channel models.VoiceChannel
	if err := h.db.Where("id = ?", channelID).First(&channel).Error; err != nil {
		http.Error(w, "Voice channel not found", http.StatusNotFound)
		return
	}

	var userRoom models.UserRoom
	if err := h.db.Where("user_id = ? AND room_id = ? AND role IN ?", userID, channel.RoomID, []string{"owner", "admin"}).First(&userRoom).Error; err != nil {
		if channel.CreatedBy != userID {
			http.Error(w, "You don't have permission to update this channel", http.StatusForbidden)
			return
		}
	}

	var req UpdateVoiceChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	updates := map[string]interface{}{
		"updated_at": time.Now(),
	}

	if req.Name != nil {
		if *req.Name == "" {
			http.Error(w, "Channel name cannot be empty", http.StatusBadRequest)
			return
		}
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.MaxParticipants != nil {
		if *req.MaxParticipants <= 0 {
			http.Error(w, "Max participants must be greater than 0", http.StatusBadRequest)
			return
		}
		if *req.MaxParticipants > 99 {
			*req.MaxParticipants = 99
		}
		updates["max_participants"] = *req.MaxParticipants
	}
	if req.IsPrivate != nil {
		updates["is_private"] = *req.IsPrivate
	}

	if err := h.db.Model(&channel).Updates(updates).Error; err != nil {
		log.Printf("Error updating voice channel: %v", err)
		http.Error(w, "Failed to update voice channel", http.StatusInternalServerError)
		return
	}

	h.db.Where("id = ?", channelID).First(&channel)

	var participants []models.VoiceChannelParticipant
	h.db.Where("channel_id = ?", channelID).Find(&participants)

	resp := h.channelToResponse(&channel, participants)
	sendVoiceChannelJSON(w, http.StatusOK, resp)
}

// DeleteChannel soft-deletes a voice channel and removes all participants.
func (h *VoiceChannelHandler) DeleteChannel(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	channelIDStr := r.URL.Query().Get("id")
	if channelIDStr == "" {
		http.Error(w, "Channel ID is required", http.StatusBadRequest)
		return
	}

	channelID, err := uuid.Parse(channelIDStr)
	if err != nil {
		http.Error(w, "Invalid channel ID", http.StatusBadRequest)
		return
	}

	var channel models.VoiceChannel
	if err := h.db.Where("id = ?", channelID).First(&channel).Error; err != nil {
		http.Error(w, "Voice channel not found", http.StatusNotFound)
		return
	}

	var userRoom models.UserRoom
	if err := h.db.Where("user_id = ? AND room_id = ? AND role IN ?", userID, channel.RoomID, []string{"owner", "admin"}).First(&userRoom).Error; err != nil {
		if channel.CreatedBy != userID {
			http.Error(w, "You don't have permission to delete this channel", http.StatusForbidden)
			return
		}
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("channel_id = ?", channelID).Delete(&models.VoiceChannelParticipant{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&channel).Error; err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		log.Printf("Error deleting voice channel: %v", err)
		http.Error(w, "Failed to delete voice channel", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(`{"message":"Voice channel deleted"}`)); err != nil {
		log.Printf("Error writing response: %v", err)
	}
}

// JoinChannel adds a user to a voice channel.
func (h *VoiceChannelHandler) JoinChannel(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		ChannelID uuid.UUID `json:"channel_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ChannelID == uuid.Nil {
		http.Error(w, "Channel ID is required", http.StatusBadRequest)
		return
	}

	var channel models.VoiceChannel
	if err := h.db.Where("id = ?", req.ChannelID).First(&channel).Error; err != nil {
		http.Error(w, "Voice channel not found", http.StatusNotFound)
		return
	}

	var userRoom models.UserRoom
	if err := h.db.Where("user_id = ? AND room_id = ?", userID, channel.RoomID).First(&userRoom).Error; err != nil {
		http.Error(w, "You are not a member of this room", http.StatusForbidden)
		return
	}

	var existingParticipant models.VoiceChannelParticipant
	if err := h.db.Where("channel_id = ? AND user_id = ?", req.ChannelID, userID).First(&existingParticipant).Error; err == nil {
		resp := VoiceParticipantResponse{
			ID:         existingParticipant.ID,
			UserID:     existingParticipant.UserID,
			IsMuted:    existingParticipant.IsMuted,
			IsDeafened: existingParticipant.IsDeafened,
			IsSpeaking: existingParticipant.IsSpeaking,
			JoinedAt:   existingParticipant.JoinedAt.Format(time.RFC3339),
		}
		sendVoiceChannelJSON(w, http.StatusOK, resp)
		return
	}

	var participantCount int64
	h.db.Model(&models.VoiceChannelParticipant{}).Where("channel_id = ?", req.ChannelID).Count(&participantCount)
	if int(participantCount) >= channel.MaxParticipants {
		http.Error(w, "Voice channel is full", http.StatusForbidden)
		return
	}

	participant := models.VoiceChannelParticipant{
		ChannelID: req.ChannelID,
		UserID:    userID,
	}

	if err := h.db.Create(&participant).Error; err != nil {
		log.Printf("Error joining voice channel: %v", err)
		http.Error(w, "Failed to join voice channel", http.StatusInternalServerError)
		return
	}

	resp := VoiceParticipantResponse{
		ID:         participant.ID,
		UserID:     participant.UserID,
		IsMuted:    participant.IsMuted,
		IsDeafened: participant.IsDeafened,
		IsSpeaking: participant.IsSpeaking,
		JoinedAt:   participant.JoinedAt.Format(time.RFC3339),
	}

	sendVoiceChannelJSON(w, http.StatusCreated, resp)
}

// LeaveChannel removes a user from a voice channel.
func (h *VoiceChannelHandler) LeaveChannel(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	channelIDStr := r.URL.Query().Get("channel_id")
	if channelIDStr == "" {
		http.Error(w, "Channel ID is required", http.StatusBadRequest)
		return
	}

	channelID, err := uuid.Parse(channelIDStr)
	if err != nil {
		http.Error(w, "Invalid channel ID", http.StatusBadRequest)
		return
	}

	result := h.db.Where("channel_id = ? AND user_id = ?", channelID, userID).Delete(&models.VoiceChannelParticipant{})
	if result.RowsAffected == 0 {
		http.Error(w, "You are not in this voice channel", http.StatusNotFound)
		return
	}

	if result.Error != nil {
		log.Printf("Error leaving voice channel: %v", result.Error)
		http.Error(w, "Failed to leave voice channel", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(`{"message":"Left voice channel"}`)); err != nil {
		log.Printf("Error writing response: %v", err)
	}
}

// UpdateParticipant updates a participant's media state (mute, deafen, speaking).
func (h *VoiceChannelHandler) UpdateParticipant(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		ChannelID  uuid.UUID `json:"channel_id"`
		IsMuted    *bool     `json:"is_muted,omitempty"`
		IsDeafened *bool     `json:"is_deafened,omitempty"`
		IsSpeaking *bool     `json:"is_speaking,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ChannelID == uuid.Nil {
		http.Error(w, "Channel ID is required", http.StatusBadRequest)
		return
	}

	var participant models.VoiceChannelParticipant
	if err := h.db.Where("channel_id = ? AND user_id = ?", req.ChannelID, userID).First(&participant).Error; err != nil {
		http.Error(w, "You are not in this voice channel", http.StatusNotFound)
		return
	}

	updates := map[string]interface{}{}
	if req.IsMuted != nil {
		updates["is_muted"] = *req.IsMuted
	}
	if req.IsDeafened != nil {
		updates["is_deafened"] = *req.IsDeafened
	}
	if req.IsSpeaking != nil {
		updates["is_speaking"] = *req.IsSpeaking
	}

	if len(updates) == 0 {
		http.Error(w, "No updates provided", http.StatusBadRequest)
		return
	}

	if err := h.db.Model(&participant).Updates(updates).Error; err != nil {
		log.Printf("Error updating participant: %v", err)
		http.Error(w, "Failed to update participant", http.StatusInternalServerError)
		return
	}

	h.db.Where("channel_id = ? AND user_id = ?", req.ChannelID, userID).First(&participant)

	resp := VoiceParticipantResponse{
		ID:         participant.ID,
		UserID:     participant.UserID,
		IsMuted:    participant.IsMuted,
		IsDeafened: participant.IsDeafened,
		IsSpeaking: participant.IsSpeaking,
		JoinedAt:   participant.JoinedAt.Format(time.RFC3339),
	}

	sendVoiceChannelJSON(w, http.StatusOK, resp)
}

// GetParticipants returns all participants in a voice channel.
func (h *VoiceChannelHandler) GetParticipants(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	channelIDStr := r.URL.Query().Get("channel_id")
	if channelIDStr == "" {
		http.Error(w, "Channel ID is required", http.StatusBadRequest)
		return
	}

	channelID, err := uuid.Parse(channelIDStr)
	if err != nil {
		http.Error(w, "Invalid channel ID", http.StatusBadRequest)
		return
	}

	var channel models.VoiceChannel
	if err := h.db.Where("id = ?", channelID).First(&channel).Error; err != nil {
		http.Error(w, "Voice channel not found", http.StatusNotFound)
		return
	}

	var userRoom models.UserRoom
	if err := h.db.Where("user_id = ? AND room_id = ?", userID, channel.RoomID).First(&userRoom).Error; err != nil {
		http.Error(w, "You are not a member of this room", http.StatusForbidden)
		return
	}

	var participants []models.VoiceChannelParticipant
	h.db.Preload("User").Where("channel_id = ?", channelID).Find(&participants)

	responses := make([]VoiceParticipantResponse, 0, len(participants))
	for _, p := range participants {
		responses = append(responses, VoiceParticipantResponse{
			ID:         p.ID,
			UserID:     p.UserID,
			Username:   p.User.Username,
			IsMuted:    p.IsMuted,
			IsDeafened: p.IsDeafened,
			IsSpeaking: p.IsSpeaking,
			JoinedAt:   p.JoinedAt.Format(time.RFC3339),
		})
	}

	sendVoiceChannelJSON(w, http.StatusOK, responses)
}
