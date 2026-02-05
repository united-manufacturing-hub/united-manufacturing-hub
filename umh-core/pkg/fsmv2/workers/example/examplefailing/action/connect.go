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
	"errors"
	"time"

	depspkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplefailing/snapshot"
)

const ConnectActionName = "connect"

// ErrSimulatedFailure is returned when the action is configured to fail.
var ErrSimulatedFailure = errors.New("simulated connection failure")

// ConnectAction establishes a connection to an external resource.
// This action can be configured to fail predictably for testing FSM error handling.
//
// When ShouldFail is true in dependencies, the action will fail for the first
// MaxFailures attempts, then succeed. This demonstrates exponential backoff
// and recovery behavior.
type ConnectAction struct {
}

// Execute attempts to acquire a connection from the pool.
// Returns ErrSimulatedFailure when configured to fail and attempt count < MaxFailures.
// Supports multiple failure cycles: each cycle fails MaxFailures times before succeeding.
func (a *ConnectAction) Execute(ctx context.Context, depsAny any) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	deps := depsAny.(snapshot.ExamplefailingDependencies)
	logger := deps.GetLogger()

	if deps.GetShouldFail() && !deps.AllCyclesComplete() {
		attempts := deps.IncrementAttempts()
		maxFailures := deps.GetMaxFailures()
		currentCycle := deps.GetCurrentCycle()
		totalCycles := deps.GetFailureCycles()

		logger.Info("connect_attempting",
			depspkg.Int("attempt", attempts),
			depspkg.Int("max_failures", maxFailures),
			depspkg.Bool("should_fail", true),
			depspkg.Int("current_cycle", currentCycle+1),
			depspkg.Int("total_cycles", totalCycles),
		)

		if attempts <= maxFailures {
			logger.Warn("connect_failed_simulated",
				depspkg.Int("attempt", attempts),
				depspkg.Int("max_failures", maxFailures),
				depspkg.Int("remaining", maxFailures-attempts),
				depspkg.Int("current_cycle", currentCycle+1),
				depspkg.Int("total_cycles", totalCycles),
			)
			// Record failure time for recovery delay and reset observation counter.
			// The observation counter tracks how many CollectObservedState calls have
			// happened since this failure - resetting it starts the delay window fresh.
			deps.SetLastFailureTime(time.Now())
			deps.ResetObservationsSinceFailure()

			return ErrSimulatedFailure
		}

		logger.Info("connect_succeeded_after_failures",
			depspkg.Int("total_attempts", attempts),
			depspkg.Int("current_cycle", currentCycle+1),
			depspkg.Int("total_cycles", totalCycles),
			depspkg.Bool("more_cycles_remaining", currentCycle+1 < totalCycles),
		)
		deps.SetConnected(true)
		deps.ResetTicksInConnected()

		return nil
	}

	logger.Info("connect_succeeded")
	deps.SetConnected(true)
	deps.ResetTicksInConnected()

	return nil
}

func (a *ConnectAction) String() string {
	return ConnectActionName
}

func (a *ConnectAction) Name() string {
	return ConnectActionName
}
