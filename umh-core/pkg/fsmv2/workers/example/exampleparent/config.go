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

package exampleparent

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// ExampleparentConfig defines the typed configuration for the parent worker.
type ExampleparentConfig struct {
	config.BaseUserSpec
	ChildWorkerType string `json:"child_worker_type" yaml:"child_worker_type"` // Optional: defaults to "examplechild"
	ChildConfig     string `json:"child_config"      yaml:"child_config"`      // Optional: config to pass to children
	ChildrenCount   int    `json:"children_count"    yaml:"children_count"`
}

// GetChildWorkerType returns the configured child worker type, defaulting to "examplechild".
func (s *ExampleparentConfig) GetChildWorkerType() string {
	if s.ChildWorkerType == "" {
		return "examplechild"
	}

	return s.ChildWorkerType
}

// ExampleparentStatus holds the runtime observation data for the parent worker.
// Only worker-specific business fields belong here; the framework supplies
// CollectedAt, State, LastActionResults, MetricsEmbedder, and ChildrenHealthy/Unhealthy
// via Observation[ExampleparentStatus].
//
// Currently empty — the parent worker has no business-level fields beyond what
// the framework supplies. Kept as a named type so future fields land in a stable
// location without rewiring the WorkerBase generics.
type ExampleparentStatus struct{}
