package apicache_test

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Davincible/d-utils/apicache"
)

func Example() {
	// Create a new cache instance with 2 second TTL
	cache, err := apicache.New[string](apicache.Options{
		DefaultTTL:   2 * time.Second,
		StaleTimeout: 5 * time.Second,
		MaxItems:     1000,
	})
	if err != nil {
		panic(err)
	}

	// Simulate a slow database query
	dbHits := 0
	var dbMu sync.Mutex
	fetchFromDB := func(ctx context.Context) (string, error) {
		// Simulate slow DB query
		time.Sleep(100 * time.Millisecond)

		dbMu.Lock()
		dbHits++
		hits := dbHits
		dbMu.Unlock()

		return fmt.Sprintf("data from db (hit #%d)", hits), nil
	}

	// Simulate multiple concurrent requests
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			ctx := context.Background()
			result, err := cache.GetOrFetch(ctx, "test-key", fetchFromDB)
			if err != nil {
				fmt.Printf("Error getting data: %v\n", err)
				return
			}

			fmt.Printf("Request %d got result: %s\n", i, result)
		}(i)
	}

	wg.Wait()

	// Wait for cache to expire
	time.Sleep(3 * time.Second)

	// Make another request - should trigger a new DB fetch
	result, _ := cache.GetOrFetch(context.Background(), "test-key", fetchFromDB)
	fmt.Printf("After expiry: %s\n", result)

	// Output:
	// Request 0 got result: data from db (hit #1)
	// Request 1 got result: data from db (hit #1)
	// Request 2 got result: data from db (hit #1)
	// Request 3 got result: data from db (hit #1)
	// Request 4 got result: data from db (hit #1)
	// After expiry: data from db (hit #2)
}

func ExampleCache_GetMetrics() {
	cache, _ := apicache.New[string](apicache.Options{
		DefaultTTL: time.Second,
	})

	// Simulate some cache operations
	fetch := func(ctx context.Context) (string, error) {
		return "test data", nil
	}

	// Make some cache hits and misses
	for i := 0; i < 3; i++ {
		cache.GetOrFetch(context.Background(), "key", fetch)
	}

	// Get metrics
	metrics := cache.GetMetrics()
	fmt.Printf("Cache metrics:\n")
	fmt.Printf("Items: %d\n", metrics.Items)
	fmt.Printf("Hits: %d\n", metrics.Hits)
	fmt.Printf("Misses: %d\n", metrics.Misses)
	fmt.Printf("Errors: %d\n", metrics.Errors)
	fmt.Printf("Inflight requests: %d\n", metrics.Inflight)

	// Output:
	// Cache metrics:
	// Items: 1
	// Hits: 2
	// Misses: 1
	// Errors: 0
	// Inflight requests: 0
}
