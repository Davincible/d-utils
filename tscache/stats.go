package tscache

import (
	"os"
	"path/filepath"
	"time"
)

// CacheStats represents statistics about the cache
type CacheStats struct {
	TotalItems      int           // Total number of items across all partitions
	PartitionCount  int           // Number of partitions
	PartitionStats  map[int64]int // Map of partition start time to item count
	OldestTimestamp int64         // Oldest item timestamp
	NewestTimestamp int64         // Newest item timestamp
	MemoryUsage     int64         // Approximate memory usage in bytes
	StorageUsage    int64         // Approximate storage usage in bytes
}

// Stats returns current cache statistics
func (c *TimeSeriesCache[T]) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := CacheStats{
		PartitionStats:  make(map[int64]int),
		OldestTimestamp: 1<<63 - 1, // Max int64
		NewestTimestamp: -1,
	}

	stats.PartitionCount = len(c.partitions)

	var memoryUsage int64
	for startTime, partition := range c.partitions {
		partition.mu.RLock()
		itemCount := len(partition.Items)
		stats.TotalItems += itemCount
		stats.PartitionStats[startTime] = itemCount

		// Update timestamps
		if itemCount > 0 {
			if partition.Items[0].Timestamp < stats.OldestTimestamp {
				stats.OldestTimestamp = partition.Items[0].Timestamp
			}
			if partition.Items[itemCount-1].Timestamp > stats.NewestTimestamp {
				stats.NewestTimestamp = partition.Items[itemCount-1].Timestamp
			}
		}

		// Approximate memory usage
		memoryUsage += int64(itemCount) * 32 // Rough estimate per item
		partition.mu.RUnlock()
	}

	stats.MemoryUsage = memoryUsage

	// Get storage usage if available
	if c.storage != nil {
		stats.StorageUsage = c.getStorageUsage()
	}

	return stats
}

func (c *TimeSeriesCache[T]) getStorageUsage() int64 {
	var total int64

	// Walk through storage directory
	filepath.Walk(c.storage.cfg.BaseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})

	return total
}

// PartitionStats returns statistics for a specific partition
type PartitionStats struct {
	StartTime       int64
	EndTime         int64
	ItemCount       int
	OldestTimestamp int64
	NewestTimestamp int64
	MemoryUsage     int64
}

// GetPartitionStats returns statistics for a specific partition
func (c *TimeSeriesCache[T]) GetPartitionStats(partitionTime time.Time) (PartitionStats, error) {
	startTime := partitionTime.Truncate(c.partitionSize).UnixNano()

	c.mu.RLock()
	partition, exists := c.partitions[startTime]
	c.mu.RUnlock()

	if !exists {
		return PartitionStats{}, ErrPartitionNotFound
	}

	partition.mu.RLock()
	defer partition.mu.RUnlock()

	stats := PartitionStats{
		StartTime: partition.StartTime,
		EndTime:   partition.EndTime,
		ItemCount: len(partition.Items),
	}

	if stats.ItemCount > 0 {
		stats.OldestTimestamp = partition.Items[0].Timestamp
		stats.NewestTimestamp = partition.Items[stats.ItemCount-1].Timestamp
		stats.MemoryUsage = int64(stats.ItemCount) * 32 // Rough estimate
	}

	return stats, nil
}
