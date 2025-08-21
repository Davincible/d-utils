package apicache

import (
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	// ErrCacheMiss indicates the key was not found in cache
	ErrCacheMiss = errors.New("cache miss")
	// ErrStaleData indicates the cached data is stale but a refresh is in progress
	ErrStaleData = errors.New("stale data")
	// ErrInvalidTTL indicates an invalid TTL was provided
	ErrInvalidTTL = errors.New("invalid TTL")
	// ErrCircuitOpen indicates the circuit breaker is open
	ErrCircuitOpen = errors.New("circuit breaker is open")
)

// Item represents a cached item with metadata
type Item[T any] struct {
	Value       T
	CreatedAt   time.Time
	ExpiresAt   time.Time
	LastAccess  time.Time
	AccessCount int64
}

// FetchFunc is a function that fetches fresh data from the source (e.g. database)
type FetchFunc[T any] func(ctx context.Context) (T, error)

// Cache represents the API cache with request coalescing
type Cache[T any] struct {
	items map[string]*Item[T]
	mu    sync.RWMutex

	// Request coalescing
	inflight   map[string]*inflightRequest[T]
	inflightMu sync.RWMutex

	// Configuration
	defaultTTL   time.Duration
	staleTimeout time.Duration
	maxItems     int
	persistPath  string

	// Circuit breaker
	circuitBreaker *CircuitBreaker

	// Prometheus metrics
	metrics *PrometheusMetrics

	// Hooks for extensibility
	onEviction func(key string, item *Item[T])
	onSet      func(key string, item *Item[T])
	onGet      func(key string, item *Item[T])
}

// inflightRequest represents an in-progress fetch operation
type inflightRequest[T any] struct {
	done    chan struct{}
	result  T
	err     error
	started time.Time
}

// Options configures the cache behavior
type Options struct {
	// DefaultTTL is the default time-to-live for cache entries
	DefaultTTL time.Duration
	// StaleTimeout is how long to wait before considering stale data unusable
	StaleTimeout time.Duration
	// MaxItems is the maximum number of items to store in cache
	MaxItems int
	// PersistPath is the file path to persist cache to disk (optional)
	PersistPath string
	// CircuitBreaker configures the circuit breaker (optional)
	CircuitBreaker *CircuitBreakerConfig
	// MetricsNamespace for Prometheus metrics (optional)
	MetricsNamespace string
	// OnEviction is called when an item is evicted
	OnEviction func(key string, item interface{})
	// OnSet is called when an item is set
	OnSet func(key string, item interface{})
	// OnGet is called when an item is accessed
	OnGet func(key string, item interface{})
}

// New creates a new API cache instance
func New[T any](opts Options) (*Cache[T], error) {
	if opts.DefaultTTL <= 0 {
		return nil, fmt.Errorf("%w: DefaultTTL must be positive", ErrInvalidTTL)
	}

	if opts.StaleTimeout <= 0 {
		opts.StaleTimeout = opts.DefaultTTL * 2
	}

	if opts.MaxItems <= 0 {
		opts.MaxItems = 10000
	}

	cache := &Cache[T]{
		items:        make(map[string]*Item[T]),
		inflight:     make(map[string]*inflightRequest[T]),
		defaultTTL:   opts.DefaultTTL,
		staleTimeout: opts.StaleTimeout,
		maxItems:     opts.MaxItems,
		persistPath:  opts.PersistPath,
	}

	// Setup circuit breaker if configured
	if opts.CircuitBreaker != nil {
		cache.circuitBreaker = NewCircuitBreaker(*opts.CircuitBreaker)
	}

	// Setup Prometheus metrics if namespace provided
	if opts.MetricsNamespace != "" {
		cache.metrics = NewPrometheusMetrics(opts.MetricsNamespace)
	}

	// Setup hooks
	if opts.OnEviction != nil {
		cache.onEviction = func(key string, item *Item[T]) {
			opts.OnEviction(key, item)
		}
	}
	if opts.OnSet != nil {
		cache.onSet = func(key string, item *Item[T]) {
			opts.OnSet(key, item)
		}
	}
	if opts.OnGet != nil {
		cache.onGet = func(key string, item *Item[T]) {
			opts.OnGet(key, item)
		}
	}

	// Load persisted cache if path provided
	if opts.PersistPath != "" {
		if err := cache.loadFromDisk(); err != nil {
			return nil, fmt.Errorf("load cache from disk: %w", err)
		}
	}

	return cache, nil
}

// GetOrFetch attempts to get an item from cache, falling back to fetching from source
func (c *Cache[T]) GetOrFetch(ctx context.Context, key string, fetch FetchFunc[T]) (T, error) {
	// Check circuit breaker if configured
	if c.circuitBreaker != nil && !c.circuitBreaker.Allow() {
		var zero T
		return zero, ErrCircuitOpen
	}

	// Try to get from cache first
	if item, ok := c.get(key); ok {
		if time.Now().Before(item.ExpiresAt) {
			return item.Value, nil
		}

		// Data is stale, but we'll return it while fetching fresh data
		if time.Now().Before(item.ExpiresAt.Add(c.staleTimeout)) {
			go c.refresh(context.Background(), key, fetch)
			if c.metrics != nil {
				c.metrics.staleServes.WithLabelValues("apicache").Inc()
			}
			return item.Value, nil
		}
	}

	// Need to fetch fresh data
	return c.fetchWithCoalescing(ctx, key, fetch)
}

// BatchGetOrFetch fetches multiple items at once
func (c *Cache[T]) BatchGetOrFetch(ctx context.Context, keys []string, fetch func(ctx context.Context, keys []string) (map[string]T, error)) (map[string]T, error) {
	result := make(map[string]T)
	missingKeys := make([]string, 0)

	// Try to get items from cache first
	for _, key := range keys {
		if item, ok := c.get(key); ok && time.Now().Before(item.ExpiresAt) {
			result[key] = item.Value
		} else {
			missingKeys = append(missingKeys, key)
		}
	}

	// Fetch missing items
	if len(missingKeys) > 0 {
		fetched, err := fetch(ctx, missingKeys)
		if err != nil {
			return result, err
		}

		// Store fetched items in cache
		for k, v := range fetched {
			c.set(k, v)
			result[k] = v
		}
	}

	return result, nil
}

// WarmUp pre-populates the cache with data
func (c *Cache[T]) WarmUp(ctx context.Context, keys []string, fetch func(ctx context.Context, keys []string) (map[string]T, error)) error {
	items, err := fetch(ctx, keys)
	if err != nil {
		return err
	}

	for k, v := range items {
		c.set(k, v)
	}

	return nil
}

// loadFromDisk loads the cache state from disk
func (c *Cache[T]) loadFromDisk() error {
	if c.persistPath == "" {
		return nil
	}

	file, err := os.Open(c.persistPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	return gob.NewDecoder(file).Decode(&c.items)
}

// SaveToDisk persists the cache state to disk
func (c *Cache[T]) SaveToDisk() error {
	if c.persistPath == "" {
		return nil
	}

	file, err := os.Create(c.persistPath)
	if err != nil {
		return err
	}
	defer file.Close()

	return gob.NewEncoder(file).Encode(c.items)
}

// get retrieves an item from cache
func (c *Cache[T]) get(key string) (*Item[T], bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, exists := c.items[key]
	if !exists {
		if c.metrics != nil {
			c.metrics.misses.WithLabelValues("apicache").Inc()
		}
		return nil, false
	}

	// Update access metadata
	item.LastAccess = time.Now()
	item.AccessCount++

	if c.metrics != nil {
		c.metrics.hits.WithLabelValues("apicache").Inc()
	}

	if c.onGet != nil {
		c.onGet(key, item)
	}

	return item, true
}

// fetchWithCoalescing handles concurrent requests for the same key
func (c *Cache[T]) fetchWithCoalescing(ctx context.Context, key string, fetch FetchFunc[T]) (T, error) {
	// Check if there's already a request in flight
	c.inflightMu.Lock()
	if req, exists := c.inflight[key]; exists {
		c.inflightMu.Unlock()
		select {
		case <-req.done:
			return req.result, req.err
		case <-ctx.Done():
			var zero T
			return zero, ctx.Err()
		}
	}

	// Create new inflight request
	req := &inflightRequest[T]{
		done:    make(chan struct{}),
		started: time.Now(),
	}
	c.inflight[key] = req
	c.inflightMu.Unlock()

	// Track metrics
	if c.metrics != nil {
		c.metrics.inFlight.WithLabelValues("apicache").Inc()
		defer c.metrics.inFlight.WithLabelValues("apicache").Dec()
	}

	// Cleanup when done
	defer func() {
		c.inflightMu.Lock()
		delete(c.inflight, key)
		c.inflightMu.Unlock()
		close(req.done)
	}()

	// Start timer for fetch duration
	var timer *prometheus.Timer
	if c.metrics != nil {
		timer = prometheus.NewTimer(c.metrics.fetchDuration.WithLabelValues("apicache"))
		defer timer.ObserveDuration()
	}

	// Fetch fresh data
	value, err := fetch(ctx)
	if err != nil {
		if c.metrics != nil {
			c.metrics.errors.WithLabelValues("apicache", "fetch").Inc()
		}
		if c.circuitBreaker != nil {
			c.circuitBreaker.Failure()
		}
		req.err = err
		return value, err
	}

	if c.circuitBreaker != nil {
		c.circuitBreaker.Success()
	}

	// Store in cache
	c.set(key, value)
	req.result = value

	return value, nil
}

// set stores an item in cache
func (c *Cache[T]) set(key string, value T) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict items if we're at capacity
	if len(c.items) >= c.maxItems {
		c.evict()
	}

	now := time.Now()
	c.items[key] = &Item[T]{
		Value:      value,
		CreatedAt:  now,
		ExpiresAt:  now.Add(c.defaultTTL),
		LastAccess: now,
	}
}

// refresh updates a cache entry in the background
func (c *Cache[T]) refresh(ctx context.Context, key string, fetch FetchFunc[T]) {
	value, err := c.fetchWithCoalescing(ctx, key, fetch)
	if err != nil {
		return // Keep using stale data
	}

	c.set(key, value)
}

// evict removes the least recently accessed items when cache is full
func (c *Cache[T]) evict() {
	// Simple LRU eviction
	var oldest time.Time
	var oldestKey string
	first := true

	for k, v := range c.items {
		if first || v.LastAccess.Before(oldest) {
			oldest = v.LastAccess
			oldestKey = k
			first = false
		}
	}

	if oldestKey != "" {
		delete(c.items, oldestKey)
	}
}

// Invalidate removes an item from the cache
func (c *Cache[T]) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}

// Clear removes all items from the cache
func (c *Cache[T]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]*Item[T])
}

// Metrics returns cache statistics
type Metrics struct {
	Items    int
	Inflight int
}

// GetMetrics returns current cache metrics
func (c *Cache[T]) GetMetrics() Metrics {
	c.mu.RLock()
	c.inflightMu.RLock()
	defer c.mu.RUnlock()
	defer c.inflightMu.RUnlock()

	return Metrics{
		Items:    len(c.items),
		Inflight: len(c.inflight),
	}
}

// GetCircuitBreaker returns the circuit breaker instance
func (c *Cache[T]) GetCircuitBreaker() *CircuitBreaker {
	return c.circuitBreaker
}

// GetPrometheusMetrics returns the Prometheus metrics instance
func (c *Cache[T]) GetPrometheusMetrics() *PrometheusMetrics {
	return c.metrics
}
