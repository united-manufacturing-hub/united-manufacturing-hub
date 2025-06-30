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
	subscribers *expiremap.ExpireMap[string, *SubscriberData]
	mu          sync.RWMutex
}

// NewRegistry creates a new subscriber registry with the given TTL and cull interval
func NewRegistry(cullInterval, ttl time.Duration) *Registry {
	return &Registry{
		subscribers: expiremap.NewEx[string, *SubscriberData](cullInterval, ttl),
	}
}

// Add adds a subscriber to the registry. If the subscriber is new, it initializes
// their bootstrapped state to false so they receive the full topic tree (see umh-core/pkg/communicator/pkg/generator/topicbrowser.go for more details)
func (r *Registry) Add(email string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Add to expiremap with combined data (this handles TTL and expiration)
	r.subscribers.Set(email, &SubscriberData{
		Email:        email,
		Bootstrapped: false, // new or returning subscribers need the full tree
	})
}

// List returns all active subscriber emails
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var subscribers []string
	r.subscribers.Range(func(key string, value *SubscriberData) bool {
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
	return (*data).Bootstrapped
}

// Unexpire resets the TTL for a subscriber by calling Set()
func (r *Registry) Unexpire(email string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if data, exists := r.subscribers.Load(email); exists {
		// Preserve existing bootstrapped state
		r.subscribers.Set(email, *data)
	} else {
		// Create new subscriber if doesn't exist
		r.subscribers.Set(email, &SubscriberData{
			Email:        email,
			Bootstrapped: false,
		})
	}
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
	r.subscribers.Range(func(key string, value *SubscriberData) bool {
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

// SetBootstrapped updates the bootstrapped state for a subscriber
func (r *Registry) SetBootstrapped(email string, bootstrapped bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Only set if the subscriber exists
	if data, exists := r.subscribers.Load(email); exists {
		(*data).Bootstrapped = bootstrapped
		// Update the expiremap entry to refresh TTL
		r.subscribers.Set(email, *data)
	}
}

// Length returns the number of active subscribers
func (r *Registry) Length() int {
	return r.subscribers.Length()
}
