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
	"time"

	topicbrowserfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/topicbrowser"
	"go.uber.org/zap"
)

// ProcessingResult contains information about what was processed during a cache update
type ProcessingResult struct {
	ProcessedCount  int       // Number of new buffers processed
	SkippedCount    int       // Number of buffers skipped due to errors
	LatestTimestamp time.Time // Latest timestamp of processed data
	DebugInfo       string    // Debug information for logging
}

// SubscriberData contains the data to be sent to subscribers
type SubscriberData struct {
	TopicCount int
	UnsBundles map[int][]byte
}

// TopicBrowserCommunicator consolidates cache and simulator functionality
// This eliminates the redundant ticker by handling updates through the subscriber pipeline
type TopicBrowserCommunicator struct {
	cache            *Cache
	simulator        *Simulator
	simulatorEnabled bool
	logger           *zap.SugaredLogger
}

// NewTopicBrowserCommunicator creates a new communicator without simulator
func NewTopicBrowserCommunicator(logger *zap.SugaredLogger) *TopicBrowserCommunicator {
	return &TopicBrowserCommunicator{
		cache:            NewCache(),
		simulator:        nil,
		simulatorEnabled: false,
		logger:           logger,
	}
}

// NewTopicBrowserCommunicatorWithSimulator creates a new communicator with simulator enabled
func NewTopicBrowserCommunicatorWithSimulator(logger *zap.SugaredLogger) *TopicBrowserCommunicator {
	simulator := NewSimulator()
	simulator.InitializeSimulator()

	return &TopicBrowserCommunicator{
		cache:            NewCache(),
		simulator:        simulator,
		simulatorEnabled: true,
		logger:           logger,
	}
}

// IsSimulatorEnabled returns whether simulator mode is enabled
func (c *TopicBrowserCommunicator) IsSimulatorEnabled() bool {
	return c.simulatorEnabled
}

// ProcessSimulatedData processes simulated data and updates the cache
func (c *TopicBrowserCommunicator) ProcessSimulatedData() (*ProcessingResult, error) {
	if !c.simulatorEnabled || c.simulator == nil {
		return &ProcessingResult{
			DebugInfo: "simulator not enabled",
		}, nil
	}

	// Tick the simulator to generate new data
	c.simulator.Tick()

	// Get the simulated observed state
	simObservedState := c.simulator.GetSimObservedState()

	// Process using the cache's incremental update method
	return c.cache.ProcessIncrementalUpdates(simObservedState)
}

// ProcessRealData processes real FSM data and updates the cache
func (c *TopicBrowserCommunicator) ProcessRealData(obs *topicbrowserfsm.ObservedStateSnapshot) (*ProcessingResult, error) {
	if c.simulatorEnabled {
		return &ProcessingResult{
			DebugInfo: "real data processing skipped - simulator enabled",
		}, nil
	}

	// Process using the cache's incremental update method
	return c.cache.ProcessIncrementalUpdates(obs)
}

// GetSubscriberData returns data for subscribers based on their bootstrap status
func (c *TopicBrowserCommunicator) GetSubscriberData(isBootstrapped bool) (*SubscriberData, error) {
	unsBundles := make(map[int][]byte)

	// For new subscribers, add cached bundle at index 0 with complete topic coverage
	if !isBootstrapped {
		unsBundles[0] = c.cache.ToUnsBundleProto()
	}

	// Add incremental content starting at the appropriate index
	// This would be implemented based on the specific logic needed
	// For now, return the basic structure

	return &SubscriberData{
		TopicCount: c.cache.Size(),
		UnsBundles: unsBundles,
	}, nil
}

// GetCache returns the underlying cache for direct access when needed
func (c *TopicBrowserCommunicator) GetCache() *Cache {
	return c.cache
}

// GetSimulator returns the underlying simulator for direct access when needed (if enabled)
func (c *TopicBrowserCommunicator) GetSimulator() *Simulator {
	return c.simulator
}
