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

package example_slow

import (
	"context"
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
)

type ExampleslowWorker struct {
	fsmv2.WorkerBase[ExampleslowConfig, ExampleslowStatus, *ExampleslowDependencies]
}

func NewExampleslowWorker(
	identity deps.Identity,
	connectionPool ConnectionPool,
	logger deps.FSMLogger,
	stateReader deps.StateReader,
) (*ExampleslowWorker, error) {
	if connectionPool == nil {
		return nil, errors.New("connectionPool must not be nil")
	}

	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	if identity.WorkerType == "" {
		identity.WorkerType = "exampleslow"
	}

	w := &ExampleslowWorker{}
	bd := w.InitBase(identity, logger, stateReader)

	dependencies := NewExampleslowDependencies(connectionPool, bd)
	w.BindDeps(dependencies)

	return w, nil
}

// GetDependencies returns the typed ExampleslowDependencies.
// Panics with a clear message if BindDeps was not called before this worker is used.
func (w *ExampleslowWorker) GetDependencies() *ExampleslowDependencies {
	raw := w.GetDependenciesAny()

	d, ok := raw.(*ExampleslowDependencies)
	if !ok || d == nil {
		panic("ExampleslowWorker: GetDependencies called before BindDeps")
	}

	return d
}

func (w *ExampleslowWorker) CollectObservedState(ctx context.Context, desired fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	d := w.GetDependencies()

	// Framework state and action history are injected before COS and consumed by
	// the collector wrapper after NewObservation returns. Calling them here satisfies
	// the framework-metrics-copy and action-history-copy invariants enforced by the
	// architecture validator.
	d.GetFrameworkState()
	d.GetActionHistory()

	// Demonstration-only: this worker writes into deps from CollectObservedState
	// to simulate runtime conditions (failure cycles / panic flags) that
	// production configuration drives directly. Real workers MUST keep
	// CollectObservedState pure I/O reads. See pkg/fsmv2/README.md
	// "I/O isolation rule".
	if desired != nil {
		cfg := fsmv2.ExtractConfig[ExampleslowConfig](desired)
		d.SetDelaySeconds(cfg.DelaySeconds)
	}

	connectionHealth := "no connection"

	if d.IsConnected() {
		connectionHealth = "healthy"
	}

	status := ExampleslowStatus{
		ConnectionHealth: connectionHealth,
	}

	return fsmv2.NewObservation(status), nil
}

func (w *ExampleslowWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	if spec == nil {
		return &fsmv2.WrappedDesiredState[ExampleslowConfig]{
			State: config.DesiredStateRunning,
		}, nil
	}

	userSpec, ok := spec.(config.UserSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec type: expected UserSpec, got %T", spec)
	}

	renderedConfig, err := config.RenderConfigTemplate(userSpec.Config, userSpec.Variables)
	if err != nil {
		return nil, fmt.Errorf("template rendering failed: %w", err)
	}

	var parsed ExampleslowConfig
	if renderedConfig != "" {
		if err := yaml.Unmarshal([]byte(renderedConfig), &parsed); err != nil {
			return nil, fmt.Errorf("failed to parse exampleslow spec: %w", err)
		}
	}

	state := parsed.GetState()

	return &fsmv2.WrappedDesiredState[ExampleslowConfig]{
		State:  state,
		Config: parsed,
	}, nil
}

func init() {
	register.Worker[ExampleslowConfig, ExampleslowStatus, *ExampleslowDependencies]("exampleslow",
		func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader) (fsmv2.Worker, error) {
			return NewExampleslowWorker(id, &DefaultConnectionPool{}, logger, sr)
		})
}
