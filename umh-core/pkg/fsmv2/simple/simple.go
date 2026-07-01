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

// Package simple provides a periodic worker that eliminates the boilerplate
// state, action, and deps files required for workers that only need to run a
// function on a fixed interval.
//
// Callers register a worker type with Register[TConfig, TStatus], supplying
// a function called once per interval with the current config and a context.
// The framework manages started/stopped lifecycle automatically; interval
// tracking and result storage live directly on the worker.
//
// Example:
//
//	simple.Register[MyConfig, MyResult]("myworker", 5*time.Second,
//	    func(ctx context.Context, cfg MyConfig) (MyResult, error) {
//	        return doWork(ctx, cfg)
//	    })
package simple

import (
	"context"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
)

// --- worker ---

// simpleWorker is a no-deps worker that calls fn once per interval in
// CollectObservedState. All mutable state lives directly on the struct; no
// separate deps layer is needed because COS runs single-threaded in the
// worker's own goroutine.
type simpleWorker[TConfig any, TStatus any] struct {
	fsmv2.WorkerBase[TConfig, TStatus, struct{}]

	fn         func(ctx context.Context, cfg TConfig) (TStatus, error)
	lastResult TStatus
	lastRunAt  time.Time
	interval   time.Duration
}

// GetDependenciesAny returns nil so the framework does not inject metrics into
// a struct{} deps box (WorkerBase boxes struct{}{} into a non-nil any, which
// silently skips injection without this override).
func (w *simpleWorker[TConfig, TStatus]) GetDependenciesAny() any { return nil }

// GetInitialState returns a fresh stoppedState without touching the global
// initial-state registry, so simple workers do not pollute the registry with
// per-instance generic instantiations.
func (w *simpleWorker[TConfig, TStatus]) GetInitialState() fsmv2.State[any, any] {
	return &stoppedState[TConfig, TStatus]{}
}

// CollectObservedState reads the current config from the desired state and,
// when the configured interval has elapsed, calls fn in-place. The result is
// returned as the observation; no action dispatch is needed.
func (w *simpleWorker[TConfig, TStatus]) CollectObservedState(ctx context.Context, desired fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	cfg := fsmv2.ExtractConfig[TConfig](desired)

	if w.lastRunAt.IsZero() || time.Since(w.lastRunAt) >= w.interval {
		// Record the attempt before calling fn so a failure still advances the
		// interval and avoids hammering a broken dependency on every tick.
		w.lastRunAt = time.Now()

		result, err := w.fn(ctx, cfg)
		if err != nil {
			return nil, err
		}

		w.lastResult = result
	}

	return fsmv2.NewObservation(w.lastResult), nil
}

// --- states ---

type stoppedState[TC any, TS any] struct {
	helpers.StoppedBase
}

func (s *stoppedState[TC, TS]) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[TC, TS](snapAny)

	if snap.IsShutdownRequested {
		return fsmv2.Transition(s, fsmv2.SignalNeedsRemoval, nil,
			"shutdown requested while stopped", nil)
	}

	if snap.IsDisabled {
		return fsmv2.Transition(s, fsmv2.SignalNone, nil,
			"disabled, staying resident", nil)
	}

	return fsmv2.Transition(&runningState[TC, TS]{}, fsmv2.SignalNone, nil,
		"starting worker", nil)
}

func (s *stoppedState[TC, TS]) String() string { return "stopped" }

type runningState[TC any, TS any] struct {
	helpers.RunningHealthyBase
}

func (s *runningState[TC, TS]) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[TC, TS](snapAny)

	if snap.ShouldStop() {
		return fsmv2.Transition(&stoppingState[TC, TS]{}, fsmv2.SignalNone, nil,
			"stop required: "+snap.StopReason(), nil)
	}

	return fsmv2.Transition(s, fsmv2.SignalNone, nil, "running", nil)
}

func (s *runningState[TC, TS]) String() string { return "running" }

type stoppingState[TC any, TS any] struct {
	helpers.StoppingBase
}

func (s *stoppingState[TC, TS]) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[TC, TS](snapAny)

	if snap.IsShutdownRequested {
		return fsmv2.Transition(&stoppedState[TC, TS]{}, fsmv2.SignalNeedsRemoval, nil,
			"stopping complete: "+snap.StopReason(), nil)
	}

	// Disabled-only stop: stay resident in stoppedState so the worker can resume
	// when re-enabled, rather than being removed entirely.
	return fsmv2.Transition(&stoppedState[TC, TS]{}, fsmv2.SignalNone, nil,
		"stopping complete: "+snap.StopReason(), nil)
}

func (s *stoppingState[TC, TS]) String() string { return "stopping" }

// --- public registration ---

// Register registers a simple periodic worker under workerType.
// fn is called with the current config once per interval.
// The worker type string must be unique across the process.
func Register[TConfig any, TStatus any](
	workerType string,
	interval time.Duration,
	fn func(ctx context.Context, cfg TConfig) (TStatus, error),
) {
	register.Worker[TConfig, TStatus, struct{}](workerType,
		func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader) (fsmv2.Worker, error) {
			w := &simpleWorker[TConfig, TStatus]{
				fn:       fn,
				interval: interval,
			}
			w.InitBase(id, logger, sr)

			return w, nil
		})
}
