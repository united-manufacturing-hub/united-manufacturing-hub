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

// Package state defines the FSM states for the benthos_monitor worker.
package state

import (
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	benthos_monitor "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/benthos_monitor"
)

func init() {
	fsmv2.RegisterInitialState(benthos_monitor.WorkerTypeName, &StoppedState{})
}

// StoppedState is the initial state where the benthos_monitor worker is not
// scraping. The worker stays here while the desired state is "stopped" or while
// the worker is administratively disabled, and transitions to Starting when
// the desired state becomes "running".
type StoppedState struct {
	helpers.StoppedBase
}

// Next implements the state transition logic.
func (s *StoppedState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[benthos_monitor.BenthosMonitorConfig, benthos_monitor.BenthosMonitorStatus](snapAny)

	// Shutdown wins removal. Use IsShutdownRequested (not ShouldStop) so an
	// admin-disabled worker is not despawned here; it stays resident-stopped.
	if snap.IsShutdownRequested {
		return fsmv2.Transition(s, fsmv2.SignalNeedsRemoval, nil,
			fmt.Sprintf("stop required: %s", snap.StopReason()), nil)
	}

	// A disabled worker must stay resident-stopped even when the desired state
	// is "running"; otherwise it would take the Starting branch and resume
	// scraping, defeating the admin disable and leaking the CPU the bet saves.
	if snap.IsDisabled {
		return fsmv2.Transition(s, fsmv2.SignalNone, nil, "staying stopped (disabled)", nil)
	}

	if snap.Config.GetState() == config.DesiredStateStopped {
		return fsmv2.Transition(s, fsmv2.SignalNone, nil, "desired state stopped", nil)
	}

	return fsmv2.Transition(&StartingState{}, fsmv2.SignalNone, nil, "desired state running, starting", nil)
}

// String returns the state name for logging and metrics.
func (s *StoppedState) String() string {
	return helpers.DeriveStateName(s)
}
