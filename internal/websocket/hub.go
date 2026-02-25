package websocket

import (
	"encoding/json"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var messageBufferPool = sync.Pool{
	New: func() interface{} {
		buf := make([]byte, 0, 512)
		return &buf
	},
}

type room struct {
	clients    map[*Client]bool
	mutex      sync.RWMutex
	lastAccess int64
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
}

type batchItem struct {
	message       *Message
	excludeUserID uuid.UUID
	roomID        uuid.UUID
	result        chan []byte
}

type Client struct {
	hub       *Hub
	conn      *websocket.Conn
	send      chan []byte
	userID    uuid.UUID
	roomID    uuid.UUID
	lastSeen  atomic.Int64
	rateLimit *RateLimiter
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
	}
	go hub.processBatch()
	return hub
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
					}
				}
				r.mutex.Unlock()
			}
			delete(h.userConnections, client.userID)
			h.activeClients.Add(-1)
			h.mutex.Unlock()

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
		for client := range r.clients {
			close(client.send)
		}
		r.clients = make(map[*Client]bool)
		r.mutex.Unlock()
	}

	h.rooms = make(map[uuid.UUID]*room)
	h.userConnections = make(map[uuid.UUID]*Client)
	h.roomMutexes = make(map[uuid.UUID]*sync.RWMutex)

	log.Println("WebSocket hub shutdown complete")
}

func (h *Hub) Shutdown() {
	close(h.shutdown)
}

func (h *Hub) GetMetrics() HubMetrics {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	return HubMetrics{
		ActiveClients: h.activeClients.Load(),
		TotalMessages: h.totalMessages.Load(),
		Uptime:        time.Since(h.startedAt),
		RoomCount:     len(h.rooms),
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
		hub:       h,
		conn:      conn,
		send:      make(chan []byte, 256),
		userID:    userID,
		roomID:    roomID,
		rateLimit: NewRateLimiter(30, 10),
	}

	h.register <- client
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
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(512 * 1024)
	if err := c.conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
		log.Printf("Error setting read deadline: %v", err)
		return
	}
	c.conn.SetPongHandler(func(string) error {
		if err := c.conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
			log.Printf("Error setting read deadline: %v", err)
			return err
		}
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		if !c.rateLimit.Allow() {
			log.Printf("Rate limit exceeded for user %s", c.userID)
			continue
		}

		c.lastSeen.Store(time.Now().Unix())

		var msg Message
		if err := msg.decode(message); err != nil {
			log.Printf("Error decoding message: %v", err)
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
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			if err := c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
				log.Printf("Error setting write deadline: %v", err)
				return
			}
			if !ok {
				if err := c.conn.WriteMessage(websocket.CloseMessage, []byte{}); err != nil {
					log.Printf("Error sending close message: %v", err)
				}
				return
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Printf("Error writing message: %v", err)
				return
			}
			c.lastSeen.Store(time.Now().Unix())

		case <-ticker.C:
			if err := c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
				log.Printf("Error setting write deadline: %v", err)
				return
			}
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("Error writing ping: %v", err)
				return
			}
		}
	}
}

func (c *Client) GetLastSeen() time.Time {
	return time.Unix(c.lastSeen.Load(), 0)
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
	case "user-left":
		notifyMsg := &Message{
			Type:      "user-left",
			RoomID:    c.roomID,
			UserID:    c.userID,
			Timestamp: time.Now(),
		}
		c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)
		c.hub.UnregisterClient(c)
	case "typing":
		notifyMsg := &Message{
			Type:      "user-typing",
			RoomID:    c.roomID,
			UserID:    c.userID,
			Timestamp: time.Now(),
		}
		c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)
	case "stop-typing":
		notifyMsg := &Message{
			Type:      "user-stopped-typing",
			RoomID:    c.roomID,
			UserID:    c.userID,
			Timestamp: time.Now(),
		}
		c.hub.broadcastToRoom(c.roomID, notifyMsg, c.userID)
	default:
		log.Printf("Unknown message type: %s", msg.Type)
	}
}
