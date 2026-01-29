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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplepanic/action"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplepanic/snapshot"
)

type TryingToConnectState struct {
	BaseExamplepanicState
}

func (s *TryingToConnectState) LifecyclePhase() config.LifecyclePhase {
	return config.PhaseStarting
}

func (s *TryingToConnectState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := helpers.ConvertSnapshot[snapshot.ExamplepanicObservedState, *snapshot.ExamplepanicDesiredState](snapAny)

	if snap.Observed.IsStopRequired() {
		return fsmv2.Result[any, any](&TryingToStopState{}, fsmv2.SignalNone, nil, "Stop required, transitioning to TryingToStop")
	}

	if snap.Observed.ConnectionHealth == "healthy" {
		return fsmv2.Result[any, any](&ConnectedState{}, fsmv2.SignalNone, nil, "Connection healthy, transitioning to Connected")
	}

	return fsmv2.Result[any, any](s, fsmv2.SignalNone, &action.ConnectAction{}, "Attempting to establish connection")
}

func (s *TryingToConnectState) String() string {
	return helpers.DeriveStateName(s)
}
