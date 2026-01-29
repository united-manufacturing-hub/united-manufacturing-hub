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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleslow/action"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleslow/snapshot"
)

type TryingToStopState struct {
	BaseExampleslowState
}

func (s *TryingToStopState) LifecyclePhase() config.LifecyclePhase {
	return config.PhaseStopping
}

func (s *TryingToStopState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := helpers.ConvertSnapshot[snapshot.ExampleslowObservedState, *snapshot.ExampleslowDesiredState](snapAny)

	if snap.Observed.ConnectionHealth == "no connection" {
		return fsmv2.Result[any, any](&StoppedState{}, fsmv2.SignalNone, nil, "connection closed, transitioning to stopped")
	}

	return fsmv2.Result[any, any](s, fsmv2.SignalNone, &action.DisconnectAction{}, "closing connections gracefully")
}

func (s *TryingToStopState) String() string {
	return helpers.DeriveStateName(s)
}
