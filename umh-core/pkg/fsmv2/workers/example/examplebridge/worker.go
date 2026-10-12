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

// Package examplebridge provides the example bridge worker, the scenario
// stand-in for a protocol converter: it owns a connection and its read and
// write flow templates, and builds the names a flow (a data-flow child that
// renders the bridge's templates) renders against. The names are the values
// a template reads by path, such as .connection.host; see
// config.BuildWorkerNames.
package examplebridge

import (
	"context"
	"errors"
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
)

// WorkerTypeName is the canonical worker type of the example bridge worker.
const WorkerTypeName = "examplebridge"

// ExamplebridgeWorker implements the FSM v2 Worker interface for the example
// bridge.
type ExamplebridgeWorker struct {
	fsmv2.WorkerBase[ExamplebridgeConfig, ExamplebridgeStatus, register.NoDeps]
}

// NewExamplebridgeWorker creates a new example bridge worker.
func NewExamplebridgeWorker(
	identity deps.Identity,
	logger deps.FSMLogger,
	stateReader deps.StateReader,
) (*ExamplebridgeWorker, error) {
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	if identity.WorkerType == "" {
		identity.WorkerType = WorkerTypeName
	}

	w := &ExamplebridgeWorker{}
	w.InitBase(identity, logger, stateReader)

	return w, nil
}

// DeriveDesiredState loads the bridge's config and builds every name a flow
// template can use: the config's own fields by their YAML path, its variables
// section, the merged agent location with the bridge's own levels, one
// shortcut each for IP, PORT, every variable and the historian, and the
// config values a parent hands down under config. The ExamplebridgeConfig
// methods supply the merged location and the shortcuts. A nil spec yields
// the default desired state, like the base.
func (w *ExamplebridgeWorker) DeriveDesiredState(spec any) (fsmv2.DesiredState, error) {
	if spec == nil {
		return &fsmv2.WrappedDesiredState[ExamplebridgeConfig]{}, nil
	}

	userSpec, ok := spec.(config.UserSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec type: expected config.UserSpec, got %T", spec)
	}

	cfg, err := config.LoadUserSpec[ExamplebridgeConfig](spec)
	if err != nil {
		return nil, fmt.Errorf("failed to load the examplebridge config: %w", err)
	}

	fields, err := config.EntryFields(userSpec.Config, &cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to read the examplebridge config fields: %w", err)
	}

	names, err := config.BuildWorkerNames(cfg, fields, cfg.Variables, userSpec.ConfigValues)
	if err != nil {
		return nil, fmt.Errorf("failed to build the template names: %w", err)
	}

	cfg.Names = names

	if g, ok := any(&cfg).(interface{ GetState() string }); ok {
		state := g.GetState()

		if state != "" {
			if err := config.ValidateDesiredState(state); err != nil {
				return nil, &config.ConfigError{
					Field: "state",
					Err:   fmt.Errorf("unknown value %q: %w", state, err),
				}
			}
		}
	}

	return &fsmv2.WrappedDesiredState[ExamplebridgeConfig]{
		Config: cfg,
	}, nil
}

// CollectObservedState returns the current observed state of the example
// bridge worker.
func (w *ExamplebridgeWorker) CollectObservedState(ctx context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return fsmv2.NewObservation(ExamplebridgeStatus{}), nil
}

func init() {
	register.Worker[ExamplebridgeConfig, ExamplebridgeStatus, register.NoDeps](WorkerTypeName,
		func(id deps.Identity, logger deps.FSMLogger, sr deps.StateReader, _ map[string]any) (fsmv2.Worker, error) {
			return NewExamplebridgeWorker(id, logger, sr)
		})
}

var _ fsmv2.Worker = (*ExamplebridgeWorker)(nil)
