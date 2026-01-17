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

package action

import (
	"context"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplefailing/snapshot"
)

const IncrementTicksActionName = "increment_connected_ticks"

// IncrementTicksAction increments the tick counter while in Connected state.
// This is used to track how long the worker has been connected so we can
// trigger the next failure cycle after a configurable number of ticks.
type IncrementTicksAction struct {
}

// Execute increments the tick counter in Connected state.
func (a *IncrementTicksAction) Execute(ctx context.Context, depsAny any) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	deps := depsAny.(snapshot.ExamplefailingDependencies)
	newTicks := deps.IncrementTicksInConnected()
	deps.GetLogger().Infow("increment_ticks_in_connected",
		"new_ticks", newTicks,
		"should_fail", deps.GetShouldFail(),
		"all_cycles_complete", deps.AllCyclesComplete(),
		"current_cycle", deps.GetCurrentCycle(),
		"total_cycles", deps.GetFailureCycles(),
	)

	return nil
}

func (a *IncrementTicksAction) String() string {
	return IncrementTicksActionName
}

func (a *IncrementTicksAction) Name() string {
	return IncrementTicksActionName
}
