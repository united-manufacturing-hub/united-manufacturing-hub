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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplepanic/snapshot"
)

type DisconnectedState struct {
	BaseExamplepanicState
}

func (s *DisconnectedState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
	snap := helpers.ConvertSnapshot[snapshot.ExamplepanicObservedState, *snapshot.ExamplepanicDesiredState](snapAny)
	snap.Observed.State = config.MakeState(config.PrefixRunning, "disconnected")

	// ParentMappedState is in Observed.DesiredState, not Desired
	if snap.Observed.IsStopRequired() {
		return &TryingToStopState{}, fsmv2.SignalNone, nil
	}

	if snap.Observed.ShouldBeRunning() {
		return &TryingToConnectState{}, fsmv2.SignalNone, nil
	}

	return s, fsmv2.SignalNone, nil
}

func (s *DisconnectedState) String() string {
	return helpers.DeriveStateName(s)
}

func (s *DisconnectedState) Reason() string {
	return "Connection lost, will retry"
}
