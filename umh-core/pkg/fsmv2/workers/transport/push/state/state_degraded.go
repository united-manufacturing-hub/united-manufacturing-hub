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
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push/action"
	push_pkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push"
)

// DegradedState represents the degraded state where the push worker has experienced consecutive errors.
type DegradedState struct {
	helpers.RunningDegradedBase
}

func (s *DegradedState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[push_pkg.PushConfig, push_pkg.PushStatus](snapAny)

	if snap.ShouldStop() {
		return fsmv2.Transition(&StoppingState{}, fsmv2.SignalNone, nil,
			fmt.Sprintf("stop required: shutdown=%t", snap.ShouldStop()), nil)
	}

	if snap.Observed.Status.ConsecutiveErrors == 0 {
		return fsmv2.Transition(&RunningState{}, fsmv2.SignalNone, nil, "errors cleared (consecutiveErrors=0), recovering to Running", nil)
	}

	if snap.Observed.Status.HasTransport && snap.Observed.Status.HasValidToken {
		backoffDelay := backoff.CalculateDelayForErrorType(
			snap.Observed.Status.LastErrorType,
			snap.Observed.Status.ConsecutiveErrors,
			snap.Observed.Status.LastRetryAfter,
		)

		shouldWait := false
		if snap.Observed.Status.LastRetryAfter > 0 && !snap.Observed.Status.LastErrorAt.IsZero() {
			shouldWait = time.Since(snap.Observed.Status.LastErrorAt) < snap.Observed.Status.LastRetryAfter
		} else if !snap.Observed.Status.DegradedEnteredAt.IsZero() {
			shouldWait = time.Since(snap.Observed.Status.DegradedEnteredAt) < backoffDelay
		}

		if shouldWait {
			return fsmv2.Transition(s, fsmv2.SignalNone, nil,
				fmt.Sprintf("degraded (%d errors, %d pending), backoff %s",
					snap.Observed.Status.ConsecutiveErrors, snap.Observed.Status.PendingMessageCount,
					backoffDelay.Round(time.Second)), nil)
		}

		return fsmv2.Transition(s, fsmv2.SignalNone, &action.PushAction{},
			fmt.Sprintf("degraded (%d consecutive errors, %d pending), still pushing",
				snap.Observed.Status.ConsecutiveErrors, snap.Observed.Status.PendingMessageCount), nil)
	}

	return fsmv2.Transition(s, fsmv2.SignalNone, nil,
		fmt.Sprintf("degraded (%d consecutive errors, %d pending), waiting: hasTransport=%t, hasValidToken=%t",
			snap.Observed.Status.ConsecutiveErrors, snap.Observed.Status.PendingMessageCount,
			snap.Observed.Status.HasTransport, snap.Observed.Status.HasValidToken), nil)
}

func (s *DegradedState) String() string {
	return helpers.DeriveStateName(s)
}
