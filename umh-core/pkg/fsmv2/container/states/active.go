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

package states

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
)

// ActiveState represents normal operation with healthy metrics.
// This is a PASSIVE state - it only observes metrics and transitions if unhealthy.
type ActiveState struct{}

// Next evaluates the snapshot and returns the next transition.
func (s *ActiveState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
	desired := snapshot.Desired
	observed := snapshot.Observed

	// ALWAYS check shutdown first
	if desired.ShutdownRequested() {
		return &StoppingState{}, fsmv2.SignalNone, nil
	}

	// Check if observed state is stale (no updates in 30 seconds)
	if time.Since(observed.GetTimestamp()) > 30*time.Second {
		return &DegradedState{reason: "stale metrics data"}, fsmv2.SignalNone, nil
	}

	// Type assert to access container-specific health check
	// TODO: Consider moving health check to a more generic interface
	type healthChecker interface {
		IsHealthy() bool
	}

	if checker, ok := observed.(healthChecker); ok {
		if !checker.IsHealthy() {
			return &DegradedState{reason: "metrics unhealthy"}, fsmv2.SignalNone, nil
		}
	}

	// Stay in active state (passive - no action needed)
	return s, fsmv2.SignalNone, nil
}

// String returns the state name for logging/debugging.
func (s *ActiveState) String() string {
	return "Active"
}

// Reason provides context for why we're in this state.
func (s *ActiveState) Reason() string {
	return "Monitoring active with healthy metrics"
}
