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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplebridge"
)

// RunningState is the steady state with every non-empty flow spawned.
// It has no start delay and no running-duration timeout: it stays here
// until shutdown is requested.
type RunningState struct {
	helpers.RunningHealthyBase
}

// Next implements the state transition logic for RunningState.
func (s *RunningState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[examplebridge.ExamplebridgeConfig, examplebridge.ExamplebridgeStatus](snapAny)

	// The shutdown check comes first: the validators require it.
	if snap.ShouldStop() {
		return fsmv2.Transition(&TryingToStopState{}, fsmv2.SignalNone, nil,
			"stop required: "+snap.StopReason(), []config.ChildSpec{})
	}

	// The flows are named after the bridge, so two bridges' flow children
	// stay apart in the child registry. The config's own name field is the
	// name the user gave the bridge; the worker id stands in when a bridge
	// carries no name.
	bridge := snap.Config.Name
	if bridge == "" {
		bridge = snap.Identity.ID
	}

	specs, err := examplebridge.RenderFlows(bridge, snap.Config)
	if err != nil {
		// RenderFlows fails only when yaml cannot encode a flow template,
		// which the templates' own type forbids. Keep the children from the
		// last good render by returning nil children, so reconcileChildren
		// falls back to the last rendered set.
		return fsmv2.Transition(s, fsmv2.SignalNone, nil,
			"running: rendering the flows failed: "+err.Error(), nil)
	}

	return fsmv2.Transition(s, fsmv2.SignalNone, nil, "running: flows spawned", specs)
}

// String returns the state name for logging and metrics.
func (s *RunningState) String() string {
	return helpers.DeriveStateName(s)
}
