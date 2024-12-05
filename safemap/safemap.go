// Package safe provides concurrency safe data types.
package safemap

import (
	"sync"
)

// Map is a thread-safe map with string keys and generic type T values.
type Map[T any] struct {
	mu    *sync.RWMutex
	items map[string]T
}

// NewSafeMap creates and returns a new empty SafeMap instance with the specified value type.
func NewSafeMap[T any]() Map[T] {
	return Map[T]{
		items: make(map[string]T),
		mu:    new(sync.RWMutex),
	}
}

// Get retrieves the value associated with the given key from the map.
// It returns the value and a boolean indicating whether the key was present.
func (m *Map[T]) Get(key string) (T, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	val, ok := m.items[key]

	return val, ok
}

// GetOrSet retrieves the value associated with the given key from the map.
// It returns the value and a boolean indicating whether the key was present.
// Only if it not set yet it will assign the new value.
func (m *Map[T]) GetOrSet(key string, val T) (T, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	valN, ok := m.items[key]
	if !ok {
		m.items[key] = val
	}

	return valN, ok
}

// GetAll returns a list of all items.
func (m *Map[T]) GetAll() []T {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]T, 0, len(m.items))

	for _, item := range m.items {
		out = append(out, item)
	}

	return out
}

// GetMap returns a copy of the internal map.
func (m *Map[T]) GetMap() map[string]T {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make(map[string]T, len(m.items))

	for key, value := range m.items {
		out[key] = value
	}

	return out
}

// Set stores the value associated with the given key in the map.
func (m *Map[T]) Set(key string, value T) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[key] = value
}

// SetMap replaces the internal map with the  provided map.
func (m *Map[T]) SetMap(n map[string]T) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.items = n
}

// Delete removes the value associated with the given key from the map.
func (m *Map[T]) Delete(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.items, key)
}

// Length returns the number of key-value pairs in the map.
func (m *Map[T]) Length() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.items)
}

// Keys returns a slice of all the keys in the map.
func (m *Map[T]) Keys() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	keys := make([]string, 0, len(m.items))
	for k := range m.items {
		keys = append(keys, k)
	}

	return keys
}
