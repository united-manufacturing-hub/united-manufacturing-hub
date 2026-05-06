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

package helpers

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
)

// StoppedNext implements the canonical two-tier StoppedState.Next pattern.
//
// PR5 introduced two distinct stop signals on top of the user-driven
// "stopped" config: IsBeingRemoved (permanent) and IsDisabled (transient
// parent-disable). StoppedState files have to discriminate between them
// because the response differs:
//
//  1. Desired.IsBeingRemoved() — emit SignalNeedsRemoval. Permanent removal:
//     Phase-1 absent-from-specs, SignalNeedsRestart, graceful shutdown,
//     or supervisor self-protection. Drop retained state when implemented.
//  2. !ShouldStop()             — advance to nextState. The umbrella signal
//     cleared; the worker may resume.
//  3. otherwise                 — stay stopped (resident), preserving
//     in-memory state. IsDisabled (transient parent-disable) and
//     Config.GetState()=="stopped" take this branch — the worker remains
//     alive and ready to resume.
//
// Use this from a worker's StoppedState.Next when the worker's stopped
// semantics match the canonical three-branch shape and there is no
// worker-specific logic (children rendering, custom signals, additional
// preconditions before advancing). Otherwise, hand-roll the three branches
// and keep this helper as the reference pattern.
func StoppedNext[TConfig any, TStatus any](
	s fsmv2.State[any, any],
	snap fsmv2.WorkerSnapshot[TConfig, TStatus],
	nextState fsmv2.State[any, any],
	advanceReason string,
) fsmv2.NextResult[any, any] {
	if snap.Desired.IsBeingRemoved() {
		return fsmv2.Transition(s, fsmv2.SignalNeedsRemoval, nil, "shutdown requested, signaling removal", nil)
	}

	if !snap.ShouldStop() {
		return fsmv2.Transition(nextState, fsmv2.SignalNone, nil, advanceReason, nil)
	}

	return fsmv2.Transition(s, fsmv2.SignalNone, nil, "worker is stopped", nil)
}
