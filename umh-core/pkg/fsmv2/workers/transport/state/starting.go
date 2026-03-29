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
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/action"
	transport_pkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
)

// StartingState represents the state where the transport worker is authenticating.
// It emits AuthenticateAction to obtain a JWT token from the relay server.
// Once authenticated, transitions to RunningState. Children start via ChildStartStates
// when the parent enters Running, avoiding a deadlock where children can't become healthy
// while the parent waits in Starting.
type StartingState struct {
	helpers.StartingBase
}

// Next evaluates the current snapshot and returns the next state or action.
func (s *StartingState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[transport_pkg.TransportConfig, transport_pkg.TransportStatus](snapAny)

	if snap.IsShutdownRequested {
		return fsmv2.Result[any, any](&StoppingState{}, fsmv2.SignalNone, nil, "Shutdown requested, transitioning to Stopping")
	}

	// If we don't have a valid token, authenticate (with backoff on repeated failures)
	if !snap.Status.HasValidToken() {
		if snap.Status.ConsecutiveErrors > 0 && !snap.Status.LastAuthAttemptAt.IsZero() {
			delay := backoff.CalculateDelayForErrorType(
				snap.Status.LastErrorType,
				snap.Status.ConsecutiveErrors,
				snap.Status.LastRetryAfter,
			)
			if time.Since(snap.Status.LastAuthAttemptAt) < delay {
				return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil,
					fmt.Sprintf("auth backoff: %d errors (%s), delay %s",
						snap.Status.ConsecutiveErrors, snap.Status.LastErrorType, delay.Round(time.Second)))
			}
		}

		authAction := action.NewAuthenticateAction(
			snap.Config.RelayURL,
			snap.Config.InstanceUUID,
			snap.Config.AuthToken,
			snap.Config.Timeout,
		)

		return fsmv2.Result[any, any](s, fsmv2.SignalNone, authAction, "No valid token, authenticating with relay")
	}

	// Authenticated — transition to Running. Children start via ChildStartStates
	// once parent enters Running; RunningState handles unhealthy children.
	return fsmv2.Result[any, any](&RunningState{}, fsmv2.SignalNone, nil, "Authenticated, transitioning to Running")
}

// String returns the state name derived from the type.
func (s *StartingState) String() string {
	return helpers.DeriveStateName(s)
}
