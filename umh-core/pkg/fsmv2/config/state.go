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
const (
	PrefixStopped       = "stopped"           // Terminal state (no suffix)
	PrefixTryingToStart = "trying_to_start_" // + worker-specific suffix
	PrefixRunning       = "running_"          // + worker-specific suffix
	PrefixTryingToStop  = "trying_to_stop_"   // + worker-specific suffix
)

// IsValidDesiredState returns true if the state is a valid desired state value.
func IsValidDesiredState(state string) bool {
	return state == DesiredStateStopped || state == DesiredStateRunning
}

// ValidateDesiredState checks that state is "stopped" or "running".
// Returns a user-friendly error per UX_STANDARDS.md Error Excellence.
//
// This validation runs at runtime when DeriveDesiredState returns, catching:
//   - Developer mistakes (hardcoded wrong values in worker.go)
//   - User configuration mistakes (wrong state: value in YAML config)
func ValidateDesiredState(state string) error {
	if IsValidDesiredState(state) {
		return nil
	}

	// Determine likely intent for actionable guidance
	var hint string
	switch state {
	case "starting", "active", "connected", "running_connected":
		hint = "Use 'running' for components that should be active."
	case "stopping", "inactive", "stopped_disconnected", "disconnected":
		hint = "Use 'stopped' for components that should be inactive."
	default:
		hint = "Use 'running' for active components or 'stopped' for inactive ones."
	}

	return fmt.Errorf("invalid desired state '%s' - only 'stopped' or 'running' are allowed. %s", state, hint)
}

// GetLifecyclePhase returns the lifecycle prefix from a state.
// Examples: "running_connected" → "running_", "stopped" → "stopped".
func GetLifecyclePhase(state string) string {
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
