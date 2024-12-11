package tscache

import (
	"testing"
	"time"
)

// func TestNewTimeSeriesCache(t *testing.T) {
// 	tests := []struct {
// 		name    string
// 		cfg     *Config
// 		wantErr bool
// 	}{
// 		{
// 			name: "valid config",
// 			cfg: &Config{
// 				StorageDir:        t.TempDir(),
// 				PartitionDuration: time.Hour,
// 				MaxPartitions:     7,
// 				CleanupInterval:   time.Minute,
// 				FlushInterval:     time.Minute,
// 			},
// 			wantErr: false,
// 		},
// 		{
// 			name:    "nil config uses defaults",
// 			cfg:     nil,
// 			wantErr: false,
// 		},
// 		{
// 			name: "invalid config",
// 			cfg: &Config{
// 				PartitionDuration: -time.Hour,
// 			},
// 			wantErr: true,
// 		},
// 	}
//
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			if tt.cfg != nil && tt.cfg.StorageDir == "" {
// 				tt.cfg.StorageDir = t.TempDir()
// 			}
// 			cache, err := NewTimeSeries[float64](tt.cfg)
// 			if (err != nil) != tt.wantErr {
// 				t.Errorf("NewTimeSeries() - %s; error = %v, wantErr %v", tt.name, err, tt.wantErr)
// 			}
// 			if err == nil && cache == nil {
// 				t.Error("expected non-nil cache")
// 			}
// 			if cache != nil {
// 				if err := cache.Close(); err != nil {
// 					t.Errorf("Close() error = %v", err)
// 				}
// 			}
// 		})
// 	}
// }

func TestCacheSetGet(t *testing.T) {
	cache, err := NewTimeSeries[float64](&Config{
		StorageDir:        t.TempDir(),
		PartitionDuration: time.Hour,
		MaxPartitions:     7,
	})
	if err != nil {
		t.Fatalf("NewTimeSeries() error = %v", err)
	}
	defer cache.Close()

	now := time.Now()
	tests := []struct {
		name      string
		timestamp time.Time
		value     float64
		ttl       time.Duration
	}{
		{"present value", now, 42.0, 0},
		{"future value", now.Add(time.Hour), 43.0, 0},
		{"past value", now.Add(-time.Hour), 41.0, 0},
		{"expiring value", now, 44.0, time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test Set
			err := cache.Set(tt.timestamp, tt.value, tt.ttl)
			if err != nil {
				t.Errorf("Set() error = %v", err)
			}

			// Test GetExact
			if tt.ttl > 0 {
				time.Sleep(tt.ttl * 2) // Wait for expiration
				if _, found := cache.GetExact(tt.timestamp); found {
					t.Error("got expired value, want not found")
				}
			} else {
				value, found := cache.GetExact(tt.timestamp)
				if !found {
					t.Error("value not found")
				}
				if value != tt.value {
					t.Errorf("got %v, want %v", value, tt.value)
				}
			}
		})
	}
}

func TestCacheGetNearest(t *testing.T) {
	cache, err := NewTimeSeries[float64](&Config{
		StorageDir:        t.TempDir(),
		PartitionDuration: time.Hour,
		MaxPartitions:     7,
	})
	if err != nil {
		t.Fatalf("NewTimeSeries() error = %v", err)
	}
	defer cache.Close()

	base := time.Now()
	// Insert test data
	data := []struct {
		timestamp time.Time
		value     float64
	}{
		{base, 1.0},
		{base.Add(10 * time.Second), 2.0},
		{base.Add(20 * time.Second), 3.0},
	}

	for _, d := range data {
		if err := cache.Set(d.timestamp, d.value); err != nil {
			t.Fatalf("Set() error = %v", err)
		}
	}

	tests := []struct {
		name      string
		timestamp time.Time
		timeRange time.Duration
		want      float64
		wantFound bool
	}{
		{"exact match", base.Add(10 * time.Second), 0, 2.0, true},
		{"between values", base.Add(14 * time.Second), 0, 2.0, true},
		{"within range", base.Add(25 * time.Second), 10 * time.Second, 3.0, true},
		{"out of range", base.Add(35 * time.Second), 10 * time.Second, 0.0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := cache.GetNearest(tt.timestamp, tt.timeRange)
			if found != tt.wantFound {
				t.Errorf("got found = %v, want %v", found, tt.wantFound)
			}
			if found && got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCacheGetRange(t *testing.T) {
	cache, err := NewTimeSeries[float64](&Config{
		StorageDir:        t.TempDir(),
		PartitionDuration: time.Hour,
		MaxPartitions:     7,
	})
	if err != nil {
		t.Fatalf("NewTimeSeries() error = %v", err)
	}
	defer cache.Close()

	base := time.Now()
	// Insert test data
	data := []struct {
		timestamp time.Time
		value     float64
	}{
		{base, 1.0},
		{base.Add(30 * time.Minute), 2.0},
		{base.Add(time.Hour), 3.0},
		{base.Add(90 * time.Minute), 4.0},
	}

	for _, d := range data {
		if err := cache.Set(d.timestamp, d.value); err != nil {
			t.Fatalf("Set() error = %v", err)
		}
	}

	tests := []struct {
		name      string
		start     time.Time
		end       time.Time
		wantCount int
	}{
		{"full range", base, base.Add(2 * time.Hour), 4},
		{"partial range", base.Add(15 * time.Minute), base.Add(45 * time.Minute), 1},
		{"cross partition", base.Add(45 * time.Minute), base.Add(96 * time.Minute), 2},
		{"empty range", base.Add(2 * time.Hour), base.Add(3 * time.Hour), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := cache.GetRange(tt.start, tt.end)
			if len(items) != tt.wantCount {
				t.Errorf("%s: got %d items, want %d", tt.name, len(items), tt.wantCount)
			}
			for i := 1; i < len(items); i++ {
				if items[i].Timestamp < items[i-1].Timestamp {
					t.Error("items not properly sorted")
				}
			}
		})
	}
}

func TestCacheDeleteRange(t *testing.T) {
	cache, err := NewTimeSeries[float64](&Config{
		StorageDir:        t.TempDir(),
		PartitionDuration: time.Hour,
		MaxPartitions:     7,
	})
	if err != nil {
		t.Fatalf("NewTimeSeries() error = %v", err)
	}
	defer cache.Close()

	base := time.Now()
	// Insert test data
	data := []struct {
		timestamp time.Time
		value     float64
	}{
		{base, 1.0},
		{base.Add(30 * time.Minute), 2.0},
		{base.Add(time.Hour), 3.0},
		{base.Add(90 * time.Minute), 4.0},
	}

	for _, d := range data {
		if err := cache.Set(d.timestamp, d.value); err != nil {
			t.Fatalf("Set() error = %v", err)
		}
	}

	// Delete middle range
	deleteStart := base.Add(15 * time.Minute)
	deleteEnd := base.Add(75 * time.Minute)
	if err := cache.DeleteRange(deleteStart, deleteEnd); err != nil {
		t.Fatalf("DeleteRange() error = %v", err)
	}

	// Verify remaining items
	items := cache.GetRange(base, base.Add(2*time.Hour))
	if len(items) != 2 {
		t.Errorf("got %d items after delete, want 2", len(items))
	}

	// Verify correct items remain
	expectedValues := map[float64]bool{1.0: true, 4.0: true}
	for _, item := range items {
		if !expectedValues[item.Value] {
			t.Errorf("unexpected value %v after delete", item.Value)
		}
	}
}

func TestCacheConcurrency(t *testing.T) {
	cache, err := NewTimeSeries[float64](&Config{
		StorageDir:        t.TempDir(),
		PartitionDuration: time.Hour,
		MaxPartitions:     7,
	})
	if err != nil {
		t.Fatalf("NewTimeSeries() error = %v", err)
	}
	defer cache.Close()

	const numGoroutines = 100
	const numOperations = 1000

	errChan := make(chan error, numGoroutines)
	doneChan := make(chan bool, numGoroutines)

	base := time.Now()

	// Start writer goroutines
	for i := 0; i < numGoroutines/2; i++ {
		go func(i int) {
			for j := 0; j < numOperations; j++ {
				timestamp := base.Add(time.Duration(i*j) * time.Second)
				if err := cache.Set(timestamp, float64(i*j)); err != nil {
					errChan <- err
					return
				}
			}
			doneChan <- true
		}(i)
	}

	// Start reader goroutines
	for i := 0; i < numGoroutines/2; i++ {
		go func(i int) {
			for j := 0; j < numOperations; j++ {
				timestamp := base.Add(time.Duration(i*j) * time.Second)
				if _, found := cache.GetExact(timestamp); !found {
					// It's ok if we don't find the value
					continue
				}
				items := cache.GetRange(timestamp, timestamp.Add(time.Second))
				if len(items) > 0 && items[0].Timestamp < 0 {
					errChan <- err
					return
				}
			}
			doneChan <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		select {
		case err := <-errChan:
			t.Errorf("concurrent operation error = %v", err)
		case <-doneChan:
			// Operation completed successfully
		}
	}
}
