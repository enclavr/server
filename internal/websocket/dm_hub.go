package websocket

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

type DMHub struct {
	conversations     map[string]*DMConversation
	conversationMutex sync.RWMutex
	userConnections   map[uuid.UUID]*DMClient
	connectionMutex   sync.RWMutex
	broadcast         chan *DMMessage
	register          chan *DMClient
	unregister        chan *DMClient
	shutdown          chan struct{}
	activeClients     atomic.Int64
	messageSequences  map[string]int64
	sequenceMutex     sync.Mutex

	typingUsers   map[string]map[uuid.UUID]*DMTypingState
	typingMutex   sync.RWMutex
	typingTimeout time.Duration

	blockedUsers map[uuid.UUID]map[uuid.UUID]bool
	blockMutex   sync.RWMutex
}

type DMConversation struct {
	ID           string
	Participants []uuid.UUID
	Clients      map[*DMClient]bool
	Mutex        sync.RWMutex
	LastActivity time.Time
}

type DMClient struct {
	userID         uuid.UUID
	conversationID string
	send           chan []byte
	lastSeen       atomic.Int64
}

type DMMessage struct {
	Type           string          `json:"type"`
	ConversationID string          `json:"conversation_id,omitempty"`
	UserID         uuid.UUID       `json:"user_id,omitempty"`
	TargetUserID   uuid.UUID       `json:"target_user_id,omitempty"`
	Payload        json.RawMessage `json:"payload,omitempty"`
	Sequence       int64           `json:"sequence,omitempty"`
	Timestamp      time.Time       `json:"timestamp"`
}

type DMTypingState struct {
	UserID    uuid.UUID
	StartedAt time.Time
	Context   string
}

type DMDeliveryStatus struct {
	MessageID   string    `json:"message_id"`
	Status      string    `json:"status"`
	SentAt      time.Time `json:"sent_at"`
	DeliveredAt time.Time `json:"delivered_at,omitempty"`
	ReadAt      time.Time `json:"read_at,omitempty"`
}

const (
	DMTypingTimeout    = 5 * time.Second
	DMTypingThrottleMs = 2000
)

func NewDMHub() *DMHub {
	hub := &DMHub{
		conversations:    make(map[string]*DMConversation),
		userConnections:  make(map[uuid.UUID]*DMClient),
		broadcast:        make(chan *DMMessage, 256),
		register:         make(chan *DMClient),
		unregister:       make(chan *DMClient),
		shutdown:         make(chan struct{}),
		messageSequences: make(map[string]int64),
		typingUsers:      make(map[string]map[uuid.UUID]*DMTypingState),
		blockedUsers:     make(map[uuid.UUID]map[uuid.UUID]bool),
		typingTimeout:    DMTypingTimeout,
	}
	go hub.run()
	go hub.cleanupTypingUsers()
	return hub
}

func (h *DMHub) run() {
	for {
		select {
		case <-h.shutdown:
			return
		case client := <-h.register:
			h.registerClient(client)
		case client := <-h.unregister:
			h.unregisterClient(client)
		case message := <-h.broadcast:
			h.broadcastMessage(message)
		}
	}
}

func (h *DMHub) registerClient(client *DMClient) {
	h.connectionMutex.Lock()
	h.userConnections[client.userID] = client
	h.connectionMutex.Unlock()

	if client.conversationID != "" {
		h.conversationMutex.Lock()
		if h.conversations[client.conversationID] == nil {
			h.conversations[client.conversationID] = &DMConversation{
				ID:           client.conversationID,
				Participants: []uuid.UUID{},
				Clients:      make(map[*DMClient]bool),
				LastActivity: time.Now(),
			}
		}
		conv := h.conversations[client.conversationID]
		conv.Mutex.Lock()
		conv.Clients[client] = true
		conv.Mutex.Unlock()
		h.conversationMutex.Unlock()
	}

	h.activeClients.Add(1)
	log.Printf("[DMHub] Client registered: user=%s, conv=%s", client.userID, client.conversationID)
}

func (h *DMHub) unregisterClient(client *DMClient) {
	h.connectionMutex.Lock()
	delete(h.userConnections, client.userID)
	h.connectionMutex.Unlock()

	if client.conversationID != "" {
		h.conversationMutex.Lock()
		if conv, ok := h.conversations[client.conversationID]; ok {
			conv.Mutex.Lock()
			delete(conv.Clients, client)
			if len(conv.Clients) == 0 {
				delete(h.conversations, client.conversationID)
			}
			conv.Mutex.Unlock()
		}
		h.conversationMutex.Unlock()
	}

	h.activeClients.Add(-1)
	close(client.send)
	log.Printf("[DMHub] Client unregistered: user=%s", client.userID)
}

func (h *DMHub) broadcastMessage(msg *DMMessage) {
	if msg.ConversationID == "" {
		return
	}

	h.conversationMutex.RLock()
	conv := h.conversations[msg.ConversationID]
	if conv == nil {
		h.conversationMutex.RUnlock()
		return
	}
	conv.Mutex.RLock()
	clients := make([]*DMClient, 0, len(conv.Clients))
	for client := range conv.Clients {
		clients = append(clients, client)
	}
	conv.Mutex.RUnlock()
	h.conversationMutex.RUnlock()

	msg.Sequence = h.getNextSequence(msg.ConversationID)
	msg.Timestamp = time.Now()

	for _, client := range clients {
		if h.isBlocked(client.userID, msg.UserID) {
			continue
		}
		select {
		case client.send <- msg.encode():
		default:
			h.unregister <- client
		}
	}
}

func (h *DMHub) getNextSequence(convID string) int64 {
	h.sequenceMutex.Lock()
	defer h.sequenceMutex.Unlock()

	current := h.messageSequences[convID]
	nextSeq := current + 1
	h.messageSequences[convID] = nextSeq
	return nextSeq
}

func (h *DMHub) HandleMessage(client *DMClient, data []byte) {
	var msg DMMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("[DMHub] Error unmarshaling message: %v", err)
		return
	}

	msg.UserID = client.userID
	client.lastSeen.Store(time.Now().Unix())

	switch msg.Type {
	case "heartbeat":
		h.handleHeartbeat(client, &msg)
	case "typing":
		h.handleTyping(client, &msg)
	case "stop-typing":
		h.handleStopTyping(client, &msg)
	case "mark-read":
		h.handleMarkRead(client, &msg)
	case "mark-delivered":
		h.handleMarkDelivered(client, &msg)
	case "join-conversation":
		h.handleJoinConversation(client, &msg)
	case "leave-conversation":
		h.handleLeaveConversation(client, &msg)
	default:
		log.Printf("[DMHub] Unknown message type: %s", msg.Type)
	}
}

func (h *DMHub) handleHeartbeat(client *DMClient, msg *DMMessage) {
	response := &DMMessage{
		Type:      "heartbeat-ack",
		Timestamp: time.Now(),
	}
	client.send <- response.encode()
}

func (h *DMHub) handleTyping(client *DMClient, msg *DMMessage) {
	if client.conversationID == "" {
		return
	}

	h.typingMutex.Lock()
	if h.typingUsers[client.conversationID] == nil {
		h.typingUsers[client.conversationID] = make(map[uuid.UUID]*DMTypingState)
	}

	h.typingUsers[client.conversationID][client.userID] = &DMTypingState{
		UserID:    client.userID,
		StartedAt: time.Now(),
		Context:   client.conversationID,
	}
	h.typingMutex.Unlock()

	typingMsg := &DMMessage{
		Type:           "dm-typing",
		ConversationID: client.conversationID,
		UserID:         client.userID,
		Timestamp:      time.Now(),
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"user_id":  client.userID.String(),
		"username": "User",
	})
	typingMsg.Payload = payload

	h.broadcastToConversation(client.conversationID, typingMsg, client.userID)
}

func (h *DMHub) handleStopTyping(client *DMClient, msg *DMMessage) {
	if client.conversationID == "" {
		return
	}

	h.typingMutex.Lock()
	if typingUsers, ok := h.typingUsers[client.conversationID]; ok {
		delete(typingUsers, client.userID)
	}
	h.typingMutex.Unlock()

	stopMsg := &DMMessage{
		Type:           "dm-stop-typing",
		ConversationID: client.conversationID,
		UserID:         client.userID,
		Timestamp:      time.Now(),
	}

	h.broadcastToConversation(client.conversationID, stopMsg, client.userID)
}

func (h *DMHub) handleMarkRead(client *DMClient, msg *DMMessage) {
	var payload struct {
		MessageID string `json:"message_id"`
	}
	if len(msg.Payload) > 0 {
		_ = json.Unmarshal(msg.Payload, &payload)
	}

	if client.conversationID == "" {
		return
	}

	readMsg := &DMMessage{
		Type:           "dm-message-read",
		ConversationID: client.conversationID,
		UserID:         client.userID,
		Timestamp:      time.Now(),
	}

	readPayload, _ := json.Marshal(map[string]interface{}{
		"message_id": payload.MessageID,
		"read_at":    time.Now().Format(time.RFC3339),
	})
	readMsg.Payload = readPayload

	h.broadcastToConversation(client.conversationID, readMsg, client.userID)
}

func (h *DMHub) handleMarkDelivered(client *DMClient, msg *DMMessage) {
	var payload struct {
		MessageID string `json:"message_id"`
	}
	if len(msg.Payload) > 0 {
		_ = json.Unmarshal(msg.Payload, &payload)
	}

	if client.conversationID == "" {
		return
	}

	deliveredMsg := &DMMessage{
		Type:           "dm-message-delivered",
		ConversationID: client.conversationID,
		UserID:         client.userID,
		Timestamp:      time.Now(),
	}

	deliveredPayload, _ := json.Marshal(map[string]interface{}{
		"message_id":   payload.MessageID,
		"delivered_at": time.Now().Format(time.RFC3339),
	})
	deliveredMsg.Payload = deliveredPayload

	h.broadcastToConversation(client.conversationID, deliveredMsg, client.userID)
}

func (h *DMHub) handleJoinConversation(client *DMClient, msg *DMMessage) {
	convID := msg.ConversationID
	if convID == "" {
		var payload struct {
			ConversationID string `json:"conversation_id"`
		}
		if len(msg.Payload) > 0 {
			_ = json.Unmarshal(msg.Payload, &payload)
			convID = payload.ConversationID
		}
	}

	if convID == "" {
		return
	}

	oldConvID := client.conversationID
	if oldConvID != "" && oldConvID != convID {
		h.handleLeaveConversation(client, &DMMessage{ConversationID: oldConvID})
	}

	client.conversationID = convID

	h.conversationMutex.Lock()
	if h.conversations[convID] == nil {
		h.conversations[convID] = &DMConversation{
			ID:           convID,
			Participants: []uuid.UUID{},
			Clients:      make(map[*DMClient]bool),
			LastActivity: time.Now(),
		}
	}
	conv := h.conversations[convID]
	conv.Mutex.Lock()
	conv.Clients[client] = true
	conv.Mutex.Unlock()
	h.conversationMutex.Unlock()

	response := &DMMessage{
		Type:           "conversation-joined",
		ConversationID: convID,
		UserID:         client.userID,
		Timestamp:      time.Now(),
	}
	client.send <- response.encode()

	log.Printf("[DMHub] Client joined conversation: user=%s, conv=%s", client.userID, convID)
}

func (h *DMHub) handleLeaveConversation(client *DMClient, msg *DMMessage) {
	convID := msg.ConversationID
	if convID == "" {
		return
	}

	h.conversationMutex.Lock()
	if conv, ok := h.conversations[convID]; ok {
		conv.Mutex.Lock()
		delete(conv.Clients, client)
		if len(conv.Clients) == 0 {
			delete(h.conversations, convID)
		}
		conv.Mutex.Unlock()
	}
	h.conversationMutex.Unlock()

	client.conversationID = ""

	log.Printf("[DMHub] Client left conversation: user=%s, conv=%s", client.userID, convID)
}

func (h *DMHub) broadcastToConversation(convID string, msg *DMMessage, excludeUserID uuid.UUID) {
	h.conversationMutex.RLock()
	conv := h.conversations[convID]
	if conv == nil {
		h.conversationMutex.RUnlock()
		return
	}
	conv.Mutex.RLock()
	clients := make([]*DMClient, 0, len(conv.Clients))
	for client := range conv.Clients {
		if excludeUserID == uuid.Nil || client.userID != excludeUserID {
			clients = append(clients, client)
		}
	}
	conv.Mutex.RUnlock()
	h.conversationMutex.RUnlock()

	msg.Sequence = h.getNextSequence(convID)
	msg.Timestamp = time.Now()

	for _, client := range clients {
		select {
		case client.send <- msg.encode():
		default:
			h.unregister <- client
		}
	}
}

func (h *DMHub) BlockUser(blockerID, blockedID uuid.UUID) {
	h.blockMutex.Lock()
	defer h.blockMutex.Unlock()

	if h.blockedUsers[blockerID] == nil {
		h.blockedUsers[blockerID] = make(map[uuid.UUID]bool)
	}
	h.blockedUsers[blockerID][blockedID] = true
}

func (h *DMHub) UnblockUser(blockerID, blockedID uuid.UUID) {
	h.blockMutex.Lock()
	defer h.blockMutex.Unlock()

	if h.blockedUsers[blockerID] != nil {
		delete(h.blockedUsers[blockerID], blockedID)
	}
}

func (h *DMHub) isBlocked(blockerID, userID uuid.UUID) bool {
	h.blockMutex.RLock()
	defer h.blockMutex.RUnlock()

	if h.blockedUsers[blockerID] != nil {
		return h.blockedUsers[blockerID][userID]
	}
	return false
}

func (h *DMHub) cleanupTypingUsers() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-h.shutdown:
			return
		case <-ticker.C:
			h.typingMutex.Lock()
			now := time.Now()
			for convID, users := range h.typingUsers {
				for userID, state := range users {
					if now.Sub(state.StartedAt) > h.typingTimeout {
						delete(users, userID)
					}
				}
				if len(users) == 0 {
					delete(h.typingUsers, convID)
				}
			}
			h.typingMutex.Unlock()
		}
	}
}

func (h *DMHub) Shutdown() {
	close(h.shutdown)
}

func (m *DMMessage) encode() []byte {
	data, err := json.Marshal(m)
	if err != nil {
		return []byte{}
	}
	return data
}

func (c *DMClient) GetConversationID() string {
	return c.conversationID
}

func (c *DMClient) GetUserID() uuid.UUID {
	return c.userID
}

func GenerateConversationID(userID1, userID2 uuid.UUID) string {
	if userID1.String() < userID2.String() {
		return fmt.Sprintf("%s-%s", userID1, userID2)
	}
	return fmt.Sprintf("%s-%s", userID2, userID1)
}

func NewDMClient(userID uuid.UUID, conversationID string, sendChan chan []byte) *DMClient {
	client := &DMClient{
		userID:         userID,
		conversationID: conversationID,
		send:           sendChan,
	}
	return client
}

func (c *DMClient) SetUserID(id uuid.UUID) {
	c.userID = id
}

func (c *DMClient) SetConversationID(id string) {
	c.conversationID = id
}

func (c *DMClient) SetSend(ch chan []byte) {
	c.send = ch
}

func (c *DMClient) GetSend() chan []byte {
	return c.send
}

func (h *DMHub) RegisterClient(client *DMClient) {
	h.register <- client
}

func (h *DMHub) UnregisterClient(client *DMClient) {
	h.unregister <- client
}

func (h *DMHub) GetActiveClients() int64 {
	return h.activeClients.Load()
}
