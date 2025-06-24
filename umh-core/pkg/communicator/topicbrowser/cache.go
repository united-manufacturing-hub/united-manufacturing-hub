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

package topicbrowser

import (
	"sync"

	tbproto "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/models/topicbrowser/pb"
	"google.golang.org/protobuf/proto"
)

// Cache maintains the latest UnsBundle for each topic key
// This provides fast bootstrap for new subscribers and avoids re-decompressing data
type Cache struct {
	mu               sync.RWMutex
	latestEventByKey map[string]*tbproto.UnsBundle // key = UnsTreeId from events
}

// NewCache creates a new topic browser cache
func NewCache() *Cache {
	return &Cache{
		latestEventByKey: make(map[string]*tbproto.UnsBundle),
	}
}

// Buffer represents a compressed data buffer from the FSM
// This is a placeholder until the actual FSM types are available
type Buffer struct {
	Payload   []byte // LZ4 compressed protobuf data
	Timestamp int64  // timestamp from the logs
}

// ObservedState represents the FSM observed state structure
// This is a placeholder until the actual FSM types are available
type ObservedState struct {
	ServiceInfo struct {
		Status struct {
			Buffer []*Buffer
		}
	}
}

// Update processes new buffers from the topic browser FSM observed state
func (c *Cache) Update(obs *ObservedState) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, buf := range obs.ServiceInfo.Status.Buffer {
		// Unmarshal the protobuf data (assuming it's already decompressed)

		var ub tbproto.UnsBundle
		if err := proto.Unmarshal(buf.Payload, &ub); err != nil {
			// Skip invalid protobuf data
			continue
		}

		// Extract the key from the events
		// We need to check which entry is the newest - typically the last one in the array
		if ub.Events != nil && len(ub.Events.Entries) > 0 {
			// Get the last (newest) entry's UnsTreeId as the key
			lastEntry := ub.Events.Entries[len(ub.Events.Entries)-1]
			key := lastEntry.UnsTreeId

			// Store a deep copy of the UnsBundle
			bundleCopy := proto.Clone(&ub).(*tbproto.UnsBundle)
			c.latestEventByKey[key] = bundleCopy
		}
	}

	return nil
}

// Snapshot returns a deep copy of all cached bundles
func (c *Cache) Snapshot() map[string]*tbproto.UnsBundle {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Create a deep copy to avoid data races
	dup := make(map[string]*tbproto.UnsBundle, len(c.latestEventByKey))
	for key, bundle := range c.latestEventByKey {
		dup[key] = proto.Clone(bundle).(*tbproto.UnsBundle)
	}

	return dup
}

// GetKeys returns all the topic keys currently in the cache
func (c *Cache) GetKeys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	keys := make([]string, 0, len(c.latestEventByKey))
	for key := range c.latestEventByKey {
		keys = append(keys, key)
	}
	return keys
}

// Size returns the number of topics currently cached
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.latestEventByKey)
}
