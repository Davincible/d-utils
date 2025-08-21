package apicache

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
)

// KeyFunc generates a cache key from an HTTP request
type KeyFunc func(*http.Request) string

// DefaultKeyFunc generates a cache key from the request method and URL
func DefaultKeyFunc(r *http.Request) string {
	return fmt.Sprintf("%s:%s", r.Method, r.URL.String())
}

// QueryKeyFunc generates a cache key from the request method, URL path, and sorted query parameters
func QueryKeyFunc(r *http.Request) string {
	// Get sorted query parameters
	query := r.URL.Query()
	keys := make([]string, 0, len(query))
	for k := range query {
		keys = append(keys, k)
	}
	// Sort keys for consistent ordering
	sort.Strings(keys)

	// Build query string
	var queryStr strings.Builder
	for i, k := range keys {
		if i > 0 {
			queryStr.WriteString("&")
		}
		queryStr.WriteString(k)
		queryStr.WriteString("=")
		queryStr.WriteString(query.Get(k))
	}

	return fmt.Sprintf("%s:%s?%s", r.Method, r.URL.Path, queryStr.String())
}

// HeaderKeyFunc generates a cache key including specified request headers
func HeaderKeyFunc(headers ...string) KeyFunc {
	return func(r *http.Request) string {
		var key strings.Builder
		key.WriteString(fmt.Sprintf("%s:%s", r.Method, r.URL.String()))

		for _, h := range headers {
			if v := r.Header.Get(h); v != "" {
				key.WriteString(fmt.Sprintf(":%s=%s", h, v))
			}
		}

		return key.String()
	}
}

// CacheMiddleware creates middleware for caching HTTP responses
func CacheMiddleware[T any](cache *Cache[T], keyFn KeyFunc) func(http.Handler) http.Handler {
	if keyFn == nil {
		keyFn = DefaultKeyFunc
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip caching for non-GET requests
			if r.Method != http.MethodGet {
				next.ServeHTTP(w, r)
				return
			}

			key := keyFn(r)

			// Create fetch function that captures the response
			fetch := func(ctx context.Context) (T, error) {
				// Create response recorder
				rec := httptest.NewRecorder()

				// Serve request
				next.ServeHTTP(rec, r.WithContext(ctx))

				// Check if response is cacheable
				if rec.Code != http.StatusOK {
					return *new(T), fmt.Errorf("non-200 status code: %d", rec.Code)
				}

				// Parse response body into type T
				var response T
				if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
					return *new(T), fmt.Errorf("decode response: %w", err)
				}

				return response, nil
			}

			// Get from cache or fetch
			response, err := cache.GetOrFetch(r.Context(), key, fetch)
			if err != nil {
				// On error, fall back to direct request
				next.ServeHTTP(w, r)
				return
			}

			// Write cached response
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Cache", "HIT")
			json.NewEncoder(w).Encode(response)
		})
	}
}

// WrapHandler wraps an http.HandlerFunc with cache middleware
func WrapHandler[T any](cache *Cache[T], keyFn KeyFunc, handler http.HandlerFunc) http.HandlerFunc {
	middleware := CacheMiddleware(cache, keyFn)
	return middleware(handler).ServeHTTP
}
