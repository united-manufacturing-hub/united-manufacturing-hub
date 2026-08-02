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
	"errors"
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// simpleWorker runs a MonitorSpec's Poll on the framework's collection cadence.
// It holds the immutable MonitorSpec; per-instance mutable state lives in
// WorkerBase's deps slot, never in the worker struct, so the same logic serves
// every simple worker type.
//
// The framework-facing status is Status[TStatus]: the developer's poll result
// wrapped with the health verdict. The bound deps are the author's own TDeps,
// which is what Poll receives.
type simpleWorker[TConfig, TStatus, TDeps any] struct {
	spec MonitorSpec[TConfig, TStatus, TDeps]
	fsmv2.WorkerBase[TConfig, Status[TStatus], TDeps]
}

// newSimpleWorker builds a simpleWorker from its MonitorSpec and framework deps.
func newSimpleWorker[TConfig, TStatus, TDeps any](
	spec MonitorSpec[TConfig, TStatus, TDeps],
	id deps.Identity,
	logger deps.FSMLogger,
	sr deps.StateReader,
) (*simpleWorker[TConfig, TStatus, TDeps], error) {
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	w := &simpleWorker[TConfig, TStatus, TDeps]{spec: spec}

	bd := w.InitBase(id, logger, sr)

	if spec.NewDeps != nil {
		w.BindDeps(spec.NewDeps(id, bd))
	}

	return w, nil
}

// pollDeps returns the value Poll receives. A spec with no NewDeps never binds,
// and WorkerBase then hands back TDeps' zero value, which is what Poll expects.
func (w *simpleWorker[TConfig, TStatus, TDeps]) pollDeps() TDeps {
	d, _ := w.GetDependenciesAny().(TDeps)

	return d
}

// reasonNoHealthCheck is the verdict reason for a good poll on a worker that
// declared no Health function.
const reasonNoHealthCheck = "running (no health check)"

// CollectObservedState runs the two-phase Poll → Health cycle and returns an
// Observation carrying the polled status plus the health verdict.
//
// Poll runs first. On a Poll error the worker is degraded with reason
// "poll error: <err>" and Health is NOT called — the error is persisted as a
// verdict on the Observation rather than returned, so the fsmv1 layer sees a
// degraded worker with a reason instead of "starting" forever. Any partial
// status the failed Poll returned is preserved as the Result, so a poll that
// observed detail before failing (e.g. reachable-but-unauthenticated) still
// surfaces it. On a good poll
// the optional Health function decides the verdict; when it is nil the worker
// is healthy with reason "running (no health check)".
func (w *simpleWorker[TConfig, TStatus, TDeps]) CollectObservedState(ctx context.Context, desired fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	cfg := fsmv2.ExtractConfig[TConfig](desired)

	status, err := w.spec.Poll(ctx, w.pollDeps(), cfg)
	if err != nil {
		return fsmv2.NewObservation(Status[TStatus]{
			// We can use status here as the result, even on error, to preserve
			// any partial state the failed poll returned.
			Result:   status,
			Degraded: true,
			Reason:   fmt.Sprintf("poll error: %v", err),
		}), nil
	}

	verdict := Healthy(reasonNoHealthCheck)
	if w.spec.Health != nil {
		verdict = w.spec.Health(cfg, status)
	}

	return fsmv2.NewObservation(Status[TStatus]{
		Result:   status,
		Degraded: verdict.Degraded,
		Reason:   verdict.Reason,
	}), nil
}
