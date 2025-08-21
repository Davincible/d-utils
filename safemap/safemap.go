// Package safe provides concurrency safe data types.
package safemap

import (
	"fmt"
	"strconv"
	"sync"
)

// Map is a thread-safe map with string keys and generic type T values.
type Map[T any] struct {
	mu    sync.RWMutex
	items map[string]T
}

// NewSafeMap creates and returns a new empty SafeMap instance with the specified value type.
func NewSafeMap[T any]() Map[T] {
	return Map[T]{
		items: make(map[string]T),
	}
}

// Get retrieves the value associated with the given key from the map.
// It returns the value and a boolean indicating whether the key was present.
func (m *Map[T]) Get(key any) (T, bool) {
	m.mu.RLock()
	val, ok := m.items[getKey(key)]
	m.mu.RUnlock()

	return val, ok
}

// GetOrSet retrieves the value associated with the given key from the map.
// It returns the value and a boolean indicating whether the key was present.
// Only if it not set yet it will assign the new value.
func (m *Map[T]) GetOrSet(key any, val T) (T, bool) {
	m.mu.Lock()
	k := getKey(key)

	valN, ok := m.items[k]
	if !ok {
		m.items[k] = val
	}

	m.mu.Unlock()

	return valN, ok
}

// GetAll returns a list of all items.
func (m *Map[T]) GetAll() []T {
	m.mu.RLock()

	out := make([]T, 0, len(m.items))

	for _, item := range m.items {
		out = append(out, item)
	}

	m.mu.RUnlock()

	return out
}

// GetMap returns a copy of the internal map.
func (m *Map[T]) GetMap() map[string]T {
	m.mu.RLock()

	out := make(map[string]T, len(m.items))

	for key, value := range m.items {
		out[key] = value
	}

	m.mu.RUnlock()

	return out
}

// Set stores the value associated with the given key in the map.
func (m *Map[T]) Set(key any, value T) {
	m.mu.Lock()
	m.items[getKey(key)] = value
	m.mu.Unlock()
}

// SetMap replaces the internal map with the  provided map.
func (m *Map[T]) SetMap(n map[string]T) {
	m.mu.Lock()
	m.items = n
	m.mu.Unlock()
}

// Delete removes the value associated with the given key from the map.
func (m *Map[T]) Delete(key any) {
	m.mu.Lock()
	delete(m.items, getKey(key))
	m.mu.Unlock()
}

// Length returns the number of key-value pairs in the map.
func (m *Map[T]) Length() int {
	m.mu.RLock()
	l := len(m.items)
	m.mu.RUnlock()

	return l
}

// Keys returns a slice of all the keys in the map.
func (m *Map[T]) Keys() []string {
	m.mu.RLock()

	keys := make([]string, 0, len(m.items))
	for k := range m.items {
		keys = append(keys, k)
	}

	m.mu.RUnlock()

	return keys
}

// getKey takes any value and returns a string representation that can be used as a key.
func getKey(value any) string {
	switch v := value.(type) {
	case nil:
		return "nil"
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	case int8:
		return strconv.FormatInt(int64(v), 10)
	case int16:
		return strconv.FormatInt(int64(v), 10)
	case int32:
		return strconv.FormatInt(int64(v), 10)
	case int64:
		return strconv.FormatInt(v, 10)
	case uint:
		return strconv.FormatUint(uint64(v), 10)
	case uint8:
		return strconv.FormatUint(uint64(v), 10)
	case uint16:
		return strconv.FormatUint(uint64(v), 10)
	case uint32:
		return strconv.FormatUint(uint64(v), 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	case []byte:
		return string(v)
	case error:
		return v.Error()
	default:
		return fmt.Sprintf("%+v", v)
	}
}

// Range iterates over the map and calls the provided function for each key-value pair.
// The iteration will stop if the function returns false.
// The map is locked during iteration to ensure thread safety.
func (m *Map[T]) Range(fn func(key string, value T) bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Create a copy of the map to iterate over
	// This ensures we don't hold the lock for too long if the callback is slow
	items := make(map[string]T, len(m.items))
	for k, v := range m.items {
		items[k] = v
	}

	// Iterate over the copy
	for k, v := range items {
		if !fn(k, v) {
			break
		}
	}
}

// RangeLocked iterates over the map and calls the provided function for each key-value pair
// while holding the read lock. This is useful when you need to ensure the map doesn't change
// during iteration. Use with caution as holding the lock for too long can impact performance.
func (m *Map[T]) RangeLocked(fn func(key string, value T) bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for k, v := range m.items {
		if !fn(k, v) {
			break
		}
	}

}

// Filter returns a new map containing only the key-value pairs that satisfy the predicate.
func (m *Map[T]) Filter(predicate func(key string, value T) bool) Map[T] {
	filtered := NewSafeMap[T]()

	m.Range(func(key string, value T) bool {
		if predicate(key, value) {
			filtered.Set(key, value)
		}
		return true
	})

	return filtered
}

// Map applies the given function to all values in the map and returns a new map
// with the transformed values.
func (m *Map[T]) Map(transform func(key string, value T) T) Map[T] {
	transformed := NewSafeMap[T]()

	m.Range(func(key string, value T) bool {
		transformed.Set(key, transform(key, value))
		return true
	})

	return transformed
}

// Reduce applies a function against an accumulator and each element in the map
// to reduce it to a single value.
func (m *Map[T]) Reduce(initial T, reducer func(acc T, key string, value T) T) T {
	result := initial

	m.Range(func(key string, value T) bool {
		result = reducer(result, key, value)
		return true
	})

	return result
}
