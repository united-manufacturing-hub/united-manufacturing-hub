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
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	benthos_monitor "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/benthos_monitor"
)

// StartingState is a short-lived transitional state between Stopped and Running.
// The benthos_monitor worker has no startup action to emit: its per-tick work is
// the CollectObservedState scrape, which begins once Running is reached. This
// state therefore advances to Running on the next tick. It is named Starting
// (not TryingToStart) because it performs no action — the architecture
// validator's "TryingTo* states must return an action" invariant applies only
// to the TryingTo prefix; a no-action transitional state is the Starting
// category (the same base, StartingBase, that transport's StartingState uses).
type StartingState struct {
	helpers.StartingBase
}

// Next implements state transition logic for StartingState.
func (s *StartingState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[benthos_monitor.BenthosMonitorConfig, benthos_monitor.BenthosMonitorStatus](snapAny)

	// Shutdown or disable: return to Stopped before reaching Running.
	if snap.ShouldStop() {
		return fsmv2.Transition(&StoppedState{}, fsmv2.SignalNone, nil,
			fmt.Sprintf("stop required: %s", snap.StopReason()), nil)
	}

	return fsmv2.Transition(&RunningState{}, fsmv2.SignalNone, nil, "started", nil)
}

// String returns the state name for logging and metrics.
func (s *StartingState) String() string {
	return helpers.DeriveStateName(s)
}
