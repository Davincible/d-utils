package tscache

import (
	"testing"
	"time"
)

func TestPartitionInsert(t *testing.T) {
	p := newPartition[float64](0, time.Hour, 10)

	items := []struct {
		timestamp int64
		value     float64
	}{
		{3, 3.0},
		{1, 1.0},
		{2, 2.0},
		{4, 4.0},
	}

	// Insert items
	for _, item := range items {
		p.insert(TimeSeriesItem[float64]{
			Timestamp: item.timestamp,
			Value:     item.value,
		})
	}

	// Verify order and count
	if len(p.Items) != 4 {
		t.Errorf("expected 4 items, got %d", len(p.Items))
	}

	// Verify sorting
	for i := 1; i < len(p.Items); i++ {
		if p.Items[i].Timestamp < p.Items[i-1].Timestamp {
			t.Error("items not properly sorted")
		}
	}

	// Verify map indices
	for i, item := range p.Items {
		if idx, exists := p.ItemsMap[item.Timestamp]; !exists || idx != i {
			t.Errorf("incorrect map index for timestamp %d", item.Timestamp)
		}
	}
}

func TestPartitionGet(t *testing.T) {
	p := newPartition[float64](0, time.Hour, 10)
	now := time.Now().UnixNano()

	// Insert test items
	testItems := []TimeSeriesItem[float64]{
		{Timestamp: now, Value: 1.0},
		{Timestamp: now + 1, Value: 2.0, Expiration: now + 1000},
		{Timestamp: now + 2, Value: 3.0, Expiration: now - 1000}, // expired
	}

	for _, item := range testItems {
		p.insert(item)
	}

	tests := []struct {
		name      string
		timestamp int64
		want      float64
		wantFound bool
	}{
		{"existing item", now, 1.0, true},
		{"expired item", now + 2, 0.0, false},
		{"non-existent item", now + 100, 0.0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := p.get(tt.timestamp)
			if found != tt.wantFound {
				t.Errorf("got found = %v, want %v", found, tt.wantFound)
			}
			if found && got.Value != tt.want {
				t.Errorf("got value = %v, want %v", got.Value, tt.want)
			}
		})
	}
}

func TestPartitionGetNearest(t *testing.T) {
	p := newPartition[float64](0, time.Hour, 10)
	base := time.Now().UnixNano()

	// Insert test items with gaps
	items := []TimeSeriesItem[float64]{
		{Timestamp: base, Value: 1.0},
		{Timestamp: base + 10, Value: 2.0},
		{Timestamp: base + 20, Value: 3.0},
	}

	for _, item := range items {
		p.insert(item)
	}

	tests := []struct {
		name      string
		timestamp int64
		want      float64
		wantFound bool
	}{
		{"exact match", base + 10, 2.0, true},
		{"between values", base + 14, 2.0, true},
		{"before first", base - 5, 1.0, true},
		{"after last", base + 25, 3.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := p.getNearest(tt.timestamp)
			if found != tt.wantFound {
				t.Errorf("%s: got found = %v, want %v", tt.name, found, tt.wantFound)
			}
			if found && got.Value != tt.want {
				t.Errorf("%s: got value = %v, want %v", tt.name, got.Value, tt.want)
			}
		})
	}
}

func TestPartitionGetRange(t *testing.T) {
	p := newPartition[float64](0, time.Hour, 10)
	base := time.Now().UnixNano()

	// Insert test items
	items := []TimeSeriesItem[float64]{
		{Timestamp: base, Value: 1.0},
		{Timestamp: base + 10, Value: 2.0},
		{Timestamp: base + 20, Value: 3.0},
		{Timestamp: base + 30, Value: 4.0, Expiration: base - 1}, // expired
	}

	for _, item := range items {
		p.insert(item)
	}

	tests := []struct {
		name      string
		start     int64
		end       int64
		wantCount int
	}{
		{"full range", base - 10, base + 40, 3}, // excludes expired item
		{"partial range", base + 5, base + 15, 1},
		{"exact bounds", base + 10, base + 20, 2},
		{"no data", base + 100, base + 200, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.getRange(tt.start, tt.end)
			if len(got) != tt.wantCount {
				t.Errorf("got %d items, want %d", len(got), tt.wantCount)
			}
			for i := 1; i < len(got); i++ {
				if got[i].Timestamp < got[i-1].Timestamp {
					t.Error("items not properly sorted")
				}
			}
		})
	}
}
