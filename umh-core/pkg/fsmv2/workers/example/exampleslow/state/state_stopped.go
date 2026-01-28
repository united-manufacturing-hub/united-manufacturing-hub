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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleslow/snapshot"
)

type StoppedState struct {
	BaseExampleslowState
}

func (s *StoppedState) LifecyclePhase() config.LifecyclePhase {
	return config.PhaseStopped
}

func (s *StoppedState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := helpers.ConvertSnapshot[snapshot.ExampleslowObservedState, *snapshot.ExampleslowDesiredState](snapAny)

	if snap.Desired.IsShutdownRequested() {
		return fsmv2.Result[any, any](s, fsmv2.SignalNeedsRemoval, nil, "shutdown requested, needs removal")
	}

	// ParentMappedState is injected into Observed.DesiredState by collector.
	if snap.Observed.ShouldBeRunning() {
		return fsmv2.Result[any, any](&TryingToConnectState{}, fsmv2.SignalNone, nil, "should be running, transitioning to trying to connect")
	}

	return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil, "slow worker is stopped, no connection")
}

func (s *StoppedState) String() string {
	return helpers.DeriveStateName(s)
}
