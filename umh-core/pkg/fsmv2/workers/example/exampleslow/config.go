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

package exampleslow

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// ExampleslowConfig defines the typed configuration for the slow worker.
type ExampleslowConfig struct {
	config.BaseUserSpec
	DelaySeconds int `yaml:"delaySeconds"`
}

// GetDelaySeconds returns the configured connect delay in seconds.
// State files and CollectObservedState should always go through this accessor
// so future defaulting/validation logic has a single insertion point.
func (s *ExampleslowConfig) GetDelaySeconds() int {
	return s.DelaySeconds
}

// ExampleslowStatus holds the runtime observation data for the slow worker.
// Only worker-specific business fields belong here; the framework supplies
// CollectedAt, State, LastActionResults, MetricsEmbedder, and ChildrenHealthy/Unhealthy
// via Observation[ExampleslowStatus].
type ExampleslowStatus struct {
	// ConnectionHealth reports the current connection state ("healthy" or "no connection").
	// Populated by CollectObservedState from dependencies.IsConnected().
	ConnectionHealth string `json:"connection_health"`
}
