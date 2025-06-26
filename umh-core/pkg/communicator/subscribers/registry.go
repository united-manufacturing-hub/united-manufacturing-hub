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

// Registry manages subscribers and their bootstrapped state with automatic expiration
type Registry struct {
	subscribers  *expiremap.ExpireMap[string, string]
	bootstrapped *expiremap.ExpireMap[string, bool]
	mu           sync.RWMutex
}

// NewRegistry creates a new subscriber registry with the given TTL and cull interval
func NewRegistry(cullInterval, ttl time.Duration) *Registry {
	return &Registry{
		subscribers:  expiremap.NewEx[string, string](cullInterval, ttl),
		bootstrapped: expiremap.NewEx[string, bool](cullInterval, ttl),
	}
}

// Add adds a subscriber to the registry. If the subscriber is new, it initializes
// their bootstrapped state to false so they receive the full topic tree (see umh-core/pkg/communicator/pkg/generator/topicbrowser.go for more details)
func (r *Registry) Add(email string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Add to expiremap (this handles TTL and expiration)
	r.subscribers.Set(email, email)

	// Set bootstrapped state to false (new or returning subscribers need the full tree)
	r.bootstrapped.Set(email, false)
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

// IsBootstrapped returns whether a subscriber has been bootstrapped. Returns false if not found.
func (r *Registry) IsBootstrapped(email string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	bootstrapped, exists := r.bootstrapped.Load(email)
	if !exists {
		return false
	}
	return *bootstrapped
}

// ForEach iterates over all active subscribers and their bootstrapped state
func (r *Registry) ForEach(fn func(email string, bootstrapped bool)) {
	// Collect all subscriber data first to avoid holding locks during callback
	type subscriberData struct {
		email        string
		bootstrapped bool
	}

	var subscribers []subscriberData

	r.mu.RLock()
	r.subscribers.Range(func(key string, value string) bool {
		bootstrapped, exists := r.bootstrapped.Load(key)
		bootstrappedValue := false
		if exists {
			bootstrappedValue = *bootstrapped
		}
		subscribers = append(subscribers, subscriberData{
			email:        key,
			bootstrapped: bootstrappedValue,
		})
		return true
	})
	r.mu.RUnlock()

	// Now call callbacks without holding any locks
	for _, sub := range subscribers {
		fn(sub.email, sub.bootstrapped)
	}
}

// SetBootstrapped updates the bootstrapped state for a subscriber
func (r *Registry) SetBootstrapped(email string, bootstrapped bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Only set if the subscriber exists
	if _, exists := r.subscribers.Load(email); exists {
		r.bootstrapped.Set(email, bootstrapped)
	}
}

// Length returns the number of active subscribers
func (r *Registry) Length() int {
	return r.subscribers.Length()
}
