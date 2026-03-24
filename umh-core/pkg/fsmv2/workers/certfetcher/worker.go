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
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/certfetcher/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/certfetcher/state"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/gatekeeper/certificatehandler"
)

// lazySubHandler resolves the SubHandler lazily via a provider callback.
type lazySubHandler struct {
	provider func() gatekeeper.SubHandler
}

func (l *lazySubHandler) Subscribers() []string {
	sh := l.provider()
	if sh == nil {
		return nil
	}
	return sh.Subscribers()
}

// IsReady returns true when the underlying subscriber handler is available.
func (l *lazySubHandler) IsReady() bool {
	return l.provider() != nil
}

var _ fsmv2.Worker = (*CertFetcherWorker)(nil)

// CertFetcherWorker fetches certificates for active subscribers.
type CertFetcherWorker struct {
	*helpers.BaseWorker[*CertFetcherDependencies]
	logger   deps.FSMLogger
	identity deps.Identity
}

// NewCertFetcherWorker creates a new cert fetcher worker.
func NewCertFetcherWorker(
	identity deps.Identity,
	logger deps.FSMLogger,
	stateReader deps.StateReader,
	subHandler gatekeeper.SubHandler,
	certHandler certificatehandler.Handler,
) (*CertFetcherWorker, error) {
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	if identity.WorkerType == "" {
		workerType, err := storage.DeriveWorkerType[snapshot.CertFetcherObservedState]()
		if err != nil {
			return nil, fmt.Errorf("failed to derive worker type: %w", err)
		}
		identity.WorkerType = workerType
	}

	d, err := NewCertFetcherDependencies(subHandler, certHandler, identity, logger, stateReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create dependencies: %w", err)
	}

	return &CertFetcherWorker{
		BaseWorker: helpers.NewBaseWorker(d),
		identity:   identity,
		logger:     logger,
	}, nil
}

// CollectObservedState snapshots the cert fetcher state.
func (w *CertFetcherWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	d := w.GetDependencies()
	emails := d.Subscribers()

	cachedCount := 0
	for _, email := range emails {
		if d.Certificate(email) != nil {
			cachedCount++
		}
	}

	observed := snapshot.CertFetcherObservedState{
		CollectedAt:       time.Now(),
		ConsecutiveErrors: d.ConsecutiveErrors(),
		SubscriberCount:   len(emails),
		CachedCertCount:   cachedCount,
		LastFetchAt:       d.LastFetchAt(),
		HasSubHandler:     d.HasSubHandler(),
	}

	if fm := d.GetFrameworkState(); fm != nil {
		observed.Metrics.Framework = *fm
	}

	observed.LastActionResults = d.GetActionHistory()

	return observed, nil
}

// DeriveDesiredState returns running state (standalone worker, always wants to run).
func (w *CertFetcherWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	if spec == nil {
		return &snapshot.CertFetcherDesiredState{
			BaseDesiredState: config.BaseDesiredState{State: config.DesiredStateRunning},
		}, nil
	}

	userSpec, ok := spec.(config.UserSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec type: expected UserSpec, got %T", spec)
	}

	leafDesired, err := config.DeriveLeafState[config.BaseUserSpec](userSpec)
	if err != nil {
		return nil, err
	}

	return &snapshot.CertFetcherDesiredState{
		BaseDesiredState: leafDesired.BaseDesiredState,
	}, nil
}

// GetInitialState returns the stopped state.
func (w *CertFetcherWorker) GetInitialState() fsmv2.State[any, any] {
	return &state.StoppedState{}
}

func init() {
	if err := factory.RegisterWorkerType[snapshot.CertFetcherObservedState, *snapshot.CertFetcherDesiredState](
		func(id deps.Identity, logger deps.FSMLogger, stateReader deps.StateReader, extraDeps map[string]any) fsmv2.Worker {
			certHandlerRaw, ok := extraDeps["certHandler"]
			if !ok || certHandlerRaw == nil {
				panic("certfetcher worker requires certHandler in extraDeps")
			}
			certHandler, ok := certHandlerRaw.(certificatehandler.Handler)
			if !ok {
				panic("certHandler must implement certificatehandler.Handler")
			}

			// SubHandler resolved lazily via provider callback.
			var subHandler gatekeeper.SubHandler
			if provider, ok := extraDeps["subHandlerProvider"]; ok && provider != nil {
				providerFn, ok := provider.(func() gatekeeper.SubHandler)
				if !ok {
					panic("subHandlerProvider must be func() gatekeeper.SubHandler")
				}
				subHandler = &lazySubHandler{provider: providerFn}
			}

			worker, err := NewCertFetcherWorker(id, logger, stateReader, subHandler, certHandler)
			if err != nil {
				panic(fmt.Sprintf("failed to create certfetcher worker: %v", err))
			}

			return worker
		},
		func(cfg interface{}) interface{} {
			return supervisor.NewSupervisor[snapshot.CertFetcherObservedState, *snapshot.CertFetcherDesiredState](
				cfg.(supervisor.Config))
		},
	); err != nil {
		panic(fmt.Sprintf("failed to register certfetcher worker: %v", err))
	}
}
