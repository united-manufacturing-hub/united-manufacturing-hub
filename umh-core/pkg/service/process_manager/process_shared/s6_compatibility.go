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

package process_shared

// IPMToS6StateMapping provides compatibility between IPM ServiceStatus and S6 operational states
// This allows existing FSM logic to work unchanged when using IPM instead of S6

// S6 operational state constants - these match the values in pkg/fsm/s6/models.go
const (
	S6OperationalStateStopped  = "stopped"
	S6OperationalStateStarting = "starting"
	S6OperationalStateRunning  = "running"
	S6OperationalStateStopping = "stopping"
	S6OperationalStateUnknown  = "unknown"
)

// MapIPMStatusToS6State converts IPM ServiceStatus to equivalent S6 operational state
// This enables existing S6-based FSM logic to work with IPM without code changes
func MapIPMStatusToS6State(status ServiceStatus) string {
	switch status {
	case ServiceUp:
		return S6OperationalStateRunning
	case ServiceDown:
		return S6OperationalStateStopped
	case ServiceRestarting:
		return S6OperationalStateStarting
	case ServiceUnknown:
		return S6OperationalStateUnknown
	default:
		return S6OperationalStateUnknown
	}
}

// MapS6StateToIPMStatus converts S6 operational state back to IPM ServiceStatus
// This can be useful for reverse compatibility or testing scenarios
func MapS6StateToIPMStatus(s6State string) ServiceStatus {
	switch s6State {
	case S6OperationalStateRunning:
		return ServiceUp
	case S6OperationalStateStopped:
		return ServiceDown
	case S6OperationalStateStarting:
		return ServiceRestarting
	case S6OperationalStateStopping:
		return ServiceDown // IPM doesn't have a distinct "stopping" state
	case S6OperationalStateUnknown:
		return ServiceUnknown
	default:
		return ServiceUnknown
	}
}

// IsCompatibleTransition checks if a state transition is valid for both S6 and IPM
// This helps ensure consistent behavior across both process managers
func IsCompatibleTransition(fromState, toState string) bool {
	// Valid S6/IPM state transitions
	validTransitions := map[string][]string{
		S6OperationalStateStopped:  {S6OperationalStateStarting},
		S6OperationalStateStarting: {S6OperationalStateRunning, S6OperationalStateStopped, S6OperationalStateUnknown},
		S6OperationalStateRunning:  {S6OperationalStateStopping, S6OperationalStateUnknown},
		S6OperationalStateStopping: {S6OperationalStateStopped},
		S6OperationalStateUnknown:  {S6OperationalStateStopped, S6OperationalStateStarting, S6OperationalStateRunning},
	}

	allowedStates, exists := validTransitions[fromState]
	if !exists {
		return false
	}

	for _, allowedState := range allowedStates {
		if allowedState == toState {
			return true
		}
	}
	return false
}
