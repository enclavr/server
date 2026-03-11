package websocket

import (
	"encoding/json"
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
	hub          *Hub
	conn         *websocket.Conn
	send         chan []byte
	userID       uuid.UUID
	roomID       uuid.UUID
	lastSeen     atomic.Int64
	rateLimit    *RateLimiter
	connectionID uuid.UUID
	state        atomic.Int32
	remoteAddr   string
	connectedAt  time.Time
	errorCount   atomic.Int32
	lastError    atomic.Value
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
		rooms:           make(map[uuid.UUID]*room),
		broadcast:       make(chan *Message, 256),
		register:        make(chan *Client),
		unregister:      make(chan *Client),
		userConnections: make(map[uuid.UUID]*Client),
		roomMutexes:     make(map[uuid.UUID]*sync.RWMutex),
		shutdown:        make(chan struct{}),
		startedAt:       time.Now(),
		batchQueue:      make(chan *batchItem, 512),
		batchTicker:     time.NewTicker(50 * time.Millisecond),
		enableRedis:     false,
		typingUsers:     make(map[uuid.UUID]*TypingState),
		typingTimeout:   5 * time.Second,
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
	client := &Client{
		hub:          h,
		conn:         conn,
		send:         make(chan []byte, 256),
		userID:       userID,
		roomID:       roomID,
		rateLimit:    NewRateLimiter(30, 10),
		connectionID: uuid.New(),
	}

	h.register <- client
	metrics.WebSocketConnections.Inc()
	metrics.ActiveUsers.Inc()
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
		c.hub.unregister <- c
		if closeErr := c.conn.Close(); closeErr != nil {
			log.Printf("[Connection %s] Error closing connection for user %s: %v", c.connectionID, c.userID, closeErr)
		}
		c.SetState(StateDisconnected)
		log.Printf("[Connection %s] Connection closed for user %s (remote: %s, errors: %d)",
			c.connectionID, c.userID, c.remoteAddr, c.GetErrorCount())
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
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseTLSHandshake) {
				log.Printf("[Connection %s] WebSocket read error for user %s: %v", c.connectionID, c.userID, err)
				c.RecordError(err)
			} else {
				log.Printf("[Connection %s] WebSocket closed for user %s: %v", c.connectionID, c.userID, err)
			}
			break
		}

		if !c.rateLimit.Allow() {
			log.Printf("[Connection %s] Rate limit exceeded for user %s", c.connectionID, c.userID)
			continue
		}

		c.lastSeen.Store(time.Now().Unix())

		var msg Message
		if err := msg.decode(message); err != nil {
			log.Printf("[Connection %s] Error decoding message from user %s: %v", c.connectionID, c.userID, err)
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

			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Printf("[Connection %s] Error writing message to user %s: %v", c.connectionID, c.userID, err)
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
}

func (c *Client) GetErrorCount() int32 {
	return c.errorCount.Load()
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
	case "typing":
		c.hub.setTypingUser(c.userID, c.roomID)
		notifyMsg := &Message{
			Type:      "user-typing",
			RoomID:    c.roomID,
			UserID:    c.userID,
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
	default:
		log.Printf("[Connection %s] Unknown message type: %s from user %s", c.connectionID, msg.Type, c.userID)
	}
}

func (h *Hub) setTypingUser(userID, roomID uuid.UUID) {
	h.typingMutex.Lock()
	defer h.typingMutex.Unlock()
	h.typingUsers[userID] = &TypingState{
		UserID:    userID,
		RoomID:    roomID,
		StartedAt: time.Now(),
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
	msg := &Message{
		Type:      "room-notifications",
		RoomID:    roomID,
		UserID:    userID,
		Payload:   json.RawMessage(`{"enabled":true,"sound":true}`),
		Timestamp: time.Now(),
	}
	h.sendToClient(userID, msg)
}

type OnlineUser struct {
	UserID        uuid.UUID `json:"user_id"`
	ConnectionID  uuid.UUID `json:"connection_id"`
	State         string    `json:"state"`
	ConnectedAt   time.Time `json:"connected_at"`
	LastSeen      time.Time `json:"last_seen"`
	RemoteAddress string    `json:"remote_address,omitempty"`
}

type RoomNotificationSettings struct {
	Enabled bool `json:"enabled"`
	Sound   bool `json:"sound"`
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
