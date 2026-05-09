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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/snapshot"
)

// AuthFailedState represents a permanent authentication failure (InvalidToken or
// InstanceDeleted). The worker dispatches no actions and waits for a configuration
// change (AuthToken, RelayURL, or InstanceUUID) before transitioning back to
// StartingState for a fresh attempt.
//
// Uses StartingBase because the worker has not reached Running -- children remain
// stopped (they only start when the parent enters RunningHealthy).
type AuthFailedState struct {
	helpers.StartingBase
}

// Next evaluates the current snapshot and returns the next state or action.
func (s *AuthFailedState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := helpers.ConvertSnapshot[snapshot.TransportObservedState, *snapshot.TransportDesiredState](snapAny)

	if snap.Desired.IsShutdownRequested() {
		return fsmv2.Result[any, any](&StoppingState{}, fsmv2.SignalNone, nil, "Shutdown requested, transitioning to Stopping")
	}

	// FailedAuthConfig is guaranteed populated here because only permanent errors
	// (which call SetFailedAuthConfig via the !IsTransient() guard in authenticate.go)
	// reach AuthFailedState via isPermanentAuthError().
	tokenChanged := snap.Desired.AuthToken != snap.Observed.FailedAuthConfig.AuthToken
	relayChanged := snap.Desired.RelayURL != snap.Observed.FailedAuthConfig.RelayURL
	uuidChanged := snap.Desired.InstanceUUID != snap.Observed.FailedAuthConfig.InstanceUUID

	if tokenChanged || relayChanged || uuidChanged {
		return fsmv2.Result[any, any](&StartingState{}, fsmv2.SignalNone, nil,
			fmt.Sprintf("config changed (token=%t, relay=%t, uuid=%t), retrying auth",
				tokenChanged, relayChanged, uuidChanged))
	}

	return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil,
		fmt.Sprintf("auth failed (%s), waiting for config change",
			snap.Observed.LastErrorType))
}

// String returns the state name derived from the type.
func (s *AuthFailedState) String() string {
	return helpers.DeriveStateName(s)
}
