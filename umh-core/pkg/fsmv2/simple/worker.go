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

// simpleWorker runs a Spec's Poll on the framework's collection cadence. It
// holds only the immutable Spec: the worker carries no mutable state, so the
// same logic serves every simple worker type. TDeps is the worker's WorkerBase
// deps sentinel (struct{}); the Spec's own TDeps flows to Poll, not through
// WorkerBase.
type simpleWorker[TConfig, TStatus, TDeps any] struct {
	fsmv2.WorkerBase[TConfig, TStatus, struct{}]
	spec Spec[TConfig, TStatus, TDeps]
}

// newSimpleWorker builds a simpleWorker from its Spec and framework deps.
func newSimpleWorker[TConfig, TStatus, TDeps any](
	spec Spec[TConfig, TStatus, TDeps],
	id deps.Identity,
	logger deps.FSMLogger,
	sr deps.StateReader,
) (*simpleWorker[TConfig, TStatus, TDeps], error) {
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	w := &simpleWorker[TConfig, TStatus, TDeps]{spec: spec}
	w.InitBase(id, logger, sr)

	return w, nil
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
// degraded worker with a reason instead of "starting" forever. On a good poll
// the optional Health function decides the verdict; when it is nil the worker
// is healthy with reason "running (no health check)".
func (w *simpleWorker[TConfig, TStatus, TDeps]) CollectObservedState(ctx context.Context, desired fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	cfg := fsmv2.ExtractConfig[TConfig](desired)

	status, err := w.spec.Poll(ctx, w.spec.Deps, cfg)
	if err != nil {
		return fsmv2.Observation[TStatus]{
			Degraded: true,
			Reason:   fmt.Sprintf("poll error: %v", err),
		}, nil
	}

	verdict := Healthy(reasonNoHealthCheck)
	if w.spec.Health != nil {
		verdict = w.spec.Health(cfg, status)
	}

	return fsmv2.Observation[TStatus]{
		Status:   status,
		Degraded: verdict.Degraded,
		Reason:   verdict.Reason,
	}, nil
}

// GetDependenciesAny returns a true nil: simpleWorker has no per-instance
// framework deps, and WorkerBase[..., struct{}] would otherwise box struct{}{}
// into a non-nil any, silently skipping the collector's metrics injection.
func (w *simpleWorker[TConfig, TStatus, TDeps]) GetDependenciesAny() any {
	return nil
}
