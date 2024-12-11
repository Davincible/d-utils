package tscache

import (
	"sort"
	"time"
)

// Iterator represents a time series iterator
type Iterator[T any] struct {
	cache       *TimeSeriesCache[T]
	partitions  []int64 // Sorted partition start times
	currPartIdx int     // Current partition index
	currItemIdx int     // Current item index in current partition
	start, end  int64   // Time range for iteration
}

// NewIterator creates an iterator for the specified time range
func (c *TimeSeriesCache[T]) NewIterator(start, end time.Time) *Iterator[T] {
	_start := start.UnixNano()
	_end := end.UnixNano()

	if _start > _end {
		_start, _end = _end, _start
	}

	c.mu.RLock()
	partitions := make([]int64, 0, len(c.partitions))
	for startTime := range c.partitions {
		if startTime+int64(c.partitionSize) >= _start && startTime <= _end {
			partitions = append(partitions, startTime)
		}
	}
	c.mu.RUnlock()

	sort.Slice(partitions, func(i, j int) bool {
		return partitions[i] < partitions[j]
	})

	return &Iterator[T]{
		cache:      c,
		partitions: partitions,
		start:      _start,
		end:        _end,
	}
}

// Next returns the next item in the iteration
func (it *Iterator[T]) Next() (TimeSeriesItem[T], bool) {
	for it.currPartIdx < len(it.partitions) {
		partitionStart := it.partitions[it.currPartIdx]

		it.cache.mu.RLock()
		partition, exists := it.cache.partitions[partitionStart]
		it.cache.mu.RUnlock()

		if !exists {
			it.currPartIdx++
			it.currItemIdx = 0
			continue
		}

		partition.mu.RLock()
		if it.currItemIdx >= len(partition.Items) {
			partition.mu.RUnlock()
			it.currPartIdx++
			it.currItemIdx = 0
			continue
		}

		item := partition.Items[it.currItemIdx]
		it.currItemIdx++
		partition.mu.RUnlock()

		if item.Timestamp >= it.start && item.Timestamp <= it.end {
			if item.Expiration == 0 || time.Now().UnixNano() <= item.Expiration {
				return item, true
			}
		}
	}

	return TimeSeriesItem[T]{}, false
}

// Reset resets the iterator to the beginning
func (it *Iterator[T]) Reset() {
	it.currPartIdx = 0
	it.currItemIdx = 0
}
