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
	transport_pkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
)

func init() {
	fsmv2.RegisterInitialState("transport", &StoppedState{})
}

// StoppedState represents the initial state where the transport worker is not running.
// The worker is not authenticated and no children are spawned.
type StoppedState struct {
	helpers.StoppedBase
}

// Next evaluates the current snapshot and returns the next state or action.
func (s *StoppedState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[transport_pkg.TransportConfig, transport_pkg.TransportStatus](snapAny)

	if snap.IsShutdownRequested {
		return fsmv2.Transition(s, fsmv2.SignalNeedsRemoval, nil, "Shutdown requested, signaling removal")
	}

	if snap.Config.GetState() == config.DesiredStateRunning {
		return fsmv2.Transition(&StartingState{}, fsmv2.SignalNone, nil, "Desired state is running, transitioning to Starting")
	}

	return fsmv2.Transition(s, fsmv2.SignalNone, nil, "Transport is stopped, waiting for running request")
}

// String returns the state name derived from the type.
func (s *StoppedState) String() string {
	return helpers.DeriveStateName(s)
}
