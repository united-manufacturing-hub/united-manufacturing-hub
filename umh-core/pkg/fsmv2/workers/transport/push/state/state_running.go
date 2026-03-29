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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push/action"
	push_pkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push"
)

const pendingDegradedThreshold = 100

// RunningState represents the operational state where the push worker is actively pushing messages.
type RunningState struct {
	helpers.RunningHealthyBase
}

func (s *RunningState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[push_pkg.PushConfig, push_pkg.PushStatus](snapAny)

	if snap.IsStopRequired() {
		return fsmv2.Result[any, any](&StoppingState{}, fsmv2.SignalNone, nil,
			fmt.Sprintf("stop required: shutdown=%t, parentState=%s", snap.IsShutdownRequested, snap.ParentMappedState))
	}

	if snap.Status.ConsecutiveErrors >= 3 {
		return fsmv2.Result[any, any](&DegradedState{}, fsmv2.SignalNone, nil,
			fmt.Sprintf("degrading: %d consecutive errors", snap.Status.ConsecutiveErrors))
	}

	if snap.Status.PendingMessageCount >= pendingDegradedThreshold {
		return fsmv2.Result[any, any](&DegradedState{}, fsmv2.SignalNone, nil,
			fmt.Sprintf("degrading: %d pending messages (threshold=%d)",
				snap.Status.PendingMessageCount, pendingDegradedThreshold))
	}

	if snap.Status.HasTransport && snap.Status.HasValidToken {
		return fsmv2.Result[any, any](s, fsmv2.SignalNone, &action.PushAction{}, "pushing messages (transport and token available)")
	}

	return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil,
		fmt.Sprintf("waiting: hasTransport=%t, hasValidToken=%t", snap.Status.HasTransport, snap.Status.HasValidToken))
}

func (s *RunningState) String() string {
	return helpers.DeriveStateName(s)
}
