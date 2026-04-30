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
	push_pkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push"
)

func init() {
	fsmv2.RegisterInitialState("push", &StoppedState{})
}

// StoppedState represents the initial state where the push worker is not active.
type StoppedState struct {
	helpers.StoppedBase
}

func (s *StoppedState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[push_pkg.PushConfig, push_pkg.PushStatus](snapAny)

	if snap.IsShutdownRequested {
		return fsmv2.Transition(s, fsmv2.SignalNeedsRemoval, nil, "shutdown requested, signaling removal", nil)
	}

	if !snap.ShouldStop() {
		return fsmv2.Transition(&RunningState{}, fsmv2.SignalNone, nil,
			"parent requests running, transitioning to Running", nil)
	}

	return fsmv2.Transition(s, fsmv2.SignalNone, nil,
		"stopped, awaiting parent run signal", nil)
}

func (s *StoppedState) String() string {
	return helpers.DeriveStateName(s)
}
