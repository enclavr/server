package websocket

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/enclavr/server/internal/metrics"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var messageBufferPool = sync.Pool{
	New: func() interface{} {
		buf := make([]byte, 0, 512)
		return &buf
	},
}

type ConnectionState int32

const (
	StateConnecting ConnectionState = iota
	StateConnected
	StateDisconnecting
	StateDisconnected
)

func (s ConnectionState) String() string {
	switch s {
	case StateConnecting:
		return "connecting"
	case StateConnected:
		return "connected"
	case StateDisconnecting:
		return "disconnecting"
	case StateDisconnected:
		return "disconnected"
	default:
		return "unknown"
	}
}

type TypingState struct {
	UserID    uuid.UUID
	RoomID    uuid.UUID
	StartedAt time.Time
	Context   string
}

type TypingPayload struct {
	Context   string `json:"context,omitempty"`
	ChannelID string `json:"channel_id,omitempty"`
	ThreadID  string `json:"thread_id,omitempty"`
}

type Hub struct {
	rooms           map[uuid.UUID]*room
	broadcast       chan *Message
	register        chan *Client
	unregister      chan *Client
	mutex           sync.RWMutex
	userConnections map[uuid.UUID]*Client
	roomMutexes     map[uuid.UUID]*sync.RWMutex

	shutdown      chan struct{}
	activeClients atomic.Int64
	totalMessages atomic.Int64
	startedAt     time.Time
	batchQueue    chan *batchItem
	batchTicker   *time.Ticker
	typingUsers   map[uuid.UUID]*TypingState
	typingMutex   sync.RWMutex
	typingTimeout time.Duration

	pubSub      *PubSubService
	enableRedis bool

	notificationSettings map[string]*RoomNotificationSettings
	notificationMutex    sync.RWMutex
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

type Message struct {
	Type         string          `json:"type"`
	RoomID       uuid.UUID       `json:"room_id,omitempty"`
	UserID       uuid.UUID       `json:"user_id,omitempty"`
	TargetUserID uuid.UUID       `json:"target_user_id,omitempty"`
	Payload      json.RawMessage `json:"payload,omitempty"`
	SDP          string          `json:"sdp,omitempty"`
	Candidate    string          `json:"candidate,omitempty"`
	Timestamp    time.Time       `json:"timestamp"`
}

type RateLimiter struct {
	mu         sync.Mutex
	messages   int
	resetTime  time.Time
	limit      int
	windowSecs int
}

func NewRateLimiter(limit int, windowSecs int) *RateLimiter {
	return &RateLimiter{
		limit:      limit,
		windowSecs: windowSecs,
		resetTime:  time.Now().Add(time.Duration(windowSecs) * time.Second),
	}
}

func (r *RateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if time.Now().After(r.resetTime) {
		r.messages = 0
		r.resetTime = time.Now().Add(time.Duration(r.windowSecs) * time.Second)
	}

	if r.messages >= r.limit {
		return false
	}
	r.messages++
	return true
}

type HubMetrics struct {
	ActiveClients int64         `json:"active_clients"`
	TotalMessages int64         `json:"total_messages"`
	Uptime        time.Duration `json:"uptime"`
	RoomCount     int           `json:"room_count"`
	RedisEnabled  bool          `json:"redis_enabled"`
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
		notificationSettings: make(map[string]*RoomNotificationSettings),
	}
	go hub.processBatch()
	go hub.cleanupTypingUsers()
	return hub
}

func NewHubWithRedis(redisHost, redisPassword string, redisDB int) (*Hub, error) {
	hub := NewHub()

	hub.pubSub = NewPubSubService(redisHost, redisPassword, redisDB)
	if err := hub.pubSub.Connect(); err != nil {
		return nil, err
	}

	if err := hub.pubSub.Subscribe("broadcast"); err != nil {
		log.Printf("Failed to subscribe to broadcast channel: %v", err)
	}

	hub.pubSub.RegisterHandler("user-message", hub.handleRedisUserMessage)
	hub.pubSub.RegisterHandler("room-message", hub.handleRedisRoomMessage)

	hub.enableRedis = true
	log.Println("WebSocket hub initialized with Redis pub/sub support")

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
			roomMutex := h.getRoomMutex(message.RoomID)
			if roomMutex != nil {
				roomMutex.RLock()
			}
			if r, ok := h.rooms[message.RoomID]; ok {
				r.mutex.RLock()
				for client := range r.clients {
					select {
					case client.send <- message.encode():
					default:
						close(client.send)
						delete(r.clients, client)
						h.activeClients.Add(-1)
					}
				}
				r.mutex.RUnlock()
				h.totalMessages.Add(1)
				metrics.MessagesSent.Add(float64(len(r.clients)))
				metrics.WebSocketMessagesTotal.WithLabelValues(message.Type, "sent").Add(float64(len(r.clients)))
			}
			if roomMutex != nil {
				roomMutex.RUnlock()
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
		roomMutex := h.getRoomMutex(roomID)
		if roomMutex != nil {
			roomMutex.RLock()
		}
		if r, ok := h.rooms[roomID]; ok {
			r.mutex.RLock()
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
			r.mutex.RUnlock()
		}
		if roomMutex != nil {
			roomMutex.RUnlock()
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
	log.Println("WebSocket hub shutting down...")
	h.batchTicker.Stop()

	h.mutex.Lock()
	defer h.mutex.Unlock()

	for _, r := range h.rooms {
		r.mutex.Lock()
		clientCount := len(r.clients)
		for client := range r.clients {
			close(client.send)
		}
		r.clients = make(map[*Client]bool)
		r.mutex.Unlock()
		h.activeClients.Add(-int64(clientCount))
	}

	h.rooms = make(map[uuid.UUID]*room)
	h.userConnections = make(map[uuid.UUID]*Client)
	h.roomMutexes = make(map[uuid.UUID]*sync.RWMutex)

	log.Println("WebSocket hub shutdown complete")
}

func (h *Hub) Shutdown() {
	log.Println("Shutting down WebSocket hub...")
	close(h.shutdown)

	h.mutex.Lock()
	defer h.mutex.Unlock()

	for _, r := range h.rooms {
		r.mutex.Lock()
		for client := range r.clients {
			close(client.send)
			delete(r.clients, client)
		}
		r.mutex.Unlock()
	}

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
		log.Printf("[WebSocket] User %s already connected, closing old connection (ID: %s)",
			userID, existingClient.connectionID)
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
	log.Printf("[Connection %s] Client registered: user=%s room=%s conn_remote=%s",
		client.connectionID, userID, roomID, remoteAddr)
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
	roomMutex := h.getRoomMutex(roomID)
	if roomMutex != nil {
		roomMutex.RLock()
	}
	if r, ok := h.rooms[roomID]; ok {
		r.mutex.RLock()
		for client := range r.clients {
			if client.userID != excludeUserID {
				select {
				case client.send <- msg.encode():
				default:
					close(client.send)
					delete(r.clients, client)
					h.activeClients.Add(-1)
				}
			}
		}
		r.mutex.RUnlock()
	}
	if roomMutex != nil {
		roomMutex.RUnlock()
	}
}

func (h *Hub) BroadcastToRoom(roomID uuid.UUID, msg *Message, excludeUserID uuid.UUID) {
	h.broadcastToRoom(roomID, msg, excludeUserID)
}

func (m *Message) encode() []byte {
	if m.Timestamp.IsZero() {
		m.Timestamp = time.Now()
	}

	bufPtr := messageBufferPool.Get().(*[]byte)
	defer messageBufferPool.Put(bufPtr)
	*bufPtr = (*bufPtr)[:0]

	data, err := json.Marshal(m)
	if err != nil {
		log.Println("Error encoding message:", err)
		return []byte{}
	}
	*bufPtr = append(*bufPtr, data...)

	result := make([]byte, len(*bufPtr))
	copy(result, *bufPtr)
	return result
}

func (m *Message) decode(data []byte) error {
	return json.Unmarshal(data, m)
}

func (c *Client) ReadPump() {
	defer func() {
		c.SetState(StateDisconnecting)
		c.hub.clearTypingUser(c.userID)
		c.hub.unregister <- c
		if closeErr := c.conn.Close(); closeErr != nil {
			log.Printf("[Connection %s] Error closing connection for user %s: %v", c.connectionID, c.userID, closeErr)
		}
		c.SetState(StateDisconnected)
		if lastErr := c.GetLastError(); lastErr != nil {
			log.Printf("[Connection %s] Connection closed for user %s (remote: %s, errors: %d, last_error: %v, packets_in: %d, bytes_in: %d)",
				c.connectionID, c.userID, c.remoteAddr, c.GetErrorCount(), lastErr, c.packetsIn.Load(), c.bytesIn.Load())
		} else {
			log.Printf("[Connection %s] Connection closed for user %s (remote: %s, errors: %d, packets_in: %d, bytes_in: %d)",
				c.connectionID, c.userID, c.remoteAddr, c.GetErrorCount(), c.packetsIn.Load(), c.bytesIn.Load())
		}
	}()

	c.SetState(StateConnecting)
	if c.conn != nil {
		c.remoteAddr = c.conn.RemoteAddr().String()
	}

	c.conn.SetReadLimit(512 * 1024)
	if err := c.conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
		log.Printf("[Connection %s] Error setting read deadline: %v", c.connectionID, err)
		c.RecordError(err)
		return
	}
	c.conn.SetPongHandler(func(string) error {
		if err := c.conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
			log.Printf("[Connection %s] Error setting read deadline in pong handler: %v", c.connectionID, err)
			c.RecordError(err)
			return err
		}
		return nil
	})

	c.SetState(StateConnected)
	log.Printf("[Connection %s] Client connected: user=%s room=%s from=%s",
		c.connectionID, c.userID, c.roomID, c.remoteAddr)

	for {
		messageType, message, err := c.conn.ReadMessage()
		if err != nil {
			c.packetsIn.Add(1)
			c.bytesIn.Add(int64(len(message)))

			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseTLSHandshake) {
				log.Printf("[Connection %s] WebSocket read error for user %s: %v (error_type: unexpected_close)",
					c.connectionID, c.userID, err)
				c.RecordError(err)
			} else {
				log.Printf("[Connection %s] WebSocket closed for user %s: %v", c.connectionID, c.userID, err)
			}
			break
		}

		c.packetsIn.Add(1)
		c.bytesIn.Add(int64(len(message)))

		if messageType == websocket.PongMessage {
			log.Printf("[Connection %s] Received pong from user %s", c.connectionID, c.userID)
			continue
		}

		if messageType == websocket.CloseMessage {
			log.Printf("[Connection %s] Received close message from user %s", c.connectionID, c.userID)
			break
		}

		if !c.rateLimit.Allow() {
			log.Printf("[Connection %s] Rate limit exceeded for user %s", c.connectionID, c.userID)
			c.RecordError(fmt.Errorf("rate limit exceeded"))
			continue
		}

		c.lastSeen.Store(time.Now().Unix())

		var msg Message
		if err := msg.decode(message); err != nil {
			log.Printf("[Connection %s] Error decoding message from user %s: %v (payload_size: %d)",
				c.connectionID, c.userID, err, len(message))
			c.RecordError(err)
			continue
		}

		msg.UserID = c.userID
		msg.RoomID = c.roomID
		msg.Timestamp = time.Now()

		c.handleMessage(&msg)
	}
}

func (c *Client) WritePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		if closeErr := c.conn.Close(); closeErr != nil {
			log.Printf("[Connection %s] Error closing connection in WritePump: %v", c.connectionID, closeErr)
		}
	}()

	for {
		select {
		case message, ok := <-c.send:
			if err := c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
				log.Printf("[Connection %s] Error setting write deadline: %v", c.connectionID, err)
				c.RecordError(err)
				return
			}
			if !ok {
				log.Printf("[Connection %s] Send channel closed, sending close message for user %s", c.connectionID, c.userID)
				if err := c.conn.WriteMessage(websocket.CloseMessage, []byte{}); err != nil {
					log.Printf("[Connection %s] Error sending close message: %v", c.connectionID, err)
					c.RecordError(err)
				}
				return
			}

			c.packetsOut.Add(1)
			c.bytesOut.Add(int64(len(message)))

			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				c.packetsOut.Add(1)
				c.bytesOut.Add(int64(len(message)))
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("[Connection %s] Unexpected close while writing to user %s: %v", c.connectionID, c.userID, err)
				} else {
					log.Printf("[Connection %s] Write error to user %s: %v", c.connectionID, c.userID, err)
				}
				c.RecordError(err)
				return
			}
			c.lastSeen.Store(time.Now().Unix())

		case <-ticker.C:
			if err := c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
				log.Printf("[Connection %s] Error setting write deadline in ping: %v", c.connectionID, err)
				c.RecordError(err)
				return
			}
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("[Connection %s] Error writing ping to user %s: %v", c.connectionID, c.userID, err)
				c.RecordError(err)
				return
			}
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

func (c *Client) handleMessage(msg *Message) {
	switch msg.Type {
	case "voice-offer", "voice-answer", "voice-ice-candidate":
		if msg.UserID != msg.TargetUserID {
			c.hub.sendToClient(msg.TargetUserID, msg)
		}
	case "voice-mute", "voice-unmute":
		c.hub.broadcast <- msg
	case "user-joined":
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
	case "user-left":
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
	case "user-away":
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
	case "user-back":
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
	case "typing":
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
	case "stop-typing":
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
	default:
		log.Printf("[Connection %s] Unknown message type: %s from user %s", c.connectionID, msg.Type, c.userID)
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

type PresenceState struct {
	UserID    uuid.UUID
	RoomID    uuid.UUID
	Status    string
	UpdatedAt time.Time
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
			onlineUsers = append(onlineUsers, OnlineUser{
				UserID:        client.userID,
				ConnectionID:  client.connectionID,
				State:         client.GetState().String(),
				ConnectedAt:   client.connectedAt,
				LastSeen:      client.GetLastSeen(),
				RemoteAddress: client.remoteAddr,
			})
		}
	}

	payload, err := json.Marshal(onlineUsers)
	if err != nil {
		log.Printf("Error marshaling online users: %v", err)
		return
	}

	msg := &Message{
		Type:      "online-users-list",
		RoomID:    roomID,
		Payload:   payload,
		Timestamp: time.Now(),
	}
	h.broadcastToRoom(roomID, msg, excludeUserID)
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
		log.Printf("Error marshaling notification settings: %v", err)
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
	log.Printf("[NotificationSettings] User %s updated settings for room %s: enabled=%v, sound=%v",
		userID, roomID, settings.Enabled, settings.Sound)
}

type RoomState struct {
	RoomID        uuid.UUID    `json:"room_id"`
	OnlineUsers   []OnlineUser `json:"online_users"`
	TypingUsers   []uuid.UUID  `json:"typing_users"`
	ActiveClients int          `json:"active_clients"`
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
		log.Printf("[Connection %s] Error marshaling room state: %v", client.connectionID, err)
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
		log.Printf("[Connection %s] Error marshaling typing users: %v", client.connectionID, err)
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

type OnlineUser struct {
	UserID        uuid.UUID `json:"user_id"`
	ConnectionID  uuid.UUID `json:"connection_id"`
	State         string    `json:"state"`
	ConnectedAt   time.Time `json:"connected_at"`
	LastSeen      time.Time `json:"last_seen"`
	RemoteAddress string    `json:"remote_address,omitempty"`
	Speaking      bool      `json:"speaking,omitempty"`
	Muted         bool      `json:"muted,omitempty"`
	Deafened      bool      `json:"deafened,omitempty"`
	ScreenSharing bool      `json:"screen_sharing,omitempty"`
}

type RoomNotificationSettings struct {
	Enabled       bool `json:"enabled"`
	Sound         bool `json:"sound"`
	MentionOnly   bool `json:"mention_only"`
	DesktopNotify bool `json:"desktop_notify"`
}

type ConnectionHealth struct {
	ConnectionID    uuid.UUID `json:"connection_id"`
	UserID          uuid.UUID `json:"user_id"`
	LatencyMs       int64     `json:"latency_ms"`
	State           string    `json:"state"`
	LastPing        time.Time `json:"last_ping"`
	LastPong        time.Time `json:"last_pong"`
	PacketsReceived int64     `json:"packets_received"`
	PacketsSent     int64     `json:"packets_sent"`
	BytesReceived   int64     `json:"bytes_received"`
	BytesSent       int64     `json:"bytes_sent"`
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
	log.Printf("[Connection %s] User %s started screen share in room %s",
		c.connectionID, c.userID, c.roomID)

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
	log.Printf("[Connection %s] User %s stopped screen share in room %s",
		c.connectionID, c.userID, c.roomID)

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
	log.Printf("[Connection %s] User %s muted status changed: muted=%v",
		c.connectionID, c.userID, muted)

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
	log.Printf("[Connection %s] User %s deafened status changed: deafened=%v",
		c.connectionID, c.userID, deafened)

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
		log.Printf("[Connection %s] Error marshaling health: %v", c.connectionID, err)
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
		log.Printf("[Connection %s] Error marshaling room users: %v", client.connectionID, err)
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
