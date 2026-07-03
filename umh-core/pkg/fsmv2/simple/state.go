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

package simple

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
)

// runningState is the healthy steady state of a simple worker. It is generic
// over the developer's config and status so Next can read the verdict off the
// wrapped Status the worker persists each tick; the generic Register
// instantiates one per worker type. Rung "states in sub-files" (later) may split
// these; for now the machine flips between running and degraded on the verdict.
type runningState[TConfig, TStatus any] struct {
	helpers.RunningHealthyBase
}

// Next stays running while the verdict is healthy and flips to degraded when the
// worker reports Degraded. The verdict's Reason is emitted via Transition so it
// reaches logs, heartbeats, and the frontend.
func (s *runningState[TConfig, TStatus]) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[TConfig, Status[TStatus]](snapAny)

	if snap.Status.Degraded {
		return fsmv2.Transition(&degradedState[TConfig, TStatus]{}, fsmv2.SignalNone, nil, snap.Status.Reason, nil)
	}

	return fsmv2.Transition(s, fsmv2.SignalNone, nil, snap.Status.Reason, nil)
}

// String returns the observed-state name, derived from the type name.
func (s *runningState[TConfig, TStatus]) String() string {
	return helpers.DeriveStateName(s)
}

// degradedState is the unhealthy-but-operational state of a simple worker,
// entered when the verdict reports Degraded (a poll error or a Health verdict).
type degradedState[TConfig, TStatus any] struct {
	helpers.RunningDegradedBase
}

// Next returns to running once the verdict clears and otherwise stays degraded,
// carrying the verdict's Reason on every Transition.
func (s *degradedState[TConfig, TStatus]) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[TConfig, Status[TStatus]](snapAny)

	if !snap.Status.Degraded {
		return fsmv2.Transition(&runningState[TConfig, TStatus]{}, fsmv2.SignalNone, nil, snap.Status.Reason, nil)
	}

	return fsmv2.Transition(s, fsmv2.SignalNone, nil, snap.Status.Reason, nil)
}

// String returns the observed-state name, derived from the type name.
func (s *degradedState[TConfig, TStatus]) String() string {
	return helpers.DeriveStateName(s)
}
