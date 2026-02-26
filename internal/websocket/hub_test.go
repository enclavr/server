package websocket

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewHub(t *testing.T) {
	hub := NewHub()

	if hub == nil {
		t.Fatal("NewHub returned nil")
	}

	if hub.rooms == nil {
		t.Error("hub.rooms is nil")
	}

	if hub.broadcast == nil {
		t.Error("hub.broadcast is nil")
	}

	if hub.register == nil {
		t.Error("hub.register is nil")
	}

	if hub.unregister == nil {
		t.Error("hub.unregister is nil")
	}

	if hub.userConnections == nil {
		t.Error("hub.userConnections is nil")
	}

	if hub.activeClients.Load() != 0 {
		t.Errorf("expected 0 active clients, got %d", hub.activeClients.Load())
	}

	hub.Shutdown()
}

func TestNewRateLimiter(t *testing.T) {
	tests := []struct {
		name       string
		limit      int
		windowSecs int
	}{
		{"standard", 30, 10},
		{"strict", 10, 5},
		{"lenient", 100, 60},
		{"single", 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rl := NewRateLimiter(tt.limit, tt.windowSecs)

			if rl.limit != tt.limit {
				t.Errorf("expected limit %d, got %d", tt.limit, rl.limit)
			}

			if rl.windowSecs != tt.windowSecs {
				t.Errorf("expected windowSecs %d, got %d", tt.windowSecs, rl.windowSecs)
			}

			if rl.messages != 0 {
				t.Errorf("expected 0 messages, got %d", rl.messages)
			}
		})
	}
}

func TestRateLimiter_Allow(t *testing.T) {
	rl := NewRateLimiter(3, 10)

	for i := 0; i < 3; i++ {
		if !rl.Allow() {
			t.Errorf("expected Allow() to return true on call %d", i+1)
		}
	}

	if rl.Allow() {
		t.Error("expected Allow() to return false after limit exceeded")
	}

	if rl.messages != 3 {
		t.Errorf("expected messages to be 3, got %d", rl.messages)
	}
}

func TestRateLimiter_Reset(t *testing.T) {
	rl := NewRateLimiter(2, 1)

	rl.Allow()
	rl.Allow()

	if rl.Allow() {
		t.Error("expected false when limit exceeded")
	}

	time.Sleep(1100 * time.Millisecond)

	if !rl.Allow() {
		t.Error("expected true after reset")
	}
}

func TestRateLimiter_Concurrent(t *testing.T) {
	rl := NewRateLimiter(100, 10)
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			rl.Allow()
		}()
		go func() {
			defer wg.Done()
			rl.Allow()
		}()
	}

	wg.Wait()
}

func TestHub_RegisterClient(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID := uuid.New()
	roomID := uuid.New()

	hub.mutex.Lock()
	hub.userConnections[userID] = &Client{
		hub:    hub,
		userID: userID,
		roomID: roomID,
	}
	hub.activeClients.Add(1)
	hub.mutex.Unlock()

	if hub.GetClientCount() != 1 {
		t.Errorf("expected 1 client, got %d", hub.GetClientCount())
	}

	client := hub.GetUserClient(userID)
	if client == nil {
		t.Error("expected to get client by user ID")
		return
	}

	if client.userID != userID {
		t.Errorf("expected userID %s, got %s", userID, client.userID)
	}
}

func TestHub_GetRoomCount(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	if hub.GetRoomCount() != 0 {
		t.Errorf("expected 0 rooms, got %d", hub.GetRoomCount())
	}

	roomID := uuid.New()
	hub.mutex.Lock()
	hub.rooms[roomID] = &room{
		clients: make(map[*Client]bool),
	}
	hub.mutex.Unlock()

	if hub.GetRoomCount() != 1 {
		t.Errorf("expected 1 room, got %d", hub.GetRoomCount())
	}
}

func TestHub_GetOnlineUsers(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	users := hub.GetOnlineUsers()
	if len(users) != 0 {
		t.Errorf("expected 0 online users, got %d", len(users))
	}

	userID1 := uuid.New()
	userID2 := uuid.New()

	hub.mutex.Lock()
	hub.userConnections[userID1] = &Client{hub: hub, userID: userID1}
	hub.userConnections[userID2] = &Client{hub: hub, userID: userID2}
	hub.mutex.Unlock()

	users = hub.GetOnlineUsers()
	if len(users) != 2 {
		t.Errorf("expected 2 online users, got %d", len(users))
	}
}

func TestHub_GetRoomClients(t *testing.T) {
	hub := NewHub()
	roomID := uuid.New()
	userID := uuid.New()

	clients := hub.GetRoomClients(roomID)
	if len(clients) != 0 {
		t.Errorf("expected 0 clients in empty room, got %d", len(clients))
	}

	hub.mutex.Lock()
	hub.rooms[roomID] = &room{
		clients: make(map[*Client]bool),
	}
	client := &Client{hub: hub, userID: userID, roomID: roomID, send: make(chan []byte, 10)}
	hub.rooms[roomID].clients[client] = true
	hub.userConnections[userID] = client
	hub.mutex.Unlock()

	clients = hub.GetRoomClients(roomID)
	if len(clients) != 1 {
		t.Errorf("expected 1 client, got %d", len(clients))
	}

	if clients[0].userID != userID {
		t.Errorf("expected userID %s, got %s", userID, clients[0].userID)
	}

	hub.Shutdown()
}

func TestHub_GetMetrics(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	metrics := hub.GetMetrics()

	if metrics.ActiveClients != 0 {
		t.Errorf("expected 0 active clients, got %d", metrics.ActiveClients)
	}

	if metrics.TotalMessages != 0 {
		t.Errorf("expected 0 total messages, got %d", metrics.TotalMessages)
	}

	if metrics.RoomCount != 0 {
		t.Errorf("expected 0 rooms, got %d", metrics.RoomCount)
	}

	if metrics.RedisEnabled {
		t.Error("expected Redis to be disabled")
	}

	if metrics.Uptime < 0 {
		t.Error("expected uptime to be positive")
	}
}

func TestHub_BroadcastToRoom(t *testing.T) {
	hub := NewHub()

	roomID := uuid.New()
	userID1 := uuid.New()
	userID2 := uuid.New()

	hub.mutex.Lock()
	hub.rooms[roomID] = &room{
		clients: make(map[*Client]bool),
	}

	client1 := &Client{
		hub:    hub,
		userID: userID1,
		roomID: roomID,
		send:   make(chan []byte, 10),
	}
	client2 := &Client{
		hub:    hub,
		userID: userID2,
		roomID: roomID,
		send:   make(chan []byte, 10),
	}

	hub.rooms[roomID].clients[client1] = true
	hub.rooms[roomID].clients[client2] = true
	hub.userConnections[userID1] = client1
	hub.userConnections[userID2] = client2
	hub.activeClients.Add(2)
	hub.mutex.Unlock()

	msg := &Message{
		Type:      "test-message",
		RoomID:    roomID,
		UserID:    userID1,
		Timestamp: time.Now(),
	}

	hub.BroadcastToRoom(roomID, msg, uuid.Nil)

	select {
	case received := <-client1.send:
		var m Message
		if err := json.Unmarshal(received, &m); err != nil {
			t.Errorf("failed to unmarshal message: %v", err)
		}
		if m.Type != "test-message" {
			t.Errorf("expected type 'test-message', got '%s'", m.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for message on client1")
	}

	select {
	case received := <-client2.send:
		var m Message
		if err := json.Unmarshal(received, &m); err != nil {
			t.Errorf("failed to unmarshal message: %v", err)
		}
		if m.Type != "test-message" {
			t.Errorf("expected type 'test-message', got '%s'", m.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for message on client2")
	}

	hub.Shutdown()
}

func TestHub_BroadcastToRoom_ExcludeUser(t *testing.T) {
	hub := NewHub()

	roomID := uuid.New()
	userID1 := uuid.New()
	userID2 := uuid.New()

	hub.mutex.Lock()
	hub.rooms[roomID] = &room{
		clients: make(map[*Client]bool),
	}

	client1 := &Client{
		hub:    hub,
		userID: userID1,
		roomID: roomID,
		send:   make(chan []byte, 10),
	}
	client2 := &Client{
		hub:    hub,
		userID: userID2,
		roomID: roomID,
		send:   make(chan []byte, 10),
	}

	hub.rooms[roomID].clients[client1] = true
	hub.rooms[roomID].clients[client2] = true
	hub.userConnections[userID1] = client1
	hub.userConnections[userID2] = client2
	hub.activeClients.Add(2)
	hub.mutex.Unlock()

	msg := &Message{
		Type:      "test-message",
		RoomID:    roomID,
		UserID:    userID1,
		Timestamp: time.Now(),
	}

	hub.BroadcastToRoom(roomID, msg, userID1)

	select {
	case <-client1.send:
		t.Error("client1 should not receive message (excluded)")
	case <-time.After(100 * time.Millisecond):
	}

	select {
	case received := <-client2.send:
		var m Message
		if err := json.Unmarshal(received, &m); err != nil {
			t.Errorf("failed to unmarshal message: %v", err)
		}
		if m.Type != "test-message" {
			t.Errorf("expected type 'test-message', got '%s'", m.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for message on client2")
	}

	hub.Shutdown()
}

func TestMessage_Encode(t *testing.T) {
	msg := &Message{
		Type:         "test-type",
		RoomID:       uuid.New(),
		UserID:       uuid.New(),
		TargetUserID: uuid.New(),
		Payload:      json.RawMessage(`{"key":"value"}`),
		Timestamp:    time.Now(),
	}

	encoded := msg.encode()

	if len(encoded) == 0 {
		t.Error("expected non-empty encoded message")
	}

	var decoded Message
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Errorf("failed to unmarshal encoded message: %v", err)
	}

	if decoded.Type != msg.Type {
		t.Errorf("expected Type '%s', got '%s'", msg.Type, decoded.Type)
	}

	if decoded.RoomID != msg.RoomID {
		t.Errorf("expected RoomID %s, got %s", msg.RoomID, decoded.RoomID)
	}
}

func TestMessage_Encode_EmptyTimestamp(t *testing.T) {
	msg := &Message{
		Type:    "test-type",
		RoomID:  uuid.New(),
		UserID:  uuid.New(),
		Payload: json.RawMessage(`{}`),
	}

	before := time.Now()
	encoded := msg.encode()
	after := time.Now()

	if msg.Timestamp.IsZero() {
		t.Error("expected Timestamp to be set")
	}

	if msg.Timestamp.Before(before) || msg.Timestamp.After(after) {
		t.Error("Timestamp not within expected range")
	}

	var decoded Message
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Errorf("failed to unmarshal: %v", err)
	}
}

func TestMessage_Decode(t *testing.T) {
	data := []byte(`{
		"type": "voice-offer",
		"room_id": "550e8400-e29b-41d4-a716-446655440000",
		"user_id": "550e8400-e29b-41d4-a716-446655440001",
		"sdp": "offer-sdp",
		"timestamp": "2024-01-01T00:00:00Z"
	}`)

	var msg Message
	err := json.Unmarshal(data, &msg)
	if err != nil {
		t.Errorf("failed to unmarshal: %v", err)
	}

	if msg.Type != "voice-offer" {
		t.Errorf("expected Type 'voice-offer', got '%s'", msg.Type)
	}

	if msg.SDP != "offer-sdp" {
		t.Errorf("expected SDP 'offer-sdp', got '%s'", msg.SDP)
	}
}

func TestMessage_Encode_Decode_RoundTrip(t *testing.T) {
	original := &Message{
		Type:         "voice-answer",
		RoomID:       uuid.New(),
		UserID:       uuid.New(),
		TargetUserID: uuid.New(),
		SDP:          "answer-sdp",
		Candidate:    "candidate-data",
		Payload:      json.RawMessage(`{"field":"value"}`),
		Timestamp:    time.Now(),
	}

	encoded := original.encode()
	var decoded Message
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Type != original.Type {
		t.Errorf("Type: expected %s, got %s", original.Type, decoded.Type)
	}
	if decoded.RoomID != original.RoomID {
		t.Errorf("RoomID: expected %s, got %s", original.RoomID, decoded.RoomID)
	}
	if decoded.UserID != original.UserID {
		t.Errorf("UserID: expected %s, got %s", original.UserID, decoded.UserID)
	}
	if decoded.TargetUserID != original.TargetUserID {
		t.Errorf("TargetUserID: expected %s, got %s", original.TargetUserID, decoded.TargetUserID)
	}
	if decoded.SDP != original.SDP {
		t.Errorf("SDP: expected %s, got %s", original.SDP, decoded.SDP)
	}
	if decoded.Candidate != original.Candidate {
		t.Errorf("Candidate: expected %s, got %s", original.Candidate, decoded.Candidate)
	}
}

func TestClient_GetLastSeen(t *testing.T) {
	client := &Client{
		lastSeen: atomic.Int64{},
	}

	now := time.Now().Unix()
	client.lastSeen.Store(now)

	lastSeen := client.GetLastSeen()

	if lastSeen.Unix() != now {
		t.Errorf("expected %d, got %d", now, lastSeen.Unix())
	}
}

func TestHub_IsRedisEnabled(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	if hub.IsRedisEnabled() {
		t.Error("expected Redis to be disabled by default")
	}
}

func TestHub_GetClientCount_Empty(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	count := hub.GetClientCount()
	if count != 0 {
		t.Errorf("expected 0 clients, got %d", count)
	}
}

func TestHub_GetRoomClients_EmptyRoom(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	roomID := uuid.New()
	clients := hub.GetRoomClients(roomID)

	if len(clients) != 0 {
		t.Errorf("expected 0 clients, got %d", len(clients))
	}
}

func TestHub_GetUserClient_NotFound(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID := uuid.New()
	client := hub.GetUserClient(userID)

	if client != nil {
		t.Error("expected nil for non-existent user")
	}
}
