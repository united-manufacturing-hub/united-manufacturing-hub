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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/communicator/action"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/communicator/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/communicator/transport"
)

type SyncingState struct {
}

func (s *SyncingState) Next(snapshot snapshot.CommunicatorSnapshot) (BaseCommunicatorState, fsmv2.Signal, fsmv2.Action) {
	desired := snapshot.Desired
	observed := snapshot.Observed

	if desired.ShutdownRequested() {
		return &StoppedState{}, fsmv2.SignalNone, nil
	}

	if observed.IsTokenExpired() {
		return &TryingToAuthenticateState{}, fsmv2.SignalNone, nil
	}

	if !observed.Authenticated {
		return &TryingToAuthenticateState{}, fsmv2.SignalNone, nil
	}

	if !observed.IsSyncHealthy() {
		return &DegradedState{}, fsmv2.SignalNone, nil
	}

	syncAction := action.NewSyncAction(
		transport.Transport{}, observed.JWTToken)
	return s, fsmv2.SignalNone, syncAction
}

func (s *SyncingState) String() string {
	return "Syncing"
}

func (s *SyncingState) Reason() string {
	return "Syncing with relay server"
}
