package tscache

import (
	"fmt"
	"time"
)

// Config represents the cache configuration
type Config struct {
	// Storage configuration
	StorageDir        string        // Directory for storing cache files
	PartitionDuration time.Duration // Duration of each partition
	MaxPartitions     int           // Maximum number of partitions to keep

	// Cache behavior
	CleanupInterval  time.Duration // Interval for cleanup operations
	FlushInterval    time.Duration // Interval for flushing to disk
	FlushImmediately bool          // Whether to flush immediately on writes

	// Performance tuning
	InitialCapacity int // Initial capacity for slices/maps
	BufferSize      int // Size of I/O buffers

	// Debug options
	Debug bool // Enable debug logging
}

// StorageConfig represents configuration specific to the storage system
type StorageConfig struct {
	BaseDir         string        // Base directory for all storage files
	PartitionFormat string        // Format for partition files (default: "2006-01-02")
	BufferSize      int           // Size of the write buffer
	CompressData    bool          // Whether to compress data
	RetentionPeriod time.Duration // How long to keep old partitions
	Debug           bool          // Enable debug logging
}

// DefaultConfig returns a new Config with default values
func DefaultConfig() *Config {
	return &Config{
		PartitionDuration: time.Hour * 24, // 1 day
		MaxPartitions:     7,              // 1 week
		CleanupInterval:   time.Minute * 5,
		FlushInterval:     time.Minute * 1,
		FlushImmediately:  false,
		InitialCapacity:   1024,
		BufferSize:        1 << 20, // 1MB
		Debug:             false,
	}
}

// Validate checks the configuration for errors
func (c *Config) Validate() error {
	if c.PartitionDuration < 0 {
		return fmt.Errorf("partition duration must be positive")
	}
	if c.MaxPartitions < 0 {
		return fmt.Errorf("max partitions must be non-negative")
	}
	if c.CleanupInterval < 0 {
		// return fmt.Errorf("cleanup interval must be positive")
		c.CleanupInterval = time.Minute * 5
	}
	if c.FlushInterval < 0 {
		// return fmt.Errorf("flush interal must be positive")
		c.FlushInterval = time.Minute * 1
	}
	if c.InitialCapacity <= 0 {
		// return fmt.Errorf("initial capacity must be positive")
		c.InitialCapacity = 1024
	}
	if c.BufferSize <= 0 {
		// return fmt.Errorf("buffer size must be positive")
		c.BufferSize = 1 << 20 // 1MB
	}
	// if c.StorageDir == "" {
	// 	return fmt.Errorf("storage directory must be specified")
	// }
	return nil
}

// NewStorageConfig creates a StorageConfig from a Config
func NewStorageConfig(cfg *Config) StorageConfig {
	return StorageConfig{
		BaseDir:         cfg.StorageDir,
		PartitionFormat: "2006-01-02",
		BufferSize:      cfg.BufferSize,
		CompressData:    false, // Future enhancement
		RetentionPeriod: time.Duration(cfg.MaxPartitions) * cfg.PartitionDuration,
		Debug:           cfg.Debug,
	}
}
