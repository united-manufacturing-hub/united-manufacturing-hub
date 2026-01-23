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

package example_child

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplechild/state"
)

// ChildWorker implements the FSM v2 Worker interface for resource management.
type ChildWorker struct {
	connection Connection
	*helpers.BaseWorker[*ExamplechildDependencies]
	logger   *zap.SugaredLogger
	identity deps.Identity
}

// NewChildWorker creates a new example child worker.
func NewChildWorker(
	identity deps.Identity,
	connectionPool ConnectionPool,
	logger *zap.SugaredLogger,
	stateReader deps.StateReader,
) (*ChildWorker, error) {
	if connectionPool == nil {
		return nil, errors.New("connectionPool must not be nil")
	}

	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	// Set workerType if not already set (derive from snapshot type)
	if identity.WorkerType == "" {
		workerType, err := storage.DeriveWorkerType[snapshot.ExamplechildObservedState]()
		if err != nil {
			return nil, fmt.Errorf("failed to derive worker type: %w", err)
		}

		identity.WorkerType = workerType
	}

	dependencies := NewExamplechildDependencies(connectionPool, logger, stateReader, identity)

	conn, err := connectionPool.Acquire()
	if err != nil {
		logger.Warnw("Failed to acquire initial connection", "error", err)
	}

	return &ChildWorker{
		BaseWorker: helpers.NewBaseWorker(dependencies),
		identity:   identity,
		logger:     logger,
		connection: conn,
	}, nil
}

// CollectObservedState returns the current observed state of the child worker.
func (w *ChildWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Get connection health from dependencies (updated by ConnectAction/DisconnectAction)
	deps := w.GetDependencies()

	connectionHealth := "no connection"

	if deps.IsConnected() {
		connectionHealth = "healthy"
	}

	observed := snapshot.ExamplechildObservedState{
		ID:               w.identity.ID,
		CollectedAt:      time.Now(),
		ConnectionHealth: connectionHealth,
		// MetricsEmbedder is embedded - zero value is valid
	}

	// Copy framework metrics from deps (set by supervisor before CollectObservedState)
	if fm := deps.GetFrameworkState(); fm != nil {
		observed.Metrics.Framework = *fm
	}

	// Copy action history from deps (set by supervisor before CollectObservedState)
	observed.LastActionResults = deps.GetActionHistory()

	return observed, nil
}

// DeriveDesiredState determines what state the child worker should be in.
// The child receives a Config template from the parent via ChildSpec.UserSpec.Config.
// This method renders the template first, then parses the result.
//
// ARCHITECTURE NOTE: Production workers follow this pattern:
//
// 1. DesiredState: Contains OriginalUserSpec (template + variables), NOT rendered config
//   - Rendered configs are ephemeral, computed by Actions
//
//  2. Action (CreateServiceAction): Renders and writes config to disk
//     rendered := config.RenderConfigTemplate(userSpec.Config, userSpec.Variables)
//     checksum := xxhash(rendered)
//     fsService.WriteFile("/data/services/benthos-"+id+"/config.yaml", rendered)
//     deps.expectedChecksum = checksum  // Store for drift detection
//
//  3. CollectObservedState: Reads file checksum, compares with expected
//     currentChecksum := xxhash(fileContent)
//     observed.IsDrifted = (currentChecksum != deps.expectedChecksum)
//
// 4. Caching layers for performance:
//   - Supervisor: hash(UserSpec) → skip DeriveDesiredState if unchanged (Stage 7)
//   - Action: Compare checksums → skip write if file already correct
//
// This example demonstrates variable inheritance and template rendering.
// For full S6 service creation, see pkg/service/s6/lifecycle.go.
func (w *ChildWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	// Handle nil spec - return default state
	if spec == nil {
		return &config.DesiredState{
			State:            config.DesiredStateRunning,
			OriginalUserSpec: nil,
		}, nil
	}

	// Cast spec to UserSpec to access Config and Variables
	userSpec, ok := spec.(config.UserSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec type: expected UserSpec, got %T", spec)
	}

	// Render the config template using the merged variables
	// Variables include: IP, PORT (from parent) + DEVICE_ID (from child)
	renderedConfig, err := config.RenderConfigTemplate(userSpec.Config, userSpec.Variables)
	if err != nil {
		return nil, fmt.Errorf("template rendering failed: %w", err)
	}

	// Create a new UserSpec with the rendered config for parsing
	renderedSpec := config.UserSpec{
		Config:    renderedConfig,
		Variables: userSpec.Variables,
	}

	// Now parse the rendered config using the helper
	desired, err := config.DeriveLeafState[ChildUserSpec](renderedSpec)
	if err != nil {
		return nil, err
	}

	return &desired, nil
}

// GetInitialState returns the state the FSM should start in.
func (w *ChildWorker) GetInitialState() fsmv2.State[any, any] {
	return &state.StoppedState{}
}

func init() {
	// Register both worker and supervisor factories atomically.
	// The worker type is derived from ExamplechildObservedState, ensuring consistency.
	if err := factory.RegisterWorkerType[snapshot.ExamplechildObservedState, *snapshot.ExamplechildDesiredState](
		func(id deps.Identity, logger *zap.SugaredLogger, stateReader deps.StateReader, _ map[string]any) fsmv2.Worker {
			pool := &DefaultConnectionPool{}
			worker, _ := NewChildWorker(id, pool, logger, stateReader)

			return worker
		},
		func(cfg interface{}) interface{} {
			return supervisor.NewSupervisor[snapshot.ExamplechildObservedState, *snapshot.ExamplechildDesiredState](
				cfg.(supervisor.Config))
		},
	); err != nil {
		panic(err)
	}
}
