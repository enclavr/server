package websocket

import (
	"encoding/json"
	"fmt"
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

func TestHub_GetUserClient_Integration(t *testing.T) {
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

func TestHub_SendToClient(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID := uuid.New()
	roomID := uuid.New()

	client := &Client{
		hub:    hub,
		userID: userID,
		roomID: roomID,
		send:   make(chan []byte, 10),
	}

	hub.mutex.Lock()
	hub.userConnections[userID] = client
	hub.mutex.Unlock()

	msg := &Message{
		Type:      "test",
		RoomID:    roomID,
		UserID:    userID,
		Timestamp: time.Now(),
	}

	hub.sendToClient(userID, msg)

	select {
	case received := <-client.send:
		if len(received) == 0 {
			t.Error("expected non-empty message")
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for message")
	}
}

func TestHub_SendToClient_NotFound(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID := uuid.New()
	msg := &Message{
		Type:      "test",
		UserID:    userID,
		Timestamp: time.Now(),
	}

	hub.sendToClient(userID, msg)
}

func TestHub_BroadcastToRoomBatch(t *testing.T) {
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
		Type:      "batch-test",
		RoomID:    roomID,
		UserID:    userID1,
		Timestamp: time.Now(),
	}

	hub.BroadcastToRoomBatch(roomID, msg, uuid.Nil)

	hub.Shutdown()
}

func TestHub_getRoomMutex(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	roomID := uuid.New()
	hub.mutex.Lock()
	hub.roomMutexes[roomID] = &sync.RWMutex{}
	hub.mutex.Unlock()

	roomMutex := hub.getRoomMutex(roomID)
	if roomMutex == nil {
		t.Error("expected non-nil room mutex")
	}

	nilMutex := hub.getRoomMutex(uuid.New())
	if nilMutex != nil {
		t.Error("expected nil for non-existent room")
	}
}

func TestMessage_Decode_Invalid(t *testing.T) {
	msg := &Message{}
	err := msg.decode([]byte(`{invalid json`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestMessage_Decode_Valid(t *testing.T) {
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

func TestRateLimiter_Reset_TimeBased(t *testing.T) {
	rl := NewRateLimiter(2, 0)

	rl.Allow()
	rl.Allow()

	if rl.Allow() {
		t.Log("Rate limiter returned true at limit (depends on implementation)")
	}
}

func TestHub_Run_GracefulShutdown(t *testing.T) {
	hub := NewHub()

	done := make(chan struct{})
	go func() {
		hub.Run()
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	hub.Shutdown()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("hub.Run() did not exit after shutdown")
	}
}

func TestHub_BroadcastToRoom_NotFound(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	roomID := uuid.New()
	msg := &Message{
		Type:   "test",
		RoomID: roomID,
	}

	hub.BroadcastToRoom(roomID, msg, uuid.Nil)
}

func TestHub_RegisterClient(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	go hub.Run()

	userID := uuid.New()
	roomID := uuid.New()

	client := hub.RegisterClient(userID, roomID, nil)

	if client == nil {
		t.Fatal("expected non-nil client")
	}

	if client.userID != userID {
		t.Errorf("expected userID %s, got %s", userID, client.userID)
	}

	if client.roomID != roomID {
		t.Errorf("expected roomID %s, got %s", roomID, client.roomID)
	}

	if client.rateLimit == nil {
		t.Error("expected rate limiter to be set")
	}

	time.Sleep(50 * time.Millisecond)

	if hub.GetClientCount() != 1 {
		t.Errorf("expected 1 client, got %d", hub.GetClientCount())
	}

	roomCount := hub.GetRoomClients(roomID)
	if len(roomCount) != 1 {
		t.Errorf("expected 1 client in room, got %d", len(roomCount))
	}
}

func TestHub_UnregisterClient(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	userID := uuid.New()
	roomID := uuid.New()

	client := hub.RegisterClient(userID, roomID, nil)

	time.Sleep(50 * time.Millisecond)

	if hub.GetClientCount() != 1 {
		t.Fatalf("expected 1 client before unregister, got %d", hub.GetClientCount())
	}

	hub.UnregisterClient(client)

	time.Sleep(50 * time.Millisecond)

	if hub.GetClientCount() != 0 {
		t.Errorf("expected 0 clients after unregister, got %d", hub.GetClientCount())
	}

	hub.Shutdown()
}

func TestClient_HandleMessage_VoiceOffer(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID1 := uuid.New()
	userID2 := uuid.New()
	roomID := uuid.New()

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

	hub.rooms[roomID].clients[client1] = true
	hub.userConnections[userID1] = client1
	hub.userConnections[userID2] = &Client{
		hub:    hub,
		userID: userID2,
		roomID: roomID,
		send:   make(chan []byte, 10),
	}
	hub.activeClients.Add(2)
	hub.mutex.Unlock()

	msg := &Message{
		Type:         "voice-offer",
		UserID:       userID1,
		TargetUserID: userID2,
		SDP:          "offer-sdp",
		RoomID:       roomID,
		Timestamp:    time.Now(),
	}

	client1.handleMessage(msg)

	select {
	case received := <-hub.userConnections[userID2].send:
		var m Message
		if err := json.Unmarshal(received, &m); err != nil {
			t.Errorf("failed to unmarshal: %v", err)
		}
		if m.Type != "voice-offer" {
			t.Errorf("expected voice-offer, got %s", m.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for voice offer")
	}
}

func TestClient_HandleMessage_VoiceMute(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID1 := uuid.New()
	roomID := uuid.New()

	hub.mutex.Lock()
	hub.rooms[roomID] = &room{
		clients: make(map[*Client]bool),
	}

	client1 := &Client{
		hub:    hub,
		userID: userID1,
		roomID: roomID,
		send:   make(chan []byte, 256),
	}

	hub.rooms[roomID].clients[client1] = true
	hub.userConnections[userID1] = client1
	hub.activeClients.Add(1)
	hub.mutex.Unlock()

	msg := &Message{
		Type:      "voice-mute",
		UserID:    userID1,
		RoomID:    roomID,
		Timestamp: time.Now(),
	}

	client1.handleMessage(msg)
}

func TestClient_HandleMessage_UserJoined(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID1 := uuid.New()
	userID2 := uuid.New()
	roomID := uuid.New()

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
		Type:      "user-joined",
		UserID:    userID1,
		RoomID:    roomID,
		Timestamp: time.Now(),
	}

	client1.handleMessage(msg)

	select {
	case received := <-client2.send:
		var m Message
		if err := json.Unmarshal(received, &m); err != nil {
			t.Errorf("failed to unmarshal: %v", err)
		}
		if m.Type != "user-joined" {
			t.Errorf("expected user-joined, got %s", m.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for user-joined message")
	}
}

func TestClient_HandleMessage_UserLeft(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	userID1 := uuid.New()
	userID2 := uuid.New()
	roomID := uuid.New()

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
		Type:      "user-left",
		UserID:    userID1,
		RoomID:    roomID,
		Timestamp: time.Now(),
	}

	client1.handleMessage(msg)

	time.Sleep(50 * time.Millisecond)

	if hub.GetClientCount() != 1 {
		t.Errorf("expected 1 client after user-left, got %d", hub.GetClientCount())
	}

	hub.Shutdown()
}

func TestClient_HandleMessage_Typing(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID1 := uuid.New()
	userID2 := uuid.New()
	roomID := uuid.New()

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
		Type:      "typing",
		UserID:    userID1,
		RoomID:    roomID,
		Timestamp: time.Now(),
	}

	client1.handleMessage(msg)

	select {
	case received := <-client2.send:
		var m Message
		if err := json.Unmarshal(received, &m); err != nil {
			t.Errorf("failed to unmarshal: %v", err)
		}
		if m.Type != "user-typing" {
			t.Errorf("expected user-typing, got %s", m.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for typing message")
	}
}

func TestClient_HandleMessage_StopTyping(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID1 := uuid.New()
	userID2 := uuid.New()
	roomID := uuid.New()

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
		Type:      "stop-typing",
		UserID:    userID1,
		RoomID:    roomID,
		Timestamp: time.Now(),
	}

	client1.handleMessage(msg)

	select {
	case received := <-client2.send:
		var m Message
		if err := json.Unmarshal(received, &m); err != nil {
			t.Errorf("failed to unmarshal: %v", err)
		}
		if m.Type != "user-stopped-typing" {
			t.Errorf("expected user-stopped-typing, got %s", m.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for stop-typing message")
	}
}

func TestClient_HandleMessage_UnknownType(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID := uuid.New()
	roomID := uuid.New()

	hub.mutex.Lock()
	hub.rooms[roomID] = &room{
		clients: make(map[*Client]bool),
	}

	client := &Client{
		hub:    hub,
		userID: userID,
		roomID: roomID,
		send:   make(chan []byte, 10),
	}

	hub.rooms[roomID].clients[client] = true
	hub.userConnections[userID] = client
	hub.activeClients.Add(1)
	hub.mutex.Unlock()

	msg := &Message{
		Type:      "unknown-type",
		UserID:    userID,
		RoomID:    roomID,
		Timestamp: time.Now(),
	}

	client.handleMessage(msg)
}

func TestHub_HandleRedisUserMessage(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID := uuid.New()
	targetUserID := uuid.New()
	roomID := uuid.New()

	hub.mutex.Lock()
	hub.userConnections[targetUserID] = &Client{
		hub:    hub,
		userID: targetUserID,
		roomID: roomID,
		send:   make(chan []byte, 10),
	}
	hub.activeClients.Add(1)
	hub.mutex.Unlock()

	msg := &PubSubMessage{
		Type:         "test-message",
		UserID:       userID,
		TargetUserID: targetUserID,
		Payload:      json.RawMessage(`{"key":"value"}`),
		Timestamp:    time.Now(),
	}

	hub.handleRedisUserMessage(msg)

	select {
	case received := <-hub.userConnections[targetUserID].send:
		if len(received) == 0 {
			t.Error("expected non-empty message")
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for message")
	}
}

func TestHub_HandleRedisRoomMessage(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	senderID := uuid.New()
	userID := uuid.New()
	roomID := uuid.New()

	hub.mutex.Lock()
	hub.rooms[roomID] = &room{
		clients: make(map[*Client]bool),
	}
	hub.roomMutexes[roomID] = &sync.RWMutex{}

	client := &Client{
		hub:    hub,
		userID: userID,
		roomID: roomID,
		send:   make(chan []byte, 10),
	}

	hub.rooms[roomID].clients[client] = true
	hub.userConnections[userID] = client
	hub.activeClients.Add(1)
	hub.mutex.Unlock()

	msg := &PubSubMessage{
		Type:      "room-message",
		RoomID:    roomID,
		UserID:    senderID,
		Payload:   json.RawMessage(`{"key":"value"}`),
		Timestamp: time.Now(),
	}

	hub.handleRedisRoomMessage(msg)

	select {
	case received := <-client.send:
		var m Message
		if err := json.Unmarshal(received, &m); err != nil {
			t.Errorf("failed to unmarshal: %v", err)
		}
		if m.Type != "room-message" {
			t.Errorf("expected room-message, got %s", m.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for message")
	}
}

func TestHub_PublishToRedis_Disabled(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	msg := &Message{
		Type:      "test",
		RoomID:    uuid.New(),
		UserID:    uuid.New(),
		Timestamp: time.Now(),
	}

	hub.publishToRedis(msg)
}

func TestHub_SubscribeToRoomRedis_Disabled(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	err := hub.SubscribeToRoomRedis(uuid.New())
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestHub_SubscribeToUserRedis_Disabled(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	err := hub.SubscribeToUserRedis(uuid.New())
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestHub_ShutdownRedis_Nil(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	err := hub.ShutdownRedis()
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestHub_BroadcastToRoom_EmptyRoom(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	roomID := uuid.New()
	msg := &Message{
		Type:      "test",
		RoomID:    roomID,
		Timestamp: time.Now(),
	}

	hub.BroadcastToRoom(roomID, msg, uuid.Nil)
}

func TestHub_GracefulShutdown(t *testing.T) {
	hub := NewHub()

	roomID := uuid.New()
	userID := uuid.New()

	client := &Client{
		hub:    hub,
		userID: userID,
		roomID: roomID,
		send:   make(chan []byte, 10),
	}

	hub.mutex.Lock()
	hub.rooms[roomID] = &room{
		clients: make(map[*Client]bool),
	}
	hub.rooms[roomID].clients[client] = true
	hub.userConnections[userID] = client
	hub.activeClients.Add(1)
	hub.mutex.Unlock()

	hub.gracefulShutdown()

	if hub.GetClientCount() != 0 {
		t.Errorf("expected 0 clients after shutdown, got %d", hub.GetClientCount())
	}

	if hub.GetRoomCount() != 0 {
		t.Errorf("expected 0 rooms after shutdown, got %d", hub.GetRoomCount())
	}
}

func TestTypingPayload_Marshal(t *testing.T) {
	payload := TypingPayload{
		Context:   "thread",
		ChannelID: "channel-123",
		ThreadID:  "thread-456",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded TypingPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Context != payload.Context {
		t.Errorf("expected Context %s, got %s", payload.Context, decoded.Context)
	}
}

func TestTypingPayload_Unmarshal(t *testing.T) {
	data := []byte(`{"context":"thread","channel_id":"ch-1","thread_id":"th-1"}`)

	var payload TypingPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if payload.Context != "thread" {
		t.Errorf("expected context 'thread', got '%s'", payload.Context)
	}
	if payload.ChannelID != "ch-1" {
		t.Errorf("expected channel_id 'ch-1', got '%s'", payload.ChannelID)
	}
}

func TestHub_SetRoomNotificationSettings(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID := uuid.New()
	roomID := uuid.New()

	settings := RoomNotificationSettings{
		Enabled:       false,
		Sound:         false,
		MentionOnly:   true,
		DesktopNotify: false,
	}

	payload, _ := json.Marshal(settings)
	hub.setRoomNotificationSettings(userID, roomID, payload)

	key := fmt.Sprintf("%s:%s", userID, roomID)
	hub.notificationMutex.RLock()
	stored, exists := hub.notificationSettings[key]
	hub.notificationMutex.RUnlock()

	if !exists {
		t.Fatal("expected notification settings to exist")
	}

	if stored.Enabled != settings.Enabled {
		t.Errorf("expected Enabled %v, got %v", settings.Enabled, stored.Enabled)
	}
	if stored.MentionOnly != settings.MentionOnly {
		t.Errorf("expected MentionOnly %v, got %v", settings.MentionOnly, stored.MentionOnly)
	}
}

func TestHub_SendRoomNotificationSettings(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID := uuid.New()
	roomID := uuid.New()

	hub.mutex.Lock()
	hub.userConnections[userID] = &Client{
		hub:    hub,
		userID: userID,
		roomID: roomID,
		send:   make(chan []byte, 10),
	}
	hub.activeClients.Add(1)
	hub.mutex.Unlock()

	hub.sendRoomNotificationSettings(userID, roomID)

	select {
	case received := <-hub.userConnections[userID].send:
		var msg Message
		if err := json.Unmarshal(received, &msg); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if msg.Type != "room-notifications" {
			t.Errorf("expected type 'room-notifications', got '%s'", msg.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for notification settings")
	}
}

func TestConnectionHealth_Marshal(t *testing.T) {
	health := ConnectionHealth{
		ConnectionID:    uuid.New(),
		UserID:          uuid.New(),
		LatencyMs:       42,
		State:           "connected",
		LastPing:        time.Now(),
		LastPong:        time.Now(),
		PacketsReceived: 100,
		PacketsSent:     50,
		BytesReceived:   1024,
		BytesSent:       512,
	}

	data, err := json.Marshal(health)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded ConnectionHealth
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.LatencyMs != health.LatencyMs {
		t.Errorf("expected LatencyMs %d, got %d", health.LatencyMs, decoded.LatencyMs)
	}
}

func TestClient_HandlePing(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID := uuid.New()
	roomID := uuid.New()

	hub.mutex.Lock()
	hub.userConnections[userID] = &Client{
		hub:    hub,
		userID: userID,
		roomID: roomID,
		send:   make(chan []byte, 10),
	}
	hub.activeClients.Add(1)
	hub.mutex.Unlock()

	client := hub.userConnections[userID]
	msg := &Message{
		Type:      "ping",
		UserID:    userID,
		RoomID:    roomID,
		Timestamp: time.Now(),
	}

	client.handlePing(msg)

	select {
	case received := <-client.send:
		var resp Message
		if err := json.Unmarshal(received, &resp); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if resp.Type != "pong" {
			t.Errorf("expected type 'pong', got '%s'", resp.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for pong")
	}
}

func TestClient_HandleHeartbeat(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID := uuid.New()
	roomID := uuid.New()

	hub.mutex.Lock()
	hub.userConnections[userID] = &Client{
		hub:    hub,
		userID: userID,
		roomID: roomID,
		send:   make(chan []byte, 10),
	}
	hub.activeClients.Add(1)
	hub.mutex.Unlock()

	client := hub.userConnections[userID]
	msg := &Message{
		Type:      "heartbeat",
		UserID:    userID,
		RoomID:    roomID,
		Timestamp: time.Now(),
	}

	client.handleHeartbeat(msg)

	select {
	case received := <-client.send:
		var resp Message
		if err := json.Unmarshal(received, &resp); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if resp.Type != "heartbeat-ack" {
			t.Errorf("expected type 'heartbeat-ack', got '%s'", resp.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for heartbeat-ack")
	}
}

func TestClient_HandleUserSpeaking(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID1 := uuid.New()
	userID2 := uuid.New()
	roomID := uuid.New()

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
		Type:      "user-speaking",
		UserID:    userID1,
		RoomID:    roomID,
		Timestamp: time.Now(),
	}

	client1.handleUserSpeaking(msg)

	select {
	case received := <-client2.send:
		var m Message
		if err := json.Unmarshal(received, &m); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if m.Type != "user-speaking" {
			t.Errorf("expected user-speaking, got %s", m.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for speaking message")
	}
}

func TestClient_HandleScreenShareStart(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID1 := uuid.New()
	userID2 := uuid.New()
	roomID := uuid.New()

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
		Type:      "user-screen-share-start",
		UserID:    userID1,
		RoomID:    roomID,
		Timestamp: time.Now(),
	}

	client1.handleScreenShareStart(msg)

	select {
	case received := <-client2.send:
		var m Message
		if err := json.Unmarshal(received, &m); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if m.Type != "user-screen-share-start" {
			t.Errorf("expected user-screen-share-start, got %s", m.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for screen share message")
	}
}

func TestClient_HandleUserMuted(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID1 := uuid.New()
	userID2 := uuid.New()
	roomID := uuid.New()

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
		Type:      "user-muted",
		UserID:    userID1,
		RoomID:    roomID,
		Timestamp: time.Now(),
	}

	client1.handleUserMuted(msg, true)

	select {
	case received := <-client2.send:
		var m Message
		if err := json.Unmarshal(received, &m); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if m.Type != "user-muted" {
			t.Errorf("expected user-muted, got %s", m.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for muted message")
	}
}

func TestClient_SendConnectionHealth(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID := uuid.New()
	roomID := uuid.New()

	hub.mutex.Lock()
	hub.userConnections[userID] = &Client{
		hub:          hub,
		userID:       userID,
		roomID:       roomID,
		send:         make(chan []byte, 10),
		connectionID: uuid.New(),
	}
	hub.activeClients.Add(1)
	hub.mutex.Unlock()

	client := hub.userConnections[userID]
	client.sendConnectionHealth()

	select {
	case received := <-client.send:
		var msg Message
		if err := json.Unmarshal(received, &msg); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if msg.Type != "connection-health" {
			t.Errorf("expected type 'connection-health', got '%s'", msg.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for connection health")
	}
}

func TestHub_SendRoomUsersDetailed(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	roomID := uuid.New()
	userID := uuid.New()

	hub.mutex.Lock()
	hub.rooms[roomID] = &room{
		clients: make(map[*Client]bool),
	}

	client := &Client{
		hub:          hub,
		userID:       userID,
		roomID:       roomID,
		send:         make(chan []byte, 10),
		connectionID: uuid.New(),
	}

	hub.rooms[roomID].clients[client] = true
	hub.userConnections[userID] = client
	hub.activeClients.Add(1)
	hub.mutex.Unlock()

	hub.sendRoomUsersDetailed(client)

	select {
	case received := <-client.send:
		var msg Message
		if err := json.Unmarshal(received, &msg); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if msg.Type != "room-users-detailed" {
			t.Errorf("expected type 'room-users-detailed', got '%s'", msg.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for room users detailed")
	}
}

func TestTypingState_WithContext(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID := uuid.New()
	roomID := uuid.New()

	hub.setTypingUser(userID, roomID, "thread:123")

	hub.typingMutex.RLock()
	state, exists := hub.typingUsers[userID]
	hub.typingMutex.RUnlock()

	if !exists {
		t.Fatal("expected typing state to exist")
	}

	if state.Context != "thread:123" {
		t.Errorf("expected context 'thread:123', got '%s'", state.Context)
	}
}

func TestHub_SetTypingUser_WithContext(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID := uuid.New()
	roomID := uuid.New()

	hub.setTypingUser(userID, roomID, "channel")

	hub.typingMutex.RLock()
	state, exists := hub.typingUsers[userID]
	hub.typingMutex.RUnlock()

	if !exists {
		t.Fatal("expected typing state to exist")
	}

	if state.UserID != userID {
		t.Errorf("expected userID %s, got %s", userID, state.UserID)
	}
	if state.RoomID != roomID {
		t.Errorf("expected roomID %s, got %s", roomID, state.RoomID)
	}
	if state.Context != "channel" {
		t.Errorf("expected context 'channel', got '%s'", state.Context)
	}
}

func TestClient_HandleMessage_TypingWithContext(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID1 := uuid.New()
	userID2 := uuid.New()
	roomID := uuid.New()

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

	payload, _ := json.Marshal(TypingPayload{Context: "thread:456"})
	msg := &Message{
		Type:      "typing",
		UserID:    userID1,
		RoomID:    roomID,
		Payload:   payload,
		Timestamp: time.Now(),
	}

	client1.handleMessage(msg)

	select {
	case received := <-client2.send:
		var m Message
		if err := json.Unmarshal(received, &m); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if m.Type != "user-typing" {
			t.Errorf("expected user-typing, got %s", m.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for typing message")
	}
}

func TestTypingDebouncer_Trigger(t *testing.T) {
	var mu sync.Mutex
	triggered := false
	debouncer := NewTypingDebouncer(100*time.Millisecond, func(uid uuid.UUID) {
		mu.Lock()
		triggered = true
		mu.Unlock()
	})

	userID := uuid.New()
	debouncer.Trigger(userID)

	mu.Lock()
	if triggered {
		mu.Unlock()
		t.Error("triggered should be false before timeout")
	}
	mu.Unlock()

	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	if !triggered {
		mu.Unlock()
		t.Error("triggered should be true after timeout")
	}
	mu.Unlock()
}

func TestTypingDebouncer_Stop(t *testing.T) {
	var mu sync.Mutex
	triggered := false
	debouncer := NewTypingDebouncer(100*time.Millisecond, func(uid uuid.UUID) {
		mu.Lock()
		triggered = true
		mu.Unlock()
	})

	userID := uuid.New()
	debouncer.Trigger(userID)
	debouncer.Stop(userID)

	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	if triggered {
		mu.Unlock()
		t.Error("triggered should be false after Stop")
	}
	mu.Unlock()
}

func TestTypingDebouncer_StopAll(t *testing.T) {
	var mu sync.Mutex
	triggered := 0
	debouncer := NewTypingDebouncer(100*time.Millisecond, func(uid uuid.UUID) {
		mu.Lock()
		triggered++
		mu.Unlock()
	})

	debouncer.Trigger(uuid.New())
	debouncer.Trigger(uuid.New())
	debouncer.Trigger(uuid.New())

	debouncer.StopAll()

	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	if triggered != 0 {
		mu.Unlock()
		t.Errorf("expected triggered to be 0, got %d", triggered)
	}
	mu.Unlock()
}

func TestPresencePayload_Marshal(t *testing.T) {
	payload := PresencePayload{
		Status:   "dnd",
		Activity: "Playing Game",
		Custom:   "BRB",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded PresencePayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Status != payload.Status {
		t.Errorf("expected Status %s, got %s", payload.Status, decoded.Status)
	}
	if decoded.Activity != payload.Activity {
		t.Errorf("expected Activity %s, got %s", payload.Activity, decoded.Activity)
	}
}

func TestPresenceInfo_Marshal(t *testing.T) {
	info := PresenceInfo{
		UserID:   uuid.New(),
		Status:   "idle",
		Since:    time.Now(),
		Activity: "coding",
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded PresenceInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Status != info.Status {
		t.Errorf("expected Status %s, got %s", info.Status, decoded.Status)
	}
}

func TestNotificationStore_Add(t *testing.T) {
	userID := uuid.New()
	notification := RoomNotification{
		ID:         uuid.New().String(),
		Type:       "user_joined",
		RoomID:     uuid.New(),
		UserID:     uuid.New(),
		Timestamp:  time.Now(),
		Read:       false,
		Actionable: false,
	}

	notificationStore.Add(userID, notification)

	notifications := notificationStore.Get(userID, 10)
	if len(notifications) != 1 {
		t.Errorf("expected 1 notification, got %d", len(notifications))
	}
}

func TestNotificationStore_MarkRead(t *testing.T) {
	userID := uuid.New()
	notifID := uuid.New()
	notification := RoomNotification{
		ID:        notifID.String(),
		Type:      "message",
		RoomID:    uuid.New(),
		Timestamp: time.Now(),
		Read:      false,
	}

	notificationStore.Add(userID, notification)
	notificationStore.MarkRead(userID, notifID)

	notifications := notificationStore.Get(userID, 10)
	if len(notifications) == 0 {
		t.Fatal("expected notifications")
	}
	if !notifications[0].Read {
		t.Error("expected notification to be marked read")
	}
}

func TestRoomNotification_Marshal(t *testing.T) {
	notification := RoomNotification{
		ID:         uuid.New().String(),
		Type:       "mention",
		RoomID:     uuid.New(),
		UserID:     uuid.New(),
		Message:    "Hey @user!",
		Timestamp:  time.Now(),
		Read:       false,
		Actionable: true,
	}

	data, err := json.Marshal(notification)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded RoomNotification
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Type != notification.Type {
		t.Errorf("expected Type %s, got %s", notification.Type, decoded.Type)
	}
	if decoded.Actionable != notification.Actionable {
		t.Errorf("expected Actionable %v, got %v", notification.Actionable, decoded.Actionable)
	}
}

func TestClient_HandleSetPresence(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID1 := uuid.New()
	userID2 := uuid.New()
	roomID := uuid.New()

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

	payload, _ := json.Marshal(PresencePayload{Status: "dnd", Activity: "gaming"})
	msg := &Message{
		Type:      "set-presence",
		UserID:    userID1,
		RoomID:    roomID,
		Payload:   payload,
		Timestamp: time.Now(),
	}

	client1.handleSetPresence(msg)

	select {
	case received := <-client2.send:
		var m Message
		if err := json.Unmarshal(received, &m); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if m.Type != "presence-updated" {
			t.Errorf("expected presence-updated, got %s", m.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for presence update")
	}
}

func TestClient_HandleTypingIndicator(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID1 := uuid.New()
	userID2 := uuid.New()
	roomID := uuid.New()

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

	payload, _ := json.Marshal(TypingIndicator{
		Context:  "channel:123",
		IsTyping: true,
	})
	msg := &Message{
		Type:      "typing-indicator",
		UserID:    userID1,
		RoomID:    roomID,
		Payload:   payload,
		Timestamp: time.Now(),
	}

	client1.handleTypingIndicator(msg)

	select {
	case received := <-client2.send:
		var m Message
		if err := json.Unmarshal(received, &m); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if m.Type != "typing-indicator" {
			t.Errorf("expected typing-indicator, got %s", m.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for typing indicator")
	}
}

func TestClient_HandleRoomEvent(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID1 := uuid.New()
	userID2 := uuid.New()
	roomID := uuid.New()

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

	eventPayload, _ := json.Marshal(RoomEvent{
		Type:    "reaction_added",
		Payload: json.RawMessage(`{"emoji":"👍"}`),
	})
	msg := &Message{
		Type:      "room-event",
		UserID:    userID1,
		RoomID:    roomID,
		Payload:   eventPayload,
		Timestamp: time.Now(),
	}

	client1.handleRoomEvent(msg)

	select {
	case received := <-client2.send:
		var m Message
		if err := json.Unmarshal(received, &m); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if m.Type != "room-event" {
			t.Errorf("expected room-event, got %s", m.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for room event")
	}
}

func TestHub_SetUserPresence(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID := uuid.New()
	roomID := uuid.New()

	hub.setUserPresence(userID, roomID, "away")

	status, exists := hub.GetUserPresence(userID)
	if !exists {
		t.Error("expected presence to exist")
	}
	if status != "away" {
		t.Errorf("expected status 'away', got '%s'", status)
	}
}

func TestHub_GetRoomPresence(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID1 := uuid.New()
	userID2 := uuid.New()
	roomID := uuid.New()

	hub.setUserPresence(userID1, roomID, "online")
	hub.setUserPresence(userID2, roomID, "idle")

	states := hub.GetRoomPresence(roomID)
	if len(states) != 2 {
		t.Errorf("expected 2 presence states, got %d", len(states))
	}
}

func TestRoomEvent_Marshal(t *testing.T) {
	event := RoomEvent{
		Type:      "user_kicked",
		RoomID:    uuid.New(),
		UserID:    uuid.New(),
		Timestamp: time.Now(),
		Payload:   json.RawMessage(`{"reason":"spam"}`),
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded RoomEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Type != event.Type {
		t.Errorf("expected Type %s, got %s", event.Type, decoded.Type)
	}
}

func TestClient_HandleGetNotifications(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID := uuid.New()
	roomID := uuid.New()

	notificationStore.Add(userID, RoomNotification{
		ID:         uuid.New().String(),
		Type:       "message",
		RoomID:     roomID,
		Timestamp:  time.Now(),
		Read:       false,
		Actionable: true,
	})

	hub.mutex.Lock()
	hub.userConnections[userID] = &Client{
		hub:    hub,
		userID: userID,
		roomID: roomID,
		send:   make(chan []byte, 10),
	}
	hub.activeClients.Add(1)
	hub.mutex.Unlock()

	client := hub.userConnections[userID]
	msg := &Message{
		Type:      "get-notifications",
		UserID:    userID,
		RoomID:    roomID,
		Timestamp: time.Now(),
	}

	client.handleGetNotifications(msg)

	select {
	case received := <-client.send:
		var m Message
		if err := json.Unmarshal(received, &m); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if m.Type != "notifications-list" {
			t.Errorf("expected notifications-list, got %s", m.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for notifications")
	}
}

func TestClient_HandleTypingWithContext(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID1 := uuid.New()
	userID2 := uuid.New()
	roomID := uuid.New()

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

	payload, _ := json.Marshal(TypingEvent{
		ChannelID:   "channel-123",
		ContextType: "channel",
		IsTyping:    true,
	})
	msg := &Message{
		Type:      "typing-with-context",
		UserID:    userID1,
		RoomID:    roomID,
		Payload:   payload,
		Timestamp: time.Now(),
	}

	client1.handleTypingWithContext(msg)

	select {
	case received := <-client2.send:
		var m Message
		if err := json.Unmarshal(received, &m); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if m.Type != "typing-with-context" {
			t.Errorf("expected typing-with-context, got %s", m.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for typing with context message")
	}
}

func TestClient_HandlePresenceBulk(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID1 := uuid.New()
	roomID := uuid.New()

	hub.mutex.Lock()
	hub.userConnections[userID1] = &Client{
		hub:    hub,
		userID: userID1,
		roomID: roomID,
		send:   make(chan []byte, 10),
	}
	hub.activeClients.Add(1)

	hub.setUserPresence(userID1, roomID, "online")
	hub.mutex.Unlock()

	userIDs := []uuid.UUID{userID1, uuid.New()}
	payload, _ := json.Marshal(map[string]interface{}{"user_ids": userIDs})
	msg := &Message{
		Type:      "presence-bulk",
		UserID:    userID1,
		RoomID:    roomID,
		Payload:   payload,
		Timestamp: time.Now(),
	}

	client := hub.userConnections[userID1]
	client.handlePresenceBulk(msg)

	select {
	case received := <-client.send:
		var m Message
		if err := json.Unmarshal(received, &m); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if m.Type != "presence-bulk-response" {
			t.Errorf("expected presence-bulk-response, got %s", m.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for presence bulk response")
	}
}

func TestClient_HandleUserKicked(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID1 := uuid.New()
	userID2 := uuid.New()
	roomID := uuid.New()

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

	payload, _ := json.Marshal(map[string]string{
		"target_user_id": userID2.String(),
		"reason":         "Violation of rules",
	})
	msg := &Message{
		Type:      "user-kicked",
		UserID:    userID1,
		RoomID:    roomID,
		Payload:   payload,
		Timestamp: time.Now(),
	}

	client1.handleUserKicked(msg)

	select {
	case received := <-client2.send:
		var m Message
		if err := json.Unmarshal(received, &m); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if m.Type != "user-kicked" {
			t.Errorf("expected user-kicked, got %s", m.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for kick notification")
	}
}

func TestClient_HandleUserBanned(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID1 := uuid.New()
	userID2 := uuid.New()
	roomID := uuid.New()

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

	payload, _ := json.Marshal(map[string]string{
		"target_user_id": userID2.String(),
		"reason":         "Repeated violations",
		"duration":       "permanent",
	})
	msg := &Message{
		Type:      "user-banned",
		UserID:    userID1,
		RoomID:    roomID,
		Payload:   payload,
		Timestamp: time.Now(),
	}

	client1.handleUserBanned(msg)

	select {
	case received := <-client2.send:
		var m Message
		if err := json.Unmarshal(received, &m); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if m.Type != "user-banned" {
			t.Errorf("expected user-banned, got %s", m.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for ban notification")
	}
}

func TestClient_HandleRoleChanged(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID1 := uuid.New()
	userID2 := uuid.New()
	roomID := uuid.New()

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

	payload, _ := json.Marshal(map[string]string{
		"target_user_id": userID2.String(),
		"old_role":       "member",
		"new_role":       "moderator",
	})
	msg := &Message{
		Type:      "role-changed",
		UserID:    userID1,
		RoomID:    roomID,
		Payload:   payload,
		Timestamp: time.Now(),
	}

	client1.handleRoleChanged(msg)

	select {
	case received := <-client2.send:
		var m Message
		if err := json.Unmarshal(received, &m); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if m.Type != "role-changed" {
			t.Errorf("expected role-changed, got %s", m.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for role change notification")
	}
}

func TestClient_HandleGetRoomPresence(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID1 := uuid.New()
	roomID := uuid.New()

	hub.mutex.Lock()
	hub.userConnections[userID1] = &Client{
		hub:    hub,
		userID: userID1,
		roomID: roomID,
		send:   make(chan []byte, 10),
	}
	hub.activeClients.Add(1)

	hub.setUserPresence(userID1, roomID, "online")
	hub.mutex.Unlock()

	msg := &Message{
		Type:      "get-room-presence",
		UserID:    userID1,
		RoomID:    roomID,
		Timestamp: time.Now(),
	}

	client := hub.userConnections[userID1]
	client.handleGetRoomPresence(msg)

	select {
	case received := <-client.send:
		var m Message
		if err := json.Unmarshal(received, &m); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if m.Type != "room-presence-list" {
			t.Errorf("expected room-presence-list, got %s", m.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for room presence list")
	}
}

func TestTypingEvent_Marshal(t *testing.T) {
	event := TypingEvent{
		UserID:      uuid.New(),
		RoomID:      uuid.New(),
		ChannelID:   "channel-123",
		ContextType: "channel",
		IsTyping:    true,
		Timestamp:   time.Now(),
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded TypingEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.ChannelID != event.ChannelID {
		t.Errorf("expected ChannelID %s, got %s", event.ChannelID, decoded.ChannelID)
	}
	if decoded.IsTyping != event.IsTyping {
		t.Errorf("expected IsTyping %v, got %v", event.IsTyping, decoded.IsTyping)
	}
}

func TestRoomNotification_Kicked(t *testing.T) {
	notification := RoomNotification{
		ID:           uuid.New().String(),
		Type:         "user_kicked",
		RoomID:       uuid.New(),
		UserID:       uuid.New(),
		TargetUserID: uuid.New(),
		Message:      "You have been kicked",
		Reason:       "Rule violation",
		Timestamp:    time.Now(),
		Read:         false,
		Actionable:   false,
	}

	data, err := json.Marshal(notification)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded RoomNotification
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Type != "user_kicked" {
		t.Errorf("expected type user_kicked, got %s", decoded.Type)
	}
	if decoded.Reason != "Rule violation" {
		t.Errorf("expected reason, got %s", decoded.Reason)
	}
}

func TestRoomNotification_Banned(t *testing.T) {
	notification := RoomNotification{
		ID:           uuid.New().String(),
		Type:         "user_banned",
		RoomID:       uuid.New(),
		UserID:       uuid.New(),
		TargetUserID: uuid.New(),
		Message:      "You have been banned",
		Reason:       "Spam",
		Duration:     "permanent",
		Timestamp:    time.Now(),
		Read:         false,
		Actionable:   false,
	}

	data, err := json.Marshal(notification)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded RoomNotification
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Type != "user_banned" {
		t.Errorf("expected type user_banned, got %s", decoded.Type)
	}
	if decoded.Duration != "permanent" {
		t.Errorf("expected duration permanent, got %s", decoded.Duration)
	}
}

func TestEnhancedTypingPayload_Marshal(t *testing.T) {
	payload := EnhancedTypingPayload{
		Context:        "thread",
		ContextID:      "thread-123",
		ChannelID:      "channel-456",
		ThreadID:       "thread-123",
		IsTyping:       true,
		MessageID:      "msg-789",
		MentionedUsers: []uuid.UUID{uuid.New(), uuid.New()},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded EnhancedTypingPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Context != "thread" {
		t.Errorf("expected context thread, got %s", decoded.Context)
	}
	if decoded.IsTyping != true {
		t.Errorf("expected isTyping true, got %v", decoded.IsTyping)
	}
	if len(decoded.MentionedUsers) != 2 {
		t.Errorf("expected 2 mentioned users, got %d", len(decoded.MentionedUsers))
	}
}

func TestEnhancedTypingEvent_Marshal(t *testing.T) {
	event := EnhancedTypingEvent{
		UserID:         uuid.New(),
		RoomID:         uuid.New(),
		Context:        "channel",
		ContextID:      "ch-123",
		IsTyping:       true,
		MessageID:      "msg-456",
		MentionedUsers: []uuid.UUID{uuid.New()},
		Timestamp:      time.Now(),
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded EnhancedTypingEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Context != "channel" {
		t.Errorf("expected context channel, got %s", decoded.Context)
	}
	if decoded.IsTyping != true {
		t.Errorf("expected isTyping true, got %v", decoded.IsTyping)
	}
}

func TestClient_HandleEnhancedTyping(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID1 := uuid.New()
	userID2 := uuid.New()
	roomID := uuid.New()

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

	payload, _ := json.Marshal(EnhancedTypingPayload{
		Context:   "thread",
		ContextID: "thread-123",
		IsTyping:  true,
		MessageID: "msg-456",
	})
	msg := &Message{
		Type:      "enhanced-typing",
		UserID:    userID1,
		RoomID:    roomID,
		Payload:   payload,
		Timestamp: time.Now(),
	}

	client1.handleMessage(msg)

	select {
	case received := <-client2.send:
		var m Message
		if err := json.Unmarshal(received, &m); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if m.Type != "enhanced-typing" {
			t.Errorf("expected enhanced-typing, got %s", m.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for enhanced typing message")
	}
}

func TestPresenceEventPayload_Marshal(t *testing.T) {
	payload := PresenceEventPayload{
		UserID:       uuid.New(),
		Status:       "dnd",
		Activity:     "Playing Game",
		CustomStatus: "BRB",
		Device:       "desktop",
		Since:        time.Now().Unix(),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded PresenceEventPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Status != "dnd" {
		t.Errorf("expected status dnd, got %s", decoded.Status)
	}
	if decoded.Activity != "Playing Game" {
		t.Errorf("expected activity Playing Game, got %s", decoded.Activity)
	}
}

func TestClient_HandlePresenceEvent(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID1 := uuid.New()
	userID2 := uuid.New()
	roomID := uuid.New()

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

	payload, _ := json.Marshal(PresenceEventPayload{
		UserID:       userID1,
		Status:       "away",
		Activity:     "Idle",
		CustomStatus: "",
		Device:       "web",
		Since:        time.Now().Unix(),
	})
	msg := &Message{
		Type:      "presence-event",
		UserID:    userID1,
		RoomID:    roomID,
		Payload:   payload,
		Timestamp: time.Now(),
	}

	client1.handleMessage(msg)

	select {
	case received := <-client2.send:
		var m Message
		if err := json.Unmarshal(received, &m); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if m.Type != "presence-event" {
			t.Errorf("expected presence-event, got %s", m.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for presence event")
	}
}

func TestClient_HandlePresenceEvent_InvalidStatus(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	userID := uuid.New()
	roomID := uuid.New()

	hub.mutex.Lock()
	hub.rooms[roomID] = &room{
		clients: make(map[*Client]bool),
	}

	client := &Client{
		hub:    hub,
		userID: userID,
		roomID: roomID,
		send:   make(chan []byte, 10),
	}

	hub.rooms[roomID].clients[client] = true
	hub.userConnections[userID] = client
	hub.activeClients.Add(1)
	hub.mutex.Unlock()

	payload, _ := json.Marshal(PresenceEventPayload{
		UserID: userID,
		Status: "invalid_status",
	})
	msg := &Message{
		Type:      "presence-event",
		UserID:    userID,
		RoomID:    roomID,
		Payload:   payload,
		Timestamp: time.Now(),
	}

	client.handleMessage(msg)

	select {
	case received := <-client.send:
		var m Message
		if err := json.Unmarshal(received, &m); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if m.Type != "error" {
			t.Errorf("expected error message, got %s", m.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for error message")
	}
}

func TestClient_HandleGetOnlineUsersDetailed(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	roomID := uuid.New()
	userID := uuid.New()

	hub.mutex.Lock()
	hub.rooms[roomID] = &room{
		clients: make(map[*Client]bool),
	}

	client := &Client{
		hub:          hub,
		userID:       userID,
		roomID:       roomID,
		send:         make(chan []byte, 10),
		connectionID: uuid.New(),
	}

	hub.rooms[roomID].clients[client] = true
	hub.userConnections[userID] = client
	hub.activeClients.Add(1)
	hub.mutex.Unlock()

	msg := &Message{
		Type:      "get-online-users-detailed",
		UserID:    userID,
		RoomID:    roomID,
		Timestamp: time.Now(),
	}

	client.handleMessage(msg)

	select {
	case received := <-client.send:
		var m Message
		if err := json.Unmarshal(received, &m); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if m.Type != "online-users-detailed" {
			t.Errorf("expected online-users-detailed, got %s", m.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for online users detailed")
	}
}

func TestReconnectBackoff_Next(t *testing.T) {
	rb := NewReconnectBackoff()

	delay1, ok := rb.Next()
	if !ok {
		t.Error("expected first attempt to succeed")
	}
	if delay1 < MinReconnectDelay {
		t.Errorf("expected delay >= %v, got %v", MinReconnectDelay, delay1)
	}

	delay2, ok := rb.Next()
	if !ok {
		t.Error("expected second attempt to succeed")
	}
	if delay2 <= delay1 {
		t.Errorf("expected delay to increase, got %v <= %v", delay2, delay1)
	}

	rb.Reset()
	delay3, ok := rb.Next()
	if !ok {
		t.Error("expected attempt after reset to succeed")
	}
	if delay3 != MinReconnectDelay {
		t.Errorf("expected delay to reset to %v, got %v", MinReconnectDelay, delay3)
	}
}

func TestReconnectBackoff_MaxAttempts(t *testing.T) {
	rb := NewReconnectBackoff()

	for i := 0; i < MaxReconnectAttempts; i++ {
		_, ok := rb.Next()
		if !ok && i < MaxReconnectAttempts-1 {
			t.Errorf("expected attempt %d to succeed", i+1)
		}
	}

	_, ok := rb.Next()
	if ok {
		t.Error("expected last attempt to fail")
	}
}

func TestTraceContext_New(t *testing.T) {
	tc := NewTraceContext()

	if tc.TraceID == uuid.Nil {
		t.Error("expected non-nil TraceID")
	}
	if tc.SpanID == uuid.Nil {
		t.Error("expected non-nil SpanID")
	}
}

func TestHub_BroadcastToRoom_NilMessage(t *testing.T) {
	hub := NewHub()
	defer hub.Shutdown()

	roomID := uuid.New()
	msg := (*Message)(nil)

	hub.BroadcastToRoom(roomID, msg, uuid.Nil)
}
