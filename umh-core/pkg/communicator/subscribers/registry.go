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

package subscribers

import (
	"sync"
	"time"

	"github.com/united-manufacturing-hub/expiremap/v2/pkg/expiremap"
)

// Meta tracks metadata for each subscriber, particularly for topic browser bootstrapping
type Meta struct {
	FirstSeen    time.Time // When the subscriber was first added
	LastSeq      uint64    // Last topic browser sequence delivered
	Bootstrapped bool      // Did we already send the backlog/full tree?
}

// Registry wraps expiremap and adds metadata tracking for subscribers
type Registry struct {
	subscribers *expiremap.ExpireMap[string, string]
	meta        map[string]*Meta
	mu          sync.RWMutex
}

// NewRegistry creates a new subscriber registry with the given TTL and cull interval
func NewRegistry(cullInterval, ttl time.Duration) *Registry {
	return &Registry{
		subscribers: expiremap.NewEx[string, string](cullInterval, ttl),
		meta:        make(map[string]*Meta),
	}
}

// Add adds a subscriber to the registry. If the subscriber is new, it initializes
// their metadata with Bootstraped=false so they receive the full topic tree (see umh-core/pkg/communicator/pkg/generator/topicbrowser.go for more details)
func (r *Registry) Add(email string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Add to expiremap (this handles TTL and expiration)
	r.subscribers.Set(email, email)

	// Initialize metadata if this is a new subscriber
	if _, exists := r.meta[email]; !exists {
		r.meta[email] = &Meta{
			FirstSeen:    time.Now(),
			LastSeq:      0,
			Bootstrapped: false,
		}
	} else {
		// If the subscriber is not new, we need to update the metadata
		r.meta[email].Bootstrapped = false
	}
}

// List returns all active subscriber emails
func (r *Registry) List() []string {
	var subscribers []string
	r.subscribers.Range(func(key string, value string) bool {
		subscribers = append(subscribers, key)
		return true
	})
	return subscribers
}

// Meta returns the metadata for a subscriber. Returns zero value if not found.
func (r *Registry) Meta(email string) Meta {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if meta, exists := r.meta[email]; exists {
		return *meta
	}
	return Meta{}
}

// ForEach iterates over all active subscribers and their metadata
func (r *Registry) ForEach(fn func(email string, m *Meta)) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.subscribers.Range(func(key string, value string) bool {
		if meta, exists := r.meta[key]; exists {
			fn(key, meta)
		}
		return true
	})
}

// UpdateMeta updates the metadata for a subscriber
func (r *Registry) UpdateMeta(email string, updateFn func(*Meta)) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if meta, exists := r.meta[email]; exists {
		updateFn(meta)
	}
}

// Length returns the number of active subscribers
func (r *Registry) Length() int {
	return r.subscribers.Length()
}
