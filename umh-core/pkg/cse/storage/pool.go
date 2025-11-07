// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package storage

import (
	"errors"
	"fmt"
	"sync"
)

// Closeable is an interface for objects that can be explicitly closed.
//
// DESIGN DECISION: Minimal interface with single Close() method
// WHY: Compatible with standard library io.Closer and database/sql closeable types
// TRADE-OFF: No Shutdown context support, but keeps interface simple
// INSPIRED BY: io.Closer, database/sql.Rows, net.Conn
type Closeable interface {
	Close() error
}

// Factory is a function that creates new objects for the pool.
//
// DESIGN DECISION: Return interface{} instead of generic type
// WHY: Go 1.18+ generics not yet widely adopted in umh-core codebase
// TRADE-OFF: Type assertions required, but simpler implementation
// INSPIRED BY: sync.Pool New function, factory pattern in design patterns
type Factory func() (interface{}, error)

// ObjectPool manages a collection of reusable objects with reference counting.
//
// DESIGN DECISION: Reference-counted pool instead of sync.Pool
// WHY: Need explicit control over object lifecycle (when to create/destroy)
// and reference tracking for shared resources (database connections, file handles)
// TRADE-OFF: Manual memory management vs sync.Pool's automatic GC-aware pooling
// INSPIRED BY: Database connection pools (c3p0, HikariCP), Apache Commons Pool
//
// Use cases:
//  1. Database connection pooling (reuse expensive connections)
//  2. Worker pools (reuse goroutines with state)
//  3. Resource sharing (multiple FSM workers accessing same sync state)
//
// Thread-safety: All methods use RWMutex for concurrent access.
//
// Example:
//
//	pool := cse.NewObjectPool()
//
//	// Create database connection with factory
//	factory := func() (interface{}, error) {
//	    return sql.Open("sqlite3", "/data/db.sqlite")
//	}
//	conn, _ := pool.GetOrCreate("db-conn", factory)
//
//	// Share connection (increment ref count)
//	pool.Acquire("db-conn")
//
//	// Release when done (decrement ref count)
//	pool.Release("db-conn") // Still in pool (ref count = 1)
//	pool.Release("db-conn") // Removed and closed (ref count = 0)
type ObjectPool struct {
	// objects maps key to pooled object
	objects map[string]interface{}

	// refs tracks reference count for each key
	refs map[string]int

	// mu protects concurrent access to objects and refs maps
	mu sync.RWMutex
}

// NewObjectPool creates a new empty object pool.
//
// DESIGN DECISION: Start with empty pool, not pre-populated
// WHY: Lazy initialization - only create objects when needed
// TRADE-OFF: First access is slower (must create object), but saves memory
// INSPIRED BY: sync.Pool (starts empty), database connection pools (configurable min size)
//
// Returns:
//   - *ObjectPool: Ready-to-use pool instance
func NewObjectPool() *ObjectPool {
	return &ObjectPool{
		objects: make(map[string]interface{}),
		refs:    make(map[string]int),
	}
}

// Get retrieves an object from the pool without affecting reference count.
//
// DESIGN DECISION: Get doesn't increment reference count, Acquire does
// WHY: Read-only access pattern - peek at object without ownership
// TRADE-OFF: Caller can't prevent object from being removed, but useful for checking existence
// INSPIRED BY: map[string]interface{} lookup semantics, peek operations
//
// Parameters:
//   - key: Object identifier
//
// Returns:
//   - interface{}: The pooled object (nil if not found)
//   - bool: True if object exists in pool
//
// Note: For proper reference counting, use GetOrCreate or Acquire instead.
func (p *ObjectPool) Get(key string) (interface{}, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	obj, found := p.objects[key]

	return obj, found
}

// Put adds an object to the pool with initial reference count of 1.
//
// DESIGN DECISION: Put always sets ref count to 1, not 0
// WHY: Caller just created object and owns first reference
// TRADE-OFF: Can't put object with ref count 0, but that would be useless anyway
// INSPIRED BY: Smart pointers (initial ref count = 1), database connection acquisition
//
// Parameters:
//   - key: Unique object identifier (must be non-empty)
//   - obj: Object to pool (must be non-nil)
//
// Returns:
//   - error: If key is empty or object is nil
//
// Note: Put does NOT check if key already exists - will overwrite existing object.
// For safer usage, check Has() first or use GetOrCreate().
func (p *ObjectPool) Put(key string, obj interface{}) error {
	if key == "" {
		return errors.New("cannot put object with empty key")
	}

	if obj == nil {
		return errors.New("cannot put nil object")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.objects[key] = obj
	p.refs[key] = 1

	return nil
}

// Remove explicitly removes an object from the pool and closes it if Closeable.
//
// DESIGN DECISION: Ignore reference count, always remove immediately
// WHY: Force removal for cleanup/shutdown scenarios
// TRADE-OFF: Can break other holders of references, but needed for cleanup
// INSPIRED BY: Database connection pool drain, force-close operations
//
// Parameters:
//   - key: Object identifier
//
// Returns:
//   - error: If Close() fails (if object implements Closeable)
//
// Note: This is idempotent - removing non-existent key returns nil.
// Close() is called if object implements Closeable interface.
func (p *ObjectPool) Remove(key string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	obj, found := p.objects[key]
	if !found {
		return nil
	}

	var closeErr error
	if closeable, ok := obj.(Closeable); ok {
		closeErr = closeable.Close()
	}

	delete(p.objects, key)
	delete(p.refs, key)

	return closeErr
}

// Has checks if an object exists in the pool.
//
// DESIGN DECISION: Return false for empty key instead of panicking
// WHY: Graceful handling of invalid input
// TRADE-OFF: Silent failure, but empty keys are never valid anyway
// INSPIRED BY: map[string]interface{} existence check pattern
//
// Parameters:
//   - key: Object identifier
//
// Returns:
//   - bool: True if object exists in pool
func (p *ObjectPool) Has(key string) bool {
	if key == "" {
		return false
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	_, found := p.objects[key]

	return found
}

// Size returns the number of objects currently in the pool.
//
// DESIGN DECISION: Return count of objects, not total ref count
// WHY: Useful for monitoring pool size (not individual references)
// TRADE-OFF: Can't see how many references exist across all objects
// INSPIRED BY: len(map), pool size metrics in connection pools
//
// Returns:
//   - int: Number of unique objects in pool
func (p *ObjectPool) Size() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return len(p.objects)
}

// Clear removes all objects from the pool and closes Closeable objects.
//
// DESIGN DECISION: Collect all Close() errors, don't stop on first failure
// WHY: Attempt to close ALL objects even if some fail
// TRADE-OFF: Complex error handling, but maximizes resource cleanup
// INSPIRED BY: defer chains (run all cleanups), multi-error patterns in Go
//
// Returns:
//   - error: Aggregated error if any Close() calls fail
//
// Note: Pool is emptied even if Close() errors occur.
// This is typically called during shutdown or test cleanup.
func (p *ObjectPool) Clear() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var errs []error

	for key, obj := range p.objects {
		if closeable, ok := obj.(Closeable); ok {
			if err := closeable.Close(); err != nil {
				errs = append(errs, fmt.Errorf("error closing %s: %w", key, err))
			}
		}
	}

	p.objects = make(map[string]interface{})
	p.refs = make(map[string]int)

	if len(errs) > 0 {
		return fmt.Errorf("errors during clear: %v", errs)
	}

	return nil
}

// GetOrCreate retrieves an existing object or creates a new one using the factory.
//
// DESIGN DECISION: Double-check locking pattern for concurrent creation
// WHY: Avoid creating duplicate objects when multiple goroutines call simultaneously
// TRADE-OFF: More complex locking, but prevents expensive factory calls
// INSPIRED BY: Singleton pattern, lazy initialization, sync.Once
//
// Parameters:
//   - key: Unique object identifier (must be non-empty)
//   - factory: Function to create object if it doesn't exist (must be non-nil)
//
// Returns:
//   - interface{}: Existing or newly created object
//   - error: If key/factory is invalid, factory fails, or factory returns nil
//
// Locking strategy:
//  1. Read lock: Check if object exists (fast path)
//  2. If not found, upgrade to write lock
//  3. Double-check: Another goroutine may have created object while waiting for lock
//  4. Call factory if still not found
//
// Example:
//
//	factory := func() (interface{}, error) {
//	    return sql.Open("sqlite3", "/data/db.sqlite")
//	}
//	conn, err := pool.GetOrCreate("db-conn", factory)
//	db := conn.(*sql.DB) // Type assertion
func (p *ObjectPool) GetOrCreate(key string, factory Factory) (interface{}, error) {
	if key == "" {
		return nil, errors.New("cannot get or create object with empty key")
	}

	if factory == nil {
		return nil, errors.New("nil factory provided")
	}

	p.mu.RLock()
	obj, found := p.objects[key]
	p.mu.RUnlock()

	if found {
		return obj, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	obj, found = p.objects[key]
	if found {
		return obj, nil
	}

	newObj, err := factory()
	if err != nil {
		return nil, err
	}

	if newObj == nil {
		return nil, errors.New("factory returned nil object")
	}

	p.objects[key] = newObj
	p.refs[key] = 1

	return newObj, nil
}

// Acquire increments the reference count for an existing object.
//
// DESIGN DECISION: Error if object doesn't exist, don't create it
// WHY: Acquire is for sharing existing resources, not creating new ones
// TRADE-OFF: Must ensure object exists first (use Has or GetOrCreate)
// INSPIRED BY: Smart pointer acquire, database connection checkout
//
// Parameters:
//   - key: Object identifier (must be non-empty)
//
// Returns:
//   - error: If key is empty or object doesn't exist
//
// Example:
//
//	// Goroutine 1 creates connection
//	pool.GetOrCreate("db-conn", factory)
//
//	// Goroutine 2 shares connection
//	pool.Acquire("db-conn") // ref count = 2
//
//	// Both release when done
//	pool.Release("db-conn") // ref count = 1
//	pool.Release("db-conn") // ref count = 0, closed
func (p *ObjectPool) Acquire(key string) error {
	if key == "" {
		return errors.New("cannot acquire object with empty key")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if _, found := p.objects[key]; !found {
		return fmt.Errorf("object with key %s not found", key)
	}

	p.refs[key]++

	return nil
}

// Release decrements the reference count and removes object if count reaches 0.
//
// DESIGN DECISION: Auto-close when ref count reaches 0
// WHY: Automatic resource cleanup when no longer needed
// TRADE-OFF: Caller must track Acquire/Release pairs carefully
// INSPIRED BY: Smart pointers (std::shared_ptr), database connection pooling
//
// Parameters:
//   - key: Object identifier (must be non-empty)
//
// Returns:
//   - error: If key is empty, object doesn't exist, or Close() fails
//
// Behavior:
//   - If ref count > 1: Decrement count, keep object in pool
//   - If ref count = 1: Decrement to 0, close if Closeable, remove from pool
//   - If ref count ≤ 0: Remove from pool (shouldn't happen, but defensive)
//
// Example:
//
//	pool.GetOrCreate("db-conn", factory) // ref count = 1
//	pool.Acquire("db-conn")              // ref count = 2
//	pool.Release("db-conn")              // ref count = 1, still in pool
//	pool.Release("db-conn")              // ref count = 0, closed and removed
func (p *ObjectPool) Release(key string) error {
	if key == "" {
		return errors.New("cannot release object with empty key")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	obj, found := p.objects[key]
	if !found {
		return fmt.Errorf("object with key %s not found", key)
	}

	p.refs[key]--

	if p.refs[key] <= 0 {
		var closeErr error
		if closeable, ok := obj.(Closeable); ok {
			closeErr = closeable.Close()
		}

		delete(p.objects, key)
		delete(p.refs, key)

		return closeErr
	}

	return nil
}
