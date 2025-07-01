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

// SubscriberData holds both the subscriber email and their bootstrapped state
type SubscriberData struct {
	Email        string
	Bootstrapped bool
}

// Registry manages subscribers and their bootstrapped state with automatic expiration
type Registry struct {
	subscribers *expiremap.ExpireMap[string, SubscriberData]
	mu          sync.RWMutex
}

// NewRegistry creates a new subscriber registry with the given TTL and cull interval
func NewRegistry(cullInterval, ttl time.Duration) *Registry {
	return &Registry{
		subscribers: expiremap.NewEx[string, SubscriberData](cullInterval, ttl),
	}
}

// AddOrRefresh ensures the subscriber exists and refreshes its TTL.
// If it's new we start with Bootstrapped = false.
func (r *Registry) AddOrRefresh(email string, bootstrapped bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, exists := r.subscribers.Load(email)
	if !exists {
		data = &SubscriberData{Email: email, Bootstrapped: bootstrapped}
	} else {
		data.Bootstrapped = bootstrapped
	}
	r.subscribers.Set(email, *data) // refresh TTL (or insert)
}

// List returns all active subscriber emails
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var subscribers []string
	r.subscribers.Range(func(key string, value SubscriberData) bool {
		subscribers = append(subscribers, value.Email)
		return true
	})
	return subscribers
}

// IsBootstrapped returns whether a subscriber has been bootstrapped. Returns false if not found.
func (r *Registry) IsBootstrapped(email string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	data, exists := r.subscribers.Load(email)
	if !exists {
		return false
	}
	return data.Bootstrapped
}

// ForEach iterates over all active subscribers and their bootstrapped state
func (r *Registry) ForEach(fn func(email string, bootstrapped bool)) {
	// Collect all subscriber data first to avoid holding locks during callback
	type subscriberInfo struct {
		email        string
		bootstrapped bool
	}

	var subscribers []subscriberInfo

	r.mu.RLock()
	r.subscribers.Range(func(key string, value SubscriberData) bool {
		subscribers = append(subscribers, subscriberInfo{
			email:        value.Email,
			bootstrapped: value.Bootstrapped,
		})
		return true
	})
	r.mu.RUnlock()

	// Now call callbacks without holding any locks
	for _, sub := range subscribers {
		fn(sub.email, sub.bootstrapped)
	}
}

// HasNewSubscribers returns true if there are new subscribers since the last call
func (r *Registry) HasNewSubscribers() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	hasNew := false
	r.subscribers.Range(func(key string, value SubscriberData) bool {
		if !value.Bootstrapped {
			hasNew = true
		}
		return true
	})

	return hasNew
}

// SetBootstrapped updates the bootstrapped state for a subscriber
func (r *Registry) SetBootstrapped(email string, bootstrapped bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Only set if the subscriber exists
	if data, exists := r.subscribers.Load(email); exists {
		updatedData := *data
		updatedData.Bootstrapped = bootstrapped
		// Update the expiremap entry to refresh TTL
		r.subscribers.Set(email, updatedData)
	}
}

// Length returns the number of active subscribers
func (r *Registry) Length() int {
	return r.subscribers.Length()
}
