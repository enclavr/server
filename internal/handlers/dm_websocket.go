package handlers

import (
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

type DMWebSocketHandler struct {
	db     *database.Database
	dmHub  *websocket.DMHub
	config *config.Config
}

func NewDMWebSocketHandler(db *database.Database, dmHub *websocket.DMHub, cfg *config.Config) *DMWebSocketHandler {
	return &DMWebSocketHandler{
		db:     db,
		dmHub:  dmHub,
		config: cfg,
	}
}

func (h *DMWebSocketHandler) HandleDMWebSocket(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	peerIDStr := r.URL.Query().Get("peer_id")

	if peerIDStr == "" {
		http.Error(w, "Peer ID is required", http.StatusBadRequest)
		return
	}

	peerID, err := uuid.Parse(peerIDStr)
	if err != nil {
		http.Error(w, "Invalid peer ID", http.StatusBadRequest)
		return
	}

	var peer models.User
	if err := h.db.First(&peer, "id = ?", peerID).Error; err != nil {
		http.Error(w, "Peer user not found", http.StatusNotFound)
		return
	}

	var blockCount int64
	if err := h.db.Model(&models.Block{}).Where("(blocker_id = ? AND blocked_id = ?) OR (blocker_id = ? AND blocked_id = ?)",
		userID, peerID, peerID, userID).Count(&blockCount).Error; err == nil && blockCount > 0 {
		http.Error(w, "Cannot connect to blocked user", http.StatusForbidden)
		return
	}

	convID := websocket.GenerateConversationID(userID, peerID)

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
				return true
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
		log.Printf("DM WebSocket upgrade error: %v", err)
		return
	}

	client := &websocket.DMClient{}
	client.SetUserID(userID)
	client.SetConversationID(convID)
	client.SetSend(make(chan []byte, 256))

	h.dmHub.RegisterClient(client)

	go func() {
		defer func() {
			h.dmHub.UnregisterClient(client)
			conn.Close()
		}()
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				break
			}
			h.dmHub.HandleMessage(client, message)
		}
	}()

	go func() {
		defer conn.Close()
		for message := range client.GetSend() {
			if err := conn.WriteMessage(ws.TextMessage, message); err != nil {
				break
			}
		}
	}()
}
