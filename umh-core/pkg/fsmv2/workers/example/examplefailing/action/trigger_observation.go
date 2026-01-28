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

const TriggerObservationActionName = "trigger_observation"

// TriggerObservationAction triggers an observation cycle via action completion.
// This is used to make the worker visible to parent supervisors by ensuring
// the collector's OnActionComplete callback is triggered, which causes an
// immediate observation rather than waiting for the next periodic interval.
//
// The action also increments the tick counter while in Connected state,
// which can be used to track how long the worker has been connected.
type TriggerObservationAction struct {
}

// Execute triggers an observation by completing successfully.
// The act of completing this action causes the collector to trigger TriggerNow(),
// making this worker's state immediately visible to parent supervisors.
func (a *TriggerObservationAction) Execute(ctx context.Context, depsAny any) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	deps := depsAny.(snapshot.ExamplefailingDependencies)
	newTicks := deps.IncrementTicksInConnected()
	deps.GetLogger().Infow("trigger_observation",
		"new_ticks", newTicks,
		"should_fail", deps.GetShouldFail(),
		"all_cycles_complete", deps.AllCyclesComplete(),
		"current_cycle", deps.GetCurrentCycle(),
		"total_cycles", deps.GetFailureCycles(),
	)

	return nil
}

func (a *TriggerObservationAction) String() string {
	return TriggerObservationActionName
}

func (a *TriggerObservationAction) Name() string {
	return TriggerObservationActionName
}
