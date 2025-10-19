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

// StoppedState represents the state when monitoring is not active.
// This is a PASSIVE state - it only observes and transitions based on conditions.
type StoppedState struct{}

// Next evaluates the snapshot and returns the next transition.
func (s *StoppedState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
	desired := snapshot.Desired

	// ALWAYS check shutdown first
	if desired.ShutdownRequested() {
		// Already stopped, signal removal
		return s, fsmv2.SignalNeedsRemoval, nil
	}

	// Agent monitoring is always active (no explicit "enabled" config)
	// Transition from stopped to starting
	return &StartingState{}, fsmv2.SignalNone, nil
}

// String returns the state name for logging/debugging.
func (s *StoppedState) String() string {
	return "Stopped"
}

// Reason provides context for why we're in this state.
func (s *StoppedState) Reason() string {
	return "Monitoring is not active"
}
