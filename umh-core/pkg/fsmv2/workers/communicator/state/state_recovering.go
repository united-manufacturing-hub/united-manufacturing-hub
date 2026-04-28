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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/snapshot"
)

// RecoveringState monitors child health and transitions back to SyncingState
// when children recover. Error handling, backoff, and transport reset are
// internal to TransportWorker and its Push/Pull children (ENG-4264).
type RecoveringState struct {
	helpers.RunningDegradedBase
}

func (s *RecoveringState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := helpers.ConvertSnapshot[snapshot.CommunicatorObservedState, *snapshot.CommunicatorDesiredState](snapAny)

	if snap.Desired.IsShutdownRequested() {
		return fsmv2.Result[any, any](&StoppedState{}, fsmv2.SignalNone, nil, "Shutdown requested during recovering state", nil)
	}

	if snap.Observed.IsSyncHealthy() {
		return fsmv2.Result[any, any](&SyncingState{}, fsmv2.SignalNone, nil,
			fmt.Sprintf("recovered: healthy=%d, unhealthy=%d",
				snap.Observed.ChildrenHealthy, snap.Observed.ChildrenUnhealthy), nil)
	}

	return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil,
		fmt.Sprintf("recovering: healthy=%d, unhealthy=%d",
			snap.Observed.ChildrenHealthy, snap.Observed.ChildrenUnhealthy), nil)
}

func (s *RecoveringState) String() string {
	return "Recovering"
}
