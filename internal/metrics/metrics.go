package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	WebSocketConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "websocket_connections_active",
			Help: "Number of active WebSocket connections",
		},
	)

	WebSocketMessagesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "websocket_messages_total",
			Help: "Total number of WebSocket messages",
		},
		[]string{"type", "direction"},
	)

	WebSocketRoomsActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "websocket_rooms_active",
			Help: "Number of active WebSocket rooms",
		},
	)

	DatabaseQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "database_query_duration_seconds",
			Help:    "Database query latency in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation", "table"},
	)

	MessagesSent = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "messages_sent_total",
			Help: "Total number of messages sent",
		},
	)

	MessagesReceived = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "messages_received_total",
			Help: "Total number of messages received",
		},
	)

	ActiveUsers = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "active_users",
			Help: "Number of currently active users",
		},
	)

	VoiceConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "voice_connections_active",
			Help: "Number of active voice connections",
		},
	)

	RedisEnabled = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "redis_enabled",
			Help: "Whether Redis pub/sub is enabled (1 = yes, 0 = no)",
		},
	)
)
