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

package state

import (
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/action"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/snapshot"
)

// DegradedState represents the state when some children have failed.
// The transport is still operational but not fully healthy.
type DegradedState struct {
	helpers.RunningDegradedBase
}

// Next evaluates the current snapshot and returns the next state or action.
func (s *DegradedState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := helpers.ConvertSnapshot[snapshot.TransportObservedState, *snapshot.TransportDesiredState](snapAny)

	if snap.Desired.IsShutdownRequested() {
		return fsmv2.Result[any, any](&StoppingState{}, fsmv2.SignalNone, nil, "Shutdown requested, transitioning to Stopping")
	}

	// If token is expired, need to re-authenticate (mirrors RunningState)
	if snap.Observed.IsTokenExpired() {
		return fsmv2.Result[any, any](&StartingState{}, fsmv2.SignalNone, nil, "Token expired, transitioning to Starting for re-authentication")
	}

	// Nuclear fallback: reset transport on prolonged child failures
	if backoff.ShouldResetTransport(snap.Observed.LastErrorType, snap.Observed.ConsecutiveErrors) {
		return fsmv2.Result[any, any](s, fsmv2.SignalNone, action.NewResetTransportAction(),
			fmt.Sprintf("resetting transport: %d consecutive errors (type=%d)",
				snap.Observed.ConsecutiveErrors, snap.Observed.LastErrorType))
	}

	// If all children are now healthy, transition back to Running
	if snap.Observed.ChildrenUnhealthy == 0 {
		return fsmv2.Result[any, any](&RunningState{}, fsmv2.SignalNone, nil, "All children now healthy, transitioning to Running")
	}

	return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil,
		fmt.Sprintf("degraded: %d unhealthy children, %d consecutive errors",
			snap.Observed.ChildrenUnhealthy, snap.Observed.ConsecutiveErrors))
}

// String returns the state name derived from the type.
func (s *DegradedState) String() string {
	return helpers.DeriveStateName(s)
}
