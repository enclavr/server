package services

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type CircuitState int

const (
	CircuitStateClosed CircuitState = iota
	CircuitStateOpen
	CircuitStateHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case CircuitStateClosed:
		return "closed"
	case CircuitStateOpen:
		return "open"
	case CircuitStateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

type CircuitBreakerConfig struct {
	FailureThreshold int
	SuccessThreshold int
	Timeout          time.Duration
	HalfOpenMaxCalls int
	ResetTimeout     time.Duration
}

type CircuitBreaker struct {
	name            string
	config          CircuitBreakerConfig
	state           CircuitState
	failures        int
	successes       int
	lastFailureTime time.Time
	lastStateChange time.Time
	mu              sync.RWMutex
	halfOpenCalls   int
}

func NewCircuitBreaker(name string, config CircuitBreakerConfig) *CircuitBreaker {
	if config.FailureThreshold == 0 {
		config.FailureThreshold = 5
	}
	if config.SuccessThreshold == 0 {
		config.SuccessThreshold = 3
	}
	if config.Timeout == 0 {
		config.Timeout = 60 * time.Second
	}
	if config.HalfOpenMaxCalls == 0 {
		config.HalfOpenMaxCalls = 3
	}
	if config.ResetTimeout == 0 {
		config.ResetTimeout = 10 * time.Second
	}

	return &CircuitBreaker{
		name:            name,
		config:          config,
		state:           CircuitStateClosed,
		lastStateChange: time.Now(),
	}
}

func (cb *CircuitBreaker) Name() string {
	return cb.name
}

func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

func (cb *CircuitBreaker) IsAvailable() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	switch cb.state {
	case CircuitStateClosed:
		return true
	case CircuitStateOpen:
		if time.Since(cb.lastFailureTime) >= cb.config.Timeout {
			return true
		}
		return false
	case CircuitStateHalfOpen:
		return cb.halfOpenCalls < cb.config.HalfOpenMaxCalls
	default:
		return false
	}
}

func (cb *CircuitBreaker) Execute(ctx context.Context, fn func() error) error {
	if !cb.IsAvailable() {
		return fmt.Errorf("circuit breaker %s is open", cb.name)
	}

	cb.mu.Lock()
	if cb.state == CircuitStateOpen && time.Since(cb.lastFailureTime) >= cb.config.Timeout {
		cb.state = CircuitStateHalfOpen
		cb.successes = 0
		cb.halfOpenCalls = 0
		cb.lastStateChange = time.Now()
	}

	if cb.state == CircuitStateHalfOpen {
		if cb.halfOpenCalls >= cb.config.HalfOpenMaxCalls {
			cb.mu.Unlock()
			return fmt.Errorf("circuit breaker %s half-open max calls reached", cb.name)
		}
		cb.halfOpenCalls++
	}
	cb.mu.Unlock()

	err := fn()

	if err != nil {
		cb.recordFailure()
		return err
	}

	cb.recordSuccess()
	return nil
}

func (cb *CircuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case CircuitStateClosed:
		if cb.failures >= cb.config.FailureThreshold {
			cb.state = CircuitStateOpen
			cb.lastStateChange = time.Now()
		}
	case CircuitStateHalfOpen:
		cb.state = CircuitStateOpen
		cb.lastStateChange = time.Now()
		cb.halfOpenCalls = 0
	}
}

func (cb *CircuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.successes++
	cb.failures = 0

	switch cb.state {
	case CircuitStateHalfOpen:
		if cb.successes >= cb.config.SuccessThreshold {
			cb.state = CircuitStateClosed
			cb.lastStateChange = time.Now()
			cb.halfOpenCalls = 0
		}
	case CircuitStateClosed:
	}
}

func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = CircuitStateClosed
	cb.failures = 0
	cb.successes = 0
	cb.halfOpenCalls = 0
	cb.lastStateChange = time.Now()
}

func (cb *CircuitBreaker) GetMetrics() CircuitBreakerMetrics {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return CircuitBreakerMetrics{
		Name:            cb.name,
		State:           cb.state.String(),
		Failures:        cb.failures,
		Successes:       cb.successes,
		LastFailureTime: cb.lastFailureTime,
		LastStateChange: cb.lastStateChange,
	}
}

type CircuitBreakerMetrics struct {
	Name            string
	State           string
	Failures        int
	Successes       int
	LastFailureTime time.Time
	LastStateChange time.Time
}

type CircuitBreakerRegistry struct {
	breakers map[string]*CircuitBreaker
	mu       sync.RWMutex
}

var globalCircuitBreakerRegistry *CircuitBreakerRegistry

func GetCircuitBreakerRegistry() *CircuitBreakerRegistry {
	if globalCircuitBreakerRegistry == nil {
		globalCircuitBreakerRegistry = &CircuitBreakerRegistry{
			breakers: make(map[string]*CircuitBreaker),
		}
	}
	return globalCircuitBreakerRegistry
}

func (r *CircuitBreakerRegistry) GetOrCreate(name string, config CircuitBreakerConfig) *CircuitBreaker {
	r.mu.Lock()
	defer r.mu.Unlock()

	if cb, ok := r.breakers[name]; ok {
		return cb
	}

	cb := NewCircuitBreaker(name, config)
	r.breakers[name] = cb
	return cb
}

func (r *CircuitBreakerRegistry) Get(name string) (*CircuitBreaker, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cb, ok := r.breakers[name]
	return cb, ok
}

func (r *CircuitBreakerRegistry) ResetAll() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, cb := range r.breakers {
		cb.Reset()
	}
}

func (r *CircuitBreakerRegistry) GetAllMetrics() []CircuitBreakerMetrics {
	r.mu.RLock()
	defer r.mu.RUnlock()

	metrics := make([]CircuitBreakerMetrics, 0, len(r.breakers))
	for _, cb := range r.breakers {
		metrics = append(metrics, cb.GetMetrics())
	}
	return metrics
}
