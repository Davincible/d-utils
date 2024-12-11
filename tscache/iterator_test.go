package tscache

import (
	"fmt"
	"testing"
	"time"
)

func TestIterator(t *testing.T) {
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
	// Insert test data across multiple partitions
	data := []struct {
		timestamp time.Time
		value     float64
	}{
		{base, 1.0},
		{base.Add(30 * time.Minute), 2.0},
		{base.Add(time.Hour), 3.0},
		{base.Add(90 * time.Minute), 4.0},
		{base.Add(2 * time.Hour), 5.0},
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
		wantFirst float64
		wantLast  float64
	}{
		{
			name:      "full range",
			start:     base,
			end:       base.Add(2 * time.Hour),
			wantCount: 5,
			wantFirst: 1.0,
			wantLast:  5.0,
		},
		{
			name:      "partial range",
			start:     base.Add(25 * time.Minute),
			end:       base.Add(75 * time.Minute),
			wantCount: 2,
			wantFirst: 2.0,
			wantLast:  3.0,
		},
		{
			name:      "cross partition",
			start:     base.Add(30 * time.Minute),
			end:       base.Add(90 * time.Minute),
			wantCount: 3,
			wantFirst: 2.0,
			wantLast:  4.0,
		},
		{
			name:      "empty range",
			start:     base.Add(3 * time.Hour),
			end:       base.Add(4 * time.Hour),
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iter := cache.NewIterator(tt.start, tt.end)
			var count int
			var firstValue, lastValue float64

			// Iterate through all items
			for item, ok := iter.Next(); ok; item, ok = iter.Next() {
				if count == 0 {
					firstValue = item.Value
				}
				lastValue = item.Value
				count++
			}

			if count != tt.wantCount {
				t.Errorf("%s: got %d items, want %d", tt.name, count, tt.wantCount)
			}

			if count > 0 {
				if firstValue != tt.wantFirst {
					t.Errorf("first value = %v, want %v", firstValue, tt.wantFirst)
				}
				if lastValue != tt.wantLast {
					t.Errorf("last value = %v, want %v", lastValue, tt.wantLast)
				}
			}

			// Test Reset and re-iteration
			iter.Reset()
			newCount := 0
			for _, ok := iter.Next(); ok; _, ok = iter.Next() {
				newCount++
			}
			if newCount != count {
				t.Errorf("after reset: got %d items, want %d", newCount, count)
			}
		})
	}
}

func TestIteratorConcurrency(t *testing.T) {
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
	for i := 0; i < 1000; i++ {
		timestamp := base.Add(time.Duration(i) * time.Second)
		if err := cache.Set(timestamp, float64(i)); err != nil {
			t.Fatalf("Set() error = %v", err)
		}
	}

	// Test concurrent iterations
	const numGoroutines = 10
	errChan := make(chan error, numGoroutines)
	doneChan := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(i int) {
			start := base.Add(time.Duration(i*100) * time.Second)
			end := start.Add(100 * time.Second)
			iter := cache.NewIterator(start, end)

			var prevTimestamp int64
			for item, ok := iter.Next(); ok; item, ok = iter.Next() {
				if item.Timestamp < prevTimestamp {
					errChan <- fmt.Errorf("timestamps not in order: prev=%d, current=%d",
						prevTimestamp, item.Timestamp)
					return
				}
				prevTimestamp = item.Timestamp
			}
			doneChan <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		select {
		case err := <-errChan:
			t.Errorf("concurrent iteration error = %v", err)
		case <-doneChan:
			// Iteration completed successfully
		}
	}
}

func TestIteratorWithExpiredItems(t *testing.T) {
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
	shortTTL := time.Millisecond * 100

	// Insert mix of regular and expiring items
	data := []struct {
		timestamp time.Time
		value     float64
		ttl       time.Duration
	}{
		{base, 1.0, 0},                             // No TTL
		{base.Add(time.Minute), 2.0, shortTTL},     // Will expire
		{base.Add(2 * time.Minute), 3.0, 0},        // No TTL
		{base.Add(3 * time.Minute), 4.0, shortTTL}, // Will expire
	}

	for _, d := range data {
		if err := cache.Set(d.timestamp, d.value, d.ttl); err != nil {
			t.Fatalf("Set() error = %v", err)
		}
	}

	// Wait for items to expire
	time.Sleep(shortTTL * 2)

	iter := cache.NewIterator(base, base.Add(4*time.Minute))

	count := 0
	expectedValues := map[float64]bool{1.0: true, 3.0: true}

	for item, ok := iter.Next(); ok; item, ok = iter.Next() {
		count++
		if !expectedValues[item.Value] {
			t.Errorf("unexpected value: %v", item.Value)
		}
	}

	if count != 2 {
		t.Errorf("got %d non-expired items, want 2", count)
	}
}
