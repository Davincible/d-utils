package tscache

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vmihailenco/msgpack/v5"
)

// Storage handles persistent storage operations for the time series cache
type Storage[T any] struct {
	cfg      StorageConfig
	mu       sync.RWMutex
	fileMu   sync.Mutex
	metadata *storageMetadata
}

type storageMetadata struct {
	LastWrite    time.Time
	PartitionMap map[int64]string // Maps partition start time to filename
}

// NewStorage creates a new storage system
func NewStorage[T any](cfg StorageConfig) (*Storage[T], error) {
	if cfg.BaseDir == "" {
		return nil, WrapStorageError("new", "", ErrInvalidConfig)
	}

	if cfg.PartitionFormat == "" {
		cfg.PartitionFormat = "2006-01-02"
	}

	if cfg.BufferSize == 0 {
		cfg.BufferSize = 1 << 20 // 1MB default
	}

	// Create base directory if it doesn't exist
	if err := os.MkdirAll(cfg.BaseDir, 0755); err != nil {
		return nil, WrapStorageError("new", cfg.BaseDir, err)
	}

	s := &Storage[T]{
		cfg: cfg,
		metadata: &storageMetadata{
			PartitionMap: make(map[int64]string),
		},
	}

	// Load existing metadata
	if err := s.loadMetadata(); err != nil {
		return nil, err
	}

	return s, nil
}

// getPartitionPath returns the full path for a partition file
func (s *Storage[T]) getPartitionPath(partitionTime int64) string {
	t := time.Unix(0, partitionTime)
	filename := fmt.Sprintf("partition_%s_%d.dat", t.Format(s.cfg.PartitionFormat), partitionTime)
	return filepath.Join(s.cfg.BaseDir, filename)
}

// SavePartition saves a single partition to disk
func (s *Storage[T]) SavePartition(partition *Partition[T]) error {
	if s.cfg.BaseDir == "" {
		return nil
	}

	s.fileMu.Lock()
	defer s.fileMu.Unlock()

	partitionPath := s.getPartitionPath(partition.StartTime)
	tempPath := partitionPath + ".tmp"

	// Create temporary file
	file, err := os.OpenFile(tempPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return WrapStorageError("save", tempPath, err)
	}
	defer func() {
		file.Close()
		os.Remove(tempPath) // Clean up temp file in case of failure
	}()

	writer := bufio.NewWriterSize(file, s.cfg.BufferSize)

	// Take a snapshot of partition data
	partition.mu.RLock()
	snapshot := &Partition[T]{
		Items:     make([]TimeSeriesItem[T], len(partition.Items)),
		ItemsMap:  make(map[int64]int, len(partition.ItemsMap)),
		StartTime: partition.StartTime,
		EndTime:   partition.EndTime,
	}
	copy(snapshot.Items, partition.Items)
	for k, v := range partition.ItemsMap {
		snapshot.ItemsMap[k] = v
	}
	partition.mu.RUnlock()

	// Encode partition data using MessagePack
	enc := msgpack.NewEncoder(writer)
	if err := enc.Encode(snapshot); err != nil {
		return WrapStorageError("encode", tempPath, err)
	}

	// Flush buffer
	if err := writer.Flush(); err != nil {
		return WrapStorageError("flush", tempPath, err)
	}

	// Sync file to disk
	if err := file.Sync(); err != nil {
		return WrapStorageError("sync", tempPath, err)
	}

	// Close file before rename
	if err := file.Close(); err != nil {
		return WrapStorageError("close", tempPath, err)
	}

	// Atomic rename
	if err := os.Rename(tempPath, partitionPath); err != nil {
		return WrapStorageError("rename", partitionPath, err)
	}

	// Update metadata
	s.mu.Lock()
	s.metadata.LastWrite = time.Now()
	s.metadata.PartitionMap[partition.StartTime] = filepath.Base(partitionPath)
	s.mu.Unlock()

	// Save updated metadata
	if err := s.saveMetadata(); err != nil {
		return err
	}

	if s.cfg.Debug {
		fmt.Printf("Saved partition %s with %d items\n", partitionPath, len(snapshot.Items))
	}

	return nil
}

// LoadPartition loads a partition from disk
func (s *Storage[T]) LoadPartition(partitionTime int64) (*Partition[T], error) {
	s.fileMu.Lock()
	defer s.fileMu.Unlock()

	return s.loadPartition(partitionTime)
}

// loadPartition loads a partition from disk.
// This method is used internally and assumes the caller holds the fileMu lock.
func (s *Storage[T]) loadPartition(partitionTime int64) (*Partition[T], error) {
	partitionPath := s.getPartitionPath(partitionTime)

	file, err := os.Open(partitionPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, WrapStorageError("load", partitionPath, err)
	}
	defer file.Close()

	reader := bufio.NewReaderSize(file, s.cfg.BufferSize)

	var partition Partition[T]
	dec := msgpack.NewDecoder(reader)
	if err := dec.Decode(&partition); err != nil {
		if err != io.EOF {
			return nil, WrapStorageError("decode", partitionPath, err)
		}
	}

	return &partition, nil
}

// DeleteOldPartitions removes partitions older than retention period
func (s *Storage[T]) DeleteOldPartitions() error {
	if s.cfg.RetentionPeriod == 0 {
		return nil
	}

	cutoff := time.Now().Add(-s.cfg.RetentionPeriod).UnixNano()

	s.mu.Lock()
	defer s.mu.Unlock()

	for partitionTime, filename := range s.metadata.PartitionMap {
		if partitionTime < cutoff {
			path := filepath.Join(s.cfg.BaseDir, filename)
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return WrapStorageError("delete", path, err)
			}

			delete(s.metadata.PartitionMap, partitionTime)

			if s.cfg.Debug {
				fmt.Printf("Deleted old partition: %s\n", path)
			}
		}
	}

	return s.saveMetadata()
}

// loadMetadata loads storage metadata from disk
func (s *Storage[T]) loadMetadata() error {
	path := filepath.Join(s.cfg.BaseDir, "metadata.dat")

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return WrapStorageError("load-metadata", path, err)
	}
	defer file.Close()

	dec := msgpack.NewDecoder(file)
	if err := dec.Decode(s.metadata); err != nil && err != io.EOF {
		return WrapStorageError("decode-metadata", path, err)
	}

	return nil
}

// saveMetadata saves storage metadata to disk
func (s *Storage[T]) saveMetadata() error {
	path := filepath.Join(s.cfg.BaseDir, "metadata.dat")
	tempPath := path + ".tmp"

	file, err := os.OpenFile(tempPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return WrapStorageError("save-metadata", tempPath, err)
	}
	defer func() {
		file.Close()
		os.Remove(tempPath)
	}()

	enc := msgpack.NewEncoder(file)
	if err := enc.Encode(s.metadata); err != nil {
		return WrapStorageError("encode-metadata", tempPath, err)
	}

	if err := file.Sync(); err != nil {
		return WrapStorageError("sync-metadata", tempPath, err)
	}

	if err := file.Close(); err != nil {
		return WrapStorageError("close-metadata", tempPath, err)
	}

	if err := os.Rename(tempPath, path); err != nil {
		return WrapStorageError("rename-metadata", path, err)
	}

	return nil
}

// Repair attempts to repair corrupted storage
func (s *Storage[T]) Repair() error {
	s.fileMu.Lock()
	defer s.fileMu.Unlock()

	files, err := os.ReadDir(s.cfg.BaseDir)
	if err != nil {
		return WrapStorageError("repair-read", s.cfg.BaseDir, err)
	}

	newMetadata := &storageMetadata{
		PartitionMap: make(map[int64]string),
	}

	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != ".dat" {
			continue
		}

		// Skip metadata file
		if file.Name() == "metadata.dat" {
			continue
		}

		// Extract partition time from filename
		// Format: partition_2024-12-11_1733932524705594573.dat
		parts := strings.Split(file.Name(), "_")
		if len(parts) != 3 {
			if s.cfg.Debug {
				fmt.Printf("Skipping malformed filename: %s\n", file.Name())
			}
			continue
		}

		// Get the timestamp part (remove .dat extension)
		timestampStr := strings.TrimSuffix(parts[2], ".dat")
		partitionTime, err := strconv.ParseInt(timestampStr, 10, 64)
		if err != nil {
			if s.cfg.Debug {
				fmt.Printf("Failed to parse timestamp from filename: %s, error: %v\n", file.Name(), err)
			}
			continue
		}

		// Try to load the partition to verify integrity
		path := filepath.Join(s.cfg.BaseDir, file.Name())
		if partition, err := s.loadPartition(partitionTime); err == nil && partition != nil {
			newMetadata.PartitionMap[partitionTime] = file.Name()
			if s.cfg.Debug {
				fmt.Printf("Successfully verified partition: %s\n", file.Name())
			}
		} else if s.cfg.Debug {
			fmt.Printf("Corrupted partition file found: %s, error: %v\n", path, err)
		}
	}

	s.metadata = newMetadata
	return s.saveMetadata()
}
