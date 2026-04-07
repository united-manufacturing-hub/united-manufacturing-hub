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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/certfetcher/action"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/certfetcher/snapshot"
)

const (
	// FetchInterval is the minimum time between certificate fetch cycles.
	FetchInterval = 1 * time.Minute
	// DegradedThreshold is the number of consecutive errors before entering degraded state.
	DegradedThreshold = 3
)

// RunningState periodically fetches certificates for active subscribers.
type RunningState struct {
	helpers.RunningHealthyBase
}

// Next checks if a fetch is due and emits FetchCertsAction if so.
func (s *RunningState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := helpers.ConvertSnapshot[snapshot.CertFetcherObservedState, *snapshot.CertFetcherDesiredState](snapAny)

	if snap.Desired.IsShutdownRequested() {
		return fsmv2.Result[any, any](&StoppedState{}, fsmv2.SignalNone, nil, "shutdown requested")
	}

	if !snap.Observed.HasSubHandler {
		return fsmv2.Result[any, any](&StoppedState{}, fsmv2.SignalNone, nil, "sub handler lost")
	}

	if snap.Observed.ConsecutiveErrors >= DegradedThreshold {
		return fsmv2.Result[any, any](&DegradedState{}, fsmv2.SignalNone, nil,
			fmt.Sprintf("degrading: %d consecutive errors", snap.Observed.ConsecutiveErrors))
	}

	timeSinceLastFetch := snap.Observed.CollectedAt.Sub(snap.Observed.LastFetchAt)
	if timeSinceLastFetch >= FetchInterval || (snap.Observed.LastFetchAt.IsZero() && timeSinceLastFetch >= 10*time.Second) {
		return fsmv2.Result[any, any](s, fsmv2.SignalNone, &action.FetchCertsAction{},
			fmt.Sprintf("fetching: %d subscribers, last fetch %s ago",
				snap.Observed.SubscriberCount, timeSinceLastFetch.Round(time.Second)))
	}

	return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil,
		fmt.Sprintf("idle: next fetch in %s, %d subscribers",
			(FetchInterval-timeSinceLastFetch).Round(time.Second), snap.Observed.SubscriberCount))
}

func (s *RunningState) String() string {
	return helpers.DeriveStateName(s)
}
