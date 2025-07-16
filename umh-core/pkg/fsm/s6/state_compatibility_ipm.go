//go:build internal_process_manager
// +build internal_process_manager

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

package s6

import (
	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/process_shared"
)

// GetCurrentFSMState returns the current FSM state with IPM compatibility
// When using IPM, this maps the underlying ServiceInfo.Status to S6 operational states
func (s *S6Instance) GetCurrentFSMState() string {
	currentState := s.baseFSMInstance.GetCurrentFSMState()

	// For lifecycle states, return as-is since they're universal across S6 and IPM
	if internalfsm.IsLifecycleState(currentState) {
		return currentState
	}

	// For operational states, use IPM's ServiceInfo.Status and map it to S6 states
	// This ensures all existing FSM logic continues to work unchanged
	if s.ObservedState.ServiceInfo.Status != "" {
		mappedState := process_shared.MapIPMStatusToS6State(s.ObservedState.ServiceInfo.Status)
		s.baseFSMInstance.GetLogger().Debugf("IPM compatibility: mapped ServiceInfo.Status '%s' to S6 state '%s' for instance %s",
			s.ObservedState.ServiceInfo.Status, mappedState, s.baseFSMInstance.GetID())
		return mappedState
	}

	// Fallback to base FSM state if no ServiceInfo available
	return currentState
}
