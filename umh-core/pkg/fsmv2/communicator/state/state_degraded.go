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
)

type DegradedState struct {
	BaseCommunicatorState
}

func (s *DegradedState) Next(snapshot snapshot.CommunicatorSnapshot) (BaseCommunicatorState, fsmv2.Signal, fsmv2.Action) {
	desired := snapshot.Desired
	observed := snapshot.Observed

	if desired.ShutdownRequested() {
		return &StoppedState{}, fsmv2.SignalNone, nil
	}

	if observed.IsSyncHealthy() && observed.GetConsecutiveErrors() == 0 {
		return &SyncingState{}, fsmv2.SignalNone, nil
	}

	// Create SyncAction with transport and channels (retry sync in degraded state)
	syncAction := action.NewSyncAction(desired.Transport, observed.JWTToken)
	return s, fsmv2.SignalNone, syncAction
}

func (s *DegradedState) String() string {
	return "Degraded"
}

func (s *DegradedState) Reason() string {
	return "Sync is experiencing errors"
}
