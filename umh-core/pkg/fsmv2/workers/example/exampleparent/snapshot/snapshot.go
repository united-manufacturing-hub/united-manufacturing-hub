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

// Package snapshot holds the exampleparent worker's Config and Status value
// types. It exists as a separate leaf package so the state sub-package can
// depend on these types without introducing an import cycle with the worker
// package.
//
// Post-PR3-C3 the exampleparent worker uses fsmv2.Observation[ExampleparentStatus]
// and *fsmv2.WrappedDesiredState[ExampleparentConfig]; the underlying value
// types are defined here and re-exported from the worker package as type
// aliases for caller convenience.
package snapshot

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// ExampleparentConfig holds the user-provided configuration for the parent
// worker. Embeds BaseUserSpec to support the StateGetter interface, allowing
// WorkerBase.DeriveDesiredState to extract the desired state from the "state"
// YAML field.
//
// ChildrenCount, ChildWorkerType and ChildConfig are parsed from the parent's
// UserSpec.Config template and consumed by the child-spec factory to declare
// the desired children set.
type ExampleparentConfig struct {
	config.BaseUserSpec `yaml:",inline"`

	// ChildWorkerType is the registered worker type to use for spawned children.
	// Defaults to "examplechild" when empty.
	ChildWorkerType string `json:"child_worker_type" yaml:"child_worker_type"`

	// ChildConfig is the UserSpec.Config template handed to each spawned child.
	// When empty the factory falls back to a built-in address/device template.
	ChildConfig string `json:"child_config" yaml:"child_config"`

	// ChildrenCount is the number of children to spawn. Zero means no children.
	ChildrenCount int `json:"children_count" yaml:"children_count"`
}

// GetChildWorkerType returns the configured child worker type, defaulting to
// "examplechild" when the field is empty.
func (c *ExampleparentConfig) GetChildWorkerType() string {
	if c.ChildWorkerType == "" {
		return "examplechild"
	}

	return c.ChildWorkerType
}

// ExampleparentStatus holds the runtime observation data for the parent worker.
// Framework fields (CollectedAt, State, LastActionResults, MetricsEmbedder,
// ShutdownRequested, children counts) are carried by
// fsmv2.Observation[ExampleparentStatus] and are not duplicated here. The
// parent worker has no worker-specific status fields of its own — its state
// machine decides transitions purely from framework-level children health
// counts and the parent-mapped state.
type ExampleparentStatus struct{}
