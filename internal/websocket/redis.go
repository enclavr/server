package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type PubSubMessage struct {
	Type         string          `json:"type"`
	RoomID       uuid.UUID       `json:"room_id,omitempty"`
	UserID       uuid.UUID       `json:"user_id,omitempty"`
	TargetUserID uuid.UUID       `json:"target_user_id,omitempty"`
	Payload      json.RawMessage `json:"payload,omitempty"`
	Timestamp    time.Time       `json:"timestamp"`
	ServerID     string          `json:"server_id"`
}

type PubSubService struct {
	client    *redis.Client
	pubsub    *redis.PubSub
	mu        sync.RWMutex
	handlers  map[string]MessageHandler
	serverID  string
	connected bool
	ctx       context.Context
	cancel    context.CancelFunc
}

type MessageHandler func(msg *PubSubMessage)

func NewPubSubService(host, password string, db int) *PubSubService {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:6379", host),
		Password: password,
		DB:       db,
	})

	ctx, cancel := context.WithCancel(context.Background())

	return &PubSubService{
		client:   client,
		handlers: make(map[string]MessageHandler),
		serverID: uuid.New().String(),
		ctx:      ctx,
		cancel:   cancel,
	}
}

func (p *PubSubService) Connect() error {
	if err := p.client.Ping(p.ctx).Err(); err != nil {
		return fmt.Errorf("failed to connect to redis: %w", err)
	}

	p.mu.Lock()
	p.connected = true
	p.mu.Unlock()

	log.Printf("Redis pub/sub connected with server ID: %s", p.serverID)

	go p.monitorConnection()

	return nil
}

func (p *PubSubService) monitorConnection() {
	ticker := time.NewTicker(ReconnectRetryDelay)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			if !p.IsConnected() {
				log.Printf("[Redis] Connection lost, attempting to reconnect...")
				if err := p.reconnect(); err != nil {
					log.Printf("[Redis] Reconnection failed: %v", err)
				} else {
					log.Printf("[Redis] Successfully reconnected")
				}
			}
		}
	}
}

func (p *PubSubService) reconnect() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.connected {
		return nil
	}

	p.cancel()

	ctx, cancel := context.WithCancel(context.Background())
	p.ctx = ctx
	p.cancel = cancel

	newClient := redis.NewClient(&redis.Options{
		Addr:     p.client.Options().Addr,
		Password: p.client.Options().Password,
		DB:       p.client.Options().DB,
	})

	if err := newClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to ping redis: %w", err)
	}

	p.client = newClient
	p.connected = true

	log.Printf("[Redis] Reconnection successful")
	return nil
}

func (p *PubSubService) Disconnect() error {
	p.cancel()

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.pubsub != nil {
		if err := p.pubsub.Close(); err != nil {
			log.Printf("Error closing pubsub: %v", err)
		}
	}

	if err := p.client.Close(); err != nil {
		log.Printf("Error closing Redis client: %v", err)
	}
	p.connected = false
	log.Println("Redis pub/sub disconnected")
	return nil
}

func (p *PubSubService) IsConnected() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.connected
}

func (p *PubSubService) GetServerID() string {
	return p.serverID
}

func (p *PubSubService) Subscribe(channel string) error {
	pubsub := p.client.Subscribe(p.ctx, channel)
	ch := pubsub.Channel()

	p.mu.Lock()
	p.pubsub = pubsub
	p.mu.Unlock()

	go func() {
		for {
			select {
			case <-p.ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}

				var psMsg PubSubMessage
				if err := json.Unmarshal([]byte(msg.Payload), &psMsg); err != nil {
					log.Printf("Error unmarshaling pubsub message: %v", err)
					continue
				}

				if psMsg.ServerID == p.serverID {
					continue
				}

				p.mu.RLock()
				handler, ok := p.handlers[psMsg.Type]
				p.mu.RUnlock()

				if ok {
					handler(&psMsg)
				}
			}
		}
	}()

	log.Printf("Subscribed to channel: %s", channel)
	return nil
}

func (p *PubSubService) SubscribeToRoom(roomID uuid.UUID) error {
	channel := fmt.Sprintf("room:%s", roomID.String())
	return p.Subscribe(channel)
}

func (p *PubSubService) SubscribeToUser(userID uuid.UUID) error {
	channel := fmt.Sprintf("user:%s", userID.String())
	return p.Subscribe(channel)
}

func (p *PubSubService) Unsubscribe(channel string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.pubsub != nil {
		return p.pubsub.Unsubscribe(p.ctx, channel)
	}
	return nil
}

func (p *PubSubService) Publish(channel string, msg *PubSubMessage) error {
	msg.ServerID = p.serverID
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	return p.client.Publish(p.ctx, channel, data).Err()
}

func (p *PubSubService) RegisterHandler(msgType string, handler MessageHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handlers[msgType] = handler
}

func (p *PubSubService) PublishToRoom(roomID uuid.UUID, msg *PubSubMessage) error {
	channel := fmt.Sprintf("room:%s", roomID.String())
	return p.Publish(channel, msg)
}

func (p *PubSubService) PublishToUser(userID uuid.UUID, msg *PubSubMessage) error {
	channel := fmt.Sprintf("user:%s", userID.String())
	return p.Publish(channel, msg)
}

func (p *PubSubService) PublishBroadcast(msg *PubSubMessage) error {
	return p.Publish("broadcast", msg)
}
