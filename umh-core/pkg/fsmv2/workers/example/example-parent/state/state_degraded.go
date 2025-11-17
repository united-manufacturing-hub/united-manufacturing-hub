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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-parent/snapshot"
)

// DegradedState represents the state when some children have failed
type DegradedState struct {
	BaseParentState
}

func NewDegradedState() *DegradedState {
	return &DegradedState{}
}

func (s *DegradedState) Next(snap fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
	parentSnap := snapshot.ParentSnapshot{
		Identity: snap.Identity,
		Observed: snap.Observed.(snapshot.ParentObservedState),
		Desired:  *snap.Desired.(*snapshot.ParentDesiredState),
	}

	if parentSnap.Desired.IsShutdownRequested() {
		return NewTryingToStopState(), fsmv2.SignalNone, nil
	}

	if parentSnap.Observed.ChildrenUnhealthy == 0 {
		return NewRunningState(), fsmv2.SignalNone, nil
	}

	return s, fsmv2.SignalNone, nil
}

func (s *DegradedState) String() string {
	return "Degraded"
}

func (s *DegradedState) Reason() string {
	return "Some children are unhealthy"
}
