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

// Package certfetcher implements an FSMv2 worker that periodically fetches
// certificates for active subscribers from the Management Console API.
package certfetcher

import (
	"context"
	"errors"
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper/certificatehandler"
)

// WorkerTypeName is the canonical worker-type identifier for the certfetcher worker.
const WorkerTypeName = "certfetcher"

const workerType = WorkerTypeName

// CertHandlerKey names the certificatehandler.Handler a CertFetcherWorker is
// built from.
var CertHandlerKey = config.NewDependencyKey[certificatehandler.Handler]("certfetcher.cert_handler")

var _ fsmv2.Worker = (*CertFetcherWorker)(nil)

// CertFetcherWorker fetches certificates for active subscribers.
type CertFetcherWorker struct {
	fsmv2.WorkerBase[CertFetcherConfig, CertFetcherStatus, *CertFetcherDependencies]
}

// NewCertFetcherWorker creates a new cert fetcher worker.
// The dependency map must hold a certificatehandler.Handler under
// CertHandlerKey; otherwise it returns an error naming the key.
func NewCertFetcherWorker(
	identity deps.Identity,
	logger deps.FSMLogger,
	stateReader deps.StateReader,
	dependencies map[string]any,
) (*CertFetcherWorker, error) {
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	if identity.WorkerType == "" {
		identity.WorkerType = workerType
	}

	handler, ok := config.LookupDependency(dependencies, CertHandlerKey)
	if !ok {
		return nil, fmt.Errorf("certfetcher: no cert handler under %q in the dependency map", CertHandlerKey.Name())
	}

	w := &CertFetcherWorker{}
	bd := w.InitBase(identity, logger, stateReader)

	d, err := NewCertFetcherDependencies(handler, bd)
	if err != nil {
		return nil, fmt.Errorf("failed to create dependencies: %w", err)
	}

	w.BindDeps(d)

	return w, nil
}

// GetDependencies returns the typed certfetcher dependencies.
// Panics with a clear message if BindDeps was not called before this worker is used.
func (w *CertFetcherWorker) GetDependencies() *CertFetcherDependencies {
	raw := w.GetDependenciesAny()

	d, ok := raw.(*CertFetcherDependencies)
	if !ok || d == nil {
		panic("CertFetcherWorker: GetDependencies called before BindDeps")
	}

	return d
}

// CollectObservedState snapshots the cert fetcher state.
func (w *CertFetcherWorker) CollectObservedState(ctx context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	d := w.GetDependencies()
	ch := d.CertHandler()
	emails := ch.Subscribers()

	cachedCount := 0

	for _, email := range emails {
		if ch.Certificate(email) != nil {
			cachedCount++
		}
	}

	// Current-state gauges for potential later exposure on the UI. These are
	// point-in-time values (not cumulative events), so they use SetGauge —
	// IncrementCounter would accumulate additively on every COS tick.
	d.MetricsRecorder().SetGauge(deps.GaugeCachedCerts, float64(cachedCount))
	d.MetricsRecorder().SetGauge(deps.GaugeConsecutiveErrors, float64(d.ConsecutiveErrors()))

	return fsmv2.NewObservation(CertFetcherStatus{
		ConsecutiveErrors: d.ConsecutiveErrors(),
		SubscriberCount:   len(emails),
		CachedCertCount:   cachedCount,
		LastFetchAt:       d.LastFetchAt(),
		HasSubHandler:     ch.HasSubHandler(),
	}), nil
}

func init() {
	register.Worker[CertFetcherConfig, CertFetcherStatus, *CertFetcherDependencies](WorkerTypeName,
		func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader, m map[string]any) (fsmv2.Worker, error) {
			return NewCertFetcherWorker(id, logger, sr, m)
		})
}
