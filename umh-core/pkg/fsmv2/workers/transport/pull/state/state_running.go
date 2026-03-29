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
	pull_pkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/pull"
)

const (
	errorDegradedThreshold   = 3
	pendingDegradedThreshold = 100
)

// RunningState represents the operational state where the pull worker actively pulls messages.
type RunningState struct {
	helpers.RunningHealthyBase
}

func (s *RunningState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[pull_pkg.PullConfig, pull_pkg.PullStatus](snapAny)

	if snap.ShouldStop() {
		return fsmv2.Result[any, any](&StoppingState{}, fsmv2.SignalNone, nil,
			fmt.Sprintf("stop required: shutdown=%t, parentState=%s", snap.IsShutdownRequested, snap.ParentMappedState), nil)
	}

	if snap.Status.ConsecutiveErrors >= errorDegradedThreshold {
		return fsmv2.Result[any, any](&DegradedState{}, fsmv2.SignalNone, nil,
			fmt.Sprintf("degrading: %d consecutive errors (threshold=%d)", snap.Status.ConsecutiveErrors, errorDegradedThreshold), nil)
	}

	if snap.Status.PendingMessageCount >= pendingDegradedThreshold {
		return fsmv2.Result[any, any](&DegradedState{}, fsmv2.SignalNone, nil,
			fmt.Sprintf("degrading: %d pending messages (threshold=%d)",
				snap.Status.PendingMessageCount, pendingDegradedThreshold), nil)
	}

	if snap.Status.HasTransport && snap.Status.HasValidToken {
		return fsmv2.Result[any, any](s, fsmv2.SignalNone, &action.PullAction{}, "pulling messages (transport and token available)", nil)
	}

	return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil,
		fmt.Sprintf("waiting: hasTransport=%t, hasValidToken=%t", snap.Status.HasTransport, snap.Status.HasValidToken), nil)
}

func (s *RunningState) String() string {
	return helpers.DeriveStateName(s)
}
