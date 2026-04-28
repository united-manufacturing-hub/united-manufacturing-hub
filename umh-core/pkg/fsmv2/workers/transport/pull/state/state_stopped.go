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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/pull/snapshot"
)

// StoppedState represents the idle state where the pull worker waits for the parent
// to transition to Running before resuming.
type StoppedState struct {
	helpers.StoppedBase
}

func (s *StoppedState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := helpers.ConvertSnapshot[snapshot.PullObservedState, *snapshot.PullDesiredState](snapAny)

	if snap.Desired.IsShutdownRequested() {
		return fsmv2.Result[any, any](s, fsmv2.SignalNeedsRemoval, nil, "shutdown requested, signaling removal", nil)
	}

	if snap.Observed.ShouldBeRunning() {
		return fsmv2.Result[any, any](&RunningState{}, fsmv2.SignalNone, nil,
			fmt.Sprintf("parent mapped state is %q, transitioning to Running", snap.Observed.ParentMappedState), nil)
	}

	return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil,
		fmt.Sprintf("stopped, parent mapped state is %q", snap.Observed.ParentMappedState), nil)
}

func (s *StoppedState) String() string {
	return helpers.DeriveStateName(s)
}
