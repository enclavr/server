package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/enclavr/server/internal/config"
	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/internal/websocket"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
	ws "github.com/gorilla/websocket"
)

type VoiceHandler struct {
	db     *database.Database
	hub    *websocket.Hub
	config *config.Config
}

func NewVoiceHandler(db *database.Database, hub *websocket.Hub, cfg *config.Config) *VoiceHandler {
	return &VoiceHandler{
		db:     db,
		hub:    hub,
		config: cfg,
	}
}

func (h *VoiceHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	if userID == uuid.Nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
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
	result := h.db.Where("user_id = ? AND room_id = ?", userID, roomID).First(&userRoom)
	if result.Error != nil {
		http.Error(w, "Not in room", http.StatusForbidden)
		return
	}

	upgrader := ws.Upgrader{
		ReadBufferSize:    1024,
		WriteBufferSize:   1024,
		EnableCompression: true,
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return false
			}

			allowedOrigins := h.config.Server.AllowedOrigins
			if len(allowedOrigins) == 0 {
				log.Printf("WARNING: No ALLOWED_ORIGINS configured. Rejecting WebSocket connection from origin: %s", origin)
				return false
			}

			for _, allowed := range allowedOrigins {
				if allowed == "*" || strings.EqualFold(allowed, origin) {
					return true
				}
			}
			return false
		},
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	client := h.hub.RegisterClient(userID, roomID, conn)

	go client.WritePump()
	go client.ReadPump()
}

type ICEConfig struct {
	ICEServers []ICEServer `json:"ice_servers"`
}

type ICEServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

func (h *VoiceHandler) GetICEConfig(w http.ResponseWriter, r *http.Request) {
	iceServers := []ICEServer{
		{URLs: []string{h.config.Voice.STUNServer}},
	}

	if h.config.Voice.TURNServer != "" {
		turnServer := ICEServer{
			URLs:       []string{h.config.Voice.TURNServer},
			Username:   h.config.Voice.TURNUser,
			Credential: h.config.Voice.TURNPass,
		}
		iceServers = append(iceServers, turnServer)
	}

	config := ICEConfig{
		ICEServers: iceServers,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(config); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}
