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
	"reflect"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper/certificatehandler"
)

var _ fsmv2.Worker = (*CertFetcherWorker)(nil)

// CertFetcherWorker fetches certificates for active subscribers.
type CertFetcherWorker struct {
	fsmv2.WorkerBase[CertFetcherConfig, CertFetcherStatus]
	deps *CertFetcherDependencies
}

// NewCertFetcherWorker creates a new cert fetcher worker.
func NewCertFetcherWorker(
	identity deps.Identity,
	logger deps.FSMLogger,
	stateReader deps.StateReader,
	certHandler certificatehandler.Handler,
) (*CertFetcherWorker, error) {
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	if identity.WorkerType == "" {
		identity.WorkerType = workerType
	}

	w := &CertFetcherWorker{}
	bd := w.InitBase(identity, logger, stateReader)

	d, err := NewCertFetcherDependencies(certHandler, bd)
	if err != nil {
		return nil, fmt.Errorf("failed to create dependencies: %w", err)
	}

	w.deps = d
	return w, nil
}

// GetDependenciesAny returns the custom CertFetcherDependencies.
func (w *CertFetcherWorker) GetDependenciesAny() any {
	return w.deps
}

// CollectObservedState snapshots the cert fetcher state.
func (w *CertFetcherWorker) CollectObservedState(ctx context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	d := w.deps
	ch := d.CertHandler()
	emails := ch.Subscribers()

	cachedCount := 0
	for _, email := range emails {
		if ch.Certificate(email) != nil {
			cachedCount++
		}
	}

	// set metrics for potential later exposure on the UI
	d.MetricsRecorder().IncrementCounter("cached_certs", int64(cachedCount))

	return fsmv2.NewObservation(CertFetcherStatus{
		ConsecutiveErrors: d.ConsecutiveErrors(),
		SubscriberCount:   len(emails),
		CachedCertCount:   cachedCount,
		LastFetchAt:       d.LastFetchAt(),
		HasSubHandler:     ch.HasSubHandler(),
	}), nil
}

const workerType = "certfetcher"

func init() {
	if err := factory.RegisterWorkerAndSupervisorFactoryByType(
		workerType,
		func(id deps.Identity, logger deps.FSMLogger, stateReader deps.StateReader, extraDeps map[string]any) fsmv2.Worker {
			certHandlerRaw, ok := extraDeps["certHandler"]
			if !ok || certHandlerRaw == nil {
				panic("certfetcher worker requires certHandler in extraDeps")
			}
			certHandler, ok := certHandlerRaw.(certificatehandler.Handler)
			if !ok {
				panic("certHandler must implement certificatehandler.Handler")
			}

			worker, err := NewCertFetcherWorker(id, logger, stateReader, certHandler)
			if err != nil {
				panic(fmt.Sprintf("failed to create certfetcher worker: %v", err))
			}

			return worker
		},
		func(cfg interface{}) interface{} {
			return supervisor.NewSupervisor[fsmv2.Observation[CertFetcherStatus], *fsmv2.WrappedDesiredState[CertFetcherConfig]](
				cfg.(supervisor.Config))
		},
	); err != nil {
		panic(fmt.Sprintf("failed to register certfetcher worker: %v", err))
	}

	observedType := reflect.TypeOf(fsmv2.Observation[CertFetcherStatus]{})
	desiredType := reflect.TypeOf(fsmv2.WrappedDesiredState[CertFetcherConfig]{})

	if err := storage.GlobalRegistry().RegisterWorkerType(workerType, observedType, desiredType); err != nil {
		panic(fmt.Sprintf("failed to register certfetcher CSE types: %v", err))
	}
}
