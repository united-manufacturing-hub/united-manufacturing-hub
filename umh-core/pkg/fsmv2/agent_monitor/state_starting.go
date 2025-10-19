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

package agent_monitor

import "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"

// StartingState represents the transition from stopped to active monitoring.
// This is a PASSIVE state - no actual startup work needed for monitoring.
type StartingState struct{}

// Next evaluates the snapshot and returns the next transition.
func (s *StartingState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
	desired := snapshot.Desired

	// ALWAYS check shutdown first
	if desired.ShutdownRequested() {
		// Transition to stopping
		return &StoppingState{}, fsmv2.SignalNone, nil
	}

	// Agent monitoring always starts in degraded state
	// (health must be verified before transitioning to active)
	return &DegradedState{}, fsmv2.SignalNone, nil
}

// String returns the state name for logging/debugging.
func (s *StartingState) String() string {
	return "Starting"
}

// Reason provides context for why we're in this state.
func (s *StartingState) Reason() string {
	return "Initializing monitoring"
}
