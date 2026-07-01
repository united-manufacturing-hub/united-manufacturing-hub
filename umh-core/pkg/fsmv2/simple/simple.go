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
// The framework manages started/stopped lifecycle, interval tracking, and
// result storage automatically.
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
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
)

// Status wraps the user-defined status type with the last result produced by
// the user function.
type Status[TStatus any] struct {
	// Inner holds the result of the last successful user function call.
	Inner TStatus `json:"inner"`
}

// --- private dependencies ---

// simpleDeps holds mutable state shared between CollectObservedState ticks.
// All fields are protected by mu.
type simpleDeps[TConfig any, TStatus any] struct {
	*deps.BaseDependencies

	fn        func(ctx context.Context, cfg TConfig) (TStatus, error)
	mu        sync.RWMutex
	cfg       TConfig
	lastResult TStatus
	lastRunAt time.Time
	interval  time.Duration
}

func newSimpleDeps[TConfig any, TStatus any](
	bd *deps.BaseDependencies,
	fn func(ctx context.Context, cfg TConfig) (TStatus, error),
	interval time.Duration,
) *simpleDeps[TConfig, TStatus] {
	return &simpleDeps[TConfig, TStatus]{
		BaseDependencies: bd,
		fn:               fn,
		interval:         interval,
	}
}

func (d *simpleDeps[TConfig, TStatus]) setConfig(cfg TConfig) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.cfg = cfg
}

func (d *simpleDeps[TConfig, TStatus]) getConfig() TConfig {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.cfg
}

func (d *simpleDeps[TConfig, TStatus]) shouldRun() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.lastRunAt.IsZero() || time.Since(d.lastRunAt) >= d.interval
}

func (d *simpleDeps[TConfig, TStatus]) setResult(r TStatus) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.lastResult = r
	d.lastRunAt = time.Now()
}

// markRunAttempt records the current time regardless of success, so a failed
// fn call still advances the interval instead of hammering on every tick.
func (d *simpleDeps[TConfig, TStatus]) markRunAttempt() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.lastRunAt = time.Now()
}

func (d *simpleDeps[TConfig, TStatus]) getResult() TStatus {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.lastResult
}

// --- worker ---

type simpleWorker[TConfig any, TStatus any] struct {
	fsmv2.WorkerBase[TConfig, Status[TStatus], *simpleDeps[TConfig, TStatus]]
}

func (w *simpleWorker[TConfig, TStatus]) GetDependencies() *simpleDeps[TConfig, TStatus] {
	raw := w.GetDependenciesAny()

	d, ok := raw.(*simpleDeps[TConfig, TStatus])
	if !ok || d == nil {
		panic("simpleWorker: GetDependencies called before BindDeps")
	}

	return d
}

// CollectObservedState extracts the current config from the desired state and,
// when the configured interval has elapsed, calls the user function in-place.
// The result is returned as the observation; no action dispatch is needed.
func (w *simpleWorker[TConfig, TStatus]) CollectObservedState(ctx context.Context, desired fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	d := w.GetDependencies()

	cfg := fsmv2.ExtractConfig[TConfig](desired)
	d.setConfig(cfg)

	if d.shouldRun() {
		d.markRunAttempt()

		result, err := d.fn(ctx, d.getConfig())
		if err != nil {
			return nil, err
		}

		d.setResult(result)
	}

	return fsmv2.NewObservation(Status[TStatus]{Inner: d.getResult()}), nil
}

// GetInitialState returns a fresh stoppedState without touching the global
// initial-state registry, so simple workers do not pollute the registry with
// per-instance generic instantiations.
func (w *simpleWorker[TConfig, TStatus]) GetInitialState() fsmv2.State[any, any] {
	return &stoppedState[TConfig, TStatus]{}
}

// --- states ---

type stoppedState[TC any, TS any] struct {
	helpers.StoppedBase
}

func (s *stoppedState[TC, TS]) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[TC, Status[TS]](snapAny)

	if snap.IsShutdownRequested {
		return fsmv2.Transition(s, fsmv2.SignalNeedsRemoval, nil,
			"shutdown requested while stopped", nil)
	}

	return fsmv2.Transition(&runningState[TC, TS]{}, fsmv2.SignalNone, nil,
		"starting worker", nil)
}

func (s *stoppedState[TC, TS]) String() string { return "stopped" }

type runningState[TC any, TS any] struct {
	helpers.RunningHealthyBase
}

func (s *runningState[TC, TS]) Next(snapAny any) fsmv2.NextResult[any, any] {
	snap := fsmv2.ConvertWorkerSnapshot[TC, Status[TS]](snapAny)

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
	snap := fsmv2.ConvertWorkerSnapshot[TC, Status[TS]](snapAny)

	return fsmv2.Transition(&stoppedState[TC, TS]{}, fsmv2.SignalNeedsRemoval, nil,
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
	register.Worker[TConfig, Status[TStatus], *simpleDeps[TConfig, TStatus]](workerType,
		func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader) (fsmv2.Worker, error) {
			w := &simpleWorker[TConfig, TStatus]{}
			bd := w.InitBase(id, logger, sr)
			d := newSimpleDeps(bd, fn, interval)
			w.BindDeps(d)

			return w, nil
		})
}
