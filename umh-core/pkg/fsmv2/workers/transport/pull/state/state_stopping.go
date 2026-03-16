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

// StoppingState represents the shutdown state where the pull worker is stopping.
type StoppingState struct {
	helpers.StoppingBase
}

func (s *StoppingState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := helpers.ConvertSnapshot[snapshot.PullObservedState, *snapshot.PullDesiredState](snapAny)

	// Unconditional transition to Stopped. The decision to stop was already made
	// on entry to StoppingState. StoppedState handles recovery via ShouldBeRunning().
	return fsmv2.Result[any, any](&StoppedState{}, fsmv2.SignalNone, nil,
		fmt.Sprintf("stop complete: shutdown=%t, parentState=%s", snap.Desired.IsShutdownRequested(), snap.Observed.ParentMappedState))
}

func (s *StoppingState) String() string {
	return helpers.DeriveStateName(s)
}
