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

// Package snapshot holds the exampleslow worker's Config and Status value
// types plus the ExampleslowDependencies interface consumed by actions. It
// exists as a separate leaf package so the state sub-package can depend on
// these types without introducing an import cycle with the worker package.
//
// Post-PR3-C1 the exampleslow worker uses fsmv2.Observation[ExampleslowStatus]
// and *fsmv2.WrappedDesiredState[ExampleslowConfig]; the underlying value
// types are defined here and re-exported from the worker package as type
// aliases for caller convenience.
package snapshot

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// ExampleslowConfig holds the user-provided configuration for the slow worker.
// Embeds BaseUserSpec to expose GetState() for WorkerBase.DeriveDesiredState, allowing
// WorkerBase.DeriveDesiredState to extract the desired state from the "state"
// YAML field.
type ExampleslowConfig struct {
	config.BaseUserSpec `yaml:",inline"`
	DelaySeconds        int `json:"delaySeconds" yaml:"delaySeconds"`
}

// ExampleslowStatus holds the runtime observation data for the slow worker.
// Framework fields (CollectedAt, State, LastActionResults, MetricsEmbedder,
// ShutdownRequested, children counts) are carried by fsmv2.Observation[ExampleslowStatus]
// and are not duplicated here.
type ExampleslowStatus struct {
	ConnectionHealth string `json:"connection_health"`
	ConnectAttempts  int    `json:"connect_attempts"`
}

// ExampleslowDependencies is the dependencies interface for exampleslow actions (avoids import cycles).
type ExampleslowDependencies interface {
	deps.Dependencies
	SetConnected(connected bool)
	IsConnected() bool
	SetDelaySeconds(delaySeconds int)
	GetDelaySeconds() int
}
