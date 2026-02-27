package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestHTTPRequestsTotal(t *testing.T) {
	counter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "test_http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	counter.WithLabelValues("GET", "/api", "200").Inc()
	counter.WithLabelValues("POST", "/api", "201").Inc()

	if testutil.ToFloat64(counter.WithLabelValues("GET", "/api", "200")) != 1 {
		t.Error("expected counter to be 1")
	}

	if testutil.ToFloat64(counter.WithLabelValues("POST", "/api", "201")) != 1 {
		t.Error("expected counter to be 1")
	}
}

func TestHTTPRequestDuration(t *testing.T) {
	histogram := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "test_http_request_duration_seconds",
			Help:    "HTTP request latency in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	hist := histogram.WithLabelValues("GET", "/api")
	hist.Observe(0.05)

	hist2 := histogram.WithLabelValues("POST", "/api")
	hist2.Observe(0.1)

	_ = hist
	_ = hist2
}

func TestWebSocketConnections(t *testing.T) {
	gauge := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "test_websocket_connections_active",
			Help: "Number of active WebSocket connections",
		},
	)

	gauge.Set(5)
	if testutil.ToFloat64(gauge) != 5 {
		t.Error("expected gauge to be 5")
	}

	gauge.Inc()
	if testutil.ToFloat64(gauge) != 6 {
		t.Error("expected gauge to be 6 after Inc")
	}

	gauge.Dec()
	if testutil.ToFloat64(gauge) != 5 {
		t.Error("expected gauge to be 5 after Dec")
	}
}

func TestWebSocketMessagesTotal(t *testing.T) {
	counter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "test_websocket_messages_total",
			Help: "Total number of WebSocket messages",
		},
		[]string{"type", "direction"},
	)

	counter.WithLabelValues("text", "in").Inc()
	counter.WithLabelValues("text", "out").Add(2)
	counter.WithLabelValues("voice", "in").Inc()

	if testutil.ToFloat64(counter.WithLabelValues("text", "in")) != 1 {
		t.Error("expected text/in to be 1")
	}

	if testutil.ToFloat64(counter.WithLabelValues("text", "out")) != 2 {
		t.Error("expected text/out to be 2")
	}
}

func TestWebSocketRoomsActive(t *testing.T) {
	gauge := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "test_websocket_rooms_active",
			Help: "Number of active WebSocket rooms",
		},
	)

	gauge.Set(10)
	if testutil.ToFloat64(gauge) != 10 {
		t.Error("expected gauge to be 10")
	}
}

func TestDatabaseQueryDuration(t *testing.T) {
	histogram := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "test_database_query_duration_seconds",
			Help:    "Database query latency in seconds",
			Buckets: []float64{0.001, 0.01, 0.1, 1},
		},
		[]string{"operation", "table"},
	)

	hist1 := histogram.WithLabelValues("select", "users")
	hist1.Observe(0.005)

	hist2 := histogram.WithLabelValues("insert", "messages")
	hist2.Observe(0.05)

	hist3 := histogram.WithLabelValues("update", "rooms")
	hist3.Observe(0.15)

	_ = hist1
	_ = hist2
	_ = hist3
}

func TestMessagesSent(t *testing.T) {
	counter := prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "test_messages_sent_total",
			Help: "Total number of messages sent",
		},
	)

	counter.Inc()
	counter.Add(9)

	if testutil.ToFloat64(counter) != 10 {
		t.Error("expected counter to be 10")
	}
}

func TestMessagesReceived(t *testing.T) {
	counter := prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "test_messages_received_total",
			Help: "Total number of messages received",
		},
	)

	counter.Add(5)

	if testutil.ToFloat64(counter) != 5 {
		t.Error("expected counter to be 5")
	}
}

func TestActiveUsers(t *testing.T) {
	gauge := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "test_active_users",
			Help: "Number of currently active users",
		},
	)

	gauge.Set(100)
	if testutil.ToFloat64(gauge) != 100 {
		t.Error("expected gauge to be 100")
	}

	gauge.Sub(50)
	if testutil.ToFloat64(gauge) != 50 {
		t.Error("expected gauge to be 50 after Sub")
	}
}

func TestVoiceConnections(t *testing.T) {
	gauge := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "test_voice_connections_active",
			Help: "Number of active voice connections",
		},
	)

	gauge.Set(0)
	if testutil.ToFloat64(gauge) != 0 {
		t.Error("expected gauge to be 0")
	}

	gauge.Inc()
	if testutil.ToFloat64(gauge) != 1 {
		t.Error("expected gauge to be 1")
	}
}

func TestRedisEnabled(t *testing.T) {
	gauge := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "test_redis_enabled",
			Help: "Whether Redis pub/sub is enabled (1 = yes, 0 = no)",
		},
	)

	gauge.Set(1)
	if testutil.ToFloat64(gauge) != 1 {
		t.Error("expected gauge to be 1 when enabled")
	}

	gauge.Set(0)
	if testutil.ToFloat64(gauge) != 0 {
		t.Error("expected gauge to be 0 when disabled")
	}
}

func TestMetricLabels(t *testing.T) {
	counter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "test_metric_labels",
			Help: "Test metric with labels",
		},
		[]string{"label1", "label2"},
	)

	counter.WithLabelValues("value1", "value2").Inc()

	labels := prometheus.Labels{"label1": "value1", "label2": "value2"}
	if testutil.ToFloat64(counter.MustCurryWith(labels)) != 1 {
		t.Error("expected counter to be 1")
	}
}

func TestGaugeOperations(t *testing.T) {
	gauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "test_gauge_operations",
		Help: "Test gauge operations",
	})

	gauge.Set(10)
	if testutil.ToFloat64(gauge) != 10 {
		t.Error("expected gauge to be 10 after Set")
	}

	gauge.Add(5)
	if testutil.ToFloat64(gauge) != 15 {
		t.Error("expected gauge to be 15 after Add")
	}

	gauge.Sub(3)
	if testutil.ToFloat64(gauge) != 12 {
		t.Error("expected gauge to be 12 after Sub")
	}

	gauge.Inc()
	if testutil.ToFloat64(gauge) != 13 {
		t.Error("expected gauge to be 13 after Inc")
	}

	gauge.Dec()
	if testutil.ToFloat64(gauge) != 12 {
		t.Error("expected gauge to be 12 after Dec")
	}
}

func TestHistogramObservations(t *testing.T) {
	histogram := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "test_histogram_observations",
		Help:    "Test histogram observations",
		Buckets: []float64{0, 1, 2, 3, 4, 5},
	})

	histogram.Observe(0.5)
	histogram.Observe(1.5)
	histogram.Observe(2.5)

	counter := prometheus.NewCounter(prometheus.CounterOpts{Name: "test_dummy_hist"})
	counter.Inc()
	_ = counter
}
