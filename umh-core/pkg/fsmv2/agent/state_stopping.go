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

package agent

import "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"

// StoppingState represents the transition from active/degraded to stopped monitoring.
// This is a PASSIVE state - no actual shutdown work needed for monitoring.
type StoppingState struct{}

// Next evaluates the snapshot and returns the next transition.
func (s *StoppingState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
	// For agent monitoring, stopping immediately goes to stopped
	// (No actual shutdown process - monitoring just stops)
	return &StoppedState{}, fsmv2.SignalNone, nil
}

// String returns the state name for logging/debugging.
func (s *StoppingState) String() string {
	return "Stopping"
}

// Reason provides context for why we're in this state.
func (s *StoppingState) Reason() string {
	return "Stopping monitoring"
}
