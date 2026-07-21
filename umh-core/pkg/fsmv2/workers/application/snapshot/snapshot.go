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

// Package snapshot holds the application worker's Config and Status value
// types as a leaf package so state/ can depend on them without an import
// cycle through the worker package.
package snapshot

import (
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// ApplicationConfig holds the user-provided configuration for the application
// supervisor. It carries the supervisor's logical Name; ChildrenSpecs flow
// directly through WrappedDesiredState (populated by the worker's custom
// DeriveDesiredState from the children: YAML block) rather than living on
// TConfig, since the application worker's passthrough model already handles
// them separately from user config.
type ApplicationConfig struct {
	config.BaseUserSpec `yaml:",inline"`

	// Name is the identifier for this application supervisor.
	Name string `json:"name" yaml:"name"`
}

// ApplicationStatus holds the runtime observation data for the application
// supervisor. Framework fields (CollectedAt, State, LastActionResults,
// MetricsEmbedder, ShutdownRequested, ChildrenHealthy, ChildrenUnhealthy,
// ChildrenView) are carried by fsmv2.Observation[ApplicationStatus] and are
// not duplicated here.
//
// ChildrenCircuitOpen and ChildrenStale are populated by the state layer from
// the ChildrenView provided by the collector, so they can be inspected during
// State.Next() for health-based transitions.
type ApplicationStatus struct {
	// ID is the identifier of this application supervisor instance.
	ID string `json:"id"`
	// Name mirrors ApplicationConfig.Name for observability.
	Name string `json:"name"`
	// DynamicChildren carries the full child specs recorded in the shared
	// registry that the application control surface reads to spawn dynamic
	// children. Each spec carries Name, WorkerType, UserSpec.Config, and Enabled
	// so a later step can emit it as a spawnable child from Next(), which only
	// sees the snapshot; a Ref (WorkerType+Name only) cannot be spawned.
	DynamicChildren []config.ChildSpec `json:"dynamic_children"`
	// ChildrenCircuitOpen is the count of children with circuit breaker open.
	ChildrenCircuitOpen int `json:"children_circuit_open"`
	// ChildrenStale is the count of children whose observations are older than
	// the stale threshold.
	ChildrenStale int `json:"children_stale"`
	// RegistryConfigured is true when a non-nil shared registry is published
	// under the application worker type.
	RegistryConfigured bool `json:"registry_configured"`
}

// HasInfrastructureIssues returns true if any children have circuit breaker
// open or stale observations.
func (s ApplicationStatus) HasInfrastructureIssues() bool {
	return s.ChildrenCircuitOpen > 0 || s.ChildrenStale > 0
}

// InfrastructureReason returns a dynamic reason string for infrastructure
// issues. Returns an empty string when there are no issues.
func (s ApplicationStatus) InfrastructureReason() string {
	if !s.HasInfrastructureIssues() {
		return ""
	}

	return fmt.Sprintf("infrastructure degraded: circuit_open=%d, stale=%d",
		s.ChildrenCircuitOpen, s.ChildrenStale)
}

// ChildrenViewToStatus derives ChildrenCircuitOpen and ChildrenStale counts
// from a ChildrenView. Returns zeroed counts if the view is not the expected
// type. Callers pass the ChildrenView from WorkerSnapshot so state transitions
// can observe per-tick infrastructure health without mutating ObservedState.
func ChildrenViewToStatus(view any) (circuitOpen, stale int) {
	cv, ok := view.(config.ChildrenView)
	if !ok {
		return 0, 0
	}

	for _, child := range cv.Children {
		if child.IsCircuitOpen {
			circuitOpen++
		}

		if child.IsStale {
			stale++
		}
	}

	return circuitOpen, stale
}
