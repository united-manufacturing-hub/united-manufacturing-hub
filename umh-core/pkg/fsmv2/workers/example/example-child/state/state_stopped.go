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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-child/snapshot"
)

// StoppedState represents the initial state where the child worker is not connected
type StoppedState struct {
	BaseChildState
	deps snapshot.ChildDependencies
}

func NewStoppedState(deps snapshot.ChildDependencies) *StoppedState {
	return &StoppedState{deps: deps}
}

func (s *StoppedState) Next(snap fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
	childSnap := snapshot.ChildSnapshot{
		Identity: snap.Identity,
		Observed: snap.Observed.(snapshot.ChildObservedState),
		Desired:  snap.Desired.(snapshot.ChildDesiredState),
	}

	if childSnap.Desired.ShutdownRequested() {
		return s, fsmv2.SignalNeedsRemoval, nil
	}

	return NewTryingToConnectState(s.deps), fsmv2.SignalNone, nil
}

func (s *StoppedState) String() string {
	return "Stopped"
}

func (s *StoppedState) Reason() string {
	return "Child is stopped, no connection"
}
