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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push/snapshot"
)

// DegradedState represents the degraded state where the push worker has experienced consecutive errors.
type DegradedState struct {
	helpers.RunningDegradedBase
}

func (s *DegradedState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := helpers.ConvertSnapshot[snapshot.PushObservedState, *snapshot.PushDesiredState](snapAny)

	if snap.Observed.IsStopRequired() {
		return fsmv2.Result[any, any](&StoppingState{}, fsmv2.SignalNone, nil,
			fmt.Sprintf("stop required: shutdown=%t, parentState(observed)=%s", snap.Desired.IsShutdownRequested(), snap.Observed.ParentMappedState))
	}

	if snap.Observed.ConsecutiveErrors == 0 {
		return fsmv2.Result[any, any](&RunningState{}, fsmv2.SignalNone, nil, "errors cleared (consecutiveErrors=0), recovering to Running")
	}

	if snap.Observed.HasTransport && snap.Observed.HasValidToken {
		backoffDelay := backoff.CalculateDelayForErrorType(
			snap.Observed.LastErrorType,
			snap.Observed.ConsecutiveErrors,
			snap.Observed.LastRetryAfter,
		)

		shouldWait := false
		if snap.Observed.LastRetryAfter > 0 && !snap.Observed.LastErrorAt.IsZero() {
			shouldWait = time.Since(snap.Observed.LastErrorAt) < snap.Observed.LastRetryAfter
		} else if !snap.Observed.DegradedEnteredAt.IsZero() {
			shouldWait = time.Since(snap.Observed.DegradedEnteredAt) < backoffDelay
		}

		if shouldWait {
			return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil,
				fmt.Sprintf("degraded (%d errors, %d pending), backoff %s",
					snap.Observed.ConsecutiveErrors, snap.Observed.PendingMessageCount,
					backoffDelay.Round(time.Second)))
		}

		return fsmv2.Result[any, any](s, fsmv2.SignalNone, &action.PushAction{},
			fmt.Sprintf("degraded (%d consecutive errors, %d pending), still pushing",
				snap.Observed.ConsecutiveErrors, snap.Observed.PendingMessageCount))
	}

	return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil,
		fmt.Sprintf("degraded (%d consecutive errors, %d pending), waiting: hasTransport=%t, hasValidToken=%t",
			snap.Observed.ConsecutiveErrors, snap.Observed.PendingMessageCount,
			snap.Observed.HasTransport, snap.Observed.HasValidToken))
}

func (s *DegradedState) String() string {
	return helpers.DeriveStateName(s)
}
