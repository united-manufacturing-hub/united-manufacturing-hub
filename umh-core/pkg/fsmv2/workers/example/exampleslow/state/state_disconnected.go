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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleslow/snapshot"
)

type DisconnectedState struct {
	helpers.RunningDegradedBase
}

func (s *DisconnectedState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := helpers.ConvertSnapshot[snapshot.ExampleslowObservedState, *snapshot.ExampleslowDesiredState](snapAny)

	// ParentMappedState is injected into Observed.DesiredState by collector.
	if snap.Observed.IsStopRequired() {
		return fsmv2.Transition(&TryingToStopState{}, fsmv2.SignalNone, nil, "stop required, transitioning to trying to stop")
	}

	if snap.Observed.ShouldBeRunning() {
		return fsmv2.Transition(&TryingToConnectState{}, fsmv2.SignalNone, nil, "should be running, attempting reconnection")
	}

	return fsmv2.Transition(s, fsmv2.SignalNone, nil, "connection lost, will retry")
}

func (s *DisconnectedState) String() string {
	return helpers.DeriveStateName(s)
}
