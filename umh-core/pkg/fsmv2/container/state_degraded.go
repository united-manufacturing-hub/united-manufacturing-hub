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

package container

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
)

// DegradedState represents operation with unhealthy metrics or stale data.
// This is a PASSIVE state - it waits for metrics to recover.
type DegradedState struct {
	reason string // Why we're degraded
}

// Next evaluates the snapshot and returns the next transition.
func (s *DegradedState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
	desired := snapshot.Desired
	observed := snapshot.Observed

	// ALWAYS check shutdown first
	if desired.ShutdownRequested() {
		return &StoppedState{}, fsmv2.SignalNone, nil
	}

	// Check if metrics recovered
	containerObserved := observed.(*ContainerObservedState)
	if IsFullyHealthy(containerObserved) {
		// Metrics recovered, transition back to active
		return &ActiveState{}, fsmv2.SignalNone, nil
	}

	// Still unhealthy, stay degraded (passive - waiting for recovery)
	return s, fsmv2.SignalNone, nil
}

// String returns the state name for logging/debugging.
func (s *DegradedState) String() string {
	return "Degraded"
}

// Reason provides context for why we're in this state.
func (s *DegradedState) Reason() string {
	if s.reason != "" {
		return "Monitoring degraded: " + s.reason
	}

	return "Monitoring degraded"
}
