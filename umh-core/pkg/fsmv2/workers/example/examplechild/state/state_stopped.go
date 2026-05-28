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
	example_child "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild"
)

func init() {
	fsmv2.RegisterInitialState("examplechild", &StoppedState{})
}

// StoppedState is the child's terminal lifecycle state. It implements the
// three-way branch between shutdown, disable, and resume.
//
// Three branches, in precedence order:
//
//  1. IsShutdownRequested wins. The supervisor wants this worker removed
//     entirely. Emit SignalNeedsRemoval and stop.
//  2. IsDisabled stays resident. The parent has set Enabled=false on this
//     child's ChildSpec. The worker stays in Stopped, preserving dependency
//     state. It does not resume until IsDisabled clears.
//  3. Otherwise observe the parent. When ParentMappedState is Running,
//     transition to TryingToConnect. Otherwise stay stopped.
//
// This is the child half of the resident-disable contract. For the parent
// half, see workers/example/exampleparent/children.go (which despawns
// instead of disabling, because exampleparent's children are stateless).
// For the resident variant in production, see workers/transport.
type StoppedState struct {
	helpers.StoppedBase
}

func (s *StoppedState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[example_child.ExamplechildConfig, example_child.ExamplechildStatus](snapAny)

	if snap.IsShutdownRequested {
		return fsmv2.Transition(s, fsmv2.SignalNeedsRemoval, nil, "shutdown requested, stopping: parentState="+snap.ParentMappedState, nil)
	}

	if snap.IsDisabled {
		return fsmv2.Transition(s, fsmv2.SignalNone, nil, "disabled by supervisor, staying stopped: parentState="+snap.ParentMappedState, nil)
	}

	if snap.ParentMappedState == config.DesiredStateRunning {
		return fsmv2.Transition(&TryingToConnectState{}, fsmv2.SignalNone, nil, "parent wants running, attempting to connect: parentState="+snap.ParentMappedState, nil)
	}

	return fsmv2.Transition(s, fsmv2.SignalNone, nil, "Child is stopped, no connection", nil)
}

func (s *StoppedState) String() string {
	return helpers.DeriveStateName(s)
}
