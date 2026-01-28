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

package config

import (
	"fmt"
	"strings"
)

// DesiredStateValues - what users can set in DesiredState.State.
const (
	DesiredStateStopped = "stopped"
	DesiredStateRunning = "running"
)

// LifecyclePrefixes - standardized prefixes for ObservedState.State.
// All observed states use prefix_suffix format where prefix indicates lifecycle phase.
//
// DEPRECATED: Use LifecyclePhase enum instead. These constants remain for
// backwards compatibility during migration.
const (
	PrefixStopped       = "stopped"          // Terminal state (no suffix)
	PrefixTryingToStart = "trying_to_start_" // + worker-specific suffix
	PrefixRunning       = "running_"         // + worker-specific suffix
	PrefixTryingToStop  = "trying_to_stop_"  // + worker-specific suffix
)

// LifecyclePhase represents the lifecycle phase of a worker state.
// This is used by parent supervisors to classify child health without
// knowing implementation details of the child's state machine.
//
// Lifecycle flow:
//
//	                    ┌─────────────────────┐
//	                    │      STOPPED        │
//	                    │   (terminal state)  │
//	                    └──────────┬──────────┘
//	                               │ start
//	                               ▼
//	                    ┌─────────────────────┐
//	                    │      STARTING       │
//	                    │  (trying to start)  │
//	                    └──────────┬──────────┘
//	                               │ ready
//	                               ▼
//	         ┌─────────────────────────────────────────┐
//	         │                RUNNING                   │
//	         │  ┌───────────────┬───────────────┐      │
//	         │  │    HEALTHY    │   DEGRADED    │      │
//	         │  │ (fully stable)│ (with issues) │      │
//	         │  └───────────────┴───────────────┘      │
//	         └─────────────────────┬───────────────────┘
//	                               │ stop
//	                               ▼
//	                    ┌─────────────────────┐
//	                    │      STOPPING       │
//	                    │  (trying to stop)   │
//	                    └──────────┬──────────┘
//	                               │ done
//	                               ▼
//	                    ┌─────────────────────┐
//	                    │      STOPPED        │
//	                    └─────────────────────┘
type LifecyclePhase int

const (
	// PhaseUnknown is the zero value for uninitialized states.
	// Health: UNHEALTHY. Prefix: "unknown_"
	PhaseUnknown LifecyclePhase = iota

	// PhaseStopped: Terminal state - cleanly shut down.
	// Health: NEUTRAL (neither healthy nor unhealthy).
	// Prefix: "stopped"
	PhaseStopped

	// PhaseStarting: Transitioning to running, not yet operational.
	// Health: UNHEALTHY - dependency not satisfied.
	// Prefix: "starting_"
	PhaseStarting

	// PhaseRunningHealthy: Operational AND stable - all good.
	// Health: HEALTHY - dependency fully satisfied.
	// Prefix: "running_healthy_"
	PhaseRunningHealthy

	// PhaseRunningDegraded: Operational but with issues.
	// Health: UNHEALTHY (operational but NOT healthy!).
	// Prefix: "running_degraded_"
	// Use case: Parent has unhealthy children but can still function.
	PhaseRunningDegraded

	// PhaseStopping: Graceful shutdown in progress.
	// Health: UNHEALTHY - dependency being torn down.
	// Prefix: "stopping_"
	PhaseStopping
)

// Prefix returns the string prefix for the observed state name.
// The full observed state name is: Prefix() + lowercase(state.String())
// For PhaseStopped, returns "stopped" with no trailing underscore.
func (p LifecyclePhase) Prefix() string {
	switch p {
	case PhaseStopped:
		return "stopped" // No trailing underscore (no suffix for stopped)
	case PhaseStarting:
		return "starting_"
	case PhaseRunningHealthy:
		return "running_healthy_"
	case PhaseRunningDegraded:
		return "running_degraded_"
	case PhaseStopping:
		return "stopping_"
	default:
		return "unknown_"
	}
}

// String returns the string representation of the lifecycle phase.
func (p LifecyclePhase) String() string {
	switch p {
	case PhaseUnknown:
		return "Unknown"
	case PhaseStopped:
		return "Stopped"
	case PhaseStarting:
		return "Starting"
	case PhaseRunningHealthy:
		return "RunningHealthy"
	case PhaseRunningDegraded:
		return "RunningDegraded"
	case PhaseStopping:
		return "Stopping"
	default:
		return "Unknown"
	}
}

// IsHealthy returns true ONLY for PhaseRunningHealthy.
// Use this for: "Should parent stay in Running state?"
// Note: PhaseRunningDegraded is NOT healthy (even though it IS operational).
func (p LifecyclePhase) IsHealthy() bool {
	return p == PhaseRunningHealthy
}

// IsOperational returns true if the system can serve requests.
// Both RunningHealthy and RunningDegraded are operational.
// Use this for: "Can the system do its job?" (yes, even if impaired)
func (p LifecyclePhase) IsOperational() bool {
	return p == PhaseRunningHealthy || p == PhaseRunningDegraded
}

// IsTransitioning returns true if starting or stopping.
// Transitioning states are NOT operational (yet/anymore).
func (p LifecyclePhase) IsTransitioning() bool {
	return p == PhaseStarting || p == PhaseStopping
}

// IsStopped returns true if cleanly shut down.
func (p LifecyclePhase) IsStopped() bool {
	return p == PhaseStopped
}

// IsDegraded returns true if operational but with issues.
func (p LifecyclePhase) IsDegraded() bool {
	return p == PhaseRunningDegraded
}

// IsValidDesiredState returns true if the state is a valid desired state value.
func IsValidDesiredState(state string) bool {
	return state == DesiredStateStopped || state == DesiredStateRunning
}

// ValidateDesiredState checks that state is "stopped" or "running".
// Returns a user-friendly error with guidance based on likely intent.
func ValidateDesiredState(state string) error {
	if IsValidDesiredState(state) {
		return nil
	}

	var hint string

	switch state {
	case "starting", "active", "connected", "running_connected":
		hint = "Use 'running' for components that should be active."
	case "stopping", "inactive", "stopped_disconnected", "disconnected":
		hint = "Use 'stopped' for components that should be inactive."
	default:
		hint = "Use 'running' for active components or 'stopped' for inactive ones."
	}

	return fmt.Errorf("invalid desired state: only 'stopped' or 'running' are allowed. %s", hint)
}

// GetLifecyclePrefix returns the lifecycle prefix from a state string.
// Examples: "running_connected" → "running_", "stopped" → "stopped".
//
// DEPRECATED: Use LifecyclePhase enum methods instead.
func GetLifecyclePrefix(state string) string {
	if state == PrefixStopped {
		return PrefixStopped
	}

	if strings.HasPrefix(state, PrefixTryingToStart) {
		return PrefixTryingToStart
	}

	if strings.HasPrefix(state, PrefixRunning) {
		return PrefixRunning
	}

	if strings.HasPrefix(state, PrefixTryingToStop) {
		return PrefixTryingToStop
	}

	return "" // Invalid state
}

// IsOperational returns true if the state is in the running_* phase.
func IsOperational(state string) bool {
	return strings.HasPrefix(state, PrefixRunning)
}

// IsStopped returns true if the state is exactly "stopped".
func IsStopped(state string) bool {
	return state == PrefixStopped
}

// IsTransitioning returns true if the state is trying_to_start_* or trying_to_stop_*.
func IsTransitioning(state string) bool {
	return strings.HasPrefix(state, PrefixTryingToStart) ||
		strings.HasPrefix(state, PrefixTryingToStop)
}

// MakeState builds a state string from prefix and suffix.
// Example: MakeState(PrefixRunning, "connected") → "running_connected".
func MakeState(prefix, suffix string) string {
	if prefix == PrefixStopped {
		return PrefixStopped
	}

	return prefix + suffix
}
