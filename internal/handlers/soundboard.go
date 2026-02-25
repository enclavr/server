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

type SoundboardHandler struct {
	db  *database.Database
	hub *websocket.Hub
}

func NewSoundboardHandler(db *database.Database, hub *websocket.Hub) *SoundboardHandler {
	return &SoundboardHandler{db: db, hub: hub}
}

type CreateSoundRequest struct {
	Name     string  `json:"name"`
	AudioURL string  `json:"audio_url"`
	Hotkey   string  `json:"hotkey"`
	Volume   float64 `json:"volume"`
}

type SoundResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	AudioURL  string    `json:"audio_url"`
	Hotkey    string    `json:"hotkey"`
	Volume    float64   `json:"volume"`
	CreatedBy uuid.UUID `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

func (h *SoundboardHandler) CreateSound(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req CreateSoundRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.AudioURL == "" {
		http.Error(w, "Name and audio URL are required", http.StatusBadRequest)
		return
	}

	if len(req.Name) > 50 {
		http.Error(w, "Sound name too long (max 50 characters)", http.StatusBadRequest)
		return
	}

	volume := 1.0
	if req.Volume > 0 && req.Volume <= 2.0 {
		volume = req.Volume
	}

	sound := &models.SoundboardSound{
		Name:      req.Name,
		AudioURL:  req.AudioURL,
		Hotkey:    req.Hotkey,
		Volume:    volume,
		CreatedBy: userID,
	}

	if err := h.db.Create(sound).Error; err != nil {
		log.Printf("Error creating sound: %v", err)
		http.Error(w, "Failed to create sound", http.StatusInternalServerError)
		return
	}

	response := SoundResponse{
		ID:        sound.ID,
		Name:      sound.Name,
		AudioURL:  sound.AudioURL,
		Hotkey:    sound.Hotkey,
		Volume:    sound.Volume,
		CreatedBy: sound.CreatedBy,
		CreatedAt: sound.CreatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *SoundboardHandler) GetSounds(w http.ResponseWriter, r *http.Request) {
	var sounds []models.SoundboardSound
	if err := h.db.Order("created_at DESC").Find(&sounds).Error; err != nil {
		log.Printf("Error fetching sounds: %v", err)
		http.Error(w, "Failed to fetch sounds", http.StatusInternalServerError)
		return
	}

	var response []SoundResponse
	for _, sound := range sounds {
		response = append(response, SoundResponse{
			ID:        sound.ID,
			Name:      sound.Name,
			AudioURL:  sound.AudioURL,
			Hotkey:    sound.Hotkey,
			Volume:    sound.Volume,
			CreatedBy: sound.CreatedBy,
			CreatedAt: sound.CreatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *SoundboardHandler) PlaySound(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		SoundID uuid.UUID `json:"sound_id"`
		RoomID  uuid.UUID `json:"room_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.SoundID == uuid.Nil {
		http.Error(w, "sound_id is required", http.StatusBadRequest)
		return
	}

	var sound models.SoundboardSound
	if err := h.db.First(&sound, "id = ?", req.SoundID).Error; err != nil {
		http.Error(w, "Sound not found", http.StatusNotFound)
		return
	}

	wsMsg := &websocket.Message{
		Type:      "soundboard-play",
		RoomID:    req.RoomID,
		UserID:    userID,
		Timestamp: time.Now(),
	}

	payload := map[string]interface{}{
		"sound_id":  sound.ID,
		"name":      sound.Name,
		"audio_url": sound.AudioURL,
		"volume":    sound.Volume,
		"user_id":   userID,
	}

	wsPayload, _ := json.Marshal(payload)
	wsMsg.Payload = wsPayload
	h.hub.BroadcastToRoom(req.RoomID, wsMsg, uuid.Nil)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "playing"}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (h *SoundboardHandler) DeleteSound(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	soundIDStr := r.URL.Query().Get("sound_id")
	if soundIDStr == "" {
		http.Error(w, "sound_id is required", http.StatusBadRequest)
		return
	}

	soundID, err := uuid.Parse(soundIDStr)
	if err != nil {
		http.Error(w, "Invalid sound_id", http.StatusBadRequest)
		return
	}

	var sound models.SoundboardSound
	if err := h.db.First(&sound, "id = ?", soundID).Error; err != nil {
		http.Error(w, "Sound not found", http.StatusNotFound)
		return
	}

	if sound.CreatedBy != userID {
		http.Error(w, "You can only delete your own sounds", http.StatusForbidden)
		return
	}

	if err := h.db.Delete(&sound).Error; err != nil {
		log.Printf("Error deleting sound: %v", err)
		http.Error(w, "Failed to delete sound", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "deleted"}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}
