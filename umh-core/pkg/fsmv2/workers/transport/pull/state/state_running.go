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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/pull/action"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/pull/snapshot"
)

const pendingDegradedThreshold = 100

type RunningState struct {
	helpers.RunningHealthyBase
}

func (s *RunningState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := helpers.ConvertSnapshot[snapshot.PullObservedState, *snapshot.PullDesiredState](snapAny)

	if snap.Observed.IsStopRequired() {
		return fsmv2.Result[any, any](&StoppingState{}, fsmv2.SignalNone, nil,
			fmt.Sprintf("stop required: shutdown=%t, parentState=%s", snap.Desired.IsShutdownRequested(), snap.Desired.ParentMappedState))
	}

	if snap.Observed.ConsecutiveErrors >= 3 {
		return fsmv2.Result[any, any](&DegradedState{}, fsmv2.SignalNone, nil,
			fmt.Sprintf("degrading: %d consecutive errors", snap.Observed.ConsecutiveErrors))
	}

	if snap.Observed.PendingMessageCount >= pendingDegradedThreshold {
		return fsmv2.Result[any, any](&DegradedState{}, fsmv2.SignalNone, nil,
			fmt.Sprintf("degrading: %d pending messages (threshold=%d)",
				snap.Observed.PendingMessageCount, pendingDegradedThreshold))
	}

	if snap.Observed.HasTransport && snap.Observed.HasValidToken {
		return fsmv2.Result[any, any](s, fsmv2.SignalNone, &action.PullAction{}, "pulling messages (transport and token available)")
	}

	return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil,
		fmt.Sprintf("waiting: hasTransport=%t, hasValidToken=%t", snap.Observed.HasTransport, snap.Observed.HasValidToken))
}

func (s *RunningState) String() string {
	return helpers.DeriveStateName(s)
}
