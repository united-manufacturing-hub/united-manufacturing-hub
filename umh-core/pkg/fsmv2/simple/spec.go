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

package simple

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
)

// MonitorSpec is the whole definition of a polling monitor worker: the developer
// fills a struct literal and registers it once via Register in an init(). The
// framework owns the worker struct, the state machine, and the health-verdict
// resolution.
//
// TConfig is the developer's config type, TStatus the polled status type, and
// TDeps the poll dependencies (use struct{} when the poll needs none).
type MonitorSpec[TConfig, TStatus, TDeps any] struct {
	// NewDeps builds the dependency value for one worker instance from its
	// identity. Optional: when set, its result is what Poll receives; when unset,
	// Poll receives TDeps' zero value (use struct{} when the poll needs no
	// dependencies). NewDeps runs once per instance, at construction, and the
	// worker keeps the returned value for its lifetime, so that value may carry
	// per-instance mutable state. Poll receives it by value: state that Poll
	// mutates must sit behind a pointer (use *TDeps, or a pointer field),
	// otherwise the mutation dies with the copy. Poll is never called
	// concurrently with itself for one instance (the observation collector
	// serializes it under a mutex), so per-instance state needs no locking.
	//
	// The framework never releases the returned value. A despawned worker is
	// dropped without any teardown call, so whatever the value holds must be
	// safe to abandon: a buffer or counter is, a connection pool or anything
	// with a background goroutine is not. To share one such resource across
	// every instance, declare it at package level and close over it here;
	// anything constructed inside the builder body is per instance. See
	// pkg/fsmv2/historian for a worked example.
	//
	// It must not fail: NewDeps has no error return, so anything fallible
	// belongs behind Poll.
	NewDeps func(id deps.Identity) TDeps
	// Poll observes the target once and returns the status. d is a copy: TDeps is
	// passed by value, so a resource assigned to a non-pointer field of d is
	// discarded when Poll returns. A non-nil error drives the worker degraded
	// with reason "poll error: <err>"; any status returned alongside the error is
	// still preserved as the Result. Required.
	Poll func(ctx context.Context, d TDeps, cfg TConfig) (TStatus, error)
	// Health turns a good poll's status into a health verdict. Optional: when
	// nil, a good poll is healthy with reason "running (no health check)". Never
	// called on a poll error.
	Health func(cfg TConfig, status TStatus) Health
	// WorkerType is the canonical worker-type name used in config and CSE
	// storage. Required.
	WorkerType string
	// Interval is the poll cadence. Optional: a non-positive value leaves the
	// worker type unregistered so the collector falls back to its default (1s).
	Interval time.Duration
}

// Register wires a MonitorSpec into the framework: it registers the worker
// factory, supervisor, and CSE type (via register.Worker) and the shared initial
// state. Call once per worker type from an init(). Panics on a missing WorkerType
// or Poll, mirroring register.Worker's fail-fast contract.
func Register[TConfig, TStatus, TDeps any](spec MonitorSpec[TConfig, TStatus, TDeps]) {
	if spec.WorkerType == "" {
		panic("simple.Register: WorkerType must be non-empty")
	}

	if spec.Poll == nil {
		panic("simple.Register: Poll must be non-nil")
	}

	// TStatus must be a struct: Status[TStatus] flattens it to top-level JSON and
	// round-trips it through CSE. A map would let the verdict keys leak into the
	// developer's status on Unmarshal, and a scalar would not marshal to an object.
	if k := reflect.TypeFor[TStatus]().Kind(); k != reflect.Struct {
		panic(fmt.Sprintf("simple.Register(%q): TStatus must be a struct, got %s", spec.WorkerType, k))
	}

	register.Worker[TConfig, Status[TStatus], register.NoDeps](spec.WorkerType,
		func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader) (fsmv2.Worker, error) {
			return newSimpleWorker(spec, id, logger, sr)
		})

	fsmv2.RegisterInitialState(spec.WorkerType, &runningState[TConfig, TStatus]{})

	// A non-positive Interval is ignored by the registry, so the collector
	// falls back to its DefaultObservationInterval.
	fsmv2.RegisterObservationInterval(spec.WorkerType, spec.Interval)
}
