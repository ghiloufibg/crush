package csync

import (
	"encoding/json"
	"iter"
	"maps"
	"sync"
)

// mapState holds the mutex and the backing map. It is indirected behind a
// pointer so that Map itself never embeds a sync.Locker by value, which lets
// JSONSchemaAlias below use a value receiver (required by
// invopop/jsonschema) without go vet flagging it as copying a lock.
type mapState[K comparable, V any] struct {
	inner map[K]V
	mu    sync.RWMutex
}

// Map is a concurrent map implementation that provides thread-safe access.
type Map[K comparable, V any] struct {
	state *mapState[K, V]
}

// NewMap creates a new thread-safe map with the specified key and value types.
func NewMap[K comparable, V any]() *Map[K, V] {
	return &Map[K, V]{
		state: &mapState[K, V]{inner: make(map[K]V)},
	}
}

// NewMapFrom creates a new thread-safe map from an existing map.
func NewMapFrom[K comparable, V any](m map[K]V) *Map[K, V] {
	return &Map[K, V]{
		state: &mapState[K, V]{inner: m},
	}
}

// NewLazyMap creates a new lazy-loaded map. The provided load function is
// executed in a separate goroutine to populate the map.
func NewLazyMap[K comparable, V any](load func() map[K]V) *Map[K, V] {
	m := &Map[K, V]{state: &mapState[K, V]{}}
	m.state.mu.Lock()
	go func() {
		defer m.state.mu.Unlock()
		m.state.inner = load()
	}()
	return m
}

// Reset replaces the inner map with the new one.
func (m *Map[K, V]) Reset(input map[K]V) {
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	m.state.inner = input
}

// Set sets the value for the specified key in the map.
func (m *Map[K, V]) Set(key K, value V) {
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	m.state.inner[key] = value
}

// Del deletes the specified key from the map.
func (m *Map[K, V]) Del(key K) {
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	delete(m.state.inner, key)
}

// CompareAndDelete deletes the key only if the current value matches the
// expected pointer. Returns true if the deletion occurred. This is the
// ABA-safe cleanup primitive: it prevents a deferred cleanup from removing
// a value that was replaced by a newer writer in the window between the
// explicit Del and the deferred Del.
func (m *Map[K, V]) CompareAndDelete(key K, expected any) bool {
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	current, ok := m.state.inner[key]
	if !ok {
		return false
	}
	if any(current) != expected {
		return false
	}
	delete(m.state.inner, key)
	return true
}

// Get gets the value for the specified key from the map.
func (m *Map[K, V]) Get(key K) (V, bool) {
	m.state.mu.RLock()
	defer m.state.mu.RUnlock()
	v, ok := m.state.inner[key]
	return v, ok
}

// Len returns the number of items in the map.
func (m *Map[K, V]) Len() int {
	m.state.mu.RLock()
	defer m.state.mu.RUnlock()
	return len(m.state.inner)
}

// GetOrSet gets and returns the key if it exists, otherwise, it executes the
// given function, set its return value for the given key, and returns it.
func (m *Map[K, V]) GetOrSet(key K, fn func() V) V {
	got, ok := m.Get(key)
	if ok {
		return got
	}
	value := fn()
	m.Set(key, value)
	return value
}

// Take gets an item and then deletes it.
func (m *Map[K, V]) Take(key K) (V, bool) {
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	v, ok := m.state.inner[key]
	delete(m.state.inner, key)
	return v, ok
}

// Copy returns a copy of the inner map.
func (m *Map[K, V]) Copy() map[K]V {
	m.state.mu.RLock()
	defer m.state.mu.RUnlock()
	return maps.Clone(m.state.inner)
}

// Seq2 returns an iter.Seq2 that yields key-value pairs from the map.
func (m *Map[K, V]) Seq2() iter.Seq2[K, V] {
	dst := m.Copy()
	return func(yield func(K, V) bool) {
		for k, v := range dst {
			if !yield(k, v) {
				return
			}
		}
	}
}

// Seq returns an iter.Seq that yields values from the map.
func (m *Map[K, V]) Seq() iter.Seq[V] {
	return func(yield func(V) bool) {
		for _, v := range m.Seq2() {
			if !yield(v) {
				return
			}
		}
	}
}

var (
	_ json.Unmarshaler = &Map[string, any]{}
	_ json.Marshaler   = &Map[string, any]{}
)

// JSONSchemaAlias returns the underlying map type for JSON schema generation.
// Value receiver is required because github.com/invopop/jsonschema checks
// interface satisfaction on the non-pointer type after stripping pointers.
func (Map[K, V]) JSONSchemaAlias() any { //nolint
	m := map[K]V{}
	return m
}

// UnmarshalJSON implements json.Unmarshaler.
func (m *Map[K, V]) UnmarshalJSON(data []byte) error {
	if m.state == nil {
		// Reached when unmarshaling into a zero-value Map, e.g.
		// json.Unmarshal(data, &Map[K, V]{}) or a nil *Map[K, V] field
		// that encoding/json allocates on our behalf.
		m.state = &mapState[K, V]{}
	}
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	m.state.inner = make(map[K]V)
	return json.Unmarshal(data, &m.state.inner)
}

// MarshalJSON implements json.Marshaler.
func (m *Map[K, V]) MarshalJSON() ([]byte, error) {
	m.state.mu.RLock()
	defer m.state.mu.RUnlock()
	return json.Marshal(m.state.inner)
}
