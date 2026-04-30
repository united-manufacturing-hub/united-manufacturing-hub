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
	transport_pkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
)

// DegradedState represents the state when some children have failed.
// The transport is still operational but not fully healthy.
type DegradedState struct {
	helpers.RunningDegradedBase
}

// Next evaluates the current snapshot and returns the next state or action.
func (s *DegradedState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[transport_pkg.TransportConfig, transport_pkg.TransportStatus](snapAny)

	if snap.Desired.IsShutdownRequested() {
		return fsmv2.Transition(&StoppingState{}, fsmv2.SignalNone, nil, "Shutdown requested, transitioning to Stopping", nil)
	}

	children := transport_pkg.RenderChildren(snap)

	// If token is expired, need to re-authenticate (mirrors RunningState)
	if snap.Observed.Status.IsTokenExpired() {
		return fsmv2.Transition(&StartingState{}, fsmv2.SignalNone, nil, "Token expired, transitioning to Starting for re-authentication", children)
	}

	// Nuclear fallback: reset transport on prolonged child failures
	if backoff.ShouldResetTransport(snap.Observed.Status.LastErrorType, snap.Observed.Status.ConsecutiveErrors) {
		return fsmv2.Transition(s, fsmv2.SignalNone, action.NewResetTransportAction(),
			fmt.Sprintf("resetting transport: %d consecutive errors (type=%d)",
				snap.Observed.Status.ConsecutiveErrors, snap.Observed.Status.LastErrorType), children)
	}

	// If all children are now healthy, transition back to Running
	if snap.Observed.ChildrenUnhealthy == 0 {
		return fsmv2.Transition(&RunningState{}, fsmv2.SignalNone, nil, "All children now healthy, transitioning to Running", children)
	}

	return fsmv2.Transition(s, fsmv2.SignalNone, nil,
		fmt.Sprintf("degraded: %d unhealthy children, %d consecutive errors",
			snap.Observed.ChildrenUnhealthy, snap.Observed.Status.ConsecutiveErrors), children)
}

// String returns the state name derived from the type.
func (s *DegradedState) String() string {
	return helpers.DeriveStateName(s)
}
