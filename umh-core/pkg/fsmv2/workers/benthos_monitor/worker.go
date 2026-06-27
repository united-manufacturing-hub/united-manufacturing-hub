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

// Package benthos_monitor provides the FSMv2 benthos monitor worker, which
// scrapes a benthos instance's /ping, /ready, /version and /metrics HTTP
// endpoints each tick and publishes the parsed Scan as its observed state.
package benthos_monitor

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthosmetrics"
)

// WorkerTypeName is the canonical worker-type identifier for the benthos
// monitor worker, registered in init() and used as the folder name.
const WorkerTypeName = "benthos_monitor"

// BenthosMonitorWorker implements the FSMv2 Worker interface. Each tick it
// scrapes the configured benthos instance and publishes a
// BenthosMonitorStatus carrying the parsed Scan.
type BenthosMonitorWorker struct {
	client *http.Client

	fsmv2.WorkerBase[BenthosMonitorConfig, BenthosMonitorStatus, *BenthosMonitorDependencies]
}

// NewBenthosMonitorWorker creates a new benthos monitor worker. The client
// argument may be nil, in which case a default HTTP client is used.
func NewBenthosMonitorWorker(
	identity deps.Identity,
	logger deps.FSMLogger,
	stateReader deps.StateReader,
	client *http.Client,
) (*BenthosMonitorWorker, error) {
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	w := &BenthosMonitorWorker{
		client: client,
	}
	if w.client == nil {
		w.client = &http.Client{Timeout: 5 * time.Second}
	}

	bd := w.InitBase(identity, logger, stateReader)
	workerDeps := NewBenthosMonitorDependencies(bd)
	w.BindDeps(workerDeps)

	return w, nil
}

// GetDependencies returns the typed BenthosMonitorDependencies. Panics with a
// clear message if BindDeps was not called before this worker is used.
func (w *BenthosMonitorWorker) GetDependencies() *BenthosMonitorDependencies {
	raw := w.GetDependenciesAny()

	d, ok := raw.(*BenthosMonitorDependencies)
	if !ok || d == nil {
		panic("BenthosMonitorWorker: GetDependencies called before BindDeps")
	}

	return d
}

// CollectObservedState scrapes the benthos instance configured on the desired
// state and publishes a BenthosMonitorStatus carrying the parsed Scan.
//
// When cfg.State is "stopped" the method returns a zero Scan
// (MetricsAvailable=false, IsLive=false) without calling benthosmetrics.Observe
// — no /ping, /ready, /version or /metrics request is issued. Any other state
// scrapes as before. The zero Scan here is not a crash or unreachable signal:
// the benthos process is intentionally stopped, so there is nothing to scrape,
// and the admin-pause status is carried by desired State and framework
// lifecycle rather than by the Scan.
//
// Uses fsmv2.NewObservation which signals the supervisor's collector to perform
// post-COS wrapping (CollectedAt, framework metrics, action history). A
// canceled context is propagated as context.Canceled alongside a nil
// observation; a down benthos is an observed state and yields a nil error.
func (w *BenthosMonitorWorker) CollectObservedState(ctx context.Context, desired fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	cfg := fsmv2.ExtractConfig[BenthosMonitorConfig](desired)

	if cfg.State == "stopped" {
		return fsmv2.NewObservation(BenthosMonitorStatus{Scan: benthosmetrics.Scan{}}), nil
	}

	scan, err := benthosmetrics.Observe(ctx, w.client, cfg.MetricsPort)
	if err != nil {
		return nil, err
	}

	return fsmv2.NewObservation(BenthosMonitorStatus{Scan: scan}), nil
}

func init() {
	register.Worker[BenthosMonitorConfig, BenthosMonitorStatus, *BenthosMonitorDependencies](WorkerTypeName,
		func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader) (fsmv2.Worker, error) {
			return NewBenthosMonitorWorker(id, logger, sr, nil)
		})
}

// ensure BenthosMonitorWorker implements Worker interfaces (compile-time check).
var _ fsmv2.Worker = (*BenthosMonitorWorker)(nil)
