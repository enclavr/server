package middleware

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/enclavr/server/internal/database"
	"github.com/enclavr/server/internal/services"
	"github.com/google/uuid"
)

type HealthChecker interface {
	Check(ctx context.Context) error
}

type HealthStatus string

const (
	StatusHealthy   HealthStatus = "healthy"
	StatusUnhealthy HealthStatus = "unhealthy"
	StatusDegraded  HealthStatus = "degraded"
)

type ComponentHealth struct {
	Status    HealthStatus `json:"status"`
	LatencyMs int64        `json:"latency_ms,omitempty"`
	Error     string       `json:"error,omitempty"`
}

type HealthResponse struct {
	Status     string                     `json:"status"`
	Version    string                     `json:"version"`
	Timestamp  string                     `json:"timestamp"`
	Uptime     int64                      `json:"uptime_seconds"`
	Components map[string]ComponentHealth `json:"components,omitempty"`
}

type HealthMiddleware struct {
	db        *database.Database
	cache     *services.CacheService
	startTime time.Time
	checkers  map[string]HealthChecker
	mu        sync.RWMutex
	version   string
}

func NewHealthMiddleware(db *database.Database, cache *services.CacheService, version string) *HealthMiddleware {
	hm := &HealthMiddleware{
		db:        db,
		cache:     cache,
		startTime: time.Now(),
		version:   version,
		checkers:  make(map[string]HealthChecker),
	}

	hm.registerDefaultCheckers()

	return hm
}

func (hm *HealthMiddleware) registerDefaultCheckers() {
	if hm.db != nil {
		hm.checkers["database"] = &dbHealthChecker{db: hm.db}
	}
	if hm.cache != nil {
		hm.checkers["cache"] = &cacheHealthChecker{cache: hm.cache}
	}
}

func (hm *HealthMiddleware) RegisterChecker(name string, checker HealthChecker) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.checkers[name] = checker
}

type dbHealthChecker struct {
	db *database.Database
}

func (c *dbHealthChecker) Check(ctx context.Context) error {
	var result int
	return c.db.WithContext(ctx).Raw("SELECT 1").Scan(&result).Error
}

type cacheHealthChecker struct {
	cache *services.CacheService
}

func (c *cacheHealthChecker) Check(ctx context.Context) error {
	return c.cache.Ping(ctx)
}

func (hm *HealthMiddleware) HealthHandler(w http.ResponseWriter, r *http.Request) {
	hm.serveHealth(w, r, false)
}

func (hm *HealthMiddleware) ReadyHandler(w http.ResponseWriter, r *http.Request) {
	hm.serveHealth(w, r, true)
}

func (hm *HealthMiddleware) serveHealth(w http.ResponseWriter, r *http.Request, readiness bool) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	response := HealthResponse{
		Status:    string(StatusHealthy),
		Version:   hm.version,
		Timestamp: time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		Uptime:    int64(time.Since(hm.startTime).Seconds()),
	}

	if len(hm.checkers) > 0 {
		response.Components = make(map[string]ComponentHealth)
		overallStatus := StatusHealthy

		for name, checker := range hm.checkers {
			start := time.Now()
			err := checker.Check(ctx)
			latency := time.Since(start).Milliseconds()

			component := ComponentHealth{
				LatencyMs: latency,
			}

			if err != nil {
				component.Status = StatusUnhealthy
				component.Error = err.Error()
				if overallStatus != StatusUnhealthy {
					overallStatus = StatusDegraded
				}
			} else {
				component.Status = StatusHealthy
			}

			response.Components[name] = component
		}

		if readiness {
			for _, comp := range response.Components {
				if comp.Status == StatusUnhealthy {
					overallStatus = StatusUnhealthy
					break
				}
			}
		}

		response.Status = string(overallStatus)
	}

	statusCode := http.StatusOK
	if response.Status == string(StatusUnhealthy) {
		statusCode = http.StatusServiceUnavailable
	} else if response.Status == string(StatusDegraded) {
		statusCode = http.StatusOK
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding health response: %v", err)
	}
}

func Health(hm *HealthMiddleware) http.Handler {
	return http.HandlerFunc(hm.HealthHandler)
}

func Ready(hm *HealthMiddleware) http.Handler {
	return http.HandlerFunc(hm.ReadyHandler)
}

type HealthCheckFunc func(ctx context.Context) error

func (f HealthCheckFunc) Check(ctx context.Context) error {
	return f(ctx)
}

func WithHealthCheck(hm *HealthMiddleware, name string, fn HealthCheckFunc) {
	hm.RegisterChecker(name, fn)
}

type SimpleHealthResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

func BasicHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(SimpleHealthResponse{
		Status:    "ok",
		Timestamp: time.Now().UTC().Format("2006-01-02T15:04:05Z"),
	}); err != nil {
		log.Printf("Error encoding health response: %v", err)
	}
}

func BasicReady(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(SimpleHealthResponse{
		Status:    "ready",
		Timestamp: time.Now().UTC().Format("2006-01-02T15:04:05Z"),
	}); err != nil {
		log.Printf("Error encoding health response: %v", err)
	}
}

func NewUUID() string {
	return uuid.New().String()
}
