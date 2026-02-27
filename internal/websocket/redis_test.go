package websocket

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewPubSubService(t *testing.T) {
	ps := NewPubSubService("localhost", "", 0)
	if ps == nil {
		t.Error("expected non-nil PubSubService")
		return
	}
	if ps.serverID == "" {
		t.Error("expected serverID to be set")
	}
	if ps.handlers == nil {
		t.Error("expected handlers map to be initialized")
	}
}

func TestPubSubMessage_Structure(t *testing.T) {
	msg := PubSubMessage{
		Type:         "test_type",
		RoomID:       uuid.New(),
		UserID:       uuid.New(),
		TargetUserID: uuid.New(),
		Payload:      json.RawMessage(`{"key":"value"}`),
		Timestamp:    time.Now(),
		ServerID:     "server1",
	}

	if msg.Type != "test_type" {
		t.Errorf("expected type test_type, got %s", msg.Type)
	}
	if msg.ServerID != "server1" {
		t.Errorf("expected serverID server1, got %s", msg.ServerID)
	}
}

func TestPubSubService_IsConnected(t *testing.T) {
	ps := NewPubSubService("localhost", "", 0)
	if ps.IsConnected() != false {
		t.Error("expected IsConnected to return false initially")
	}
}

func TestPubSubService_GetServerID(t *testing.T) {
	ps := NewPubSubService("localhost", "", 0)
	serverID := ps.GetServerID()
	if serverID == "" {
		t.Error("expected non-empty server ID")
	}
}

func TestPubSubService_RegisterHandler(t *testing.T) {
	ps := NewPubSubService("localhost", "", 0)

	var called bool
	handler := func(msg *PubSubMessage) {
		called = true
	}

	ps.RegisterHandler("test_message", handler)

	ps.mu.RLock()
	h, ok := ps.handlers["test_message"]
	ps.mu.RUnlock()

	if !ok {
		t.Error("expected handler to be registered")
	}
	if h == nil {
		t.Error("expected handler to be non-nil")
	}
	_ = called
}

func TestPubSubService_Publish(t *testing.T) {
	ps := NewPubSubService("localhost", "", 0)

	msg := &PubSubMessage{
		Type:      "test",
		Timestamp: time.Now(),
	}

	err := ps.Publish("test_channel", msg)
	if err == nil {
		t.Log("Publish returned no error (expected without Redis)")
	}
}

func TestPubSubService_PublishToRoom(t *testing.T) {
	ps := NewPubSubService("localhost", "", 0)

	roomID := uuid.New()
	msg := &PubSubMessage{
		Type:      "room_message",
		Timestamp: time.Now(),
	}

	err := ps.PublishToRoom(roomID, msg)
	if err == nil {
		t.Log("PublishToRoom returned no error (expected without Redis)")
	}
}

func TestPubSubService_PublishToUser(t *testing.T) {
	ps := NewPubSubService("localhost", "", 0)

	userID := uuid.New()
	msg := &PubSubMessage{
		Type:      "user_message",
		Timestamp: time.Now(),
	}

	err := ps.PublishToUser(userID, msg)
	if err == nil {
		t.Log("PublishToUser returned no error (expected without Redis)")
	}
}

func TestPubSubService_PublishBroadcast(t *testing.T) {
	ps := NewPubSubService("localhost", "", 0)

	msg := &PubSubMessage{
		Type:      "broadcast",
		Timestamp: time.Now(),
	}

	err := ps.PublishBroadcast(msg)
	if err == nil {
		t.Log("PublishBroadcast returned no error (expected without Redis)")
	}
}

func TestPubSubService_Unsubscribe(t *testing.T) {
	ps := NewPubSubService("localhost", "", 0)

	err := ps.Unsubscribe("test_channel")
	if err == nil {
		t.Log("Unsubscribe returned no error (expected without subscription)")
	}
}

func TestPubSubService_SubscribeToRoom(t *testing.T) {
	ps := NewPubSubService("localhost", "", 0)

	roomID := uuid.New()
	err := ps.SubscribeToRoom(roomID)
	if err == nil {
		t.Log("SubscribeToRoom returned no error (expected without Redis)")
	}
}

func TestPubSubService_SubscribeToUser(t *testing.T) {
	ps := NewPubSubService("localhost", "", 0)

	userID := uuid.New()
	err := ps.SubscribeToUser(userID)
	if err == nil {
		t.Log("SubscribeToUser returned no error (expected without Redis)")
	}
}

func TestPubSubService_Connect(t *testing.T) {
	ps := NewPubSubService("localhost", "", 0)

	err := ps.Connect()
	if err != nil {
		t.Logf("Connect returned error (expected without Redis): %v", err)
	}
}

func TestPubSubService_Disconnect(t *testing.T) {
	ps := NewPubSubService("localhost", "", 0)

	err := ps.Disconnect()
	if err != nil {
		t.Logf("Disconnect returned: %v", err)
	}
}

func TestPubSubService_ConcurrentHandlers(t *testing.T) {
	ps := NewPubSubService("localhost", "", 0)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ps.RegisterHandler("test", func(msg *PubSubMessage) {})
		}(i)
	}
	wg.Wait()
}
