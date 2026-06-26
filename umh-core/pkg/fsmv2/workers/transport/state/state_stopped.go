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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/snapshot"
)

// StoppedState represents the initial state where the transport worker is not running.
// The worker is not authenticated and no children are spawned.
//
// Transport is a top-level worker, not a child. Reason strings show
// snap.Config.ShouldBeRunning() and snap.IsShutdownRequested directly.
type StoppedState struct {
	helpers.StoppedBase
}

// Next evaluates the current snapshot and returns the next state or action.
func (s *StoppedState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[snapshot.TransportDesiredState, snapshot.TransportStatus](snapAny)

	// Stopped emits children with enabled=false (resident, not despawned).
	// On IsShutdownRequested, SignalNeedsRemoval drives removal so the children arg is ignored.
	stopChildren, err := snapshot.RenderChildren(snap.Config, snap.Status, false)
	if err != nil {
		stopChildren = nil
	}

	if snap.IsShutdownRequested {
		return fsmv2.Transition(s, fsmv2.SignalNeedsRemoval, nil,
			fmt.Sprintf("removal signaled: shouldBeRunning=%t", snap.Config.ShouldBeRunning()),
			stopChildren)
	}

	if snap.IsDisabled {
		return fsmv2.Transition(s, fsmv2.SignalNone, nil,
			fmt.Sprintf("staying stopped (disabled): shouldBeRunning=%t", snap.Config.ShouldBeRunning()),
			stopChildren)
	}

	if snap.Config.ShouldBeRunning() {
		// Transitioning to Starting: emit aliveChildren NOW (one tick early)
		// so the supervisor flips enabled=true before Starting reads its first snapshot.
		aliveChildren, aerr := snapshot.RenderChildren(snap.Config, snap.Status, true)
		if aerr != nil {
			aliveChildren = nil
		}

		return fsmv2.Transition(&StartingState{}, fsmv2.SignalNone, nil,
			fmt.Sprintf("transitioning to Starting: shutdown=%t", snap.IsShutdownRequested),
			aliveChildren)
	}

	return fsmv2.Transition(s, fsmv2.SignalNone, nil,
		fmt.Sprintf("stopped, waiting: shouldBeRunning=%t, shutdown=%t",
			snap.Config.ShouldBeRunning(), snap.IsShutdownRequested),
		stopChildren)
}

// String returns the state name derived from the type.
func (s *StoppedState) String() string {
	return helpers.DeriveStateName(s)
}
