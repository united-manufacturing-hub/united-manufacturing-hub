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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/snapshot"
)

// StoppingState represents the graceful shutdown state.
// Waits for all children to stop before transitioning to StoppedState.
type StoppingState struct {
	helpers.StoppingBase
}

func (s *StoppingState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := helpers.ConvertSnapshot[snapshot.TransportObservedState, *snapshot.TransportDesiredState](snapAny)

	// All children must be stopped before transitioning to Stopped
	if snap.Observed.ChildrenHealthy == 0 && snap.Observed.ChildrenUnhealthy == 0 {
		return fsmv2.Result[any, any](&StoppedState{}, fsmv2.SignalNone, nil, "All children stopped, transitioning to Stopped")
	}

	return fsmv2.Result[any, any](s, fsmv2.SignalNone, nil, "Gracefully stopping all children")
}

func (s *StoppingState) String() string {
	return helpers.DeriveStateName(s)
}
