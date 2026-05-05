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

package examplechild

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// ExamplechildConfig defines the typed configuration for the child worker.
// This is parsed from the UserSpec.Config YAML/JSON string.
type ExamplechildConfig struct {
	config.BaseUserSpec // Provides State field with GetState() defaulting to "running"
}

// ExamplechildStatus holds the runtime observation data for the child worker.
// Only worker-specific business fields belong here; the framework supplies
// CollectedAt, State, LastActionResults, MetricsEmbedder, ChildrenHealthy/Unhealthy
// via Observation[ExamplechildStatus].
type ExamplechildStatus struct {
	// ConnectionHealth reports the current connection state ("healthy" or "no connection").
	// Populated by CollectObservedState from dependencies.IsConnected().
	ConnectionHealth string `json:"connection_health"`
}
