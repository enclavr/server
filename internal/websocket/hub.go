package websocket

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/enclavr/server/internal/metrics"
	"github.com/enclavr/server/pkg/validator"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Hub struct {
	rooms           map[uuid.UUID]*room
	broadcast       chan *Message
	register        chan *Client
	unregister      chan *Client
	mutex           sync.RWMutex
	userConnections map[uuid.UUID]*Client
	roomMutexes     map[uuid.UUID]*sync.RWMutex

	shutdown         chan struct{}
	activeClients    atomic.Int64
	totalMessages    atomic.Int64
	startedAt        time.Time
	batchQueue       chan *batchItem
	batchTicker      *time.Ticker
	typingUsers      map[uuid.UUID]*TypingState
	typingMutex      sync.RWMutex
	typingTimeout    time.Duration
	typingThrottle   map[uuid.UUID]time.Time
	typingThrottleMu sync.Mutex

	pubSub      *PubSubService
	enableRedis bool

	notificationSettings map[string]*RoomNotificationSettings
	notificationMutex    sync.RWMutex

	userActivities  map[uuid.UUID]*RoomActivity
	activityMutex   sync.RWMutex
	idleCheckTicker *time.Ticker

	roomStats      map[uuid.UUID]*RoomStats
	roomStatsMutex sync.RWMutex

	reconnectStates map[uuid.UUID]*ReconnectState
	reconnectMutex  sync.RWMutex
	reconnectTicker *time.Ticker

	blockedUsers map[uuid.UUID]map[uuid.UUID]bool
	blockMutex   sync.RWMutex

	readReceipts     map[uuid.UUID]map[uuid.UUID]time.Time
	readReceiptMutex sync.RWMutex

	pendingMessages      map[uuid.UUID][]PendingMessage
	pendingMessagesMutex sync.RWMutex
	deliveryStatus       map[string]*DeliveryStatus
	deliveryMutex        sync.RWMutex
}

type room struct {
	clients    map[*Client]bool
	mutex      sync.RWMutex
	lastAccess int64
}

type batchItem struct {
	message       *Message
	excludeUserID uuid.UUID
	roomID        uuid.UUID
	result        chan []byte
}

type Client struct {
	hub           *Hub
	conn          *websocket.Conn
	send          chan []byte
	userID        uuid.UUID
	roomID        uuid.UUID
	lastSeen      atomic.Int64
	rateLimit     *RateLimiter
	connectionID  uuid.UUID
	state         atomic.Int32
	remoteAddr    string
	connectedAt   time.Time
	errorCount    atomic.Int32
	lastError     atomic.Value
	lastErrorTime atomic.Int64
	lastPing      atomic.Int64
	latency       atomic.Int64
	packetsIn     atomic.Int64
	packetsOut    atomic.Int64
	bytesIn       atomic.Int64
	bytesOut      atomic.Int64
}

func NewHub() *Hub {
	hub := &Hub{
		rooms:                make(map[uuid.UUID]*room),
		broadcast:            make(chan *Message, 256),
		register:             make(chan *Client),
		unregister:           make(chan *Client),
		userConnections:      make(map[uuid.UUID]*Client),
		roomMutexes:          make(map[uuid.UUID]*sync.RWMutex),
		shutdown:             make(chan struct{}),
		startedAt:            time.Now(),
		batchQueue:           make(chan *batchItem, 512),
		batchTicker:          time.NewTicker(50 * time.Millisecond),
		enableRedis:          false,
		typingUsers:          make(map[uuid.UUID]*TypingState),
		typingTimeout:        5 * time.Second,
		typingThrottle:       make(map[uuid.UUID]time.Time),
		notificationSettings: make(map[string]*RoomNotificationSettings),
		userActivities:       make(map[uuid.UUID]*RoomActivity),
		roomStats:            make(map[uuid.UUID]*RoomStats),
		idleCheckTicker:      time.NewTicker(30 * time.Second),
		reconnectStates:      make(map[uuid.UUID]*ReconnectState),
		reconnectTicker:      time.NewTicker(10 * time.Second),
		blockedUsers:         make(map[uuid.UUID]map[uuid.UUID]bool),
		readReceipts:         make(map[uuid.UUID]map[uuid.UUID]time.Time),
		pendingMessages:      make(map[uuid.UUID][]PendingMessage),
		deliveryStatus:       make(map[string]*DeliveryStatus),
	}
	go hub.processBatch()
	go hub.cleanupTypingUsers()
	go hub.cleanupIdleUsers()
	go hub.updateRoomStats()
	go hub.cleanupReconnectStates()
	go hub.cleanupPendingMessages()
	go hub.cleanupDeliveryStatus()
	return hub
}

func NewHubWithRedis(redisHost, redisPassword string, redisDB int) (*Hub, error) {
	hub := NewHub()

	hub.pubSub = NewPubSubService(redisHost, redisPassword, redisDB)
	if err := hub.pubSub.Connect(); err != nil {
		close(hub.shutdown)
		return nil, err
	}

	if err := hub.pubSub.Subscribe("broadcast"); err != nil {
		wsLogger.Error("Failed to subscribe to broadcast channel",
			"error", err)
	}

	hub.pubSub.RegisterHandler("user-message", hub.handleRedisUserMessage)
	hub.pubSub.RegisterHandler("room-message", hub.handleRedisRoomMessage)

	hub.enableRedis = true
	wsLogger.Info("WebSocket hub initialized with Redis pub/sub support")

	return hub, nil
}

func (h *Hub) Run() {
	for {
		select {
		case <-h.shutdown:
			h.gracefulShutdown()
			return
		case client := <-h.register:
			h.mutex.Lock()
			if h.rooms[client.roomID] == nil {
				h.rooms[client.roomID] = &room{
					clients: make(map[*Client]bool),
					mutex:   sync.RWMutex{},
				}
				h.roomMutexes[client.roomID] = &sync.RWMutex{}
				metrics.WebSocketRoomsActive.Inc()
			}
			h.rooms[client.roomID].clients[client] = true
			h.rooms[client.roomID].lastAccess = time.Now().Unix()
			h.userConnections[client.userID] = client
			h.activeClients.Add(1)
			client.lastSeen.Store(time.Now().Unix())
			h.mutex.Unlock()

		case client := <-h.unregister:
			h.mutex.Lock()
			if r, ok := h.rooms[client.roomID]; ok {
				r.mutex.Lock()
				if _, ok := r.clients[client]; ok {
					delete(r.clients, client)
					close(client.send)
					if len(r.clients) == 0 {
						delete(h.rooms, client.roomID)
						delete(h.roomMutexes, client.roomID)
						metrics.WebSocketRoomsActive.Dec()
					}
				}
				r.mutex.Unlock()
			}
			delete(h.userConnections, client.userID)
			h.activeClients.Add(-1)
			h.mutex.Unlock()
			metrics.WebSocketConnections.Dec()
			metrics.ActiveUsers.Dec()

		case message := <-h.broadcast:
			h.mutex.RLock()
			r, ok := h.rooms[message.RoomID]
			h.mutex.RUnlock()
			if ok {
				r.mutex.Lock()
				for client := range r.clients {
					select {
					case client.send <- message.encode():
					default:
						close(client.send)
						delete(r.clients, client)
						h.activeClients.Add(-1)
					}
				}
				clientCount := len(r.clients)
				r.mutex.Unlock()
				h.totalMessages.Add(1)
				metrics.MessagesSent.Add(float64(clientCount))
				metrics.WebSocketMessagesTotal.WithLabelValues(message.Type, "sent").Add(float64(clientCount))
			}
		}
	}
}

func (h *Hub) getRoomMutex(roomID uuid.UUID) *sync.RWMutex {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	return h.roomMutexes[roomID]
}

func (h *Hub) processBatch() {
	batch := make([]*batchItem, 0, 50)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-h.shutdown:
			return
		case item := <-h.batchQueue:
			batch = append(batch, item)
			if len(batch) >= 50 {
				h.flushBatch(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				h.flushBatch(batch)
				batch = batch[:0]
			}
		}
	}
}

func (h *Hub) flushBatch(batch []*batchItem) {
	if len(batch) == 0 {
		return
	}

	roomGroups := make(map[uuid.UUID][]*batchItem)
	for _, item := range batch {
		roomGroups[item.roomID] = append(roomGroups[item.roomID], item)
	}

	for roomID, items := range roomGroups {
		h.mutex.RLock()
		r, ok := h.rooms[roomID]
		h.mutex.RUnlock()
		if ok {
			r.mutex.Lock()
			for _, item := range items {
				encoded := item.message.encode()
				for client := range r.clients {
					if item.excludeUserID == (uuid.UUID{}) || client.userID != item.excludeUserID {
						select {
						case client.send <- encoded:
						default:
							close(client.send)
							delete(r.clients, client)
							h.activeClients.Add(-1)
						}
					}
				}
			}
			r.mutex.Unlock()
		}

		for _, item := range items {
			item.result <- []byte{}
		}
	}
}

func (h *Hub) BroadcastToRoomBatch(roomID uuid.UUID, msg *Message, excludeUserID uuid.UUID) {
	item := &batchItem{
		message:       msg,
		excludeUserID: excludeUserID,
		roomID:        roomID,
		result:        make(chan []byte, 1),
	}
	h.batchQueue <- item
	<-item.result
	close(item.result)
}

func (h *Hub) gracefulShutdown() {
	wsLogger.Info("WebSocket hub shutting down...",
		"active_clients", h.activeClients.Load(),
		"rooms", len(h.rooms))
	h.batchTicker.Stop()

	h.mutex.Lock()
	defer h.mutex.Unlock()

	for roomID, r := range h.rooms {
		r.mutex.Lock()
		clientCount := len(r.clients)
		for client := range r.clients {
			close(client.send)
		}
		r.clients = make(map[*Client]bool)
		r.mutex.Unlock()
		h.activeClients.Add(-int64(clientCount))
		wsLogger.Debug("Room disconnected during shutdown",
			"room_id", roomID,
			"clients", clientCount)
	}

	h.rooms = make(map[uuid.UUID]*room)
	h.userConnections = make(map[uuid.UUID]*Client)
	h.roomMutexes = make(map[uuid.UUID]*sync.RWMutex)

	wsLogger.Info("WebSocket hub shutdown complete",
		"total_messages", h.totalMessages.Load(),
		"uptime", time.Since(h.startedAt))
}

func (h *Hub) Shutdown() {
	wsLogger.Info("Shutting down WebSocket hub...")
	close(h.shutdown)
	log.Println("WebSocket hub shutdown complete")
}

func (h *Hub) GetMetrics() HubMetrics {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	return HubMetrics{
		ActiveClients: h.activeClients.Load(),
		TotalMessages: h.totalMessages.Load(),
		Uptime:        time.Since(h.startedAt),
		RoomCount:     len(h.rooms),
		RedisEnabled:  h.enableRedis,
	}
}

func (h *Hub) GetClientCount() int64 {
	return h.activeClients.Load()
}

func (h *Hub) GetRoomCount() int {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	return len(h.rooms)
}

func (h *Hub) RegisterClient(userID, roomID uuid.UUID, conn *websocket.Conn) *Client {
	h.mutex.Lock()
	existingClient, exists := h.userConnections[userID]
	if exists {
		wsLogger.Warn("User already connected, closing old connection",
			"user_id", userID,
			"old_connection_id", existingClient.connectionID)
		existingClient.SetState(StateDisconnecting)
		close(existingClient.send)
		delete(h.userConnections, userID)
		h.activeClients.Add(-1)
		if r, ok := h.rooms[existingClient.roomID]; ok {
			r.mutex.Lock()
			delete(r.clients, existingClient)
			r.mutex.Unlock()
		}
	}
	h.mutex.Unlock()

	client := &Client{
		hub:          h,
		conn:         conn,
		send:         make(chan []byte, 256),
		userID:       userID,
		roomID:       roomID,
		rateLimit:    NewRateLimiter(30, 10),
		connectionID: uuid.New(),
		connectedAt:  time.Now(),
	}

	h.register <- client
	metrics.WebSocketConnections.Inc()
	metrics.ActiveUsers.Inc()
	remoteAddr := "nil"
	if conn != nil {
		remoteAddr = conn.RemoteAddr().String()
	}
	wsLogger.Info("Client registered",
		"connection_id", client.connectionID,
		"user_id", userID,
		"room_id", roomID,
		"remote_addr", remoteAddr)
	return client
}

func (h *Hub) UnregisterClient(client *Client) {
	h.unregister <- client
}

func (h *Hub) GetRoomClients(roomID uuid.UUID) []*Client {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	clients := make([]*Client, 0)
	if r, ok := h.rooms[roomID]; ok {
		r.mutex.RLock()
		for client := range r.clients {
			clients = append(clients, client)
		}
		r.mutex.RUnlock()
	}
	return clients
}

func (h *Hub) GetUserClient(userID uuid.UUID) *Client {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	return h.userConnections[userID]
}

func (h *Hub) GetOnlineUsers() []uuid.UUID {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	users := make([]uuid.UUID, 0, len(h.userConnections))
	for userID := range h.userConnections {
		users = append(users, userID)
	}
	return users
}

func (h *Hub) sendToClient(targetUserID uuid.UUID, msg *Message) {
	h.mutex.RLock()
	client, ok := h.userConnections[targetUserID]
	h.mutex.RUnlock()

	if ok {
		select {
		case client.send <- msg.encode():
		default:
		}
	}
}

func (h *Hub) broadcastToRoom(roomID uuid.UUID, msg *Message, excludeUserID uuid.UUID) {
	if roomID == uuid.Nil {
		wsLogger.Warn("broadcastToRoom called with nil roomID",
			"exclude_user_id", excludeUserID,
			"message_type", msg.Type)
		return
	}

	if msg == nil {
		wsLogger.Error("broadcastToRoom called with nil message",
			"room_id", roomID,
			"exclude_user_id", excludeUserID)
		return
	}

	h.mutex.RLock()
	r, ok := h.rooms[roomID]
	h.mutex.RUnlock()
	if !ok {
		wsLogger.Debug("Broadcast to non-existent room",
			"room_id", roomID,
			"message_type", msg.Type)
		return
	}

	r.mutex.Lock()
	clientCount := len(r.clients)
	if clientCount == 0 {
		wsLogger.Debug("Broadcast to empty room",
			"room_id", roomID,
			"message_type", msg.Type)
		r.mutex.Unlock()
		return
	}

	deliveredCount := 0
	for client := range r.clients {
		if excludeUserID != (uuid.UUID{}) && client.userID == excludeUserID {
			continue
		}
		select {
		case client.send <- msg.encode():
			deliveredCount++
		default:
			wsLogger.Warn("Failed to deliver message, channel full",
				"connection_id", client.connectionID,
				"user_id", client.userID,
				"room_id", roomID)
			close(client.send)
			delete(r.clients, client)
			h.activeClients.Add(-1)
		}
	}
	r.mutex.Unlock()

	h.totalMessages.Add(1)
	metrics.MessagesSent.Add(float64(deliveredCount))
	metrics.WebSocketMessagesTotal.WithLabelValues(msg.Type, "sent").Add(float64(deliveredCount))

	wsLogger.Debug("Broadcast to room",
		"room_id", roomID,
		"message_type", msg.Type,
		"client_count", clientCount,
		"delivered", deliveredCount,
		"excluded", excludeUserID)
}

func (h *Hub) BroadcastToRoom(roomID uuid.UUID, msg *Message, excludeUserID uuid.UUID) {
	h.broadcastToRoom(roomID, msg, excludeUserID)
}

func (h *Hub) updateUserActivity(userID, roomID uuid.UUID) {
	h.activityMutex.Lock()
	defer h.activityMutex.Unlock()

	activity, exists := h.userActivities[userID]
	if !exists {
		activity = &RoomActivity{
			UserID:   userID,
			RoomID:   roomID,
			Status:   "active",
			JoinedAt: time.Now(),
		}
		h.userActivities[userID] = activity
	}

	now := time.Now()
	if activity.Status == "idle" {
		activeMsg := &Message{
			Type:      "user-active",
			RoomID:    roomID,
			UserID:    userID,
			Timestamp: now,
		}
		h.broadcastToRoom(roomID, activeMsg, userID)
		wsLogger.Info("User became active in room",
			"user_id", userID,
			"room_id", roomID)
	}

	activity.Status = "active"
	activity.LastActivity = now
}

func (h *Hub) setUserIdle(userID uuid.UUID) {
	h.activityMutex.Lock()
	defer h.activityMutex.Unlock()

	if activity, exists := h.userActivities[userID]; exists {
		activity.Status = "idle"
		activity.LastActivity = time.Now()
	}
}

func (h *Hub) GetUserActivity(userID uuid.UUID) (*RoomActivity, bool) {
	h.activityMutex.RLock()
	defer h.activityMutex.RUnlock()
	activity, exists := h.userActivities[userID]
	return activity, exists
}

func (c *Client) ReadPump() {
	defer func() {
		c.SetState(StateDisconnecting)
		c.hub.clearTypingUser(c.userID)

		select {
		case c.hub.unregister <- c:
		case <-c.hub.shutdown:
		}

		disconnectDuration := time.Since(c.connectedAt)
		closeCode := websocket.CloseNormalClosure
		closeReason := "normal"

		if c.conn != nil {
			if closeErr := c.conn.Close(); closeErr != nil {
				if ce, ok := closeErr.(*websocket.CloseError); ok {
					closeCode = ce.Code
					closeReason = string(websocket.FormatCloseMessage(ce.Code, ce.Text))
				}
				wsLogger.Error("Error closing connection",
					"connection_id", c.connectionID,
					"user_id", c.userID,
					"remote_addr", c.remoteAddr,
					"close_code", closeCode,
					"close_reason", closeReason,
					"error", closeErr.Error())
			}
		} else {
			wsLogger.Warn("Connection was nil during cleanup",
				"connection_id", c.connectionID,
				"user_id", c.userID)
		}

		c.SetState(StateDisconnected)

		errorCount := c.GetErrorCount()
		lastErr := c.GetLastError()

		fields := []interface{}{
			"connection_id", c.connectionID,
			"user_id", c.userID,
			"room_id", c.roomID,
			"remote_addr", c.remoteAddr,
			"error_count", errorCount,
			"packets_in", c.packetsIn.Load(),
			"bytes_in", c.bytesIn.Load(),
			"packets_out", c.packetsOut.Load(),
			"bytes_out", c.bytesOut.Load(),
			"connected_duration", disconnectDuration.String(),
			"close_code", closeCode,
			"close_reason", closeReason,
		}

		if lastErr != nil {
			wsLogger.Info("Connection closed with errors",
				append(fields, "last_error", lastErr.Error())...)
		} else {
			wsLogger.Info("Connection closed cleanly", fields...)
		}
	}()

	c.SetState(StateConnecting)
	if c.conn != nil {
		c.remoteAddr = c.conn.RemoteAddr().String()
	} else {
		wsLogger.Error("Cannot set remote address - connection is nil",
			"connection_id", c.connectionID,
			"user_id", c.userID)
		return
	}

	c.conn.SetReadLimit(MaxMessageSize)
	if err := c.conn.SetReadDeadline(time.Now().Add(PongTimeout)); err != nil {
		wsLogger.Error("Error setting read deadline",
			"connection_id", c.connectionID,
			"error", err)
		c.RecordError(err)
		return
	}
	c.conn.SetPongHandler(func(string) error {
		if err := c.conn.SetReadDeadline(time.Now().Add(PongTimeout)); err != nil {
			wsLogger.Error("Error setting read deadline in pong handler",
				"connection_id", c.connectionID,
				"error", err)
			c.RecordError(err)
			return err
		}
		return nil
	})

	c.SetState(StateConnected)
	wsLogger.Info("Client connected",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"room_id", c.roomID,
		"remote_addr", c.remoteAddr)

	for {
		messageType, message, err := c.conn.ReadMessage()
		if err != nil {
			c.packetsIn.Add(1)
			if len(message) > 0 {
				c.bytesIn.Add(int64(len(message)))
			}

			errCategory, errDetail := getErrorCategory(err)
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseTLSHandshake) {
				wsLogger.Error("WebSocket unexpected close error",
					"connection_id", c.connectionID,
					"user_id", c.userID,
					"room_id", c.roomID,
					"error_category", errCategory,
					"close_code", getCloseCode(err),
					"error", err.Error())
				c.RecordError(err)
			} else if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				wsLogger.Info("WebSocket closed normally",
					"connection_id", c.connectionID,
					"user_id", c.userID,
					"room_id", c.roomID,
					"error_category", errCategory,
					"close_code", getCloseCode(err))
			} else if isTemporaryNetworkError(err) {
				wsLogger.Warn("WebSocket temporary network error (will retry)",
					"connection_id", c.connectionID,
					"user_id", c.userID,
					"error_category", errCategory,
					"error", err.Error())
				c.RecordError(err)
			} else {
				wsLogger.Error("WebSocket read error",
					"connection_id", c.connectionID,
					"user_id", c.userID,
					"error_category", errCategory,
					"error", err.Error())
				c.RecordError(err)
			}

			c.sendDisconnectNotification(errCategory, errDetail)
			break
		}

		c.packetsIn.Add(1)
		if len(message) > 0 {
			c.bytesIn.Add(int64(len(message)))
		}

		if messageType == websocket.PongMessage {
			continue
		}

		if messageType == websocket.CloseMessage {
			wsLogger.Info("Received close message",
				"connection_id", c.connectionID,
				"user_id", c.userID)
			break
		}

		if messageType != websocket.TextMessage {
			wsLogger.Debug("Ignoring non-text message",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"message_type", messageType)
			continue
		}

		if len(message) > MaxMessageSize {
			wsLogger.Warn("Message too large",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"size", len(message),
				"max_size", MaxMessageSize)
			c.RecordError(fmt.Errorf("message too large: %d bytes", len(message)))
			c.sendErrorMessage("Message too large")
			continue
		}

		if !c.rateLimit.Allow() {
			wsLogger.Warn("Rate limit exceeded",
				"connection_id", c.connectionID,
				"user_id", c.userID)
			c.RecordError(fmt.Errorf("rate limit exceeded"))
			c.sendErrorMessage("Rate limit exceeded")
			continue
		}

		if c.GetState() != StateConnected {
			wsLogger.Debug("Discarding message in non-connected state",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"state", c.GetState())
			continue
		}

		c.lastSeen.Store(time.Now().Unix())
		c.hub.updateUserActivity(c.userID, c.roomID)

		var msg Message
		if err := msg.decode(message); err != nil {
			wsLogger.Warn("Error decoding message",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"error", err,
				"payload_size", len(message))
			c.RecordError(err)
			c.sendErrorMessage("Invalid message format")
			continue
		}

		if msg.Type == "" {
			wsLogger.Warn("Message missing type",
				"connection_id", c.connectionID,
				"user_id", c.userID)
			continue
		}

		if c.GetErrorCount() >= MaxErrorThreshold {
			wsLogger.Error("Client exceeded max error threshold, disconnecting",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"error_count", c.GetErrorCount(),
				"max_threshold", MaxErrorThreshold)
			c.sendErrorMessage("Too many errors, connection closed")
			break
		}

		msg.UserID = c.userID
		msg.RoomID = c.roomID
		msg.Timestamp = time.Now()

		c.handleMessage(&msg)
	}
}

func (c *Client) sendErrorMessage(errMsg string) {
	errPayload := map[string]string{"error": errMsg}
	payload, _ := json.Marshal(errPayload)
	msg := &Message{
		Type:      "error",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   payload,
		Timestamp: time.Now(),
	}
	c.hub.sendToClient(c.userID, msg)
}

func (c *Client) sendDisconnectNotification(errType, errDetail string) {
	disconnectPayload := map[string]interface{}{
		"type":        errType,
		"detail":      errDetail,
		"reconnect":   true,
		"retry_after": 5000,
	}
	payload, _ := json.Marshal(disconnectPayload)
	msg := &Message{
		Type:      "disconnect-notification",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   payload,
		Timestamp: time.Now(),
	}
	select {
	case c.send <- msg.encode():
	default:
	}
}

func (c *Client) WritePump() {
	ticker := time.NewTicker(PingInterval)
	defer func() {
		ticker.Stop()
		if c.conn != nil {
			if closeErr := c.conn.Close(); closeErr != nil {
				wsLogger.Error("Error closing connection in WritePump",
					"connection_id", c.connectionID,
					"user_id", c.userID,
					"error", closeErr)
			}
		} else {
			wsLogger.Warn("Connection was nil in WritePump cleanup",
				"connection_id", c.connectionID,
				"user_id", c.userID)
		}
	}()

	writeTimeout := 10 * time.Second

	for {
		select {
		case message, ok := <-c.send:
			if c.conn == nil {
				wsLogger.Warn("Cannot write message - connection is nil",
					"connection_id", c.connectionID,
					"user_id", c.userID)
				return
			}

			if err := c.conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
				wsLogger.Error("Error setting write deadline",
					"connection_id", c.connectionID,
					"user_id", c.userID,
					"error", err)
				c.RecordError(err)
				return
			}
			if !ok {
				wsLogger.Info("Send channel closed, sending close message",
					"connection_id", c.connectionID,
					"user_id", c.userID,
					"room_id", c.roomID)
				if err := c.conn.WriteMessage(websocket.CloseMessage, []byte{}); err != nil {
					wsLogger.Warn("Error sending close message",
						"connection_id", c.connectionID,
						"user_id", c.userID,
						"error", err)
					c.RecordError(err)
				}
				return
			}

			c.packetsOut.Add(1)
			c.bytesOut.Add(int64(len(message)))

			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					wsLogger.Warn("Unexpected close while writing",
						"connection_id", c.connectionID,
						"user_id", c.userID,
						"room_id", c.roomID,
						"error", err)
				} else {
					wsLogger.Warn("Write error",
						"connection_id", c.connectionID,
						"user_id", c.userID,
						"room_id", c.roomID,
						"error", err)
				}
				c.RecordError(err)
				return
			}
			c.lastSeen.Store(time.Now().Unix())

		case <-ticker.C:
			if c.conn == nil {
				wsLogger.Warn("Cannot send ping - connection is nil",
					"connection_id", c.connectionID,
					"user_id", c.userID)
				return
			}

			if err := c.conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
				wsLogger.Error("Error setting write deadline in ping",
					"connection_id", c.connectionID,
					"user_id", c.userID,
					"error", err)
				c.RecordError(err)
				return
			}
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				wsLogger.Warn("Error writing ping",
					"connection_id", c.connectionID,
					"user_id", c.userID,
					"room_id", c.roomID,
					"error", err)
				c.RecordError(err)
				return
			}
			wsLogger.Debug("Ping sent",
				"connection_id", c.connectionID,
				"user_id", c.userID)
		}
	}
}

func (c *Client) GetLastSeen() time.Time {
	return time.Unix(c.lastSeen.Load(), 0)
}

func (c *Client) GetState() ConnectionState {
	return ConnectionState(c.state.Load())
}

func (c *Client) SetState(state ConnectionState) {
	c.state.Store(int32(state))
}

func (c *Client) GetConnectionID() uuid.UUID {
	return c.connectionID
}

func (c *Client) RecordError(err error) {
	c.errorCount.Add(1)
	c.lastError.Store(err)
	c.lastErrorTime.Store(time.Now().Unix())
}

func (c *Client) GetErrorCount() int32 {
	return c.errorCount.Load()
}

func (c *Client) GetLastError() error {
	if err, ok := c.lastError.Load().(error); ok && err != nil {
		return err
	}
	return nil
}

func (c *Client) GetLastErrorTime() time.Time {
	ts := c.lastErrorTime.Load()
	if ts > 0 {
		return time.Unix(ts, 0)
	}
	return time.Time{}
}

func (c *Client) GetRemoteAddr() string {
	return c.remoteAddr
}

func (h *Hub) cleanupTypingUsers() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-h.shutdown:
			return
		case <-ticker.C:
			h.typingMutex.Lock()
			now := time.Now()
			for userID, state := range h.typingUsers {
				if now.Sub(state.StartedAt) > h.typingTimeout {
					notifyMsg := &Message{
						Type:      "user-stopped-typing",
						RoomID:    state.RoomID,
						UserID:    state.UserID,
						Timestamp: now,
					}
					if state.Context != "" {
						payload, _ := json.Marshal(TypingPayload{Context: state.Context})
						notifyMsg.Payload = payload
					}
					h.broadcastToRoom(state.RoomID, notifyMsg, state.UserID)
					delete(h.typingUsers, userID)
				}
			}
			h.typingMutex.Unlock()
		}
	}
}

func (h *Hub) ValidateAndSanitizeMessage(msgType string, payload json.RawMessage) (json.RawMessage, error) {
	if msgType == "" {
		return nil, errors.New("message type is required")
	}

	validTypes := map[string]bool{
		"typing": true, "stop-typing": true, "typing-start": true, "typing_stop": true,
		"message-updated": true, "thread-reply": true, "thread-message": true,
		"message-deleted": true, "thread-message-updated": true, "thread-message-deleted": true,
		"message-read": true, "typing-indicator": true, "typing-status": true,
		"enhanced-typing": true, "user-focus": true, "media-state-sync": true,
		"user-activity": true, "presence-event": true, "broadcast-status-update": true,
		"message-ack": true, "message-delivery-status": true,
		"typing-pause": true, "user-focus-in": true, "user-focus-out": true,
		"reaction-add": true, "reaction-remove": true, "message-pinned": true,
		"message-unpinned": true, "user-viewing-channel": true, "user-left-channel": true,
		"room-config-update": true,
	}

	if !validTypes[msgType] {
		return payload, nil
	}

	if len(payload) == 0 {
		return payload, nil
	}

	var contentMap map[string]interface{}
	if err := json.Unmarshal(payload, &contentMap); err != nil {
		return nil, errors.New("invalid payload format")
	}

	if content, ok := contentMap["content"].(string); ok && content != "" {
		if err := validator.ValidateMessageContent(content); err != nil {
			return nil, err
		}
		contentMap["content"] = validator.SanitizeMessageContent(content)
	}

	if content, ok := contentMap["message"].(string); ok && content != "" {
		if err := validator.ValidateMessageContent(content); err != nil {
			return nil, err
		}
		contentMap["message"] = validator.SanitizeMessageContent(content)
	}

	sanitized, err := json.Marshal(contentMap)
	if err != nil {
		return nil, errors.New("failed to serialize sanitized payload")
	}

	return sanitized, nil
}

func (h *Hub) cleanupIdleUsers() {
	defer h.idleCheckTicker.Stop()

	for {
		select {
		case <-h.shutdown:
			return
		case <-h.idleCheckTicker.C:
			h.activityMutex.Lock()
			now := time.Now()
			for userID, activity := range h.userActivities {
				if activity.Status == "idle" {
					continue
				}
				if now.Sub(activity.LastActivity) > IdleTimeout {
					oldStatus := activity.Status
					activity.Status = "idle"
					activity.LastActivity = now

					idleMsg := &Message{
						Type:      "user-idle",
						RoomID:    activity.RoomID,
						UserID:    userID,
						Timestamp: now,
					}
					h.broadcastToRoom(activity.RoomID, idleMsg, userID)
					wsLogger.Info("User transitioned to idle",
						"user_id", userID,
						"old_status", oldStatus,
						"room_id", activity.RoomID)
				}
			}
			h.activityMutex.Unlock()
		}
	}
}

func (h *Hub) updateRoomStats() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-h.shutdown:
			return
		case <-ticker.C:
			h.mutex.RLock()
			roomIDs := make([]uuid.UUID, 0, len(h.rooms))
			for roomID := range h.rooms {
				roomIDs = append(roomIDs, roomID)
			}
			h.mutex.RUnlock()

			for _, roomID := range roomIDs {
				h.updateRoomStat(roomID)
			}
		}
	}
}

func (h *Hub) cleanupReconnectStates() {
	defer h.reconnectTicker.Stop()

	for {
		select {
		case <-h.shutdown:
			return
		case <-h.reconnectTicker.C:
			h.reconnectMutex.Lock()
			now := time.Now()
			for userID, state := range h.reconnectStates {
				if state.Completed && now.Sub(state.Timestamp) > 5*time.Minute {
					delete(h.reconnectStates, userID)
				}
				if !state.Completed && now.Sub(state.Timestamp) > 2*time.Minute {
					delete(h.reconnectStates, userID)
				}
			}
			if len(h.reconnectStates) > MaxReconnectStates {
				h.reconnectStates = make(map[uuid.UUID]*ReconnectState)
			}
			h.reconnectMutex.Unlock()
		}
	}
}

func (h *Hub) cleanupPendingMessages() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-h.shutdown:
			return
		case <-ticker.C:
			h.pendingMessagesMutex.Lock()
			now := time.Now()
			for userID, messages := range h.pendingMessages {
				var validMessages []PendingMessage
				for _, msg := range messages {
					if now.Sub(msg.CreatedAt) < 5*time.Minute {
						validMessages = append(validMessages, msg)
					}
				}
				if len(validMessages) == 0 {
					delete(h.pendingMessages, userID)
				} else if len(validMessages) < len(messages) {
					if len(validMessages) > MaxPendingMessages {
						h.pendingMessages[userID] = validMessages[len(validMessages)-MaxPendingMessages:]
					} else {
						h.pendingMessages[userID] = validMessages
					}
				}
			}
			h.pendingMessagesMutex.Unlock()
		}
	}
}

func (h *Hub) cleanupDeliveryStatus() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-h.shutdown:
			return
		case <-ticker.C:
			h.deliveryMutex.Lock()
			now := time.Now()
			for key, status := range h.deliveryStatus {
				age := now.Sub(status.SentAt)
				if age > 24*time.Hour {
					delete(h.deliveryStatus, key)
				} else if status.Status == "delivered" && now.Sub(status.DeliveredAt) > 1*time.Hour {
					delete(h.deliveryStatus, key)
				} else if status.Status == "failed" && status.RetryCount > 5 {
					delete(h.deliveryStatus, key)
				}
			}
			h.deliveryMutex.Unlock()
		}
	}
}

func (h *Hub) AddReconnectState(userID, oldConnID, roomID uuid.UUID) *ReconnectState {
	h.reconnectMutex.Lock()
	defer h.reconnectMutex.Unlock()

	state := &ReconnectState{
		UserID:        userID,
		OldConnection: oldConnID,
		NewConnection: uuid.Nil,
		RoomID:        roomID,
		Timestamp:     time.Now(),
		Attempts:      1,
		Completed:     false,
	}
	h.reconnectStates[userID] = state
	return state
}

func (h *Hub) CompleteReconnect(userID, newConnID uuid.UUID) {
	h.reconnectMutex.Lock()
	defer h.reconnectMutex.Unlock()

	if state, exists := h.reconnectStates[userID]; exists {
		state.NewConnection = newConnID
		state.Completed = true
		state.Timestamp = time.Now()
	}
}

func (h *Hub) GetReconnectState(userID uuid.UUID) (*ReconnectState, bool) {
	h.reconnectMutex.RLock()
	defer h.reconnectMutex.RUnlock()

	state, exists := h.reconnectStates[userID]
	return state, exists
}

func (h *Hub) BlockUser(blockerID, blockedID uuid.UUID) {
	h.blockMutex.Lock()
	defer h.blockMutex.Unlock()

	if h.blockedUsers[blockerID] == nil {
		h.blockedUsers[blockerID] = make(map[uuid.UUID]bool)
	}
	h.blockedUsers[blockerID][blockedID] = true

	wsLogger.Info("User blocked another user",
		"blocker_id", blockerID,
		"blocked_id", blockedID)
}

func (h *Hub) UnblockUser(blockerID, blockedID uuid.UUID) {
	h.blockMutex.Lock()
	defer h.blockMutex.Unlock()

	if h.blockedUsers[blockerID] != nil {
		delete(h.blockedUsers[blockerID], blockedID)
	}

	wsLogger.Info("User unblocked another user",
		"blocker_id", blockerID,
		"blocked_id", blockedID)
}

func (h *Hub) IsUserBlocked(blockerID, userID uuid.UUID) bool {
	h.blockMutex.RLock()
	defer h.blockMutex.RUnlock()

	if h.blockedUsers[blockerID] != nil {
		return h.blockedUsers[blockerID][userID]
	}
	return false
}

func (h *Hub) GetBlockedUsers(blockerID uuid.UUID) []uuid.UUID {
	h.blockMutex.RLock()
	defer h.blockMutex.RUnlock()

	var blocked []uuid.UUID
	if h.blockedUsers[blockerID] != nil {
		for userID := range h.blockedUsers[blockerID] {
			blocked = append(blocked, userID)
		}
	}
	return blocked
}

func (h *Hub) MarkMessageRead(userID, roomID, messageID uuid.UUID) {
	h.readReceiptMutex.Lock()
	defer h.readReceiptMutex.Unlock()

	if h.readReceipts[userID] == nil {
		h.readReceipts[userID] = make(map[uuid.UUID]time.Time)
	}
	h.readReceipts[userID][messageID] = time.Now()

	if len(h.readReceipts) > 10000 {
		for uid := range h.readReceipts {
			delete(h.readReceipts, uid)
			break
		}
	}
}

func (h *Hub) GetLastReadMessage(userID, roomID uuid.UUID) (uuid.UUID, time.Time, bool) {
	h.readReceiptMutex.RLock()
	defer h.readReceiptMutex.RUnlock()

	if h.readReceipts[userID] == nil {
		return uuid.Nil, time.Time{}, false
	}

	var lastMsgID uuid.UUID
	var lastReadTime time.Time

	for msgID, readAt := range h.readReceipts[userID] {
		if readAt.After(lastReadTime) {
			lastMsgID = msgID
			lastReadTime = readAt
		}
	}

	if lastMsgID != uuid.Nil {
		return lastMsgID, lastReadTime, true
	}

	return uuid.Nil, time.Time{}, false
}

func (h *Hub) GetReadReceipts(messageID uuid.UUID) []uuid.UUID {
	h.readReceiptMutex.RLock()
	defer h.readReceiptMutex.RUnlock()

	var users []uuid.UUID
	for userID, messages := range h.readReceipts {
		if _, read := messages[messageID]; read {
			users = append(users, userID)
		}
	}
	return users
}

func (h *Hub) BroadcastMessageRead(userID, roomID, messageID uuid.UUID) {
	h.MarkMessageRead(userID, roomID, messageID)

	readReceiptPayload := map[string]interface{}{
		"message_id": messageID.String(),
		"user_id":    userID.String(),
		"read_at":    time.Now().UnixMilli(),
	}
	payloadBytes, _ := json.Marshal(readReceiptPayload)

	notifyMsg := &Message{
		Type:      "message-read",
		RoomID:    roomID,
		UserID:    userID,
		Payload:   payloadBytes,
		Timestamp: time.Now(),
	}
	h.broadcastToRoom(roomID, notifyMsg, userID)
}

func (h *Hub) updateRoomStat(roomID uuid.UUID) {
	h.mutex.RLock()
	r, ok := h.rooms[roomID]
	if !ok {
		h.mutex.RUnlock()
		return
	}

	r.mutex.RLock()
	clientCount := len(r.clients)
	r.mutex.RUnlock()

	typingUsers := h.GetTypingUsers(roomID)

	h.roomStatsMutex.Lock()
	stats, exists := h.roomStats[roomID]
	if !exists {
		stats = &RoomStats{RoomID: roomID}
		h.roomStats[roomID] = stats
	}

	stats.TotalUsers = clientCount
	stats.TypingUsers = len(typingUsers)
	stats.LastActivity = time.Now()

	var activeUsers, idleUsers, mutedUsers, deafenedUsers, speakingUsers, screenSharing int

	h.activityMutex.RLock()
	r.mutex.RLock()
	for client := range r.clients {
		activity, actExists := h.userActivities[client.userID]
		if actExists && activity.Status == "active" {
			activeUsers++
		} else if actExists && activity.Status == "idle" {
			idleUsers++
		} else {
			activeUsers++
		}
	}
	r.mutex.RUnlock()
	h.activityMutex.RUnlock()

	stats.ActiveUsers = activeUsers
	stats.IdleUsers = idleUsers
	stats.MutedUsers = mutedUsers
	stats.DeafenedUsers = deafenedUsers
	stats.SpeakingUsers = speakingUsers
	stats.ScreenSharing = screenSharing

	h.roomStatsMutex.Unlock()
	h.mutex.RUnlock()
}

func (h *Hub) GetRoomStats(roomID uuid.UUID) *RoomStats {
	h.roomStatsMutex.RLock()
	defer h.roomStatsMutex.RUnlock()
	stats, exists := h.roomStats[roomID]
	if !exists {
		return &RoomStats{RoomID: roomID}
	}
	return stats
}

func (c *Client) handleMessage(msg *Message) {
	defer func() {
		if r := recover(); r != nil {
			wsLogger.Error("Panic recovered in handleMessage",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"room_id", c.roomID,
				"message_type", msg.Type,
				"panic", r)
			c.RecordError(fmt.Errorf("panic in message handler: %v", r))
			c.sendErrorMessage("Internal error processing message")
		}
	}()

	if msg == nil {
		wsLogger.Error("Received nil message",
			"connection_id", c.connectionID,
			"user_id", c.userID)
		return
	}

	if msg.Type == "" {
		wsLogger.Warn("Received message with empty type",
			"connection_id", c.connectionID,
			"user_id", c.userID,
			"room_id", c.roomID)
		c.sendErrorMessage("Message type is required")
		return
	}

	if c.GetState() != StateConnected && c.GetState() != StateConnecting {
		wsLogger.Warn("Received message in invalid state",
			"connection_id", c.connectionID,
			"user_id", c.userID,
			"state", c.GetState(),
			"message_type", msg.Type)
		return
	}

	if msg.RoomID == uuid.Nil && msg.Type != "ping" && msg.Type != "heartbeat" && msg.Type != "client-ready" {
		wsLogger.Warn("Message missing room_id for non-ping message",
			"connection_id", c.connectionID,
			"user_id", c.userID,
			"message_type", msg.Type)
	}

	switch msg.Type {
	case "voice-offer", "voice-answer", "voice-ice-candidate":
		if msg.TargetUserID != uuid.Nil && msg.UserID != msg.TargetUserID {
			c.hub.sendToClient(msg.TargetUserID, msg)
		} else {
			wsLogger.Debug("Voice message ignored - invalid target",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"target_user_id", msg.TargetUserID)
		}
	case "voice-mute", "voice-unmute":
		c.hub.broadcast <- msg
	case "user-joined":
		c.handleUserJoined(msg)
	case "user-left":
		c.handleUserLeft(msg)
	case "user-away":
		c.handleUserAway(msg)
	case "user-back":
		c.handleUserBack(msg)
	case "typing":
		c.handleTyping(msg)
	case "stop-typing":
		c.handleStopTyping(msg)
	case "typing-start":
		c.handleTypingStart(msg)
	case "typing_stop":
		c.handleTypingStop(msg)
	case "get-online-users":
		c.hub.sendOnlineUsersList(c.roomID, c.userID)
	case "get-room-notifications":
		c.hub.sendRoomNotificationSettings(c.userID, c.roomID)
	case "set-room-notifications":
		c.hub.setRoomNotificationSettings(c.userID, c.roomID, msg.Payload)
	case "get-room-state":
		c.hub.sendRoomState(c)
	case "typing-users":
		c.hub.sendTypingUsersList(c)
	case "ping":
		c.handlePing(msg)
	case "heartbeat":
		c.handleHeartbeat(msg)
	case "user-speaking":
		c.handleUserSpeaking(msg)
	case "user-stopped-speaking":
		c.handleUserStoppedSpeaking(msg)
	case "user-screen-share-start":
		c.handleScreenShareStart(msg)
	case "user-screen-share-stop":
		c.handleScreenShareStop(msg)
	case "user-muted":
		c.handleUserMuted(msg, true)
	case "user-unmuted":
		c.handleUserMuted(msg, false)
	case "user-deafened":
		c.handleUserDeafened(msg, true)
	case "user-undeafened":
		c.handleUserDeafened(msg, false)
	case "get-connection-health":
		c.sendConnectionHealth()
	case "get-room-users":
		c.hub.sendRoomUsersDetailed(c)
	case "get-room-stats":
		c.hub.sendRoomStats(c)
	case "user-activity":
		c.handleUserActivity(msg)
	case "subscribe-room-events":
		c.handleSubscribeRoomEvents(msg)
	case "unsubscribe-room-events":
		c.handleUnsubscribeRoomEvents(msg)
	case "get-connection-quality":
		c.handleGetConnectionQuality()
	case "ping-with-timestamp":
		c.handlePingWithTimestamp(msg)
	case "set-presence":
		c.handleSetPresence(msg)
	case "get-presence":
		c.handleGetPresence(msg)
	case "typing-indicator":
		c.handleTypingIndicator(msg)
	case "room-event":
		c.handleRoomEvent(msg)
	case "mark-notification-read":
		c.handleMarkNotificationRead(msg)
	case "get-notifications":
		c.handleGetNotifications(msg)
	case "message-read":
		c.handleMessageRead(msg)
	case "message-updated":
		c.handleMessageUpdated(msg)
	case "thread-reply":
		c.handleThreadReply(msg)
	case "thread-message":
		c.handleThreadMessage(msg)
	case "user-blocked":
		c.handleUserBlocked(msg)
	case "user-unblocked":
		c.handleUserUnblocked(msg)
	case "client-reconnect":
		c.handleClientReconnect(msg)
	case "message-deleted":
		c.handleMessageDeleted(msg)
	case "thread-message-updated":
		c.handleThreadMessageUpdated(msg)
	case "thread-message-deleted":
		c.handleThreadMessageDeleted(msg)
	case "typing-with-context":
		c.handleTypingWithContext(msg)
	case "presence-bulk":
		c.handlePresenceBulk(msg)
	case "room-notification-event":
		c.handleRoomNotificationEvent(msg)
	case "get-room-presence":
		c.handleGetRoomPresence(msg)
	case "user-kicked":
		c.handleUserKicked(msg)
	case "user-banned":
		c.handleUserBanned(msg)
	case "role-changed":
		c.handleRoleChanged(msg)
	case "typing-bulk":
		c.handleTypingBulk(msg)
	case "typing-broadcast":
		c.handleTypingBroadcast(msg)
	case "room-state-subscribe":
		c.handleRoomStateSubscribe(msg)
	case "room-state-unsubscribe":
		c.handleRoomStateUnsubscribe(msg)
	case "user-presence-subscribe":
		c.handleUserPresenceSubscribe(msg)
	case "ping-pong":
		c.handlePingPong(msg)
	case "client-ready":
		c.handleClientReady(msg)
	case "typing-status":
		c.handleTypingStatus(msg)
	case "user-presence-bulk-subscribe":
		c.handlePresenceBulkSubscribe(msg)
	case "notification-preferences":
		c.handleNotificationPreferences(msg)
	case "get-notification-preferences":
		c.handleGetNotificationPreferences(msg)
	case "enhanced-typing":
		c.handleEnhancedTyping(msg)
	case "presence-event":
		c.handlePresenceEvent(msg)
	case "subscribe-room-notifications":
		c.handleSubscribeRoomNotifications(msg)
	case "unsubscribe-room-notifications":
		c.handleUnsubscribeRoomNotifications(msg)
	case "get-online-users-detailed":
		c.handleGetOnlineUsersDetailed(msg)
	case "broadcast-status-update":
		c.handleBroadcastStatusUpdate(msg)
	case "message-ack":
		c.handleMessageAck(msg)
	case "message-delivery-status":
		c.handleMessageDeliveryStatus(msg)
	case "user-focus":
		c.handleUserFocus(msg)
	case "user-focus-query":
		c.handleUserFocusQuery(msg)
	case "media-state-sync":
		c.handleMediaStateSync(msg)
	case "media-state-request":
		c.handleMediaStateRequest(msg)
	case "typing-pause":
		c.handleTypingPause(msg)
	case "user-focus-in":
		c.handleUserFocusWindow(msg)
	case "user-focus-out":
		c.handleUserFocusWindow(msg)
	case "reaction-add":
		c.handleReactionAdd(msg)
	case "reaction-remove":
		c.handleReactionRemove(msg)
	case "message-pinned":
		c.handleMessagePin(msg)
	case "message-unpinned":
		c.handleMessagePin(msg)
	case "user-viewing-channel":
		c.handleUserViewingChannel(msg)
	case "user-left-channel":
		c.handleUserViewingChannel(msg)
	case "room-config-update":
		c.handleRoomConfigUpdate(msg)
	default:
		wsLogger.Warn("Unknown message type",
			"connection_id", c.connectionID,
			"message_type", msg.Type,
			"user_id", c.userID)
	}
}

func (h *Hub) setTypingUser(userID, roomID uuid.UUID, context string) {
	h.typingMutex.Lock()
	defer h.typingMutex.Unlock()
	h.typingUsers[userID] = &TypingState{
		UserID:    userID,
		RoomID:    roomID,
		StartedAt: time.Now(),
		Context:   context,
	}
}

func (h *Hub) clearTypingUser(userID uuid.UUID) {
	h.typingMutex.Lock()
	defer h.typingMutex.Unlock()
	delete(h.typingUsers, userID)
}

func (h *Hub) isTypingThrottled(userID uuid.UUID) bool {
	h.typingThrottleMu.Lock()
	defer h.typingThrottleMu.Unlock()

	if lastTime, exists := h.typingThrottle[userID]; exists {
		if time.Since(lastTime) < TypingThrottleDuration {
			return true
		}
	}
	h.typingThrottle[userID] = time.Now()
	return false
}

func (h *Hub) GetTypingUsers(roomID uuid.UUID) []uuid.UUID {
	h.typingMutex.RLock()
	defer h.typingMutex.RUnlock()

	var users []uuid.UUID
	for userID, state := range h.typingUsers {
		if state.RoomID == roomID {
			users = append(users, userID)
		}
	}
	return users
}

var userPresence = struct {
	sync.RWMutex
	states map[uuid.UUID]*PresenceState
}{states: make(map[uuid.UUID]*PresenceState)}

func (h *Hub) setUserPresence(userID, roomID uuid.UUID, status string) {
	userPresence.Lock()
	defer userPresence.Unlock()
	userPresence.states[userID] = &PresenceState{
		UserID:    userID,
		RoomID:    roomID,
		Status:    status,
		UpdatedAt: time.Now(),
	}
}

func (h *Hub) GetUserPresence(userID uuid.UUID) (string, bool) {
	userPresence.RLock()
	defer userPresence.RUnlock()
	if state, ok := userPresence.states[userID]; ok {
		return state.Status, true
	}
	return "", false
}

func (h *Hub) GetRoomPresence(roomID uuid.UUID) []*PresenceState {
	userPresence.RLock()
	defer userPresence.RUnlock()
	var states []*PresenceState
	for _, state := range userPresence.states {
		if state.RoomID == roomID {
			states = append(states, state)
		}
	}
	return states
}

func (h *Hub) sendOnlineUsersList(roomID, excludeUserID uuid.UUID) {
	clients := h.GetRoomClients(roomID)
	if len(clients) == 0 {
		return
	}

	onlineUsers := make([]OnlineUser, 0, len(clients))
	for _, client := range clients {
		if client.userID != excludeUserID {
			presenceStatus, _ := h.GetUserPresence(client.userID)
			activity, hasActivity := h.GetUserActivity(client.userID)

			onlineUser := OnlineUser{
				UserID:         client.userID,
				ConnectionID:   client.connectionID,
				State:          client.GetState().String(),
				ConnectedAt:    client.connectedAt,
				LastSeen:       client.GetLastSeen(),
				RemoteAddress:  client.remoteAddr,
				PresenceStatus: presenceStatus,
			}

			if hasActivity {
				onlineUser.ActiveChannelID = activity.RoomID.String()
			}

			onlineUsers = append(onlineUsers, onlineUser)
		}
	}

	payload, err := json.Marshal(onlineUsers)
	if err != nil {
		wsLogger.Error("Error marshaling online users",
			"room_id", roomID,
			"error", err)
		return
	}

	msg := &Message{
		Type:      "online-users-list",
		RoomID:    roomID,
		Payload:   payload,
		Timestamp: time.Now(),
	}
	h.broadcastToRoom(roomID, msg, excludeUserID)

	wsLogger.Debug("Sent online users list",
		"room_id", roomID,
		"count", len(onlineUsers))
}

func (h *Hub) sendRoomNotificationSettings(userID, roomID uuid.UUID) {
	key := fmt.Sprintf("%s:%s", userID, roomID)
	h.notificationMutex.RLock()
	settings, exists := h.notificationSettings[key]
	h.notificationMutex.RUnlock()

	if !exists {
		settings = &RoomNotificationSettings{
			Enabled:       true,
			Sound:         true,
			MentionOnly:   false,
			DesktopNotify: true,
		}
	}

	payload, err := json.Marshal(settings)
	if err != nil {
		wsLogger.Error("Error marshaling notification settings",
			"error", err)
		return
	}

	msg := &Message{
		Type:      "room-notifications",
		RoomID:    roomID,
		UserID:    userID,
		Payload:   payload,
		Timestamp: time.Now(),
	}
	h.sendToClient(userID, msg)
}

func (h *Hub) setRoomNotificationSettings(userID, roomID uuid.UUID, payload json.RawMessage) {
	key := fmt.Sprintf("%s:%s", userID, roomID)

	var settings RoomNotificationSettings
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &settings); err != nil {
			log.Printf("[Connection] Error parsing notification settings: %v", err)
			return
		}
	} else {
		settings = RoomNotificationSettings{
			Enabled:       true,
			Sound:         true,
			MentionOnly:   false,
			DesktopNotify: true,
		}
	}

	h.notificationMutex.Lock()
	h.notificationSettings[key] = &settings
	h.notificationMutex.Unlock()

	confirmMsg := &Message{
		Type:      "room-notifications-updated",
		RoomID:    roomID,
		UserID:    userID,
		Payload:   payload,
		Timestamp: time.Now(),
	}
	h.sendToClient(userID, confirmMsg)
	wsLogger.Info("User updated notification settings",
		"user_id", userID,
		"room_id", roomID,
		"enabled", settings.Enabled,
		"sound", settings.Sound)
}

func (h *Hub) sendRoomState(client *Client) {
	clients := h.GetRoomClients(client.roomID)
	typingUsers := h.GetTypingUsers(client.roomID)

	onlineUsers := make([]OnlineUser, 0, len(clients))
	for _, c := range clients {
		onlineUsers = append(onlineUsers, OnlineUser{
			UserID:        c.userID,
			ConnectionID:  c.connectionID,
			State:         c.GetState().String(),
			ConnectedAt:   c.connectedAt,
			LastSeen:      c.GetLastSeen(),
			RemoteAddress: c.remoteAddr,
		})
	}

	roomState := RoomState{
		RoomID:        client.roomID,
		OnlineUsers:   onlineUsers,
		TypingUsers:   typingUsers,
		ActiveClients: len(clients),
	}

	payload, err := json.Marshal(roomState)
	if err != nil {
		wsLogger.Error("Error marshaling room state",
			"connection_id", client.connectionID,
			"error", err)
		return
	}

	msg := &Message{
		Type:      "room-state",
		RoomID:    client.roomID,
		UserID:    client.userID,
		Payload:   payload,
		Timestamp: time.Now(),
	}
	h.sendToClient(client.userID, msg)
}

func (h *Hub) sendTypingUsersList(client *Client) {
	typingUsers := h.GetTypingUsers(client.roomID)
	payload, err := json.Marshal(typingUsers)
	if err != nil {
		wsLogger.Error("Error marshaling typing users",
			"connection_id", client.connectionID,
			"error", err)
		return
	}

	msg := &Message{
		Type:      "typing-users-list",
		RoomID:    client.roomID,
		UserID:    client.userID,
		Payload:   payload,
		Timestamp: time.Now(),
	}
	h.sendToClient(client.userID, msg)
}

func NewTypingDebouncer(delay time.Duration, onTimeout func(uuid.UUID)) *TypingDebouncer {
	return &TypingDebouncer{
		timers:    make(map[uuid.UUID]*time.Timer),
		delay:     delay,
		onTimeout: onTimeout,
	}
}

func (td *TypingDebouncer) Trigger(userID uuid.UUID) {
	td.mutex.Lock()
	defer td.mutex.Unlock()

	if timer, exists := td.timers[userID]; exists {
		timer.Stop()
	}

	td.timers[userID] = time.AfterFunc(td.delay, func() {
		td.mutex.Lock()
		delete(td.timers, userID)
		td.mutex.Unlock()
		if td.onTimeout != nil {
			td.onTimeout(userID)
		}
	})
}

func (td *TypingDebouncer) Stop(userID uuid.UUID) {
	td.mutex.Lock()
	defer td.mutex.Unlock()
	if timer, exists := td.timers[userID]; exists {
		timer.Stop()
		delete(td.timers, userID)
	}
}

func (td *TypingDebouncer) StopAll() {
	td.mutex.Lock()
	defer td.mutex.Unlock()
	for _, timer := range td.timers {
		timer.Stop()
	}
	td.timers = make(map[uuid.UUID]*time.Timer)
}

func (h *Hub) handleRedisUserMessage(msg *PubSubMessage) {
	h.sendToClient(msg.TargetUserID, &Message{
		Type:         msg.Type,
		UserID:       msg.UserID,
		TargetUserID: msg.TargetUserID,
		Payload:      msg.Payload,
		Timestamp:    msg.Timestamp,
	})
}

func (h *Hub) handleRedisRoomMessage(msg *PubSubMessage) {
	h.broadcastToRoom(msg.RoomID, &Message{
		Type:      msg.Type,
		RoomID:    msg.RoomID,
		UserID:    msg.UserID,
		Payload:   msg.Payload,
		Timestamp: msg.Timestamp,
	}, msg.UserID)
}

func (h *Hub) publishToRedis(msg *Message) {
	if h.pubSub == nil || !h.enableRedis {
		return
	}

	psMsg := &PubSubMessage{
		Type:      msg.Type,
		RoomID:    msg.RoomID,
		UserID:    msg.UserID,
		Payload:   msg.Payload,
		Timestamp: msg.Timestamp,
	}

	if msg.TargetUserID != (uuid.UUID{}) {
		_ = h.pubSub.PublishToUser(msg.TargetUserID, psMsg)
	}
	if msg.RoomID != (uuid.UUID{}) {
		_ = h.pubSub.PublishToRoom(msg.RoomID, psMsg)
	}
	_ = h.pubSub.PublishBroadcast(psMsg)
}

func (h *Hub) SubscribeToRoomRedis(roomID uuid.UUID) error {
	if h.pubSub == nil || !h.enableRedis {
		return nil
	}
	return h.pubSub.SubscribeToRoom(roomID)
}

func (h *Hub) SubscribeToUserRedis(userID uuid.UUID) error {
	if h.pubSub == nil || !h.enableRedis {
		return nil
	}
	return h.pubSub.SubscribeToUser(userID)
}

func (h *Hub) ShutdownRedis() error {
	if h.pubSub != nil {
		return h.pubSub.Disconnect()
	}
	return nil
}

func (h *Hub) IsRedisEnabled() bool {
	return h.enableRedis
}

func (c *Client) handlePing(msg *Message) {
	c.lastPing.Store(time.Now().UnixMilli())

	pongMsg := &Message{
		Type:      "pong",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Timestamp: time.Now(),
	}
	c.hub.sendToClient(c.userID, pongMsg)
}

func (c *Client) handleHeartbeat(msg *Message) {
	now := time.Now()
	c.lastPing.Store(now.UnixMilli())
	c.latency.Store(0)

	health := ConnectionHealth{
		ConnectionID:    c.connectionID,
		UserID:          c.userID,
		LatencyMs:       0,
		State:           c.GetState().String(),
		LastPing:        now,
		LastPong:        now,
		PacketsReceived: c.packetsIn.Load(),
		PacketsSent:     c.packetsOut.Load(),
		BytesReceived:   c.bytesIn.Load(),
		BytesSent:       c.bytesOut.Load(),
	}

	payload, _ := json.Marshal(health)
	responseMsg := &Message{
		Type:      "heartbeat-ack",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   payload,
		Timestamp: now,
	}
	c.hub.sendToClient(c.userID, responseMsg)
}

func (c *Client) handleUserSpeaking(msg *Message) {
	notifyMsg := &Message{
		Type:      "user-speaking",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   msg.Payload,
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)
	if c.hub.enableRedis {
		c.hub.publishToRedis(notifyMsg)
	}
}

func (c *Client) handleUserStoppedSpeaking(msg *Message) {
	notifyMsg := &Message{
		Type:      "user-stopped-speaking",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   msg.Payload,
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)
	if c.hub.enableRedis {
		c.hub.publishToRedis(notifyMsg)
	}
}

func (c *Client) handleScreenShareStart(msg *Message) {
	wsLogger.Info("User started screen share",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"room_id", c.roomID)

	notifyMsg := &Message{
		Type:      "user-screen-share-start",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   msg.Payload,
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)
	if c.hub.enableRedis {
		c.hub.publishToRedis(notifyMsg)
	}
}

func (c *Client) handleScreenShareStop(msg *Message) {
	wsLogger.Info("User stopped screen share",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"room_id", c.roomID)

	notifyMsg := &Message{
		Type:      "user-screen-share-stop",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   msg.Payload,
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)
	if c.hub.enableRedis {
		c.hub.publishToRedis(notifyMsg)
	}
}

func (c *Client) handleUserMuted(msg *Message, muted bool) {
	wsLogger.Info("User muted status changed",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"muted", muted)

	notifyMsg := &Message{
		Type:      "user-muted",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   msg.Payload,
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)
	if c.hub.enableRedis {
		c.hub.publishToRedis(notifyMsg)
	}
}

func (c *Client) handleUserDeafened(msg *Message, deafened bool) {
	wsLogger.Info("User deafened status changed",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"deafened", deafened)

	notifyMsg := &Message{
		Type:      "user-deafened",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   msg.Payload,
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)
	if c.hub.enableRedis {
		c.hub.publishToRedis(notifyMsg)
	}
}

func (c *Client) sendConnectionHealth() {
	health := ConnectionHealth{
		ConnectionID:    c.connectionID,
		UserID:          c.userID,
		LatencyMs:       c.latency.Load(),
		State:           c.GetState().String(),
		LastPing:        time.UnixMilli(c.lastPing.Load()),
		LastPong:        time.Now(),
		PacketsReceived: c.packetsIn.Load(),
		PacketsSent:     c.packetsOut.Load(),
		BytesReceived:   c.bytesIn.Load(),
		BytesSent:       c.bytesOut.Load(),
	}

	payload, err := json.Marshal(health)
	if err != nil {
		wsLogger.Error("Error marshaling health",
			"connection_id", c.connectionID,
			"error", err)
		return
	}

	msg := &Message{
		Type:      "connection-health",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   payload,
		Timestamp: time.Now(),
	}
	c.hub.sendToClient(c.userID, msg)
}

func (h *Hub) sendRoomUsersDetailed(client *Client) {
	clients := h.GetRoomClients(client.roomID)
	typingUsers := h.GetTypingUsers(client.roomID)

	typingUserSet := make(map[uuid.UUID]bool)
	for _, uid := range typingUsers {
		typingUserSet[uid] = true
	}

	onlineUsers := make([]OnlineUser, 0, len(clients))
	for _, c := range clients {
		user := OnlineUser{
			UserID:        c.userID,
			ConnectionID:  c.connectionID,
			State:         c.GetState().String(),
			ConnectedAt:   c.connectedAt,
			LastSeen:      c.GetLastSeen(),
			RemoteAddress: c.remoteAddr,
			Speaking:      false,
			Muted:         false,
			Deafened:      false,
			ScreenSharing: false,
		}
		onlineUsers = append(onlineUsers, user)
	}

	payload, err := json.Marshal(onlineUsers)
	if err != nil {
		wsLogger.Error("Error marshaling room users",
			"connection_id", client.connectionID,
			"error", err)
		return
	}

	msg := &Message{
		Type:      "room-users-detailed",
		RoomID:    client.roomID,
		UserID:    client.userID,
		Payload:   payload,
		Timestamp: time.Now(),
	}
	h.sendToClient(client.userID, msg)
}

func (h *Hub) sendRoomStats(client *Client) {
	stats := h.GetRoomStats(client.roomID)
	payload, err := json.Marshal(stats)
	if err != nil {
		wsLogger.Error("Error marshaling room stats",
			"connection_id", client.connectionID,
			"error", err)
		return
	}

	msg := &Message{
		Type:      "room-stats",
		RoomID:    client.roomID,
		UserID:    client.userID,
		Payload:   payload,
		Timestamp: time.Now(),
	}
	h.sendToClient(client.userID, msg)
	wsLogger.Debug("Sent room stats",
		"connection_id", client.connectionID,
		"user_id", client.userID,
		"total", stats.TotalUsers,
		"active", stats.ActiveUsers,
		"typing", stats.TypingUsers)
}

func (c *Client) handleUserActivity(msg *Message) {
	var payload UserActivityPayload
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing user activity payload",
				"connection_id", c.connectionID,
				"error", err)
			return
		}
	}

	switch payload.Status {
	case "active":
		c.hub.updateUserActivity(c.userID, c.roomID)
	case "idle":
		c.hub.setUserIdle(c.userID)
		idleMsg := &Message{
			Type:      "user-idle",
			RoomID:    c.roomID,
			UserID:    c.userID,
			Timestamp: time.Now(),
		}
		c.hub.broadcastToRoom(c.roomID, idleMsg, c.userID)
	case "away":
		c.hub.setUserPresence(c.userID, c.roomID, "away")
		awayMsg := &Message{
			Type:      "user-away",
			RoomID:    c.roomID,
			UserID:    c.userID,
			Timestamp: time.Now(),
		}
		c.hub.broadcastToRoom(c.roomID, awayMsg, c.userID)
	}

	wsLogger.Debug("User activity updated",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"status", payload.Status)
}

func (c *Client) handleTypingStart(msg *Message) {
	if c.hub.isTypingThrottled(c.userID) {
		wsLogger.Debug("Typing start event throttled",
			"connection_id", c.connectionID,
			"user_id", c.userID,
			"room_id", c.roomID)
		return
	}

	var payload TypingIndicator
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing typing indicator payload",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"error", err)
			c.sendErrorMessage("Invalid typing payload format")
			return
		}
	}

	contextStr := payload.Context
	if payload.ChannelID != "" {
		contextStr = "channel:" + payload.ChannelID
	} else if payload.ThreadID != "" {
		contextStr = "thread:" + payload.ThreadID
	} else if payload.ContextType != "" {
		contextStr = payload.ContextType
	}

	c.hub.setTypingUser(c.userID, c.roomID, contextStr)

	typingPayload := TypingIndicator{
		UserID:      c.userID,
		RoomID:      c.roomID,
		Context:     contextStr,
		ContextID:   payload.ContextID,
		ContextType: payload.ContextType,
		ChannelID:   payload.ChannelID,
		ThreadID:    payload.ThreadID,
		IsTyping:    true,
		Timestamp:   time.Now(),
	}

	payloadBytes, err := json.Marshal(typingPayload)
	if err != nil {
		wsLogger.Error("Error marshaling typing payload",
			"connection_id", c.connectionID,
			"error", err)
		return
	}
	notifyMsg := &Message{
		Type:      "user-typing-start",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   payloadBytes,
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)
	if c.hub.enableRedis {
		c.hub.publishToRedis(notifyMsg)
	}
	wsLogger.Debug("User started typing",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"room_id", c.roomID,
		"context", contextStr)
}

func (c *Client) handleTypingStop(msg *Message) {
	c.hub.clearTypingUser(c.userID)

	typingPayload := TypingIndicator{
		UserID:    c.userID,
		RoomID:    c.roomID,
		IsTyping:  false,
		Timestamp: time.Now(),
	}

	payloadBytes, err := json.Marshal(typingPayload)
	if err != nil {
		wsLogger.Error("Error marshaling typing stop payload",
			"connection_id", c.connectionID,
			"error", err)
		return
	}
	notifyMsg := &Message{
		Type:      "user-typing-stop",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   payloadBytes,
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)
	if c.hub.enableRedis {
		c.hub.publishToRedis(notifyMsg)
	}
	wsLogger.Debug("User stopped typing",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"room_id", c.roomID)
}

func (c *Client) handleSubscribeRoomEvents(msg *Message) {
	wsLogger.Info("User subscribed to room events",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"room_id", c.roomID)

	confirmMsg := &Message{
		Type:      "room-events-subscribed",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Timestamp: time.Now(),
	}
	c.hub.sendToClient(c.userID, confirmMsg)
}

func (c *Client) handleUnsubscribeRoomEvents(msg *Message) {
	wsLogger.Info("User unsubscribed from room events",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"room_id", c.roomID)

	confirmMsg := &Message{
		Type:      "room-events-unsubscribed",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Timestamp: time.Now(),
	}
	c.hub.sendToClient(c.userID, confirmMsg)
}

func (c *Client) handleGetConnectionQuality() {
	quality := c.calculateConnectionQuality()

	payload, err := json.Marshal(quality)
	if err != nil {
		wsLogger.Error("Error marshaling connection quality",
			"connection_id", c.connectionID,
			"error", err)
		return
	}

	msg := &Message{
		Type:      "connection-quality",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   payload,
		Timestamp: time.Now(),
	}
	c.hub.sendToClient(c.userID, msg)
	wsLogger.Debug("Sent connection quality",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"quality", quality.Quality,
		"latency_ms", quality.LatencyMs)
}

func (c *Client) calculateConnectionQuality() *ConnectionQuality {
	latency := c.latency.Load()

	var quality string
	switch {
	case latency < 50:
		quality = "excellent"
	case latency < 100:
		quality = "good"
	case latency < 200:
		quality = "fair"
	default:
		quality = "poor"
	}

	return &ConnectionQuality{
		ConnectionID: c.connectionID,
		UserID:       c.userID,
		Quality:      quality,
		LatencyMs:    latency,
	}
}

func (c *Client) handlePingWithTimestamp(msg *Message) {
	var clientTime int64
	if len(msg.Payload) > 0 {
		var payload struct {
			ClientTime int64 `json:"client_time"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err == nil {
			clientTime = payload.ClientTime
		}
	}

	serverTime := time.Now().UnixMilli()
	var roundTrip int64
	if clientTime > 0 {
		roundTrip = serverTime - clientTime
		c.latency.Store(roundTrip / 2)
	}

	pongPayload := struct {
		ServerTime int64 `json:"server_time"`
		LatencyMs  int64 `json:"latency_ms"`
	}{
		ServerTime: serverTime,
		LatencyMs:  roundTrip / 2,
	}

	payloadBytes, _ := json.Marshal(pongPayload)
	pongMsg := &Message{
		Type:      "pong-with-timestamp",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   payloadBytes,
		Timestamp: time.Now(),
	}
	c.hub.sendToClient(c.userID, pongMsg)
}

func (c *Client) handleUserJoined(msg *Message) {
	notifyMsg := &Message{
		Type:      "user-joined",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)
	if c.hub.enableRedis {
		c.hub.publishToRedis(notifyMsg)
	}
	c.hub.sendOnlineUsersList(c.roomID, c.userID)

	wsLogger.Info("User joined room",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"room_id", c.roomID)
}

func (c *Client) handleUserLeft(msg *Message) {
	c.hub.clearTypingUser(c.userID)
	notifyMsg := &Message{
		Type:      "user-left",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)
	if c.hub.enableRedis {
		c.hub.publishToRedis(notifyMsg)
	}
	c.hub.UnregisterClient(c)
	c.hub.sendOnlineUsersList(c.roomID, uuid.Nil)

	wsLogger.Info("User left room",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"room_id", c.roomID)
}

func (c *Client) handleUserAway(msg *Message) {
	c.hub.setUserPresence(c.userID, c.roomID, "away")
	notifyMsg := &Message{
		Type:      "user-away",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)
	if c.hub.enableRedis {
		c.hub.publishToRedis(notifyMsg)
	}

	wsLogger.Info("User set away status",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"room_id", c.roomID)
}

func (c *Client) handleUserBack(msg *Message) {
	c.hub.setUserPresence(c.userID, c.roomID, "online")
	notifyMsg := &Message{
		Type:      "user-online",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)
	if c.hub.enableRedis {
		c.hub.publishToRedis(notifyMsg)
	}

	wsLogger.Info("User back online",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"room_id", c.roomID)
}

func (c *Client) handleTyping(msg *Message) {
	if c.hub.isTypingThrottled(c.userID) {
		wsLogger.Debug("Typing event throttled",
			"connection_id", c.connectionID,
			"user_id", c.userID,
			"room_id", c.roomID)
		return
	}

	var payload TypingPayload
	if len(msg.Payload) > 0 {
		_ = json.Unmarshal(msg.Payload, &payload)
	}
	c.hub.setTypingUser(c.userID, c.roomID, payload.Context)
	notifyMsg := &Message{
		Type:      "user-typing",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   msg.Payload,
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)
	if c.hub.enableRedis {
		c.hub.publishToRedis(notifyMsg)
	}
	wsLogger.Debug("User typing broadcasted",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"room_id", c.roomID,
		"context", payload.Context)
}

func (c *Client) handleStopTyping(msg *Message) {
	c.hub.clearTypingUser(c.userID)
	notifyMsg := &Message{
		Type:      "user-stopped-typing",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)
	if c.hub.enableRedis {
		c.hub.publishToRedis(notifyMsg)
	}
}

func (c *Client) handleSetPresence(msg *Message) {
	var payload PresencePayload
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing presence payload",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"error", err)
			return
		}
	}

	c.hub.setUserPresence(c.userID, c.roomID, payload.Status)

	presenceMsg := &Message{
		Type:      "presence-updated",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   msg.Payload,
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, presenceMsg, c.userID)

	wsLogger.Info("User presence updated",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"status", payload.Status)
}

func (c *Client) handleGetPresence(msg *Message) {
	var payload struct {
		UserIDs []uuid.UUID `json:"user_ids"`
	}
	if len(msg.Payload) > 0 {
		_ = json.Unmarshal(msg.Payload, &payload)
	}

	presences := make([]PresenceInfo, 0)
	for _, uid := range payload.UserIDs {
		if status, exists := c.hub.GetUserPresence(uid); exists {
			presences = append(presences, PresenceInfo{
				UserID: uid,
				Status: status,
			})
		}
	}

	payloadBytes, _ := json.Marshal(presences)
	responseMsg := &Message{
		Type:      "presence-list",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   payloadBytes,
		Timestamp: time.Now(),
	}
	c.hub.sendToClient(c.userID, responseMsg)
}

func (c *Client) handleTypingIndicator(msg *Message) {
	var payload TypingIndicator
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing typing indicator payload",
				"connection_id", c.connectionID,
				"error", err)
			return
		}
	}

	if payload.IsTyping {
		c.hub.setTypingUser(c.userID, c.roomID, payload.Context)
	} else {
		c.hub.clearTypingUser(c.userID)
	}

	payload.UserID = c.userID
	payload.Timestamp = time.Now()

	payloadBytes, _ := json.Marshal(payload)
	notifyMsg := &Message{
		Type:      "typing-indicator",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   payloadBytes,
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)

	if c.hub.enableRedis {
		c.hub.publishToRedis(notifyMsg)
	}
}

func (c *Client) handleRoomEvent(msg *Message) {
	var payload RoomEvent
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing room event payload",
				"connection_id", c.connectionID,
				"error", err)
			return
		}
	}

	payload.UserID = c.userID
	payload.Timestamp = time.Now()

	notifyMsg := &Message{
		Type:      "room-event",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   msg.Payload,
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)

	if c.hub.enableRedis {
		c.hub.publishToRedis(notifyMsg)
	}
}

var notificationStore = &NotificationStore{
	notifications: make(map[uuid.UUID][]RoomNotification),
}

func (ns *NotificationStore) Add(userID uuid.UUID, notification RoomNotification) {
	ns.Lock()
	defer ns.Unlock()
	ns.notifications[userID] = append([]RoomNotification{notification}, ns.notifications[userID]...)

	if len(ns.notifications[userID]) > 100 {
		ns.notifications[userID] = ns.notifications[userID][:100]
	}
}

func (ns *NotificationStore) Get(userID uuid.UUID, limit int) []RoomNotification {
	ns.RLock()
	defer ns.RUnlock()
	notifs := ns.notifications[userID]
	if len(notifs) > limit {
		return notifs[:limit]
	}
	return notifs
}

func (ns *NotificationStore) MarkRead(userID, notificationID uuid.UUID) {
	ns.Lock()
	defer ns.Unlock()
	for i := range ns.notifications[userID] {
		if ns.notifications[userID][i].ID == notificationID.String() {
			ns.notifications[userID][i].Read = true
			break
		}
	}
}

func (c *Client) handleMarkNotificationRead(msg *Message) {
	var payload struct {
		NotificationID string `json:"notification_id"`
	}
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing notification read payload",
				"connection_id", c.connectionID,
				"error", err)
			return
		}
	}

	if notificationID, err := uuid.Parse(payload.NotificationID); err == nil {
		notificationStore.MarkRead(c.userID, notificationID)

		responseMsg := &Message{
			Type:      "notification-marked-read",
			RoomID:    c.roomID,
			UserID:    c.userID,
			Payload:   msg.Payload,
			Timestamp: time.Now(),
		}
		c.hub.sendToClient(c.userID, responseMsg)
	}
}

func (c *Client) handleGetNotifications(msg *Message) {
	var payload struct {
		Limit int `json:"limit"`
	}
	if len(msg.Payload) > 0 {
		_ = json.Unmarshal(msg.Payload, &payload)
	}
	if payload.Limit == 0 {
		payload.Limit = 20
	}

	notifications := notificationStore.Get(c.userID, payload.Limit)

	payloadBytes, _ := json.Marshal(notifications)
	responseMsg := &Message{
		Type:      "notifications-list",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   payloadBytes,
		Timestamp: time.Now(),
	}
	c.hub.sendToClient(c.userID, responseMsg)
}

func (c *Client) handleMessageRead(msg *Message) {
	var payload struct {
		MessageID string `json:"message_id"`
	}
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing message read payload",
				"connection_id", c.connectionID,
				"error", err)
			return
		}
	}

	messageID, err := uuid.Parse(payload.MessageID)
	if err != nil {
		wsLogger.Warn("Invalid message ID in message-read",
			"connection_id", c.connectionID,
			"message_id", payload.MessageID)
		return
	}

	c.hub.BroadcastMessageRead(c.userID, c.roomID, messageID)

	wsLogger.Info("User read message",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"message_id", payload.MessageID)
}

func (c *Client) handleMessageUpdated(msg *Message) {
	var payload struct {
		MessageID string `json:"message_id"`
		Content   string `json:"content"`
	}
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing message updated payload",
				"connection_id", c.connectionID,
				"error", err)
			return
		}
	}

	if err := validator.ValidateMessageContent(payload.Content); err != nil {
		wsLogger.Warn("Invalid message content in message-updated",
			"connection_id", c.connectionID,
			"error", err)
		c.sendErrorMessage("Invalid message content: " + err.Error())
		return
	}

	sanitizedContent := validator.SanitizeMessageContent(payload.Content)

	notifyMsg := &Message{
		Type:      "message-updated",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   json.RawMessage(fmt.Sprintf(`{"message_id":"%s","content":"%s"}`, payload.MessageID, sanitizedContent)),
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)

	wsLogger.Info("User updated message",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"message_id", payload.MessageID)
}

func (c *Client) handleThreadReply(msg *Message) {
	var payload struct {
		ParentID string `json:"parent_id"`
		Content  string `json:"content"`
	}
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing thread reply payload",
				"connection_id", c.connectionID,
				"error", err)
			return
		}
	}

	if err := validator.ValidateMessageContent(payload.Content); err != nil {
		wsLogger.Warn("Invalid thread reply content",
			"connection_id", c.connectionID,
			"error", err)
		c.sendErrorMessage("Invalid message content: " + err.Error())
		return
	}

	sanitizedContent := validator.SanitizeMessageContent(payload.Content)

	notifyMsg := &Message{
		Type:      "thread-reply",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   json.RawMessage(fmt.Sprintf(`{"parent_id":"%s","content":"%s"}`, payload.ParentID, sanitizedContent)),
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)

	notificationStore.Add(c.userID, RoomNotification{
		ID:         uuid.New().String(),
		Type:       "message",
		RoomID:     c.roomID,
		UserID:     c.userID,
		Message:    "Thread reply received",
		Timestamp:  time.Now(),
		Read:       false,
		Actionable: true,
	})

	wsLogger.Info("User replied to thread",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"parent_id", payload.ParentID)
}

func (c *Client) handleThreadMessage(msg *Message) {
	var payload struct {
		ThreadID string `json:"thread_id"`
		Content  string `json:"content"`
	}
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing thread message payload",
				"connection_id", c.connectionID,
				"error", err)
			return
		}
	}

	if err := validator.ValidateMessageContent(payload.Content); err != nil {
		wsLogger.Warn("Invalid thread message content",
			"connection_id", c.connectionID,
			"error", err)
		c.sendErrorMessage("Invalid message content: " + err.Error())
		return
	}

	sanitizedContent := validator.SanitizeMessageContent(payload.Content)

	notifyMsg := &Message{
		Type:      "thread-message",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   json.RawMessage(fmt.Sprintf(`{"thread_id":"%s","content":"%s"}`, payload.ThreadID, sanitizedContent)),
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)

	wsLogger.Info("User sent thread message",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"thread_id", payload.ThreadID)
}

func (c *Client) handleUserBlocked(msg *Message) {
	var payload struct {
		BlockedUserID string `json:"blocked_user_id"`
		Reason        string `json:"reason"`
	}
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing user blocked payload",
				"connection_id", c.connectionID,
				"error", err)
			return
		}
	}

	blockedUserID, err := uuid.Parse(payload.BlockedUserID)
	if err != nil {
		wsLogger.Warn("Invalid blocked user ID",
			"connection_id", c.connectionID,
			"blocked_user_id", payload.BlockedUserID)
		return
	}

	if blockedUserID == c.userID {
		wsLogger.Warn("User cannot block themselves",
			"connection_id", c.connectionID,
			"user_id", c.userID)
		return
	}

	c.hub.BlockUser(c.userID, blockedUserID)

	blockEventPayload := map[string]interface{}{
		"blocked_user_id": blockedUserID.String(),
		"reason":          payload.Reason,
	}
	payloadBytes, _ := json.Marshal(blockEventPayload)

	notifyMsg := &Message{
		Type:         "user-blocked",
		RoomID:       c.roomID,
		UserID:       c.userID,
		TargetUserID: blockedUserID,
		Payload:      payloadBytes,
		Timestamp:    time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)

	blockedClient := c.hub.GetUserClient(blockedUserID)
	if blockedClient != nil {
		blockNotification := &Message{
			Type:         "you-were-blocked",
			RoomID:       c.roomID,
			UserID:       c.userID,
			TargetUserID: blockedUserID,
			Payload:      payloadBytes,
			Timestamp:    time.Now(),
		}
		c.hub.sendToClient(blockedUserID, blockNotification)
	}

	wsLogger.Info("User blocked another user",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"blocked_user_id", blockedUserID)
}

func (c *Client) handleUserUnblocked(msg *Message) {
	var payload struct {
		UnblockedUserID string `json:"unblocked_user_id"`
	}
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing user unblocked payload",
				"connection_id", c.connectionID,
				"error", err)
			return
		}
	}

	unblockedUserID, err := uuid.Parse(payload.UnblockedUserID)
	if err != nil {
		wsLogger.Warn("Invalid unblocked user ID",
			"connection_id", c.connectionID,
			"unblocked_user_id", payload.UnblockedUserID)
		return
	}

	c.hub.UnblockUser(c.userID, unblockedUserID)

	unblockPayload := map[string]interface{}{
		"unblocked_user_id": unblockedUserID.String(),
	}
	payloadBytes, _ := json.Marshal(unblockPayload)

	notifyMsg := &Message{
		Type:         "user-unblocked",
		RoomID:       c.roomID,
		UserID:       c.userID,
		TargetUserID: unblockedUserID,
		Payload:      payloadBytes,
		Timestamp:    time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)

	wsLogger.Info("User unblocked another user",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"unblocked_user_id", unblockedUserID)
}

func (c *Client) handleClientReconnect(msg *Message) {
	wsLogger.Info("Client reconnecting",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"room_id", c.roomID)

	c.hub.sendRoomState(c)

	clients := c.hub.GetRoomClients(c.roomID)
	typingUsers := c.hub.GetTypingUsers(c.roomID)

	typingPayload, _ := json.Marshal(typingUsers)
	typingMsg := &Message{
		Type:      "typing-users-list",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   typingPayload,
		Timestamp: time.Now(),
	}
	c.hub.sendToClient(c.userID, typingMsg)

	reconnectResponse := &Message{
		Type:      "reconnect-ack",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Timestamp: time.Now(),
	}
	c.hub.sendToClient(c.userID, reconnectResponse)

	onlineUsers := make([]OnlineUser, 0, len(clients))
	for _, client := range clients {
		onlineUsers = append(onlineUsers, OnlineUser{
			UserID:        client.userID,
			ConnectionID:  client.connectionID,
			State:         client.GetState().String(),
			ConnectedAt:   client.connectedAt,
			LastSeen:      client.GetLastSeen(),
			RemoteAddress: client.remoteAddr,
		})
	}

	onlinePayload, _ := json.Marshal(onlineUsers)
	onlineMsg := &Message{
		Type:      "online-users-list",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   onlinePayload,
		Timestamp: time.Now(),
	}
	c.hub.sendToClient(c.userID, onlineMsg)

	wsLogger.Info("Client reconnect complete",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"room_id", c.roomID,
		"online_users", len(onlineUsers),
		"typing_users", len(typingUsers))
}

func (c *Client) handleMessageDeleted(msg *Message) {
	var payload struct {
		MessageID string `json:"message_id"`
	}
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing message deleted payload",
				"connection_id", c.connectionID,
				"error", err)
			return
		}
	}

	notifyMsg := &Message{
		Type:      "message-deleted",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   msg.Payload,
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)

	wsLogger.Info("User deleted message",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"message_id", payload.MessageID)
}

func (c *Client) handleThreadMessageUpdated(msg *Message) {
	var payload struct {
		MessageID string `json:"message_id"`
		Content   string `json:"content"`
	}
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing thread message updated payload",
				"connection_id", c.connectionID,
				"error", err)
			return
		}
	}

	if err := validator.ValidateMessageContent(payload.Content); err != nil {
		wsLogger.Warn("Invalid thread message content",
			"connection_id", c.connectionID,
			"error", err)
		c.sendErrorMessage("Invalid message content: " + err.Error())
		return
	}

	sanitizedContent := validator.SanitizeMessageContent(payload.Content)

	notifyMsg := &Message{
		Type:      "thread-message-updated",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   json.RawMessage(fmt.Sprintf(`{"message_id":"%s","content":"%s"}`, payload.MessageID, sanitizedContent)),
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)

	wsLogger.Info("User updated thread message",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"message_id", payload.MessageID)
}

func (c *Client) handleThreadMessageDeleted(msg *Message) {
	var payload struct {
		MessageID string `json:"message_id"`
	}
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing thread message deleted payload",
				"connection_id", c.connectionID,
				"error", err)
			return
		}
	}

	notifyMsg := &Message{
		Type:      "thread-message-deleted",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   msg.Payload,
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)

	wsLogger.Info("User deleted thread message",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"message_id", payload.MessageID)
}

func (c *Client) handleTypingWithContext(msg *Message) {
	var payload TypingEvent
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing typing with context payload",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"error", err)
			c.sendErrorMessage("Invalid typing payload format")
			return
		}
	}

	if payload.IsTyping {
		contextStr := payload.ContextType
		if payload.ChannelID != "" {
			contextStr = "channel:" + payload.ChannelID
		} else if payload.ThreadID != "" {
			contextStr = "thread:" + payload.ThreadID
		}
		c.hub.setTypingUser(c.userID, c.roomID, contextStr)
	} else {
		c.hub.clearTypingUser(c.userID)
	}

	payload.UserID = c.userID
	payload.RoomID = c.roomID
	payload.Timestamp = time.Now()

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		wsLogger.Error("Error marshaling typing with context payload",
			"connection_id", c.connectionID,
			"error", err)
		return
	}

	notifyMsg := &Message{
		Type:      "typing-with-context",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   payloadBytes,
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)

	if c.hub.enableRedis {
		c.hub.publishToRedis(notifyMsg)
	}

	wsLogger.Debug("Typing with context",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"is_typing", payload.IsTyping,
		"context", payload.ContextType)
}

func (c *Client) handlePresenceBulk(msg *Message) {
	var payload struct {
		UserIDs []uuid.UUID `json:"user_ids"`
	}
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing presence bulk payload",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"error", err)
			c.sendErrorMessage("Invalid payload format")
			return
		}
	}

	presences := make([]PresenceInfo, 0, len(payload.UserIDs))
	for _, uid := range payload.UserIDs {
		if status, exists := c.hub.GetUserPresence(uid); exists {
			presences = append(presences, PresenceInfo{
				UserID: uid,
				Status: status,
				Since:  time.Now(),
			})
		} else {
			presences = append(presences, PresenceInfo{
				UserID: uid,
				Status: "offline",
				Since:  time.Now(),
			})
		}
	}

	payloadBytes, err := json.Marshal(presences)
	if err != nil {
		wsLogger.Error("Error marshaling presence bulk response",
			"connection_id", c.connectionID,
			"error", err)
		return
	}

	responseMsg := &Message{
		Type:      "presence-bulk-response",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   payloadBytes,
		Timestamp: time.Now(),
	}
	c.hub.sendToClient(c.userID, responseMsg)

	wsLogger.Debug("Sent presence bulk response",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"count", len(presences))
}

func (c *Client) handleRoomNotificationEvent(msg *Message) {
	var payload RoomNotification
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing room notification event payload",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"error", err)
			c.sendErrorMessage("Invalid notification payload format")
			return
		}
	}

	payload.RoomID = c.roomID
	payload.UserID = c.userID
	payload.Timestamp = time.Now()

	if payload.ID == "" {
		payload.ID = uuid.New().String()
	}

	notificationStore.Add(c.userID, payload)

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		wsLogger.Error("Error marshaling room notification event",
			"connection_id", c.connectionID,
			"error", err)
		return
	}

	notifyMsg := &Message{
		Type:      "room-notification-event",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   payloadBytes,
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)

	if c.hub.enableRedis {
		c.hub.publishToRedis(notifyMsg)
	}

	wsLogger.Info("Room notification event",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"room_id", c.roomID,
		"type", payload.Type)
}

func (c *Client) handleGetRoomPresence(msg *Message) {
	presenceList := c.hub.GetRoomPresence(c.roomID)

	presences := make([]PresenceInfo, 0, len(presenceList))
	for _, p := range presenceList {
		presences = append(presences, PresenceInfo{
			UserID:       p.UserID,
			Status:       p.Status,
			Since:        p.UpdatedAt,
			CustomStatus: "",
		})
	}

	payloadBytes, err := json.Marshal(presences)
	if err != nil {
		wsLogger.Error("Error marshaling room presence",
			"connection_id", c.connectionID,
			"error", err)
		return
	}

	responseMsg := &Message{
		Type:      "room-presence-list",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   payloadBytes,
		Timestamp: time.Now(),
	}
	c.hub.sendToClient(c.userID, responseMsg)

	wsLogger.Debug("Sent room presence list",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"count", len(presences))
}

func (c *Client) handleUserKicked(msg *Message) {
	var payload struct {
		TargetUserID string `json:"target_user_id"`
		Reason       string `json:"reason"`
	}
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing user kicked payload",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"error", err)
			c.sendErrorMessage("Invalid payload format")
			return
		}
	}

	targetUserID, err := uuid.Parse(payload.TargetUserID)
	if err != nil {
		wsLogger.Warn("Invalid target user ID for kick",
			"connection_id", c.connectionID,
			"target_user_id", payload.TargetUserID)
		return
	}

	notification := RoomNotification{
		ID:           uuid.New().String(),
		Type:         "user_kicked",
		RoomID:       c.roomID,
		UserID:       c.userID,
		TargetUserID: targetUserID,
		Message:      "You have been kicked from the room",
		Reason:       payload.Reason,
		Timestamp:    time.Now(),
		Read:         false,
		Actionable:   false,
	}

	notificationStore.Add(targetUserID, notification)

	payloadBytes, err := json.Marshal(notification)
	if err != nil {
		wsLogger.Error("Error marshaling kick notification",
			"connection_id", c.connectionID,
			"error", err)
		return
	}

	notifyMsg := &Message{
		Type:         "user-kicked",
		RoomID:       c.roomID,
		UserID:       c.userID,
		TargetUserID: targetUserID,
		Payload:      payloadBytes,
		Timestamp:    time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, uuid.Nil)

	wsLogger.Info("User kicked",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"target_user_id", targetUserID,
		"reason", payload.Reason)
}

func (c *Client) handleUserBanned(msg *Message) {
	var payload struct {
		TargetUserID string `json:"target_user_id"`
		Reason       string `json:"reason"`
		Duration     string `json:"duration"` // "permanent" or "24h", "7d", etc.
	}
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing user banned payload",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"error", err)
			c.sendErrorMessage("Invalid payload format")
			return
		}
	}

	targetUserID, err := uuid.Parse(payload.TargetUserID)
	if err != nil {
		wsLogger.Warn("Invalid target user ID for ban",
			"connection_id", c.connectionID,
			"target_user_id", payload.TargetUserID)
		return
	}

	duration := "permanent"
	if payload.Duration != "" {
		duration = payload.Duration
	}

	notification := RoomNotification{
		ID:           uuid.New().String(),
		Type:         "user_banned",
		RoomID:       c.roomID,
		UserID:       c.userID,
		TargetUserID: targetUserID,
		Message:      "You have been banned from the room",
		Reason:       payload.Reason,
		Duration:     duration,
		Timestamp:    time.Now(),
		Read:         false,
		Actionable:   false,
	}

	notificationStore.Add(targetUserID, notification)

	payloadBytes, err := json.Marshal(notification)
	if err != nil {
		wsLogger.Error("Error marshaling ban notification",
			"connection_id", c.connectionID,
			"error", err)
		return
	}

	notifyMsg := &Message{
		Type:         "user-banned",
		RoomID:       c.roomID,
		UserID:       c.userID,
		TargetUserID: targetUserID,
		Payload:      payloadBytes,
		Timestamp:    time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, uuid.Nil)

	wsLogger.Info("User banned",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"target_user_id", targetUserID,
		"reason", payload.Reason,
		"duration", duration)
}

func (c *Client) handleRoleChanged(msg *Message) {
	var payload struct {
		TargetUserID string `json:"target_user_id"`
		OldRole      string `json:"old_role"`
		NewRole      string `json:"new_role"`
	}
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing role changed payload",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"error", err)
			c.sendErrorMessage("Invalid payload format")
			return
		}
	}

	targetUserID, err := uuid.Parse(payload.TargetUserID)
	if err != nil {
		wsLogger.Warn("Invalid target user ID for role change",
			"connection_id", c.connectionID,
			"target_user_id", payload.TargetUserID)
		return
	}

	rolePayload := map[string]string{
		"target_user_id": targetUserID.String(),
		"old_role":       payload.OldRole,
		"new_role":       payload.NewRole,
	}
	payloadBytes, err := json.Marshal(rolePayload)
	if err != nil {
		wsLogger.Error("Error marshaling role changed payload",
			"connection_id", c.connectionID,
			"error", err)
		return
	}

	notification := RoomNotification{
		ID:         uuid.New().String(),
		Type:       "role_changed",
		RoomID:     c.roomID,
		UserID:     c.userID,
		Message:    fmt.Sprintf("Role changed from %s to %s", payload.OldRole, payload.NewRole),
		Timestamp:  time.Now(),
		Read:       false,
		Actionable: false,
	}

	notificationStore.Add(targetUserID, notification)

	notifyMsg := &Message{
		Type:         "role-changed",
		RoomID:       c.roomID,
		UserID:       c.userID,
		TargetUserID: targetUserID,
		Payload:      payloadBytes,
		Timestamp:    time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, uuid.Nil)

	wsLogger.Info("User role changed",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"target_user_id", targetUserID,
		"old_role", payload.OldRole,
		"new_role", payload.NewRole)
}

func (c *Client) handleTypingBulk(msg *Message) {
	var payload TypingBulkPayload
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing typing bulk payload",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"error", err)
			c.sendErrorMessage("Invalid payload format")
			return
		}
	}

	for _, typingUser := range payload.TypingUsers {
		if typingUser.IsTyping {
			contextStr := typingUser.ContextType
			if typingUser.ChannelID != "" {
				contextStr = "channel:" + typingUser.ChannelID
			} else if typingUser.ThreadID != "" {
				contextStr = "thread:" + typingUser.ThreadID
			}
			c.hub.setTypingUser(typingUser.UserID, c.roomID, contextStr)
		} else {
			c.hub.clearTypingUser(typingUser.UserID)
		}
	}

	typingUsers := c.hub.GetTypingUsers(c.roomID)
	typingPayload, err := json.Marshal(typingUsers)
	if err != nil {
		wsLogger.Error("Error marshaling typing bulk response",
			"connection_id", c.connectionID,
			"error", err)
		return
	}

	responseMsg := &Message{
		Type:      "typing-bulk-response",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   typingPayload,
		Timestamp: time.Now(),
	}
	c.hub.sendToClient(c.userID, responseMsg)

	wsLogger.Debug("Handled typing bulk request",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"count", len(payload.TypingUsers))
}

var roomStateSubscriptions = &RoomStateSubscription{
	subscribers: make(map[uuid.UUID]map[uuid.UUID]bool),
}

func (rss *RoomStateSubscription) Subscribe(roomID, userID uuid.UUID) {
	rss.mu.Lock()
	defer rss.mu.Unlock()
	if rss.subscribers[roomID] == nil {
		rss.subscribers[roomID] = make(map[uuid.UUID]bool)
	}
	rss.subscribers[roomID][userID] = true
	wsLogger.Debug("User subscribed to room state",
		"room_id", roomID,
		"user_id", userID)
}

func (rss *RoomStateSubscription) Unsubscribe(roomID, userID uuid.UUID) {
	rss.mu.Lock()
	defer rss.mu.Unlock()
	if rss.subscribers[roomID] != nil {
		delete(rss.subscribers[roomID], userID)
		if len(rss.subscribers[roomID]) == 0 {
			delete(rss.subscribers, roomID)
		}
	}
	wsLogger.Debug("User unsubscribed from room state",
		"room_id", roomID,
		"user_id", userID)
}

func (rss *RoomStateSubscription) GetSubscribers(roomID uuid.UUID) []uuid.UUID {
	rss.mu.RLock()
	defer rss.mu.RUnlock()
	var subs []uuid.UUID
	if rss.subscribers[roomID] != nil {
		for userID := range rss.subscribers[roomID] {
			subs = append(subs, userID)
		}
	}
	return subs
}

func (c *Client) handleTypingBroadcast(msg *Message) {
	var payload struct {
		Context     string `json:"context"`
		ChannelID   string `json:"channel_id"`
		ThreadID    string `json:"thread_id"`
		ContextType string `json:"context_type"`
		IsTyping    bool   `json:"is_typing"`
	}
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing typing broadcast payload",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"error", err)
			c.sendErrorMessage("Invalid typing payload format")
			return
		}
	}

	if payload.IsTyping {
		contextStr := payload.Context
		if payload.ChannelID != "" {
			contextStr = "channel:" + payload.ChannelID
		} else if payload.ThreadID != "" {
			contextStr = "thread:" + payload.ThreadID
		} else if payload.ContextType != "" {
			contextStr = payload.ContextType
		}
		c.hub.setTypingUser(c.userID, c.roomID, contextStr)
	} else {
		c.hub.clearTypingUser(c.userID)
	}

	broadcastPayload := TypingIndicator{
		UserID:      c.userID,
		RoomID:      c.roomID,
		Context:     payload.Context,
		ContextID:   payload.ChannelID,
		ContextType: payload.ContextType,
		ChannelID:   payload.ChannelID,
		ThreadID:    payload.ThreadID,
		IsTyping:    payload.IsTyping,
		Timestamp:   time.Now(),
	}

	payloadBytes, _ := json.Marshal(broadcastPayload)
	notifyMsg := &Message{
		Type:      "typing-broadcast",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   payloadBytes,
		Timestamp: time.Now(),
	}

	subscribers := roomStateSubscriptions.GetSubscribers(c.roomID)
	for _, subUserID := range subscribers {
		if subUserID != c.userID {
			c.hub.sendToClient(subUserID, notifyMsg)
		}
	}

	wsLogger.Debug("Typing broadcast handled",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"is_typing", payload.IsTyping,
		"context", payload.Context)
}

func (c *Client) handleRoomStateSubscribe(msg *Message) {
	roomStateSubscriptions.Subscribe(c.roomID, c.userID)

	confirmMsg := &Message{
		Type:      "room-state-subscribed",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Timestamp: time.Now(),
	}
	c.hub.sendToClient(c.userID, confirmMsg)

	c.hub.sendRoomState(c)

	wsLogger.Info("User subscribed to room state updates",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"room_id", c.roomID)
}

func (c *Client) handleRoomStateUnsubscribe(msg *Message) {
	roomStateSubscriptions.Unsubscribe(c.roomID, c.userID)

	confirmMsg := &Message{
		Type:      "room-state-unsubscribed",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Timestamp: time.Now(),
	}
	c.hub.sendToClient(c.userID, confirmMsg)

	wsLogger.Info("User unsubscribed from room state updates",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"room_id", c.roomID)
}

var userPresenceSubscriptions = struct {
	sync.RWMutex
	subscriptions map[uuid.UUID]map[uuid.UUID]bool
}{
	subscriptions: make(map[uuid.UUID]map[uuid.UUID]bool),
}

func (c *Client) handleUserPresenceSubscribe(msg *Message) {
	var payload struct {
		UserIDs []uuid.UUID `json:"user_ids"`
	}
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing presence subscribe payload",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"error", err)
			c.sendErrorMessage("Invalid payload format")
			return
		}
	}

	userPresenceSubscriptions.Lock()
	defer userPresenceSubscriptions.Unlock()
	if userPresenceSubscriptions.subscriptions[c.userID] == nil {
		userPresenceSubscriptions.subscriptions[c.userID] = make(map[uuid.UUID]bool)
	}
	for _, targetUserID := range payload.UserIDs {
		userPresenceSubscriptions.subscriptions[c.userID][targetUserID] = true
	}

	confirmMsg := &Message{
		Type:      "presence-subscribed",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   msg.Payload,
		Timestamp: time.Now(),
	}
	c.hub.sendToClient(c.userID, confirmMsg)

	wsLogger.Info("User subscribed to presence updates",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"target_users", len(payload.UserIDs))
}

func (c *Client) handlePingPong(msg *Message) {
	var clientTime int64
	if len(msg.Payload) > 0 {
		var payload struct {
			Timestamp int64 `json:"timestamp"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err == nil {
			clientTime = payload.Timestamp
		}
	}

	serverTime := time.Now().UnixMilli()
	var latencyMs int64
	if clientTime > 0 {
		latencyMs = (serverTime - clientTime) / 2
		c.latency.Store(latencyMs)
	}

	responsePayload := struct {
		ServerTime int64 `json:"server_time"`
		LatencyMs  int64 `json:"latency_ms"`
	}{
		ServerTime: serverTime,
		LatencyMs:  latencyMs,
	}

	payloadBytes, _ := json.Marshal(responsePayload)
	responseMsg := &Message{
		Type:      "pong",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   payloadBytes,
		Timestamp: time.Now(),
	}
	c.hub.sendToClient(c.userID, responseMsg)

	wsLogger.Debug("Ping-pong handled",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"latency_ms", latencyMs)
}

func (c *Client) handleClientReady(msg *Message) {
	var payload ClientReadyPayload
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing client-ready payload",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"error", err)
			return
		}
	}

	wsLogger.Info("Client ready",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"room_id", c.roomID,
		"client_version", payload.ClientVersion,
		"platform", payload.Platform)

	c.SetState(StateConnected)

	confirmMsg := &Message{
		Type:      "client-ready-ack",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Timestamp: time.Now(),
	}
	c.hub.sendToClient(c.userID, confirmMsg)

	c.hub.sendRoomState(c)
	c.hub.sendOnlineUsersList(c.roomID, c.userID)

	typingUsers := c.hub.GetTypingUsers(c.roomID)
	typingPayload, _ := json.Marshal(typingUsers)
	typingMsg := &Message{
		Type:      "typing-users-list",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   typingPayload,
		Timestamp: time.Now(),
	}
	c.hub.sendToClient(c.userID, typingMsg)

	wsLogger.Debug("Sent initial room state to client",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"room_id", c.roomID)
}

func (c *Client) handleTypingStatus(msg *Message) {
	var payload TypingStatusPayload
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing typing-status payload",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"error", err)
			c.sendErrorMessage("Invalid typing status payload")
			return
		}
	}

	contextKey := payload.Context
	if payload.ContextID != "" {
		if payload.ContextType != "" {
			contextKey = payload.ContextType + ":" + payload.ContextID
		} else if payload.Context != "" {
			contextKey = payload.Context
		}
	}

	if payload.IsTyping {
		c.hub.setTypingUser(c.userID, c.roomID, contextKey)
	} else {
		c.hub.clearTypingUser(c.userID)
	}

	statusPayload := TypingStatusPayload{
		Context:     contextKey,
		ContextID:   payload.ContextID,
		ContextType: payload.ContextType,
		IsTyping:    payload.IsTyping,
	}

	payloadBytes, _ := json.Marshal(statusPayload)
	notifyMsg := &Message{
		Type:      "typing-status-update",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   payloadBytes,
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)

	if c.hub.enableRedis {
		c.hub.publishToRedis(notifyMsg)
	}

	wsLogger.Debug("Typing status updated",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"context", contextKey,
		"is_typing", payload.IsTyping)
}

func (c *Client) handlePresenceBulkSubscribe(msg *Message) {
	var payload struct {
		UserIDs []uuid.UUID `json:"user_ids"`
	}
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing presence bulk subscribe payload",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"error", err)
			c.sendErrorMessage("Invalid payload format")
			return
		}
	}

	userPresenceSubscriptions.Lock()
	defer userPresenceSubscriptions.Unlock()
	if userPresenceSubscriptions.subscriptions[c.userID] == nil {
		userPresenceSubscriptions.subscriptions[c.userID] = make(map[uuid.UUID]bool)
	}
	for _, targetUserID := range payload.UserIDs {
		userPresenceSubscriptions.subscriptions[c.userID][targetUserID] = true
	}

	presences := make([]PresenceInfo, 0, len(payload.UserIDs))
	for _, uid := range payload.UserIDs {
		if status, exists := c.hub.GetUserPresence(uid); exists {
			presences = append(presences, PresenceInfo{
				UserID: uid,
				Status: status,
				Since:  time.Now(),
			})
		} else {
			presences = append(presences, PresenceInfo{
				UserID: uid,
				Status: "offline",
			})
		}
	}

	responsePayload, _ := json.Marshal(presences)
	responseMsg := &Message{
		Type:      "presence-bulk-update",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   responsePayload,
		Timestamp: time.Now(),
	}
	c.hub.sendToClient(c.userID, responseMsg)

	wsLogger.Debug("Presence bulk subscription handled",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"count", len(payload.UserIDs))
}

func (c *Client) handleNotificationPreferences(msg *Message) {
	var payload NotificationPreferencePayload
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing notification preferences payload",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"error", err)
			c.sendErrorMessage("Invalid notification preferences payload")
			return
		}
	}

	roomID := c.roomID
	if payload.RoomID != "" {
		if parsedRoomID, err := uuid.Parse(payload.RoomID); err == nil {
			roomID = parsedRoomID
		}
	}

	key := fmt.Sprintf("%s:%s", c.userID, roomID)
	c.hub.notificationMutex.RLock()
	existing, exists := c.hub.notificationSettings[key]
	c.hub.notificationMutex.RUnlock()

	prefs := &RoomNotificationPreferences{
		RoomID:    roomID.String(),
		UpdatedAt: time.Now(),
	}

	if exists && existing != nil {
		prefs.Mentions = existing.MentionOnly || existing.Enabled
		prefs.Messages = existing.Enabled
		prefs.SoundEnabled = existing.Sound
		prefs.DesktopEnabled = existing.DesktopNotify
	} else {
		prefs.Mentions = true
		prefs.Messages = true
		prefs.Reactions = true
		prefs.VoiceActivity = true
		prefs.ThreadReplies = true
		prefs.DirectMessages = true
		prefs.SoundEnabled = true
		prefs.DesktopEnabled = true
		prefs.MobileEnabled = true
	}

	if payload.Mentions != nil {
		prefs.Mentions = *payload.Mentions
	}
	if payload.Messages != nil {
		prefs.Messages = *payload.Messages
	}
	if payload.Reactions != nil {
		prefs.Reactions = *payload.Reactions
	}
	if payload.VoiceActivity != nil {
		prefs.VoiceActivity = *payload.VoiceActivity
	}
	if payload.ThreadReplies != nil {
		prefs.ThreadReplies = *payload.ThreadReplies
	}
	if payload.DirectMessages != nil {
		prefs.DirectMessages = *payload.DirectMessages
	}
	if payload.CustomKeyword != "" {
		prefs.CustomKeyword = payload.CustomKeyword
	}
	if payload.SoundEnabled != nil {
		prefs.SoundEnabled = *payload.SoundEnabled
	}
	if payload.DesktopEnabled != nil {
		prefs.DesktopEnabled = *payload.DesktopEnabled
	}
	if payload.MobileEnabled != nil {
		prefs.MobileEnabled = *payload.MobileEnabled
	}
	if payload.MuteDuration > 0 {
		prefs.MuteDuration = payload.MuteDuration
	}

	c.hub.notificationMutex.Lock()
	c.hub.notificationSettings[key] = &RoomNotificationSettings{
		Enabled:       prefs.Messages,
		Sound:         prefs.SoundEnabled,
		MentionOnly:   !prefs.Mentions,
		DesktopNotify: prefs.DesktopEnabled,
	}
	c.hub.notificationMutex.Unlock()

	prefsPayload, _ := json.Marshal(prefs)
	confirmMsg := &Message{
		Type:      "notification-preferences-updated",
		RoomID:    roomID,
		UserID:    c.userID,
		Payload:   prefsPayload,
		Timestamp: time.Now(),
	}
	c.hub.sendToClient(c.userID, confirmMsg)

	wsLogger.Info("Notification preferences updated",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"room_id", roomID)
}

func (c *Client) handleGetNotificationPreferences(msg *Message) {
	roomID := c.roomID
	var payload struct {
		RoomID string `json:"room_id,omitempty"`
	}
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err == nil && payload.RoomID != "" {
			if parsedRoomID, err := uuid.Parse(payload.RoomID); err == nil {
				roomID = parsedRoomID
			}
		}
	}

	key := fmt.Sprintf("%s:%s", c.userID, roomID)
	c.hub.mutex.RLock()
	existing, exists := c.hub.notificationSettings[key]
	c.hub.mutex.RUnlock()

	prefs := &RoomNotificationPreferences{
		RoomID:    roomID.String(),
		UpdatedAt: time.Now(),
	}

	if exists && existing != nil {
		prefs.Mentions = existing.MentionOnly || existing.Enabled
		prefs.Messages = existing.Enabled
		prefs.SoundEnabled = existing.Sound
		prefs.DesktopEnabled = existing.DesktopNotify
	}

	prefsPayload, _ := json.Marshal(prefs)
	responseMsg := &Message{
		Type:      "notification-preferences",
		RoomID:    roomID,
		UserID:    c.userID,
		Payload:   prefsPayload,
		Timestamp: time.Now(),
	}
	c.hub.sendToClient(c.userID, responseMsg)
}

func (c *Client) handleEnhancedTyping(msg *Message) {
	var payload EnhancedTypingPayload
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing enhanced typing payload",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"error", err)
			c.sendErrorMessage("Invalid typing payload format")
			return
		}
	}

	contextKey := payload.Context
	if payload.ContextID != "" {
		contextKey = payload.Context + ":" + payload.ContextID
	} else if payload.ChannelID != "" {
		contextKey = "channel:" + payload.ChannelID
	} else if payload.ThreadID != "" {
		contextKey = "thread:" + payload.ThreadID
	} else if payload.DmID != "" {
		contextKey = "dm:" + payload.DmID
	}

	if payload.IsTyping {
		c.hub.setTypingUser(c.userID, c.roomID, contextKey)
	} else {
		c.hub.clearTypingUser(c.userID)
	}

	typingEvent := EnhancedTypingEvent{
		UserID:         c.userID,
		RoomID:         c.roomID,
		Context:        payload.Context,
		ContextID:      payload.ContextID,
		IsTyping:       payload.IsTyping,
		MessageID:      payload.MessageID,
		MentionedUsers: payload.MentionedUsers,
		Timestamp:      time.Now(),
	}

	payloadBytes, err := json.Marshal(typingEvent)
	if err != nil {
		wsLogger.Error("Error marshaling enhanced typing event",
			"connection_id", c.connectionID,
			"error", err)
		return
	}

	notifyMsg := &Message{
		Type:      "enhanced-typing",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   payloadBytes,
		Timestamp: time.Now(),
	}

	if payload.MentionedUsers != nil && len(payload.MentionedUsers) > 0 {
		for _, mentionedUser := range payload.MentionedUsers {
			c.hub.sendToClient(mentionedUser, notifyMsg)
		}
	} else {
		c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)
	}

	if c.hub.enableRedis {
		c.hub.publishToRedis(notifyMsg)
	}

	wsLogger.Debug("Enhanced typing event",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"context", contextKey,
		"is_typing", payload.IsTyping,
		"message_id", payload.MessageID)
}

func (c *Client) handlePresenceEvent(msg *Message) {
	var payload PresenceEventPayload
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing presence event payload",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"error", err)
			c.sendErrorMessage("Invalid presence payload format")
			return
		}
	}

	validStatuses := map[string]bool{
		"online": true, "idle": true, "away": true, "dnd": true, "offline": true,
	}
	if !validStatuses[payload.Status] {
		wsLogger.Warn("Invalid presence status",
			"connection_id", c.connectionID,
			"user_id", c.userID,
			"status", payload.Status)
		c.sendErrorMessage("Invalid presence status")
		return
	}

	c.hub.setUserPresence(c.userID, c.roomID, payload.Status)

	presenceEvent := PresenceEvent{
		UserID:       c.userID,
		Status:       payload.Status,
		Activity:     payload.Activity,
		CustomStatus: payload.CustomStatus,
		Device:       payload.Device,
		Since:        time.Unix(payload.Since, 0),
		Timestamp:    time.Now(),
	}

	payloadBytes, err := json.Marshal(presenceEvent)
	if err != nil {
		wsLogger.Error("Error marshaling presence event",
			"connection_id", c.connectionID,
			"error", err)
		return
	}

	notifyMsg := &Message{
		Type:      "presence-event",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   payloadBytes,
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)

	if c.hub.enableRedis {
		c.hub.publishToRedis(notifyMsg)
	}

	wsLogger.Info("Presence event",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"status", payload.Status,
		"activity", payload.Activity)
}

var roomNotificationSubscriptions = struct {
	sync.RWMutex
	subscriptions map[uuid.UUID]map[uuid.UUID][]string
}{
	subscriptions: make(map[uuid.UUID]map[uuid.UUID][]string),
}

func (c *Client) handleSubscribeRoomNotifications(msg *Message) {
	var payload RoomNotificationSubscription
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing room notification subscription payload",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"error", err)
			c.sendErrorMessage("Invalid subscription payload format")
			return
		}
	}

	roomID := payload.RoomID
	if roomID == uuid.Nil {
		roomID = c.roomID
	}

	validTypes := map[string]bool{
		"user_joined": true, "user_left": true, "typing": true,
		"message": true, "mention": true, "reaction": true,
		"voice_activity": true, "screen_share": true,
	}

	filteredTypes := make([]string, 0)
	for _, t := range payload.Types {
		if validTypes[t] {
			filteredTypes = append(filteredTypes, t)
		}
	}

	roomNotificationSubscriptions.Lock()
	if roomNotificationSubscriptions.subscriptions[roomID] == nil {
		roomNotificationSubscriptions.subscriptions[roomID] = make(map[uuid.UUID][]string)
	}
	roomNotificationSubscriptions.subscriptions[roomID][c.userID] = filteredTypes
	roomNotificationSubscriptions.Unlock()

	confirmMsg := &Message{
		Type:      "room-notification-subscribed",
		RoomID:    roomID,
		UserID:    c.userID,
		Payload:   msg.Payload,
		Timestamp: time.Now(),
	}
	c.hub.sendToClient(c.userID, confirmMsg)

	wsLogger.Info("User subscribed to room notifications",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"room_id", roomID,
		"types", filteredTypes)
}

func (c *Client) handleUnsubscribeRoomNotifications(msg *Message) {
	var payload struct {
		RoomID uuid.UUID `json:"room_id"`
	}
	if len(msg.Payload) > 0 {
		_ = json.Unmarshal(msg.Payload, &payload)
	}

	roomID := payload.RoomID
	if roomID == uuid.Nil {
		roomID = c.roomID
	}

	roomNotificationSubscriptions.Lock()
	if roomNotificationSubscriptions.subscriptions[roomID] != nil {
		delete(roomNotificationSubscriptions.subscriptions[roomID], c.userID)
		if len(roomNotificationSubscriptions.subscriptions[roomID]) == 0 {
			delete(roomNotificationSubscriptions.subscriptions, roomID)
		}
	}
	roomNotificationSubscriptions.Unlock()

	confirmMsg := &Message{
		Type:      "room-notification-unsubscribed",
		RoomID:    roomID,
		UserID:    c.userID,
		Timestamp: time.Now(),
	}
	c.hub.sendToClient(c.userID, confirmMsg)

	wsLogger.Info("User unsubscribed from room notifications",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"room_id", roomID)
}

func (c *Client) handleGetOnlineUsersDetailed(msg *Message) {
	var payload struct {
		IncludePresence bool `json:"include_presence"`
		IncludeActivity bool `json:"include_activity"`
	}
	if len(msg.Payload) > 0 {
		_ = json.Unmarshal(msg.Payload, &payload)
	}

	clients := c.hub.GetRoomClients(c.roomID)

	onlineUsers := make([]OnlineUser, 0, len(clients))
	for _, client := range clients {
		if client.userID == c.userID && c.roomID != uuid.Nil {
			continue
		}

		presenceStatus, _ := c.hub.GetUserPresence(client.userID)
		activity, hasActivity := c.hub.GetUserActivity(client.userID)

		onlineUser := OnlineUser{
			UserID:         client.userID,
			ConnectionID:   client.connectionID,
			State:          client.GetState().String(),
			ConnectedAt:    client.connectedAt,
			LastSeen:       client.GetLastSeen(),
			PresenceStatus: presenceStatus,
		}

		if hasActivity {
			onlineUser.ActiveChannelID = activity.RoomID.String()
		}

		onlineUsers = append(onlineUsers, onlineUser)
	}

	payloadBytes, err := json.Marshal(onlineUsers)
	if err != nil {
		wsLogger.Error("Error marshaling detailed online users",
			"room_id", c.roomID,
			"error", err)
		return
	}

	responseMsg := &Message{
		Type:      "online-users-detailed",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   payloadBytes,
		Timestamp: time.Now(),
	}
	c.hub.sendToClient(c.userID, responseMsg)

	wsLogger.Debug("Sent detailed online users",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"room_id", c.roomID,
		"count", len(onlineUsers))
}

func (h *Hub) broadcastUserStatusUpdate(userID, roomID uuid.UUID, status, customStatus, device string) {
	statusUpdate := UserStatusUpdate{
		UserID:       userID,
		RoomID:       roomID,
		Status:       status,
		CustomStatus: customStatus,
		Device:       device,
		Timestamp:    time.Now(),
	}

	payloadBytes, err := json.Marshal(statusUpdate)
	if err != nil {
		wsLogger.Error("Error marshaling user status update",
			"user_id", userID,
			"error", err)
		return
	}

	msg := &Message{
		Type:      "user-status-update",
		RoomID:    roomID,
		UserID:    userID,
		Payload:   payloadBytes,
		Timestamp: time.Now(),
	}
	h.broadcastToRoom(roomID, msg, userID)
}

func (c *Client) handleBroadcastStatusUpdate(msg *Message) {
	var payload struct {
		Status       string `json:"status"`
		CustomStatus string `json:"custom_status,omitempty"`
		Device       string `json:"device,omitempty"`
	}
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing broadcast status update payload",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"error", err)
			c.sendErrorMessage("Invalid payload format")
			return
		}
	}

	validStatuses := map[string]bool{
		"online": true, "idle": true, "away": true, "dnd": true, "offline": true,
	}
	if !validStatuses[payload.Status] {
		wsLogger.Warn("Invalid status in broadcast",
			"connection_id", c.connectionID,
			"user_id", c.userID,
			"status", payload.Status)
		c.sendErrorMessage("Invalid status")
		return
	}

	c.hub.setUserPresence(c.userID, c.roomID, payload.Status)
	c.hub.broadcastUserStatusUpdate(c.userID, c.roomID, payload.Status, payload.CustomStatus, payload.Device)

	wsLogger.Info("Broadcast status update",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"room_id", c.roomID,
		"status", payload.Status)
}

func (c *Client) handleMessageAck(msg *Message) {
	var payload MessageAckPayload
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing message ack payload",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"error", err)
			return
		}
	}

	wsLogger.Debug("Message ack received",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"message_id", payload.MessageID,
		"delivered", payload.Delivered,
		"sequence", payload.SequenceNum)

	ackResponse := map[string]interface{}{
		"message_id":   payload.MessageID,
		"server_time":  time.Now().UnixMilli(),
		"ack":          true,
		"sequence_num": payload.SequenceNum,
	}
	payloadBytes, _ := json.Marshal(ackResponse)
	responseMsg := &Message{
		Type:      "message-ack-confirmed",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   payloadBytes,
		Timestamp: time.Now(),
	}
	c.hub.sendToClient(c.userID, responseMsg)
}

func (c *Client) handleMessageDeliveryStatus(msg *Message) {
	var payload MessageDeliveryStatusPayload
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing message delivery status payload",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"error", err)
			return
		}
	}

	validStatuses := map[string]bool{
		"delivered": true, "read": true, "failed": true,
	}
	if !validStatuses[payload.Status] {
		wsLogger.Warn("Invalid delivery status",
			"connection_id", c.connectionID,
			"status", payload.Status)
		return
	}

	wsLogger.Debug("Message delivery status update",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"message_count", len(payload.MessageIDs),
		"status", payload.Status)

	notifyMsg := &Message{
		Type:      "message-delivery-status",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   msg.Payload,
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)

	if c.hub.enableRedis {
		c.hub.publishToRedis(notifyMsg)
	}
}

var userFocusState = struct {
	sync.RWMutex
	focus map[uuid.UUID]UserFocusPayload
}{
	focus: make(map[uuid.UUID]UserFocusPayload),
}

func (c *Client) handleUserFocus(msg *Message) {
	var payload UserFocusPayload
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing user focus payload",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"error", err)
			return
		}
	}

	validContexts := map[string]bool{
		"room": true, "channel": true, "thread": true,
		"dm": true, "settings": true, "profile": true,
	}
	if !validContexts[payload.FocusContext] {
		wsLogger.Warn("Invalid focus context",
			"connection_id", c.connectionID,
			"context", payload.FocusContext)
		return
	}

	userFocusState.Lock()
	if payload.IsFocused {
		userFocusState.focus[c.userID] = payload
	} else {
		delete(userFocusState.focus, c.userID)
	}
	userFocusState.Unlock()

	focusPayload := UserFocusPayload{
		FocusContext: payload.FocusContext,
		ContextID:    payload.ContextID,
		PanelID:      payload.PanelID,
		IsFocused:    payload.IsFocused,
	}
	payloadBytes, _ := json.Marshal(focusPayload)

	notifyMsg := &Message{
		Type:      "user-focus",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   payloadBytes,
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)

	wsLogger.Debug("User focus changed",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"context", payload.FocusContext,
		"panel", payload.PanelID,
		"focused", payload.IsFocused)
}

func (c *Client) handleUserFocusQuery(msg *Message) {
	var payload struct {
		UserIDs []uuid.UUID `json:"user_ids"`
	}
	if len(msg.Payload) > 0 {
		_ = json.Unmarshal(msg.Payload, &payload)
	}

	userFocusState.RLock()
	focusData := make(map[uuid.UUID]UserFocusPayload)
	for _, uid := range payload.UserIDs {
		if focus, exists := userFocusState.focus[uid]; exists {
			focusData[uid] = focus
		}
	}
	userFocusState.RUnlock()

	payloadBytes, _ := json.Marshal(focusData)
	responseMsg := &Message{
		Type:      "user-focus-data",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   payloadBytes,
		Timestamp: time.Now(),
	}
	c.hub.sendToClient(c.userID, responseMsg)
}

var mediaStateStore = struct {
	sync.RWMutex
	states map[uuid.UUID]MediaStatePayload
}{
	states: make(map[uuid.UUID]MediaStatePayload),
}

func (c *Client) handleMediaStateSync(msg *Message) {
	var payload MediaStatePayload
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing media state sync payload",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"error", err)
			return
		}
	}

	mediaStateStore.Lock()
	mediaStateStore.states[c.userID] = payload
	mediaStateStore.Unlock()

	payloadBytes, _ := json.Marshal(payload)
	notifyMsg := &Message{
		Type:      "media-state-update",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   payloadBytes,
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)

	if c.hub.enableRedis {
		c.hub.publishToRedis(notifyMsg)
	}

	wsLogger.Debug("Media state synced",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"muted", payload.Muted,
		"deafened", payload.Deafened,
		"screen_sharing", payload.ScreenSharing)
}

func (c *Client) handleMediaStateRequest(msg *Message) {
	var payload struct {
		UserIDs []uuid.UUID `json:"user_ids"`
	}
	if len(msg.Payload) > 0 {
		_ = json.Unmarshal(msg.Payload, &payload)
	}

	mediaStateStore.RLock()
	states := make(map[uuid.UUID]MediaStatePayload)
	for _, uid := range payload.UserIDs {
		if state, exists := mediaStateStore.states[uid]; exists {
			states[uid] = state
		}
	}
	mediaStateStore.RUnlock()

	payloadBytes, _ := json.Marshal(states)
	responseMsg := &Message{
		Type:      "media-state-data",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   payloadBytes,
		Timestamp: time.Now(),
	}
	c.hub.sendToClient(c.userID, responseMsg)

	wsLogger.Debug("Media state request handled",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"requested_users", len(payload.UserIDs),
		"returned_states", len(states))
}

func (c *Client) handleTypingPause(msg *Message) {
	var payload TypingPausePayload
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing typing-pause payload",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"error", err)
			c.sendErrorMessage("Invalid typing pause payload format")
			return
		}
	}

	wsLogger.Debug("User typing paused",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"context", payload.Context,
		"paused", payload.Paused)

	payloadBytes, _ := json.Marshal(payload)
	notifyMsg := &Message{
		Type:      "typing-pause",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   payloadBytes,
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)

	if c.hub.enableRedis {
		c.hub.publishToRedis(notifyMsg)
	}
}

func (c *Client) handleUserFocusWindow(msg *Message) {
	var payload UserFocusWindowPayload
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing user-focus-window payload",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"error", err)
			c.sendErrorMessage("Invalid focus window payload format")
			return
		}
	}

	validContexts := map[string]bool{
		"room": true, "channel": true, "thread": true, "dm": true,
	}
	if !validContexts[payload.Context] {
		wsLogger.Warn("Invalid focus context",
			"connection_id", c.connectionID,
			"user_id", c.userID,
			"context", payload.Context)
		return
	}

	msgType := "user-focus-out"
	if payload.Focused {
		msgType = "user-focus-in"
	}

	payloadBytes, _ := json.Marshal(payload)
	notifyMsg := &Message{
		Type:      msgType,
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   payloadBytes,
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)

	wsLogger.Debug("User focus window changed",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"context", payload.Context,
		"focused", payload.Focused)
}

func (c *Client) handleReactionAdd(msg *Message) {
	var payload ReactionPayload
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing reaction payload",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"error", err)
			c.sendErrorMessage("Invalid reaction payload format")
			return
		}
	}

	if payload.MessageID == "" || payload.Emoji == "" {
		wsLogger.Warn("Invalid reaction payload - missing required fields",
			"connection_id", c.connectionID,
			"user_id", c.userID,
			"message_id", payload.MessageID,
			"emoji", payload.Emoji)
		return
	}

	payloadBytes, _ := json.Marshal(payload)
	notifyMsg := &Message{
		Type:      "reaction-add",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   payloadBytes,
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)

	wsLogger.Debug("User added reaction",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"message_id", payload.MessageID,
		"emoji", payload.Emoji)
}

func (c *Client) handleReactionRemove(msg *Message) {
	var payload ReactionPayload
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing reaction remove payload",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"error", err)
			return
		}
	}

	if payload.MessageID == "" {
		wsLogger.Warn("Invalid reaction remove payload - missing message_id",
			"connection_id", c.connectionID,
			"user_id", c.userID)
		return
	}

	payloadBytes, _ := json.Marshal(payload)
	notifyMsg := &Message{
		Type:      "reaction-remove",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   payloadBytes,
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)

	wsLogger.Debug("User removed reaction",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"message_id", payload.MessageID,
		"emoji", payload.Emoji)
}

func (c *Client) handleMessagePin(msg *Message) {
	var payload MessagePinPayload
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing message pin payload",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"error", err)
			c.sendErrorMessage("Invalid pin payload format")
			return
		}
	}

	if payload.MessageID == "" {
		wsLogger.Warn("Invalid message pin payload - missing message_id",
			"connection_id", c.connectionID,
			"user_id", c.userID)
		return
	}

	msgType := "message-pinned"
	if payload.Action == "unpin" {
		msgType = "message-unpinned"
	}

	payloadBytes, _ := json.Marshal(payload)
	notifyMsg := &Message{
		Type:      msgType,
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   payloadBytes,
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)

	if c.hub.enableRedis {
		c.hub.publishToRedis(notifyMsg)
	}

	wsLogger.Info("Message pinned/unpinned",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"message_id", payload.MessageID,
		"action", payload.Action)
}

func (c *Client) handleUserViewingChannel(msg *Message) {
	var payload UserViewingChannelPayload
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing user-viewing-channel payload",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"error", err)
			return
		}
	}

	msgType := "user-viewing-channel"
	if !payload.Viewing {
		msgType = "user-left-channel"
	}

	payloadBytes, _ := json.Marshal(payload)
	notifyMsg := &Message{
		Type:      msgType,
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   payloadBytes,
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)

	wsLogger.Debug("User viewing channel changed",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"channel_id", payload.ChannelID,
		"viewing", payload.Viewing)
}

func (c *Client) handleRoomConfigUpdate(msg *Message) {
	var payload RoomConfigPayload
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			wsLogger.Warn("Error parsing room-config payload",
				"connection_id", c.connectionID,
				"user_id", c.userID,
				"error", err)
			c.sendErrorMessage("Invalid room config payload format")
			return
		}
	}

	validKeys := map[string]bool{
		"name": true, "description": true, "icon": true,
		"notifications": true, "slowmode": true,
	}
	if !validKeys[payload.Key] {
		wsLogger.Warn("Invalid room config key",
			"connection_id", c.connectionID,
			"user_id", c.userID,
			"key", payload.Key)
		return
	}

	notifyMsg := &Message{
		Type:      "room-config-updated",
		RoomID:    c.roomID,
		UserID:    c.userID,
		Payload:   msg.Payload,
		Timestamp: time.Now(),
	}
	c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)

	if c.hub.enableRedis {
		c.hub.publishToRedis(notifyMsg)
	}

	wsLogger.Info("Room config updated",
		"connection_id", c.connectionID,
		"user_id", c.userID,
		"room_id", c.roomID,
		"key", payload.Key)
}
