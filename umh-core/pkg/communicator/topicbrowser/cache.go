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
	mu                 sync.RWMutex
	eventMap           map[string]*tbproto.EventTableEntry // key = UnsTreeId from events
	unsMap             *tbproto.TopicMap
	lastCacheTimestamp int64
	lastSentTimestamp  int64
}

// NewCache creates a new topic browser cache
func NewCache() *Cache {
	return &Cache{
		eventMap: make(map[string]*tbproto.EventTableEntry),
		unsMap: &tbproto.TopicMap{
			Entries: make(map[string]*tbproto.TopicInfo),
		},
		lastCacheTimestamp: 0,
		lastSentTimestamp:  0,
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

	// determine the relevant buffer elements (UnsBundles) to process
	// the relevant buffers are the ones that have a timestamp greater than the last cache timestamp
	// with this logic, we can avoid processing the same buffer multiple times becuase it will stay in the buffer for a while
	latestProcessedTimestamp := c.lastCacheTimestamp
	relevantBuffers := make([]*Buffer, 0)
	for _, buf := range obs.ServiceInfo.Status.Buffer {
		if buf.Timestamp > c.lastCacheTimestamp {
			relevantBuffers = append(relevantBuffers, buf)
			if buf.Timestamp > latestProcessedTimestamp {
				latestProcessedTimestamp = buf.Timestamp
			}
		}
	}

	// process the relevant buffers
	for _, buf := range relevantBuffers {
		// Unmarshal the protobuf data (assuming it's already decompressed)

		var ub tbproto.UnsBundle
		if err := proto.Unmarshal(buf.Payload, &ub); err != nil {
			// Skip invalid protobuf data
			continue
		}

		// upsert the latest event by UnsTreeId (key)
		// if the event is newer, we overwrite the existing event
		// if the event is older, we skip it
		for _, entry := range ub.Events.Entries {
			existing, exists := c.eventMap[entry.UnsTreeId]
			if !exists || entry.ProducedAtMs > existing.ProducedAtMs {
				c.eventMap[entry.UnsTreeId] = entry
			}
		}

		// upsert the uns map
		for _, entry := range ub.UnsMap.Entries {
			c.unsMap.Entries[entry.Name] = entry
		}
	}

	// update the last cache timestamp
	c.lastCacheTimestamp = latestProcessedTimestamp

	return nil
}

// this function is used to convert the cache into one proto-encoded UnsBundle
func (c *Cache) ToUnsBundleProto() []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// create a new uns bundle
	ub := &tbproto.UnsBundle{
		Events: &tbproto.EventTable{
			Entries: make([]*tbproto.EventTableEntry, 0, len(c.eventMap)),
		},
		UnsMap: &tbproto.TopicMap{
			Entries: make(map[string]*tbproto.TopicInfo),
		},
	}

	// add the latest events to the uns bundle
	for _, entry := range c.eventMap {
		ub.Events.Entries = append(ub.Events.Entries, entry)
	}

	// add the uns map to the uns bundle
	for _, entry := range c.unsMap.Entries {
		ub.UnsMap.Entries[entry.Name] = entry
	}

	// proto encode the uns bundle
	encoded, err := proto.Marshal(ub)
	if err != nil {
		return nil
	}

	return encoded
}

// Snapshot returns a deep copy of all cached bundles
func (c *Cache) Snapshot() map[string]*tbproto.UnsBundle {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Create a deep copy to avoid data races
	dup := make(map[string]*tbproto.UnsBundle, len(c.eventMap))
	for key, bundle := range c.eventMap {
		dup[key] = proto.Clone(bundle).(*tbproto.UnsBundle)
	}

	return dup
}

// GetKeys returns all the topic keys currently in the cache
func (c *Cache) GetKeys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	keys := make([]string, 0, len(c.eventMap))
	for key := range c.eventMap {
		keys = append(keys, key)
	}
	return keys
}

// Size returns the number of topics currently cached
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.eventMap)
}

// GetLastCachedTimestamp returns the timestamp of the last bundle processed into the cache
func (c *Cache) GetLastCachedTimestamp() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastCacheTimestamp
}

// GetLastSentTimestamp returns the timestamp of the last bundle sent to subscribers
func (c *Cache) GetLastSentTimestamp() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastSentTimestamp
}

// SetLastSentTimestamp updates the timestamp of the last bundle sent to subscribers
func (c *Cache) SetLastSentTimestamp(timestamp int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastSentTimestamp = timestamp
}
