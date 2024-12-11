package tscache

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewStorage(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name    string
		cfg     StorageConfig
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: StorageConfig{
				BaseDir:    tempDir,
				BufferSize: 1024,
				Debug:      true,
			},
			wantErr: false,
		},
		{
			name: "missing base dir",
			cfg: StorageConfig{
				BufferSize: 1024,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage, err := NewStorage[float64](tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewStorage() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && storage == nil {
				t.Error("expected non-nil storage")
			}
		})
	}
}

func TestStorageSaveLoad(t *testing.T) {
	tempDir := t.TempDir()
	storage, err := NewStorage[float64](StorageConfig{BaseDir: tempDir})
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}

	// Create test partition
	now := time.Now().UnixNano()
	partition := newPartition[float64](now, time.Hour, 10)
	partition.insert(TimeSeriesItem[float64]{
		Timestamp: now,
		Value:     42.0,
	})

	// Test save
	if err := storage.SavePartition(partition); err != nil {
		t.Fatalf("SavePartition() error = %v", err)
	}

	// Verify file exists
	files, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(files) == 0 {
		t.Error("no files created")
	}

	// Test load
	loaded, err := storage.LoadPartition(now)
	if err != nil {
		t.Fatalf("LoadPartition() error = %v", err)
	}
	if loaded == nil {
		t.Fatal("loaded partition is nil")
	}
	if len(loaded.Items) != 1 {
		t.Errorf("got %d items, want 1", len(loaded.Items))
	}
	if loaded.Items[0].Value != 42.0 {
		t.Errorf("got value %v, want 42.0", loaded.Items[0].Value)
	}
}

func TestStorageDeleteOldPartitions(t *testing.T) {
	tempDir := t.TempDir()
	storage, err := NewStorage[float64](StorageConfig{
		BaseDir:         tempDir,
		RetentionPeriod: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}

	// Create test partitions
	now := time.Now()
	oldPartition := newPartition[float64](
		now.Add(-2*time.Hour).UnixNano(),
		time.Hour,
		10,
	)
	newPartition := newPartition[float64](
		now.UnixNano(),
		time.Hour,
		10,
	)

	// Save partitions
	if err := storage.SavePartition(oldPartition); err != nil {
		t.Fatalf("SavePartition() error = %v", err)
	}
	if err := storage.SavePartition(newPartition); err != nil {
		t.Fatalf("SavePartition() error = %v", err)
	}

	// Delete old partitions
	if err := storage.DeleteOldPartitions(); err != nil {
		t.Fatalf("DeleteOldPartitions() error = %v", err)
	}

	// Verify only new partition remains
	files, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}

	var partitionFiles int
	for _, file := range files {
		if filepath.Ext(file.Name()) == ".dat" &&
			!file.IsDir() &&
			file.Name() != "metadata.dat" {
			partitionFiles++
		}
	}

	if partitionFiles != 1 {
		t.Errorf("got %d partition files, want 1", partitionFiles)
	}
}

func TestStorageRepair(t *testing.T) {
	tempDir := t.TempDir()
	storage, err := NewStorage[float64](StorageConfig{
		BaseDir: tempDir,
		Debug:   true,
	})
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}

	// Create valid partition
	now := time.Now().UnixNano()
	validPartition := newPartition[float64](now, time.Hour, 10)
	validPartition.insert(TimeSeriesItem[float64]{
		Timestamp: now,
		Value:     42.0,
	})

	// Save valid partition
	if err := storage.SavePartition(validPartition); err != nil {
		t.Fatalf("SavePartition() error = %v", err)
	}

	// Create corrupted files
	corruptedFile := filepath.Join(tempDir, fmt.Sprintf("partition_%s_%d.dat",
		time.Unix(0, now).Format("2006-01-02"), now+1))
	if err := os.WriteFile(corruptedFile, []byte("corrupted data"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Create malformed filename
	malformedFile := filepath.Join(tempDir, "partition_invalid.dat")
	if err := os.WriteFile(malformedFile, []byte("malformed filename"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Run repair
	if err := storage.Repair(); err != nil {
		t.Fatalf("Repair() error = %v", err)
	}

	// Verify only valid partition remains in metadata
	storage.mu.RLock()
	partitionCount := len(storage.metadata.PartitionMap)
	storage.mu.RUnlock()

	if partitionCount != 1 {
		t.Errorf("got %d partitions in metadata, want 1", partitionCount)
	} else {
		t.Logf("metadata: %v", storage.metadata.PartitionMap)
	}

	// Verify corrupted and malformed files are not in metadata
	storage.mu.RLock()
	for _, filename := range storage.metadata.PartitionMap {
		if strings.Contains(filename, "corrupted") || strings.Contains(filename, "invalid") {
			t.Errorf("found invalid file in metadata: %s", filename)
		}
	}
	storage.mu.RUnlock()
}

func TestStorageConcurrency(t *testing.T) {
	tempDir := t.TempDir()
	storage, err := NewStorage[float64](StorageConfig{BaseDir: tempDir})
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}

	// Create multiple partitions concurrently
	const numGoroutines = 10
	errChan := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(i int) {
			partition := newPartition[float64](int64(i)*int64(time.Hour), time.Hour, 10)
			partition.insert(TimeSeriesItem[float64]{
				Timestamp: int64(i),
				Value:     float64(i),
			})
			errChan <- storage.SavePartition(partition)
		}(i)
	}

	// Collect errors
	for i := 0; i < numGoroutines; i++ {
		if err := <-errChan; err != nil {
			t.Errorf("concurrent SavePartition() error = %v", err)
		}
	}

	// Verify all partitions were saved
	files, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}

	var partitionFiles int
	for _, file := range files {
		if filepath.Ext(file.Name()) == ".dat" &&
			!file.IsDir() &&
			file.Name() != "metadata.dat" {
			partitionFiles++
		}
	}

	if partitionFiles != numGoroutines {
		t.Errorf("got %d partition files, want %d", partitionFiles, numGoroutines)
	}
}
