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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application/snapshot"
)

// DegradedState represents the state when infrastructure issues are detected.
// The application enters this state when children have circuit breaker open or stale observations.
type DegradedState struct {
	BaseApplicationState
}

func (s *DegradedState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := helpers.ConvertSnapshot[snapshot.ApplicationObservedState, *snapshot.ApplicationDesiredState](snapAny)
	snap.Observed.State = config.MakeState(config.PrefixRunning, "degraded")

	if snap.Desired.IsShutdownRequested() {
		return fsmv2.Result[any, any](&StoppedState{}, fsmv2.SignalNone, nil, "Shutdown requested")
	}

	// Recovered when no infrastructure issues remain
	if !snap.Observed.HasInfrastructureIssues() {
		return fsmv2.Result[any, any](&RunningState{}, fsmv2.SignalNone, nil, "Recovered from infrastructure issues")
	}

	return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil, snap.Observed.InfrastructureReason())
}

func (s *DegradedState) String() string {
	return helpers.DeriveStateName(s)
}
