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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/action"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/snapshot"
)

// StartingState represents the state where the transport worker is authenticating.
// It emits AuthenticateAction to obtain a JWT token from the relay server.
// Once authenticated and children are healthy, transitions to RunningState.
type StartingState struct {
	helpers.StartingBase
}

// Next evaluates the current snapshot and returns the next state or action.
func (s *StartingState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := helpers.ConvertSnapshot[snapshot.TransportObservedState, *snapshot.TransportDesiredState](snapAny)

	if snap.Desired.IsShutdownRequested() {
		return fsmv2.Result[any, any](&StoppingState{}, fsmv2.SignalNone, nil, "Shutdown requested, transitioning to Stopping")
	}

	// If we don't have a valid token, authenticate
	if !snap.Observed.HasValidToken() {
		authAction := action.NewAuthenticateAction(
			snap.Desired.RelayURL,
			snap.Desired.InstanceUUID,
			snap.Desired.AuthToken,
			snap.Desired.Timeout,
		)

		return fsmv2.Result[any, any](s, fsmv2.SignalNone, authAction, "No valid token, authenticating with relay")
	}

	// Requires healthy children (PushWorker + PullWorker) — added in ENG-4262, ENG-4263.
	// Zero children: stays in Starting. This is intentional — avoids reporting Running
	// without actual push/pull capability.
	if snap.Observed.ChildrenHealthy > 0 && snap.Observed.ChildrenUnhealthy == 0 {
		return fsmv2.Result[any, any](&RunningState{}, fsmv2.SignalNone, nil, "All children healthy, transitioning to Running")
	}

	return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil, "Token valid, waiting for children to become healthy")
}

// String returns the state name derived from the type.
func (s *StartingState) String() string {
	return helpers.DeriveStateName(s)
}
