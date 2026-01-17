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

// TryingToConnectState represents the state where the worker is attempting to establish a connection.
type TryingToConnectState struct {
	BaseFailingState
}

func (s *TryingToConnectState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
	snap := helpers.ConvertSnapshot[snapshot.ExamplefailingObservedState, *snapshot.ExamplefailingDesiredState](snapAny)
	snap.Observed.State = config.MakeState(config.PrefixTryingToStart, "connection")

	if snap.Observed.IsStopRequired() {
		return &TryingToStopState{}, fsmv2.SignalNone, nil
	}

	// Check for restart threshold - emit SignalNeedsRestart if we've hit the limit
	// This triggers a full worker restart (graceful shutdown → reset → restart)
	if snap.Observed.RestartAfterFailures > 0 &&
		snap.Observed.ConnectAttempts >= snap.Observed.RestartAfterFailures {
		return s, fsmv2.SignalNeedsRestart, nil
	}

	// Failing worker is "connected" when observed state shows healthy connection
	// After ConnectAction executes, we can transition to Connected
	if snap.Observed.ConnectionHealth == "healthy" {
		return &ConnectedState{}, fsmv2.SignalNone, nil
	}

	return s, fsmv2.SignalNone, &action.ConnectAction{}
}

func (s *TryingToConnectState) String() string {
	return helpers.DeriveStateName(s)
}

func (s *TryingToConnectState) Reason() string {
	return "Attempting to establish connection"
}
