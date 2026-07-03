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

// Spec is the whole definition of a polling monitor worker: the developer fills
// a struct literal and registers it once via Register in an init(). The
// framework owns the worker struct, the state machine, and (later rungs)
// freshness and health-verdict resolution (ENG-5305).
//
// TConfig is the developer's config type, TStatus the polled status type, and
// TDeps the poll dependencies (use struct{} when the poll needs none).
type Spec[TConfig, TStatus, TDeps any] struct {
	// WorkerType is the canonical worker-type name used in config and CSE
	// storage. Required.
	WorkerType string
	// Poll observes the target once and returns the status. A non-nil error
	// drives the worker degraded with reason "poll error: <err>". Required.
	Poll func(ctx context.Context, d TDeps, cfg TConfig) (TStatus, error)
	// Health turns a good poll's status into a health verdict. Optional: when
	// nil, a good poll is healthy with reason "running (no health check)". Never
	// called on a poll error.
	Health func(cfg TConfig, status TStatus) Health
	// Deps is the dependency value passed to every Poll. Optional: use struct{}
	// when the poll needs none. It is shared across ticks and instances, so it
	// must be stateless (e.g. an *http.Client, not a per-tick buffer).
	Deps TDeps
	// Interval is the poll cadence. Optional: a non-positive value leaves the
	// worker type unregistered so the collector falls back to its default (1s).
	Interval time.Duration
}

// Register wires a Spec into the framework: it registers the worker factory,
// supervisor, and CSE type (via register.Worker) and the shared initial state.
// Call once per worker type from an init(). Panics on a missing WorkerType or
// Poll, mirroring register.Worker's fail-fast contract.
func Register[TConfig, TStatus, TDeps any](spec Spec[TConfig, TStatus, TDeps]) {
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
