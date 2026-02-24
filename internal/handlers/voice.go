package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/models"
	"github.com/enclavr/server/internal/websocket"
	"github.com/enclavr/server/pkg/middleware"
	"github.com/google/uuid"
	ws "github.com/gorilla/websocket"
)

type VoiceHandler struct {
	db  *database.Database
	hub *websocket.Hub
}

func NewVoiceHandler(db *database.Database, hub *websocket.Hub) *VoiceHandler {
	return &VoiceHandler{
		db:  db,
		hub: hub,
	}
}

func (h *VoiceHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
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
	result := h.db.Where("user_id = ? AND room_id = ?", userID, roomID).First(&userRoom)
	if result.Error != nil {
		http.Error(w, "Not in room", http.StatusForbidden)
		return
	}

	upgrader := ws.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
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
	config := ICEConfig{
		ICEServers: []ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}
