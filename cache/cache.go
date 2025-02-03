package cache

import (
	"bufio"
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

// CacheItem represents a single cache item with a value and an expiration time.
type CacheItem[T any] struct {
	Value      T
	Expiration int64
}

// Cache represents the cache with a generic type.
type Cache[T any] struct {
	items       map[string]CacheItem[T]
	mu          sync.RWMutex
	fileMu      sync.Mutex
	defaultTTL  time.Duration
	flushToFile string

	cfg    *Config
	natsKV nats.KeyValue
}

type Config struct {
	File             string
	DefaultTTL       time.Duration
	FlushInterval    time.Duration
	CleanupInterval  time.Duration
	FlushImmediately bool
	Debug            bool
	NATSURL          string
	NATSBucket       string
}

// New creates a new Cache, optionally initializing from a file and setting a default TTL.
func New[T any](cfg *Config) (*Cache[T], error) {
	c := &Cache[T]{
		items:       make(map[string]CacheItem[T]),
		defaultTTL:  cfg.DefaultTTL,
		flushToFile: cfg.File,
		cfg:         cfg,
	}

	if cfg.File != "" {
		if err := c.loadFromFile(); err != nil {
			return nil, fmt.Errorf("error loading cache from file: %w", err)
		}
	}

	if cfg.NATSURL != "" && cfg.NATSBucket != "" {
		if err := c.setupNATS(); err != nil {
			return nil, fmt.Errorf("error setting up NATS: %w", err)
		}
	}

	go c.cleanup(cfg.CleanupInterval, cfg.FlushInterval)

	return c, nil
}

func (c *Cache[T]) Close() error {
	return c.flushToFileFunc()
}

func (c *Cache[T]) setupNATS() error {
	nc, err := nats.Connect(c.cfg.NATSURL,
		nats.Timeout(5*time.Second),
		nats.PingInterval(time.Second),
		nats.MaxPingsOutstanding(3),
	)
	if err != nil {
		return fmt.Errorf("error connecting to NATS: %w", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		return fmt.Errorf("error getting JetStream context: %w", err)
	}

	kv, err := js.KeyValue(c.cfg.NATSBucket)
	if err != nil && errors.Is(err, nats.ErrBucketNotFound) {
		kv, err = js.CreateKeyValue(&nats.KeyValueConfig{
			Bucket:  c.cfg.NATSBucket,
			Storage: nats.FileStorage,
			TTL:     c.cfg.DefaultTTL,
		})
		if err != nil {
			return fmt.Errorf("create NATS KV: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("error getting NATS KV: %w", err)
	}

	c.natsKV = kv

	return nil
}

// loadFromFile loads cache items from a file.
func (c *Cache[T]) loadFromFile() error {
	file, err := os.Open(c.flushToFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		if f, err := os.Create(c.flushToFile); err != nil {
			return fmt.Errorf("error creating cache file: %w", err)
		} else {
			f.Close()
		}

		return fmt.Errorf("error opening cache file: %w", err)
	}
	defer file.Close()

	decoder := gob.NewDecoder(file)
	return decoder.Decode(&c.items)
}

// flushToFile writes the cache items to the specified file.
func (c *Cache[T]) flushToFileFunc(noLock ...bool) error {
	if c.flushToFile == "" {
		return nil
	}

	c.fileMu.Lock()
	defer c.fileMu.Unlock()

	if len(noLock) == 0 || !noLock[0] {
		c.mu.RLock()
		defer c.mu.RUnlock()
	}

	if c.cfg.Debug {
		fmt.Printf("Flushing %d items to %s\n", len(c.items), c.flushToFile)
	}

	// First check if directory exists, if not create it
	dir := filepath.Dir(c.flushToFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating directory structure: %w", err)
	}

	// Open file with proper flags
	file, err := os.OpenFile(c.flushToFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("opening cache file: %w", err)
	}

	// Create a buffered writer
	bufferedWriter := bufio.NewWriter(file)

	// Encode data
	if err := gob.NewEncoder(bufferedWriter).Encode(c.items); err != nil {
		file.Close()
		return fmt.Errorf("encoding to gob: %w", err)
	}

	// Explicitly flush the buffer before closing
	if err := bufferedWriter.Flush(); err != nil {
		file.Close()
		return fmt.Errorf("flushing buffer: %w", err)
	}

	// Close the file
	if err := file.Close(); err != nil {
		return fmt.Errorf("closing file: %w", err)
	}

	return nil
}

// syncWithNATS synchronizes the cache item with NATS KV.
func (c *Cache[T]) syncWithNATS(key string, item CacheItem[T]) error {
	if c.natsKV == nil {
		return nil
	}

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(item); err != nil {
		return fmt.Errorf("encoding cache item: %w", err)
	}

	if _, err := c.natsKV.Put(key, buf.Bytes()); err != nil {
		return fmt.Errorf("putting to NATS KV (key: '%s'): %w", key, err)
	}

	return nil
}

// Set adds an item to the cache, with an optional TTL. If no TTL is provided, the default TTL is used.
func (c *Cache[T]) Set(key string, value T, ttl ...time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var expiration time.Duration
	if len(ttl) > 0 {
		expiration = ttl[0]
	} else {
		expiration = c.defaultTTL
	}

	var expirationTime int64
	if expiration > 0 {
		expirationTime = time.Now().Add(expiration).UnixNano()
	}

	item := CacheItem[T]{Value: value, Expiration: expirationTime}
	c.items[key] = item

	if err := c.syncWithNATS(key, item); err != nil {
		return fmt.Errorf("error syncing with NATS: %w", err)
	}

	if c.cfg.FlushImmediately {
		return c.flushToFileFunc(true)
	}

	return nil
}

// Edit updates an item in the cache, takes a function that modifies the item.
func (c *Cache[T]) Edit(key string, editFunc func(T) T) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, found := c.items[key]
	if !found {
		return fmt.Errorf("key '%s' not found", key)
	}

	item.Value = editFunc(item.Value)
	c.items[key] = item

	if err := c.syncWithNATS(key, item); err != nil {
		return fmt.Errorf("error syncing with NATS: %w", err)
	}

	if c.cfg.FlushImmediately {
		return c.flushToFileFunc(true)
	}

	return nil
}

// Get retrieves an item from the cache. Returns the item and true if found and not expired, otherwise returns zero value and false.
func (c *Cache[T]) Get(key string) (T, bool) {
	c.mu.RLock()
	item, found := c.items[key]
	c.mu.RUnlock()

	if found && (item.Expiration == 0 || time.Now().UnixNano() <= item.Expiration) {
		return item.Value, true
	}

	// Try to fetch from NATS if not found locally or expired
	if c.natsKV != nil {
		entry, err := c.natsKV.Get(key)
		if err == nil {
			var natsItem CacheItem[T]
			if err := gob.NewDecoder(bytes.NewReader(entry.Value())).Decode(&natsItem); err == nil {
				if natsItem.Expiration == 0 || time.Now().UnixNano() <= natsItem.Expiration {
					c.mu.Lock()
					c.items[key] = natsItem
					c.mu.Unlock()
					return natsItem.Value, true
				}
			}
		}
	}

	return *new(T), false // Return zero value of T if not found or expired
}

func (c *Cache[T]) Del(key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.items, key)

	// Delete from NATS if configured
	if c.natsKV != nil {
		return c.natsKV.Delete(key)
	}

	return nil
}

// Len returns the number of items in the cache.
func (c *Cache[T]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.items)
}

// Keys returns a slice of all keys in the cache.
func (c *Cache[T]) Keys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	keys := make([]string, 0, len(c.items))
	for key := range c.items {
		keys = append(keys, key)
	}

	return keys
}

// cleanup removes expired items from the cache and optionally flushes to file. Intended to be run in a background goroutine.
func (c *Cache[T]) cleanup(interval time.Duration, flushInterval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Minute
	}

	if flushInterval <= 0 {
		flushInterval = 5 * time.Minute
	}

	ticker := time.NewTicker(interval)
	flushTicker := time.NewTicker(flushInterval)
	for {
		select {
		case <-ticker.C:
			c.mu.Lock()
			for key, item := range c.items {
				if item.Expiration > 0 && time.Now().UnixNano() > item.Expiration {
					if c.cfg.Debug {
						fmt.Printf("Cache item %s expired, '%d'\n", key, item.Expiration)
					}

					delete(c.items, key)
				}
			}

			if c.cfg.FlushImmediately {
				c.flushToFileFunc(true)
			}

			c.mu.Unlock()
		case <-flushTicker.C:
			c.flushToFileFunc()
		}
	}
}
