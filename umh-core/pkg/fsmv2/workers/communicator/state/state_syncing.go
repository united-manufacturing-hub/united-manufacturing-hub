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
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/action"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/snapshot"
)

// SyncingState is the primary operational state for bidirectional message sync.
// Emits SyncAction continuously while healthy and authenticated.
//
// Transitions:
//   - → TryingToAuthenticateState: on token expiry or auth loss
//   - → DegradedState: when !IsSyncHealthy()
//   - → StoppedState: if shutdown requested
//   - → self: continuous sync loop (C5)
//
// Enforces C2 (token expiry), C4 (shutdown priority), C5 (syncing loop).
type SyncingState struct {
}

func (s *SyncingState) LifecyclePhase() config.LifecyclePhase {
	return config.PhaseRunningHealthy
}

func (s *SyncingState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := helpers.ConvertSnapshot[snapshot.CommunicatorObservedState, *snapshot.CommunicatorDesiredState](snapAny)

	if snap.Desired.IsShutdownRequested() {
		return fsmv2.Result[any, any](&StoppedState{}, fsmv2.SignalNone, nil, "Shutdown requested during sync")
	}

	if snap.Observed.IsTokenExpired() {
		return fsmv2.Result[any, any](&TryingToAuthenticateState{}, fsmv2.SignalNone, nil, "Token expired, re-authenticating")
	}

	if !snap.Observed.Authenticated {
		return fsmv2.Result[any, any](&TryingToAuthenticateState{}, fsmv2.SignalNone, nil, "Not authenticated, re-authenticating")
	}

	if !snap.Observed.IsSyncHealthy() {
		return fsmv2.Result[any, any](&DegradedState{}, fsmv2.SignalNone, nil, "Sync health check failed")
	}

	syncAction := action.NewSyncAction(snap.Observed.JWTToken)

	return fsmv2.Result[any, any](s, fsmv2.SignalNone, syncAction, "Syncing with relay server")
}

func (s *SyncingState) String() string {
	return "Syncing"
}

// GetBackoffDelay calculates exponential backoff based on consecutive errors.
func (s *SyncingState) GetBackoffDelay(observed snapshot.CommunicatorObservedState) time.Duration {
	return backoff.CalculateDelay(observed.ConsecutiveErrors)
}
