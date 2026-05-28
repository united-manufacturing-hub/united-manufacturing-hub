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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	exampleparent "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent/action"
)

// TryingToStopState commits to cleanup. It does NOT check IsShutdownRequested
// to abort cleanup mid-flight. Once the worker enters this state, it runs
// to completion (Stopped) before any resume can begin. See pkg/fsmv2/doc.go
// § One-way stop trajectory for the rule and the architecture-validator name.
//
// Returns an empty []config.ChildSpec{} so children despawn (exampleparent's
// children are stateless). Then waits for all children to stop before
// transitioning to StoppedState.
type TryingToStopState struct {
	helpers.StoppingBase
}

func (s *TryingToStopState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[exampleparent.ExampleparentConfig, exampleparent.ExampleparentStatus](snapAny)

	despawnChildren := []config.ChildSpec{}

	if snap.Status.ID == "" {
		return fsmv2.Transition(s, fsmv2.SignalNone, &action.StopAction{}, "ID not set, executing StopAction", despawnChildren)
	}

	if snap.ChildrenHealthy == 0 && snap.ChildrenUnhealthy == 0 {
		return fsmv2.Transition(&StoppedState{}, fsmv2.SignalNone, nil, "All children stopped, transitioning to Stopped", despawnChildren)
	}

	return fsmv2.Transition(s, fsmv2.SignalNone, nil, "Gracefully stopping all children", despawnChildren)
}

func (s *TryingToStopState) String() string {
	return helpers.DeriveStateName(s)
}
