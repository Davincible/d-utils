package tscache

import (
	"fmt"
	"sync"
	"time"
)

// TimeSeriesCache represents a partitioned cache optimized for time series data
type TimeSeriesCache[T any] struct {
	partitions    map[int64]*Partition[T] // Map of partition start time to partition
	partitionSize time.Duration           // Size of each partition
	maxPartitions int                     // Maximum number of partitions to keep
	storage       *Storage[T]             // Persistent storage
	mu            sync.RWMutex            // Global lock for partition management
	cfg           *Config                 // Cache configuration
	stopChan      chan struct{}           // Channel to signal goroutine shutdown
	wg            sync.WaitGroup          // WaitGroup for graceful shutdown
}

// NewTimeSeries creates a new partitioned TimeSeriesCache
func NewTimeSeries[T any](cfg *Config) (*TimeSeriesCache[T], error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Initialize storage
	storage, err := NewStorage[T](NewStorageConfig(cfg))
	if err != nil {
		return nil, fmt.Errorf("initializing storage: %w", err)
	}

	c := &TimeSeriesCache[T]{
		partitions:    make(map[int64]*Partition[T]),
		partitionSize: cfg.PartitionDuration,
		maxPartitions: cfg.MaxPartitions,
		storage:       storage,
		cfg:           cfg,
		stopChan:      make(chan struct{}),
	}

	// Load existing partitions from storage
	if err := c.loadPartitions(); err != nil {
		return nil, fmt.Errorf("loading partitions: %w", err)
	}

	c.wg.Add(1)
	go c.maintenance()

	return c, nil
}

// loadPartitions loads all partitions from storage
func (c *TimeSeriesCache[T]) loadPartitions() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.storage.mu.RLock()
	for startTime := range c.storage.metadata.PartitionMap {
		partition, err := c.storage.LoadPartition(startTime)
		if err != nil {
			c.storage.mu.RUnlock()
			return fmt.Errorf("loading partition %d: %w", startTime, err)
		}
		if partition != nil {
			c.partitions[startTime] = partition
		}
	}
	c.storage.mu.RUnlock()

	return nil
}

// Close gracefully shuts down the cache
func (c *TimeSeriesCache[T]) Close() error {
	close(c.stopChan)
	c.wg.Wait()

	// Save all partitions
	var lastErr error
	c.mu.RLock()
	for _, partition := range c.partitions {
		if err := c.storage.SavePartition(partition); err != nil {
			lastErr = err
		}
	}
	c.mu.RUnlock()

	return lastErr
}

// maintenance handles periodic cleanup and flush operations
func (c *TimeSeriesCache[T]) maintenance() {
	defer c.wg.Done()

	if c.cfg.CleanupInterval == 0 {
		c.cfg.CleanupInterval = time.Second * 30
	}

	if c.cfg.FlushInterval == 0 {
		c.cfg.FlushInterval = time.Minute
	}

	cleanup := time.NewTicker(c.cfg.CleanupInterval)
	flush := time.NewTicker(c.cfg.FlushInterval)
	defer cleanup.Stop()
	defer flush.Stop()

	for {
		select {
		case <-c.stopChan:
			return

		case <-cleanup.C:
			c.cleanup()

		case <-flush.C:
			c.flush()
		}
	}
}

// cleanup removes expired items and empty partitions
func (c *TimeSeriesCache[T]) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for startTime, partition := range c.partitions {
		if !partition.cleanup() {
			delete(c.partitions, startTime)
		}
	}

	// Remove old partitions if we exceed maxPartitions
	if c.maxPartitions > 0 && len(c.partitions) > c.maxPartitions {
		var oldestTime int64 = 1<<63 - 1
		for t := range c.partitions {
			if t < oldestTime {
				oldestTime = t
			}
		}

		if partition, exists := c.partitions[oldestTime]; exists {
			if err := c.storage.SavePartition(partition); err != nil && c.cfg.Debug {
				fmt.Printf("Error saving partition before deletion: %v\n", err)
			}
			delete(c.partitions, oldestTime)
		}
	}
}

// flush saves all modified partitions to storage
func (c *TimeSeriesCache[T]) flush() {
	if c.cfg.StorageDir == "" {
		return
	}

	c.mu.RLock()
	for _, partition := range c.partitions {
		if err := c.storage.SavePartition(partition); err != nil && c.cfg.Debug {
			fmt.Printf("Error flushing partition: %v\n", err)
		}
	}
	c.mu.RUnlock()
}

// getPartition returns the partition for a given timestamp
func (c *TimeSeriesCache[T]) getPartition(t time.Time) *Partition[T] {
	partitionTime := t.Truncate(c.partitionSize)
	partitionStart := partitionTime.UnixNano()

	c.mu.RLock()
	partition, exists := c.partitions[partitionStart]
	c.mu.RUnlock()

	if exists {
		return partition
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double check after acquiring write lock
	if partition, exists = c.partitions[partitionStart]; exists {
		return partition
	}

	// Try to load from storage first
	partition, err := c.storage.LoadPartition(partitionStart)
	if err != nil && c.cfg.Debug {
		fmt.Printf("Error loading partition: %v\n", err)
	}

	if partition == nil {
		// Create new partition if not found in storage
		partition = newPartition[T](partitionStart, c.partitionSize, c.cfg.InitialCapacity)
	}

	c.partitions[partitionStart] = partition
	return partition
}

// Set adds a time series data point
func (c *TimeSeriesCache[T]) Set(timestamp time.Time, value T, ttl ...time.Duration) error {
	_timestamp := timestamp.UnixNano()
	partition := c.getPartition(timestamp)

	var expiration int64
	if len(ttl) > 0 && ttl[0] > 0 {
		expiration = time.Now().Add(ttl[0]).UnixNano()
	}

	item := TimeSeriesItem[T]{
		Value:      value,
		Timestamp:  _timestamp,
		Expiration: expiration,
	}

	partition.insert(item)

	if c.cfg.FlushImmediately {
		return c.storage.SavePartition(partition)
	}

	return nil
}

// GetExact retrieves a value at exact timestamp
func (c *TimeSeriesCache[T]) GetExact(timestamp time.Time) (T, bool) {
	partition := c.getPartition(timestamp)
	item, exists := partition.get(timestamp.UnixNano())
	if !exists {
		return *new(T), false
	}
	return item.Value, true
}

// GetNearest returns the value with the closest timestamp
func (c *TimeSeriesCache[T]) GetNearest(timestamp time.Time, timeRange ...time.Duration) (T, bool) {
	_timestamp := timestamp.UnixNano()
	partition := c.getPartition(timestamp)

	item, found := partition.getNearest(_timestamp)

	withinRange := func(item TimeSeriesItem[T]) bool {
		if len(timeRange) == 0 || timeRange[0] == 0 {
			return true
		}
		diff := item.Timestamp - _timestamp
		if diff < 0 {
			diff = -diff
		}
		return diff <= int64(timeRange[0])
	}

	if found && withinRange(item) {
		return item.Value, true
	}

	return *new(T), false
}

// GetRange returns all values within a time range
func (c *TimeSeriesCache[T]) GetRange(start, end time.Time) []TimeSeriesItem[T] {
	if start.After(end) {
		start, end = end, start
	}

	_start := start.UnixNano()
	_end := end.UnixNano()

	result := make([]TimeSeriesItem[T], 0)

	c.mu.RLock()
	defer c.mu.RUnlock()

	startPartition := start.Truncate(c.partitionSize).UnixNano()
	endPartition := end.Truncate(c.partitionSize).UnixNano()

	for partitionTime := startPartition; partitionTime <= endPartition; partitionTime += int64(c.partitionSize) {
		if partition, exists := c.partitions[partitionTime]; exists {
			items := partition.getRange(_start, _end)
			result = append(result, items...)
		}
	}

	return result
}

// Clear removes all items from the cache
func (c *TimeSeriesCache[T]) Clear() error {
	c.mu.Lock()
	oldPartitions := c.partitions
	c.partitions = make(map[int64]*Partition[T])
	c.mu.Unlock()

	// Save empty state to all partitions
	for _, partition := range oldPartitions {
		if err := c.storage.SavePartition(partition); err != nil {
			return fmt.Errorf("saving cleared partition: %w", err)
		}
	}

	return nil
}

// DeleteRange removes all items within the specified time range
func (c *TimeSeriesCache[T]) DeleteRange(start, end time.Time) error {
	if start.After(end) {
		start, end = end, start
	}

	_start := start.UnixNano()
	_end := end.UnixNano()

	c.mu.Lock()
	defer c.mu.Unlock()

	startPartition := start.Truncate(c.partitionSize).UnixNano()
	endPartition := end.Truncate(c.partitionSize).UnixNano()

	for partitionTime := startPartition; partitionTime <= endPartition; partitionTime += int64(c.partitionSize) {
		if partition, exists := c.partitions[partitionTime]; exists {
			// If entire partition is within range, delete it
			if partitionTime >= _start && partitionTime+int64(c.partitionSize) <= _end {
				delete(c.partitions, partitionTime)
				continue
			}

			// Otherwise, remove individual items
			partition.mu.Lock()
			newItems := make([]TimeSeriesItem[T], 0, len(partition.Items))
			newMap := make(map[int64]int)

			for _, item := range partition.Items {
				if item.Timestamp < _start || item.Timestamp > _end {
					idx := len(newItems)
					newItems = append(newItems, item)
					newMap[item.Timestamp] = idx
				}
			}

			partition.Items = newItems
			partition.ItemsMap = newMap
			partition.mu.Unlock()

			// Save modified partition
			if err := c.storage.SavePartition(partition); err != nil {
				return fmt.Errorf("saving modified partition: %w", err)
			}
		}
	}

	return nil
}
