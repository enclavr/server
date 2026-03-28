package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
)

// VoiceSessionHandler handles voice session history and statistics.
type VoiceSessionHandler struct {
	db *database.Database
}

// NewVoiceSessionHandler creates a new VoiceSessionHandler instance.
func NewVoiceSessionHandler(db *database.Database) *VoiceSessionHandler {
	return &VoiceSessionHandler{db: db}
}

// VoiceSessionResponse represents a voice session in API responses.
type VoiceSessionResponse struct {
	ID        uuid.UUID `json:"id"`
	RoomID    uuid.UUID `json:"room_id"`
	RoomName  string    `json:"room_name"`
	UserID    uuid.UUID `json:"user_id"`
	Username  string    `json:"username"`
	StartedAt string    `json:"started_at"`
	EndedAt   *string   `json:"ended_at,omitempty"`
	Duration  int       `json:"duration_seconds"`
	IsActive  bool      `json:"is_active"`
}

// VoiceSessionStats represents aggregated voice session statistics.
type VoiceSessionStats struct {
	TotalSessions  int     `json:"total_sessions"`
	ActiveSessions int     `json:"active_sessions"`
	TotalMinutes   int     `json:"total_minutes"`
	AvgDuration    float64 `json:"avg_duration_seconds"`
}

// GetUserVoiceSessions returns voice session history for the authenticated user.
func (h *VoiceSessionHandler) GetUserVoiceSessions(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")
	activeOnly := r.URL.Query().Get("active") == "true"

	limit := 50
	offset := 0
	if limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	if offsetStr != "" {
		if parsed, err := strconv.Atoi(offsetStr); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	query := h.db.Preload("Room").Preload("User").
		Where("user_id = ?", userID).
		Order("started_at DESC").
		Limit(limit).Offset(offset)

	if activeOnly {
		query = query.Where("ended_at IS NULL")
	}

	var sessions []models.VoiceSession
	query.Find(&sessions)

	responses := make([]VoiceSessionResponse, 0, len(sessions))
	for _, s := range sessions {
		responses = append(responses, voiceSessionToResponse(&s))
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(responses); err != nil {
		log.Printf("Error encoding voice sessions response: %v", err)
	}
}

// GetRoomVoiceSessions returns voice session history for a specific room.
func (h *VoiceSessionHandler) GetRoomVoiceSessions(w http.ResponseWriter, r *http.Request) {
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

	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	var sessions []models.VoiceSession
	h.db.Preload("Room").Preload("User").
		Where("room_id = ?", roomID).
		Order("started_at DESC").
		Limit(limit).
		Find(&sessions)

	responses := make([]VoiceSessionResponse, 0, len(sessions))
	for _, s := range sessions {
		responses = append(responses, voiceSessionToResponse(&s))
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(responses); err != nil {
		log.Printf("Error encoding voice sessions response: %v", err)
	}
}

// GetVoiceSessionStats returns aggregated voice session statistics for the authenticated user.
func (h *VoiceSessionHandler) GetVoiceSessionStats(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	roomIDStr := r.URL.Query().Get("room_id")

	query := h.db.Model(&models.VoiceSession{}).Where("user_id = ?", userID)
	if roomIDStr != "" {
		roomID, err := uuid.Parse(roomIDStr)
		if err != nil {
			http.Error(w, "Invalid room ID", http.StatusBadRequest)
			return
		}
		query = query.Where("room_id = ?", roomID)
	}

	var totalSessions int64
	query.Count(&totalSessions)

	var activeSessions int64
	query.Where("ended_at IS NULL").Count(&activeSessions)

	var totalMinutes float64
	h.db.Model(&models.VoiceSession{}).
		Where("user_id = ? AND ended_at IS NOT NULL", userID).
		Select("COALESCE(EXTRACT(EPOCH FROM SUM(ended_at - started_at)) / 60, 0)").
		Scan(&totalMinutes)

	var avgDuration float64
	h.db.Model(&models.VoiceSession{}).
		Where("user_id = ? AND ended_at IS NOT NULL", userID).
		Select("COALESCE(AVG(EXTRACT(EPOCH FROM ended_at - started_at)), 0)").
		Scan(&avgDuration)

	stats := VoiceSessionStats{
		TotalSessions:  int(totalSessions),
		ActiveSessions: int(activeSessions),
		TotalMinutes:   int(totalMinutes),
		AvgDuration:    avgDuration,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		log.Printf("Error encoding voice stats response: %v", err)
	}
}

// EndVoiceSession marks a voice session as ended.
func (h *VoiceSessionHandler) EndVoiceSession(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	sessionIDStr := r.URL.Query().Get("id")
	if sessionIDStr == "" {
		http.Error(w, "Session ID is required", http.StatusBadRequest)
		return
	}

	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		http.Error(w, "Invalid session ID", http.StatusBadRequest)
		return
	}

	var session models.VoiceSession
	if err := h.db.Where("id = ? AND user_id = ?", sessionID, userID).First(&session).Error; err != nil {
		http.Error(w, "Voice session not found", http.StatusNotFound)
		return
	}

	if session.EndedAt != nil {
		http.Error(w, "Voice session already ended", http.StatusBadRequest)
		return
	}

	now := time.Now()
	if err := h.db.Model(&session).Update("ended_at", now).Error; err != nil {
		log.Printf("Error ending voice session: %v", err)
		http.Error(w, "Failed to end voice session", http.StatusInternalServerError)
		return
	}

	session.EndedAt = &now
	resp := voiceSessionToResponse(&session)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

// CreateVoiceSession creates a new voice session record.
func (h *VoiceSessionHandler) CreateVoiceSession(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req struct {
		RoomID uuid.UUID `json:"room_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
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

	session := models.VoiceSession{
		RoomID: req.RoomID,
		UserID: userID,
	}

	if err := h.db.Create(&session).Error; err != nil {
		log.Printf("Error creating voice session: %v", err)
		http.Error(w, "Failed to create voice session", http.StatusInternalServerError)
		return
	}

	resp := voiceSessionToResponse(&session)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func voiceSessionToResponse(s *models.VoiceSession) VoiceSessionResponse {
	resp := VoiceSessionResponse{
		ID:        s.ID,
		RoomID:    s.RoomID,
		UserID:    s.UserID,
		StartedAt: s.StartedAt.Format(time.RFC3339),
		IsActive:  s.EndedAt == nil,
	}

	if s.Room.Name != "" {
		resp.RoomName = s.Room.Name
	}
	if s.User.Username != "" {
		resp.Username = s.User.Username
	}

	if s.EndedAt != nil {
		endStr := s.EndedAt.Format(time.RFC3339)
		resp.EndedAt = &endStr
		resp.Duration = int(s.EndedAt.Sub(s.StartedAt).Seconds())
	} else {
		resp.Duration = int(time.Since(s.StartedAt).Seconds())
	}

	return resp
}
