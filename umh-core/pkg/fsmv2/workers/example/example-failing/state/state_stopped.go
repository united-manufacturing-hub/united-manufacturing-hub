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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-failing/snapshot"
)

// StoppedState represents the initial state where the failing worker is not connected.
type StoppedState struct {
	BaseFailingState
}

func (s *StoppedState) Next(snapAny any) (fsmv2.State[any, any], fsmv2.Signal, fsmv2.Action[any]) {
	snap := helpers.ConvertSnapshot[snapshot.FailingObservedState, *snapshot.FailingDesiredState](snapAny)

	if snap.Desired.IsShutdownRequested() {
		return s, fsmv2.SignalNeedsRemoval, nil
	}

	// Only transition to connecting if desired state wants us running
	if snap.Desired.ShouldBeRunning() {
		return &TryingToConnectState{}, fsmv2.SignalNone, nil
	}

	return s, fsmv2.SignalNone, nil
}

func (s *StoppedState) String() string {
	return helpers.DeriveStateName(s)
}

func (s *StoppedState) Reason() string {
	return "Failing worker is stopped, no connection"
}
