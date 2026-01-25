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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplefailing/action"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplefailing/snapshot"
)

// TryingToStopState represents the state where the worker is attempting to disconnect cleanly.
type TryingToStopState struct {
	BaseFailingState
}

func (s *TryingToStopState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := helpers.ConvertSnapshot[snapshot.ExamplefailingObservedState, *snapshot.ExamplefailingDesiredState](snapAny)
	snap.Observed.State = config.MakeState(config.PrefixTryingToStop, "connection")

	if snap.Observed.ConnectionHealth == "no connection" {
		return fsmv2.Result[any, any](&StoppedState{}, fsmv2.SignalNone, nil, "disconnected successfully")
	}

	return fsmv2.Result[any, any](s, fsmv2.SignalNone, &action.DisconnectAction{}, "attempting to disconnect cleanly")
}

func (s *TryingToStopState) String() string {
	return helpers.DeriveStateName(s)
}
