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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator"
)

// SyncingState is the primary operational state for the communicator orchestrator.
// TransportWorker (child) handles authentication, push, and pull operations.
// CommunicatorWorker monitors child health and transitions to RecoveringState
// when children report unhealthy.
//
// Transitions:
//   - → RecoveringState: when ChildrenUnhealthy > 0
//   - → StoppedState: if shutdown requested
//   - → self: children healthy, continue orchestration
type SyncingState struct {
	helpers.RunningHealthyBase
}

func (s *SyncingState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[communicator.CommunicatorConfig, communicator.CommunicatorStatus](snapAny)

	if snap.Desired.IsShutdownRequested() {
		return fsmv2.Transition(&StoppedState{}, fsmv2.SignalNone, nil, "Shutdown requested during sync", nil)
	}

	children := communicator.RenderChildren(snap)

	if snap.Observed.ChildrenHealthy == 0 || snap.Observed.ChildrenUnhealthy > 0 {
		return fsmv2.Transition(&RecoveringState{}, fsmv2.SignalNone, nil,
			fmt.Sprintf("children unhealthy: healthy=%d, unhealthy=%d",
				snap.Observed.ChildrenHealthy, snap.Observed.ChildrenUnhealthy), children)
	}

	return fsmv2.Transition(s, fsmv2.SignalNone, nil,
		fmt.Sprintf("syncing: healthy=%d, unhealthy=%d",
			snap.Observed.ChildrenHealthy, snap.Observed.ChildrenUnhealthy), children)
}

func (s *SyncingState) String() string {
	return "Syncing"
}
