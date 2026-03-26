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

// Package communicator implements the CommunicatorWorker, a parent orchestrator
// that manages bidirectional message exchange between Edge and Backend tiers.
//
// # Architecture
//
// CommunicatorWorker delegates all transport operations to a TransportWorker child.
// TransportWorker handles authentication, push, pull, backoff, and transport reset.
// CommunicatorWorker monitors child health and manages lifecycle transitions.
//
// Channel sharing: Both communicator and transport packages use a ChannelProvider
// singleton to supply inbound and outbound message channels. Call
// communicator.SetChannelProvider() and transport.SetChannelProvider() before
// starting the supervisor.
//
// # Worker API v2
//
// This package uses the WorkerBase[TConfig, TStatus] API with custom overrides:
//   - Custom CollectObservedState: builds status from deps, delegates metric accumulation to WrapStatusAccumulated
//   - Post-parse hook: applies timeout default
//   - ChildSpecFactory: produces TransportWorker child specs
//
// # States and Transitions
//
// State flow:
//
//	Stopped → Syncing ↔ Recovering → Stopped
//
// TransportWorker runs as a child when the parent is in Syncing or Recovering.
package communicator

import (
	"context"
	"errors"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
)

// CommunicatorWorker implements the FSMv2 Worker interface using the WorkerBase API.
type CommunicatorWorker struct {
	deps *CommunicatorDependencies
	fsmv2.WorkerBase[CommunicatorConfig, CommunicatorStatus]
}

// NewCommunicatorWorker creates a new communicator worker with the standard framework dependencies.
func NewCommunicatorWorker(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader) (fsmv2.Worker, error) {
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	w := &CommunicatorWorker{}
	baseDeps := w.InitBase(id, logger, sr)
	w.deps = NewCommunicatorDependencies(baseDeps)

	w.SetPostParseHook(func(cfg *CommunicatorConfig) error {
		if cfg.Timeout == 0 {
			cfg.Timeout = httpTransport.LongPollingDuration + httpTransport.LongPollingBuffer
		}
		return nil
	})

	w.SetChildSpecsFactory(func(_ CommunicatorConfig, spec config.UserSpec) []config.ChildSpec {
		return []config.ChildSpec{{
			Name:             "transport",
			WorkerType:       "transport",
			UserSpec:         spec,
			ChildStartStates: []string{"Syncing", "Recovering"},
		}}
	})

	return w, nil
}

// GetDependencies returns the typed communicator dependencies.
func (w *CommunicatorWorker) GetDependencies() *CommunicatorDependencies {
	return w.deps
}

// GetDependenciesAny returns the worker's dependencies for action execution.
// Overrides WorkerBase.GetDependenciesAny to return *CommunicatorDependencies
// instead of *deps.BaseDependencies.
func (w *CommunicatorWorker) GetDependenciesAny() any {
	return w.deps
}

// CollectObservedState returns the current observed state of the communicator.
// Records the consecutive-errors gauge and delegates metric accumulation
// to WrapStatusAccumulated. Deprecated transport fields (JWT, messages,
// auth state) are now tracked by TransportWorker (ENG-4264).
func (w *CommunicatorWorker) CollectObservedState(ctx context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	d := w.deps

	d.MetricsRecorder().SetGauge(deps.GaugeConsecutiveErrors, float64(d.GetConsecutiveErrors()))

	status := CommunicatorStatus{
		DegradedEnteredAt: d.GetDegradedEnteredAt(),
	}

	return w.WrapStatusAccumulated(ctx, status), nil
}

func init() {
	register.Worker[CommunicatorConfig, CommunicatorStatus]("communicator", NewCommunicatorWorker)
}
