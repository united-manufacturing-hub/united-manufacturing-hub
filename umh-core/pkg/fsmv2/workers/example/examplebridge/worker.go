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
// stand-in for a protocol converter: it owns a connection and a read flow
// template, and builds the names a flow (a data-flow child that renders the
// bridge's templates) renders against.
package examplebridge

import (
	"context"
	"errors"
	"fmt"
	"sort"

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
// section, the agent location merged with the bridge's own levels as the
// ready-made location and location_path, and one shortcut each for IP, PORT,
// every variable and the historian. Two shortcuts that claim one name are a
// *ConfigError naming both sources. A nil spec yields the default desired
// state, like the base.
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

	agentLocation, err := agentLocationFrom(userSpec.ConfigValues)
	if err != nil {
		return nil, err
	}

	ownLocation, err := config.LocationFromAny(fields["location"])
	if err != nil {
		return nil, fmt.Errorf("failed to read the examplebridge location: %w", err)
	}

	merged, locationPath := config.MergeAgentLocation(agentLocation, ownLocation)

	mergedLocation := make(map[string]any, len(merged))
	for level, value := range merged {
		mergedLocation[level] = value
	}

	connShortcuts, err := connectionShortcuts(fields)
	if err != nil {
		return nil, err
	}

	shortcutSources := []struct {
		label  string
		values map[string]any
	}{
		{"the connection shortcut", connShortcuts},
		{"a variable", config.VariableShortcuts(cfg.Variables)},
		{"the historian shortcut", config.HistorianShortcut(userSpec.ConfigValues)},
	}

	shortcuts := map[string]any{}
	shortcutSource := map[string]string{}

	for _, source := range shortcutSources {
		for _, name := range sortedKeys(source.values) {
			previous, exists := shortcutSource[name]
			if exists {
				return nil, &config.ConfigError{
					Field: name,
					Err:   fmt.Errorf("names: %q is both %s and %s", name, previous, source.label),
				}
			}

			shortcutSource[name] = source.label
			shortcuts[name] = source.values[name]
		}
	}

	names, err := config.BuildNames(config.Entry{
		Fields:    fields,
		Variables: cfg.Variables,
		Config:    userSpec.ConfigValues,
		ReadyMade: map[string]any{
			"location":      mergedLocation,
			"location_path": locationPath,
		},
		Shortcuts: shortcuts,
	})
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
					Err:   fmt.Errorf("invalid desired state: unknown value %q: %w", state, err),
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

// agentLocationFrom reads the agent's location out of the config values a
// parent hands down. A parent that hands down no agent yields the empty
// location; an agent that is not a mapping is a *ConfigError on the agent
// field.
func agentLocationFrom(cfgValues map[string]any) (map[string]string, error) {
	raw, exists := cfgValues["agent"]
	if !exists {
		return map[string]string{}, nil
	}

	agent, ok := raw.(map[string]any)
	if !ok {
		return nil, &config.ConfigError{
			Field: "agent",
			Err:   fmt.Errorf("agent must be a mapping, got %T", raw),
		}
	}

	location, err := config.LocationFromAny(agent["location"])
	if err != nil {
		return nil, fmt.Errorf("failed to read the agent location: %w", err)
	}

	return location, nil
}

// connectionShortcuts returns the IP and PORT names a template reads without
// the connection prefix. A config that carries no connection yields none; a
// connection that is not a mapping is a *ConfigError on the connection field.
func connectionShortcuts(fields map[string]any) (map[string]any, error) {
	shortcuts := map[string]any{}

	raw, exists := fields["connection"]
	if !exists {
		return shortcuts, nil
	}

	connection, ok := raw.(map[string]any)
	if !ok {
		return nil, &config.ConfigError{
			Field: "connection",
			Err:   fmt.Errorf("connection must be a mapping, got %T", raw),
		}
	}

	if host, ok := connection["host"]; ok {
		shortcuts["IP"] = host
	}

	if port, ok := connection["port"]; ok {
		shortcuts["PORT"] = port
	}

	return shortcuts, nil
}

// sortedKeys returns the keys of m in sorted order, so a name two sources
// claim always produces the same error.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))

	for key := range m {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}

var _ fsmv2.Worker = (*ExamplebridgeWorker)(nil)
