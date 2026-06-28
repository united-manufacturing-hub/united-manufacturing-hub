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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	benthos_monitor "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/benthos_monitor"
)

// StateNameRunning is the observed-state string a benthos_monitor worker reports
// while it is running. It is derived from RunningState's type name so a rename
// of RunningState moves this constant with it.
var StateNameRunning = helpers.DeriveStateName(&RunningState{})

// RunningState is the steady state where the worker scrapes the benthos
// instance each tick via CollectObservedState. The worker stays here until a
// shutdown or disable is requested, at which point it transitions directly
// back to StoppedState (there is no Stopping state).
type RunningState struct {
	helpers.RunningHealthyBase
}

// Next implements state transition logic for RunningState.
func (s *RunningState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[benthos_monitor.BenthosMonitorConfig, benthos_monitor.BenthosMonitorStatus](snapAny)

	// Shutdown or disable: transition directly to Stopped (no Stopping state).
	if snap.ShouldStop() {
		return fsmv2.Transition(&StoppedState{}, fsmv2.SignalNone, nil,
			fmt.Sprintf("stop required: %s", snap.StopReason()), nil)
	}

	// A desired state of "stopped" also returns to Stopped so the COS guard can
	// short-circuit the scrape on the next tick.
	if snap.Config.GetState() == config.DesiredStateStopped {
		return fsmv2.Transition(&StoppedState{}, fsmv2.SignalNone, nil, "desired state stopped", nil)
	}

	return fsmv2.Transition(s, fsmv2.SignalNone, nil, "running, scraping benthos", nil)
}

// String returns the state name for logging and metrics.
func (s *RunningState) String() string {
	return helpers.DeriveStateName(s)
}
