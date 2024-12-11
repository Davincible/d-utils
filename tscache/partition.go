package tscache

import (
	"sort"
	"sync"
	"time"
)

// TimeSeriesItem represents a single time series data point
type TimeSeriesItem[T any] struct {
	Value      T
	Timestamp  int64
	Expiration int64
}

// Partition represents a time-based partition of the cache
type Partition[T any] struct {
	Items     []TimeSeriesItem[T] // Sorted slice for binary search
	ItemsMap  map[int64]int       // Map of timestamp to index in items slice
	StartTime int64               // Start time of this partition
	EndTime   int64               // End time of this partition
	mu        sync.RWMutex        // Per-partition lock for better concurrency
}

// newPartition creates a new partition
func newPartition[T any](startTime int64, duration time.Duration, initialCapacity int) *Partition[T] {
	return &Partition[T]{
		Items:     make([]TimeSeriesItem[T], 0, initialCapacity),
		ItemsMap:  make(map[int64]int, initialCapacity),
		StartTime: startTime,
		EndTime:   startTime + int64(duration),
	}
}

// insert adds or updates an item in the partition
func (p *Partition[T]) insert(item TimeSeriesItem[T]) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check if timestamp already exists
	if idx, exists := p.ItemsMap[item.Timestamp]; exists {
		p.Items[idx] = item
		return
	}

	// Find insert position
	idx := sort.Search(len(p.Items), func(i int) bool {
		return p.Items[i].Timestamp >= item.Timestamp
	})

	// Insert item
	p.Items = append(p.Items, TimeSeriesItem[T]{})
	copy(p.Items[idx+1:], p.Items[idx:])
	p.Items[idx] = item

	// Update index map
	for i := idx; i < len(p.Items); i++ {
		p.ItemsMap[p.Items[i].Timestamp] = i
	}
}

// get retrieves an item by exact timestamp
func (p *Partition[T]) get(timestamp int64) (TimeSeriesItem[T], bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if idx, exists := p.ItemsMap[timestamp]; exists {
		item := p.Items[idx]
		if item.Expiration == 0 || time.Now().UnixNano() <= item.Expiration {
			return item, true
		}
	}

	return TimeSeriesItem[T]{}, false
}

// getNearest finds the nearest item to the given timestamp
func (p *Partition[T]) getNearest(timestamp int64) (TimeSeriesItem[T], bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.Items) == 0 {
		return TimeSeriesItem[T]{}, false
	}

	idx := sort.Search(len(p.Items), func(i int) bool {
		return p.Items[i].Timestamp >= timestamp
	})

	if idx == len(p.Items) {
		return p.Items[len(p.Items)-1], true
	}

	if idx == 0 {
		return p.Items[0], true
	}

	// Compare with previous item to find closest
	curr := p.Items[idx]
	prev := p.Items[idx-1]
	if timestamp-prev.Timestamp < curr.Timestamp-timestamp {
		return prev, true
	}

	return curr, true
}

// getRange returns all items within the given time range
func (p *Partition[T]) getRange(start, end int64) []TimeSeriesItem[T] {
	p.mu.RLock()
	defer p.mu.RUnlock()

	startIdx := sort.Search(len(p.Items), func(i int) bool {
		return p.Items[i].Timestamp >= start
	})

	endIdx := sort.Search(len(p.Items), func(i int) bool {
		return p.Items[i].Timestamp > end
	})

	result := make([]TimeSeriesItem[T], 0, endIdx-startIdx)
	now := time.Now().UnixNano()

	for i := startIdx; i < endIdx; i++ {
		item := p.Items[i]
		if item.Expiration == 0 || now <= item.Expiration {
			result = append(result, item)
		}
	}

	return result
}

// cleanup removes expired items
func (p *Partition[T]) cleanup() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now().UnixNano()
	newItems := make([]TimeSeriesItem[T], 0, len(p.Items))
	newMap := make(map[int64]int, len(p.Items))

	for _, item := range p.Items {
		if item.Expiration == 0 || now <= item.Expiration {
			idx := len(newItems)
			newItems = append(newItems, item)
			newMap[item.Timestamp] = idx
		}
	}

	p.Items = newItems
	p.ItemsMap = newMap

	return len(p.Items) > 0
}
