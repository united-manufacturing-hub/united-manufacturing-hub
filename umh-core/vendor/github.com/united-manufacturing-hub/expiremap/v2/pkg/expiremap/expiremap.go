package expiremap

import (
	"sync"
	"time"
)

// item struct represents an individual element with a value and its expiration time.
type item[V any] struct {
	value     V
	expiresAt time.Time
}

// ExpireMap is a generic map structure that allows setting and retrieving items with expiration.
type ExpireMap[T comparable, V any] struct {
	m          map[T]item[V] // Holds the actual data with their expiration details.
	lock       sync.RWMutex  // Mutex for ensuring concurrent access.
	cullPeriod time.Duration // Duration to wait before cleaning expired items.
	defaultTTL time.Duration // Default time to live for items if not specified.
}

// New creates a new instance of ExpireMap with default cullPeriod and defaultTTL set to 1 minute.
func New[T comparable, V any]() *ExpireMap[T, V] {
	return NewEx[T, V](time.Minute, time.Minute)
}

// NewEx creates a new instance of ExpireMap with specified cullPeriod and defaultTTL.
func NewEx[T comparable, V any](cullPeriod, defaultTTL time.Duration) *ExpireMap[T, V] {
	var m = ExpireMap[T, V]{
		m:          make(map[T]item[V]),
		cullPeriod: cullPeriod,
		defaultTTL: defaultTTL,
		lock:       sync.RWMutex{},
	}
	go m.cull()
	return &m
}

// Set adds a new item to the map with the default TTL.
func (m *ExpireMap[T, V]) Set(key T, value V) {
	m.SetEx(key, value, m.defaultTTL)
}

// SetEx adds a new item to the map with a specified TTL.
func (m *ExpireMap[T, V]) SetEx(key T, value V, ttl time.Duration) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.m[key] = item[V]{value: value, expiresAt: time.Now().Add(ttl)}
}

// LoadOrStore retrieves an item from the map by key. If it doesn't exist, stores the provided value with the default TTL.

func (m *ExpireMap[T, V]) LoadOrStore(key T, value V) (*V, bool) {
	return m.LoadOrStoreEx(key, value, m.defaultTTL)
}

// LoadOrStoreEx retrieves an item from the map by key. If it doesn't exist, stores the provided value with the specified TTL.
func (m *ExpireMap[T, V]) LoadOrStoreEx(key T, value V, ttl time.Duration) (*V, bool) {
	m.lock.Lock()
	defer m.lock.Unlock()

	if v, ok := m.m[key]; ok && v.expiresAt.After(time.Now()) {
		return &v.value, true
	}
	m.m[key] = item[V]{value: value, expiresAt: time.Now().Add(ttl)}
	return &value, false
}

// LoadAndDelete retrieves the newest valid item from the map by key and then deletes it.
func (m *ExpireMap[T, V]) LoadAndDelete(key T) (*V, bool) {
	m.lock.Lock()
	defer m.lock.Unlock()

	if v, ok := m.m[key]; ok && v.expiresAt.After(time.Now()) {
		delete(m.m, key)
		return &v.value, true
	}
	return nil, false
}

// Load retrieves the newest valid item from the map by key.
func (m *ExpireMap[T, V]) Load(key T) (*V, bool) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	if v, ok := m.m[key]; ok && v.expiresAt.After(time.Now()) {
		return &v.value, true
	}
	return nil, false
}

// Delete removes all items associated with the provided key from the map.
func (m *ExpireMap[T, V]) Delete(key T) {
	m.lock.Lock()
	defer m.lock.Unlock()
	delete(m.m, key)
}

// cull periodically cleans up expired items from the map.
func (m *ExpireMap[T, V]) cull() {
	ticker := time.NewTicker(m.cullPeriod)
	defer ticker.Stop()

	for {
		<-ticker.C
		m.lock.Lock()
		now := time.Now()
		for k, v := range m.m {
			if v.expiresAt.Before(now) {
				delete(m.m, k)
			}
		}
		m.lock.Unlock()
	}
}

// Range iterates over each key-value pair in the map and calls the provided function until it returns false.
func (m *ExpireMap[T, V]) Range(f func(key T, value V) bool) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	for k, v := range m.m {
		if !f(k, v.value) {
			break
		}
	}
}

// Length returns the number of items in the map.
func (m *ExpireMap[T, V]) Length() int {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return len(m.m)
}
