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

// Package snapshot holds the examplechild worker's Config and Status value
// types plus the ExamplechildDependencies interface consumed by actions. It
// exists as a separate leaf package so the state sub-package can depend on
// these types without introducing an import cycle with the worker package.
//
// Post-PR3-C2 the examplechild worker uses fsmv2.Observation[ExamplechildStatus]
// and *fsmv2.WrappedDesiredState[ExamplechildConfig]; the underlying value
// types are defined here and re-exported from the worker package as type
// aliases for caller convenience.
package snapshot

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// ExamplechildConfig holds the user-provided configuration for the child worker.
// Embeds BaseUserSpec to support the StateGetter interface, allowing
// WorkerBase.DeriveDesiredState to extract the desired state from the "state"
// YAML field. The child worker has no additional config fields — its lifecycle
// is driven by the parent's Enabled field.
type ExamplechildConfig struct {
	config.BaseUserSpec `yaml:",inline"`
}

// ExamplechildStatus holds the runtime observation data for the child worker.
// Framework fields (CollectedAt, State, LastActionResults, MetricsEmbedder,
// ShutdownRequested, children counts) are carried by
// fsmv2.Observation[ExamplechildStatus] and are not duplicated here.
type ExamplechildStatus struct {
	ConnectionHealth string `json:"connection_health"`
	ConnectAttempts  int    `json:"connect_attempts"`
}

// ExamplechildDependencies is the dependencies interface for examplechild
// actions (avoids import cycles between action/ and the worker package).
type ExamplechildDependencies interface {
	deps.Dependencies
	SetConnected(connected bool)
	IsConnected() bool
}
