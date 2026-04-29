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
	transport_pkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
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
	snap := fsmv2.ConvertWorkerSnapshot[transport_pkg.TransportConfig, transport_pkg.TransportStatus](snapAny)

	if snap.Desired.IsShutdownRequested() {
		return fsmv2.Transition(&StoppingState{}, fsmv2.SignalNone, nil, "Shutdown requested, transitioning to Stopping", nil)
	}

	children := transport_pkg.RenderChildren(snap)

	// FailedAuthConfig is guaranteed populated here because only permanent errors
	// (which call SetFailedAuthConfig via the !IsTransient() guard in authenticate.go)
	// reach AuthFailedState via isPermanentAuthError().
	tokenChanged := snap.Desired.Config.AuthToken != snap.Observed.Status.FailedAuthConfig.AuthToken
	relayChanged := snap.Desired.Config.RelayURL != snap.Observed.Status.FailedAuthConfig.RelayURL
	uuidChanged := snap.Desired.Config.InstanceUUID != snap.Observed.Status.FailedAuthConfig.InstanceUUID

	if tokenChanged || relayChanged || uuidChanged {
		return fsmv2.Transition(&StartingState{}, fsmv2.SignalNone, nil,
			fmt.Sprintf("config changed (token=%t, relay=%t, uuid=%t), retrying auth",
				tokenChanged, relayChanged, uuidChanged), children)
	}

	return fsmv2.Transition(s, fsmv2.SignalNone, nil,
		fmt.Sprintf("auth failed (%s), waiting for config change",
			snap.Observed.Status.LastErrorType), children)
}

// String returns the state name derived from the type.
func (s *AuthFailedState) String() string {
	return helpers.DeriveStateName(s)
}
