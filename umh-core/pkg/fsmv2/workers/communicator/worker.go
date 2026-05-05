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

// Package communicator is the parent orchestrator that manages a TransportWorker
// child for bidirectional message exchange with the backend.
//
// Setup: call communicator.SetChannelProvider() and transport.SetChannelProvider()
// before starting the supervisor — both packages share inbound/outbound channels
// through a ChannelProvider singleton.
package communicator

import (
	"context"
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
)

// workerType is the registered type name for this worker.
const workerType = "communicator"

// defaultCommunicatorTimeout is the HTTP timeout applied when the user spec does not
// specify one. It covers the full long-poll round-trip including the server-side wait.
const defaultCommunicatorTimeout = httpTransport.LongPollingDuration + httpTransport.LongPollingBuffer

// CommunicatorWorker implements the FSMv2 Worker interface using the WorkerBase API.
type CommunicatorWorker struct {
	deps *CommunicatorDependencies
	fsmv2.WorkerBase[CommunicatorConfig, CommunicatorStatus, *CommunicatorDependencies]
}

// NewCommunicatorWorker creates a new communicator worker with the standard framework dependencies.
func NewCommunicatorWorker(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader) (fsmv2.Worker, error) {
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	w := &CommunicatorWorker{}
	baseDeps := w.InitBase(id, logger, sr)
	w.deps = NewCommunicatorDependencies(baseDeps)
	w.BindDeps(w.deps)

	return w, nil
}

// DeriveDesiredState parses the user spec and returns the desired state.
// Applies the default Timeout when the spec does not specify one.
// Timeout is captured at construction time in the returned WrappedDesiredState.Config
// so it survives the supervisor's JSON round-trip through CSE storage.
func (w *CommunicatorWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	if spec == nil {
		return &fsmv2.WrappedDesiredState[CommunicatorConfig]{
			Config: CommunicatorConfig{
				Timeout: defaultCommunicatorTimeout,
			},
		}, nil
	}

	userSpec, ok := spec.(fsmv2config.UserSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec type: expected UserSpec, got %T", spec)
	}

	renderedConfig, err := fsmv2config.RenderConfigTemplate(userSpec.Config, userSpec.Variables)
	if err != nil {
		return nil, fmt.Errorf("template rendering failed: %w", err)
	}

	var cfg CommunicatorConfig
	if renderedConfig != "" {
		if err := yaml.Unmarshal([]byte(renderedConfig), &cfg); err != nil {
			return nil, fmt.Errorf("config unmarshal failed: %w", err)
		}
	}

	if cfg.Timeout == 0 {
		cfg.Timeout = defaultCommunicatorTimeout
	}

	return &fsmv2.WrappedDesiredState[CommunicatorConfig]{
		Config: cfg,
	}, nil
}

// GetDependencies returns the typed communicator dependencies.
func (w *CommunicatorWorker) GetDependencies() *CommunicatorDependencies {
	return w.deps
}

// CollectObservedState returns the current observed state of the communicator.
// Records the consecutive-errors gauge. Returns NewObservation; the collector
// handles CollectedAt, framework metrics, action history, and metric
// accumulation (load previous from CSE, drain, merge) automatically.
func (w *CommunicatorWorker) CollectObservedState(_ context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	d := w.deps

	d.MetricsRecorder().SetGauge(deps.GaugeConsecutiveErrors, float64(d.GetConsecutiveErrors()))

	status := CommunicatorStatus{
		DegradedEnteredAt: d.GetDegradedEnteredAt(),
	}

	return fsmv2.NewObservation(status), nil
}

func init() {
	register.Worker[CommunicatorConfig, CommunicatorStatus, register.NoDeps](workerType, NewCommunicatorWorker)
}
