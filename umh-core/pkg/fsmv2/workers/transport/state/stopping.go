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
	transport_pkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
)

// StoppingState represents the graceful shutdown state.
// Transitions unconditionally to StoppedState; the supervisor handles child teardown independently.
type StoppingState struct {
	helpers.StoppingBase
}

// Next evaluates the current snapshot and returns the next state or action.
func (s *StoppingState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[transport_pkg.TransportConfig, transport_pkg.TransportStatus](snapAny)

	if snap.ShouldStop() { //nolint:staticcheck // architecture invariant: shutdown check must be first conditional
	}

	children := transport_pkg.RenderChildren(snap)
	// Stopping: keep children resident but disabled so the CHANGE-19 reducer
	// drives RequestShutdown without despawning. Children resume cleanly when
	// the parent re-enters a running path and emits Enabled=true again.
	for i := range children {
		children[i].Enabled = false
	}

	// Cleanup hook: add resource cleanup here if needed.
	// Self-return during cleanup MUST carry an action — never nil.

	return fsmv2.Transition(&StoppedState{}, fsmv2.SignalNone, nil,
		fmt.Sprintf("stop complete: children healthy=%d, unhealthy=%d",
			snap.Observed.ChildrenHealthy, snap.Observed.ChildrenUnhealthy), children)
}

// String returns the state name derived from the type.
func (s *StoppingState) String() string {
	return helpers.DeriveStateName(s)
}
