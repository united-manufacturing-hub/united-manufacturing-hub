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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild/action"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild/snapshot"
)

// TryingToStopState represents the shutdown state where the worker is closing connections.
type TryingToStopState struct {
	helpers.StoppingBase
}

func (s *TryingToStopState) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[snapshot.ExamplechildConfig, snapshot.ExamplechildStatus](snapAny)

	// Child worker is "stopped" when connection health shows not healthy (disconnected).
	// After DisconnectAction executes and collector runs, ConnectionHealth will show "no connection".
	if snap.Status.ConnectionHealth != "healthy" {
		return fsmv2.Transition(&StoppedState{}, fsmv2.SignalNone, nil, "disconnection complete, child stopped", nil)
	}

	return fsmv2.Transition(s, fsmv2.SignalNone, &action.DisconnectAction{}, "closing connections gracefully", nil)
}

func (s *TryingToStopState) String() string {
	return helpers.DeriveStateName(s)
}
