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

// Package state defines the FSM states for the helloworld worker.
//
// State machine:
//
//	stopped -> trying_to_start -> running
//	   ^                |        mood="sad" | ^ mood!="sad"
//	   |                |                   v |
//	   |                |               degraded
//	   └── (shutdown) ──┴──────────────────┘
package state

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld/snapshot"
)

// StoppedState is the initial state where the worker is not running.
// It waits for the desired state to request running, then transitions.
type StoppedState struct {
	helpers.StoppedBase
}

// Next implements the state transition logic.
func (s *StoppedState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := helpers.ConvertSnapshot[snapshot.HelloworldObservedState, *snapshot.HelloworldDesiredState](snapAny)

	// 1. Check shutdown first
	if snap.Desired.IsShutdownRequested() {
		return fsmv2.Result[any, any](s, fsmv2.SignalNeedsRemoval, nil, "Shutdown requested, signaling removal", nil)
	}

	// 2. Start running
	return fsmv2.Result[any, any](&TryingToStartState{}, fsmv2.SignalNone, nil, "Starting worker", nil)
}

// String returns the state name for logging and metrics.
// Use helpers.DeriveStateName to get consistent naming.
func (s *StoppedState) String() string {
	return helpers.DeriveStateName(s)
}
