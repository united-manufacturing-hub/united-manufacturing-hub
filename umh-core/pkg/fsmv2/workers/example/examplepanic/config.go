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

package example_panic

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// ExamplepanicConfig is the typed configuration for the panic worker.
type ExamplepanicConfig struct {
	config.BaseUserSpec      // Provides State field with GetState() defaulting to "running"
	ShouldRun           bool `yaml:"should_run"`
	ShouldPanic         bool `yaml:"should_panic"`
}

// GetShouldPanic returns whether the worker should panic during connect action.
// CollectObservedState should always go through this accessor so future
// defaulting/validation logic has a single insertion point.
func (s *ExamplepanicConfig) GetShouldPanic() bool {
	return s.ShouldPanic
}

// ExamplepanicStatus holds the runtime observation data for the panic worker.
// Only worker-specific business fields belong here; the framework supplies
// CollectedAt, State, LastActionResults, MetricsEmbedder, and ChildrenHealthy/Unhealthy
// via Observation[ExamplepanicStatus].
type ExamplepanicStatus struct {
	// ConnectionHealth reports the current connection state ("healthy" or "no connection").
	// Populated by CollectObservedState from dependencies.IsConnected().
	ConnectionHealth string `json:"connection_health"`
}
