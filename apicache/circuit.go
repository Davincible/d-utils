package apicache

import (
	"sync"
	"time"
)

// CircuitBreakerState represents the state of the circuit breaker
type CircuitBreakerState int

const (
	// CircuitClosed indicates normal operation
	CircuitClosed CircuitBreakerState = iota
	// CircuitOpen indicates the circuit is open and requests will fail fast
	CircuitOpen
	// CircuitHalfOpen indicates the circuit is testing if service has recovered
	CircuitHalfOpen
)

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	mu sync.RWMutex

	state           CircuitBreakerState
	failures        int
	lastFailure     time.Time
	lastSuccess     time.Time
	halfOpenAttempt time.Time

	// Configuration
	maxFailures     int
	resetTimeout    time.Duration
	halfOpenTimeout time.Duration
}

// CircuitBreakerConfig configures the circuit breaker
type CircuitBreakerConfig struct {
	// MaxFailures is the number of consecutive failures before opening
	MaxFailures int
	// ResetTimeout is how long to wait before attempting to close the circuit
	ResetTimeout time.Duration
	// HalfOpenTimeout is how long to wait in half-open state before fully closing
	HalfOpenTimeout time.Duration
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	if config.MaxFailures <= 0 {
		config.MaxFailures = 5
	}
	if config.ResetTimeout <= 0 {
		config.ResetTimeout = 10 * time.Second
	}
	if config.HalfOpenTimeout <= 0 {
		config.HalfOpenTimeout = 30 * time.Second
	}

	return &CircuitBreaker{
		state:           CircuitClosed,
		maxFailures:     config.MaxFailures,
		resetTimeout:    config.ResetTimeout,
		halfOpenTimeout: config.HalfOpenTimeout,
	}
}

// Allow checks if a request should be allowed
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		// Check if we should try half-open
		if time.Since(cb.lastFailure) > cb.resetTimeout {
			cb.mu.RUnlock()
			cb.mu.Lock()
			cb.state = CircuitHalfOpen
			cb.halfOpenAttempt = time.Now()
			cb.mu.Unlock()
			cb.mu.RLock()
			return true
		}
		return false
	case CircuitHalfOpen:
		// Only allow one request in half-open state
		if time.Since(cb.halfOpenAttempt) > cb.halfOpenTimeout {
			return true
		}
		return false
	default:
		return false
	}
}

// Success records a successful request
func (cb *CircuitBreaker) Success() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures = 0
	cb.lastSuccess = time.Now()

	if cb.state == CircuitHalfOpen {
		cb.state = CircuitClosed
	}
}

// Failure records a failed request
func (cb *CircuitBreaker) Failure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailure = time.Now()

	if cb.failures >= cb.maxFailures {
		cb.state = CircuitOpen
	}
}

// State returns the current state of the circuit breaker
func (cb *CircuitBreaker) State() CircuitBreakerState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Reset forces the circuit breaker back to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = CircuitClosed
	cb.failures = 0
}
