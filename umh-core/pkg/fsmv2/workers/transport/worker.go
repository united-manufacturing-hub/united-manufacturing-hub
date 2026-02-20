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

// Package transport implements the Transport FSM worker for bidirectional
// message exchange between Edge and Backend tiers via HTTP relay.
//
// # Architecture
//
// TransportWorker is an orchestrator parent that manages the following children:
//   - PushWorker: Handles outbound message pushing to backend
//   - PullWorker: Handles inbound message pulling from backend
//
// The worker authenticates with the relay server and coordinates its children
// for continuous message exchange.
//
// # FSM v2 Pattern
//
// This package follows the FSM v2 pattern:
//   - worker.go: Implements Worker interface (CollectObservedState, DeriveDesiredState)
//   - state/*.go: Defines state machine states and transitions
//   - action/*.go: Idempotent actions executed during state transitions
//   - snapshot/snapshot.go: Observed and desired state structures
//
// # States and Transitions
//
// State flow:
//
//	Stopped ──→ Starting ──→ Running ⇄ Degraded
//	                ↑         ↓ (token expired)
//	                └─────────┘
//
// All active states transition to Stopping → Stopped on shutdown.
// Auth failures in Starting retry in place (no dedicated error state).
package transport

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/state"
)

// defaultAuthenticateTimeout is the fallback timeout for authentication when not specified in config.
// Defined locally to avoid import cycle with the action package.
const defaultAuthenticateTimeout = 10 * time.Second

// Compile-time interface check: TransportWorker implements fsmv2.Worker.
var _ fsmv2.Worker = (*TransportWorker)(nil)

// TransportWorker implements the FSM v2 Worker interface for HTTP transport.
// It manages authentication and coordinates PushWorker/PullWorker children
// for bidirectional message exchange with the backend relay server.
type TransportWorker struct {
	*helpers.BaseWorker[*TransportDependencies]
}

// NewTransportWorker creates a new Transport worker in Stopped state.
// Returns an error if required dependencies are missing.
func NewTransportWorker(
	identity deps.Identity,
	logger deps.FSMLogger,
	stateReader deps.StateReader,
) (*TransportWorker, error) {
	// Dependency validation: reject nil logger (architecture requirement)
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	// Derive worker type if not set
	if identity.WorkerType == "" {
		workerType, err := storage.DeriveWorkerType[snapshot.TransportObservedState]()
		if err != nil {
			return nil, fmt.Errorf("failed to derive worker type: %w", err)
		}

		identity.WorkerType = workerType
	}

	// Create dependencies (will panic if ChannelProvider not set)
	dependencies := NewTransportDependencies(nil, logger, stateReader, identity)

	return &TransportWorker{
		BaseWorker: helpers.NewBaseWorker(dependencies),
	}, nil
}

// CollectObservedState returns the current observed state of the transport worker.
// Handles context cancellation at entry as required by architecture tests.
func (w *TransportWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	// Context cancellation check at entry (architecture requirement)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	deps := w.GetDependencies()

	// Build observed state
	observed := snapshot.TransportObservedState{
		CollectedAt: time.Now(),
		JWTToken:    deps.GetJWTToken(),
		JWTExpiry:   deps.GetJWTExpiry(),
	}

	// Framework metrics copy (architecture requirement)
	if fm := deps.GetFrameworkState(); fm != nil {
		observed.Metrics.Framework = *fm
	}

	// Action history copy (architecture requirement)
	observed.LastActionResults = deps.GetActionHistory()

	return observed, nil
}

// DeriveDesiredState determines what state the transport worker should be in.
// Must be PURE - only uses the spec parameter, never dependencies.
// Returns "running" or "stopped" as valid state values.
func (w *TransportWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	// Nil spec defaults to "running" — matches CommunicatorWorker convention.
	// Transport will attempt auth with empty credentials, fail, and retry with backoff.
	// This enables self-healing: if spec delivery is delayed during startup, the worker
	// retries until config arrives. Field validation below catches empty fields once
	// a real spec is parsed.
	if spec == nil {
		return &snapshot.TransportDesiredState{
			BaseDesiredState: config.BaseDesiredState{
				State: config.DesiredStateRunning,
			},
		}, nil
	}

	// Type cast to UserSpec
	userSpec, ok := spec.(config.UserSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec type: expected UserSpec, got %T", spec)
	}

	// Render template variables
	renderedConfig, err := config.RenderConfigTemplate(userSpec.Config, userSpec.Variables)
	if err != nil {
		return nil, fmt.Errorf("template rendering failed: %w", err)
	}

	// Parse YAML config
	var transportSpec TransportUserSpec
	if err := yaml.Unmarshal([]byte(renderedConfig), &transportSpec); err != nil {
		return nil, fmt.Errorf("config parse failed: %w", err)
	}

	// Validate required fields when worker should be running
	if transportSpec.GetState() == config.DesiredStateRunning {
		if transportSpec.RelayURL == "" {
			return nil, fmt.Errorf("relayURL is required when state is running")
		}

		if transportSpec.InstanceUUID == "" {
			return nil, fmt.Errorf("instanceUUID is required when state is running")
		}

		if transportSpec.AuthToken == "" {
			return nil, fmt.Errorf("authToken is required when state is running")
		}

		if transportSpec.Timeout == 0 {
			transportSpec.Timeout = defaultAuthenticateTimeout
		}
	}

	// Build desired state with valid state values only ("stopped" or "running")
	return &snapshot.TransportDesiredState{
		BaseDesiredState: config.BaseDesiredState{
			State: transportSpec.GetState(), // Returns "running" or "stopped"
		},
		RelayURL:     transportSpec.RelayURL,
		InstanceUUID: transportSpec.InstanceUUID,
		AuthToken:    transportSpec.AuthToken,
		Timeout:      transportSpec.Timeout,
	}, nil
}

// GetInitialState returns StoppedState as the initial FSM state.
func (w *TransportWorker) GetInitialState() fsmv2.State[any, any] {
	return &state.StoppedState{}
}

// init registers the transport worker and supervisor factory.
// This is called automatically when the package is imported.
func init() {
	if err := factory.RegisterWorkerType[snapshot.TransportObservedState, *snapshot.TransportDesiredState](
		// Worker factory function
		func(id deps.Identity, logger deps.FSMLogger, stateReader deps.StateReader, _ map[string]any) fsmv2.Worker {
			worker, err := NewTransportWorker(id, logger, stateReader)
			if err != nil {
				panic(fmt.Sprintf("failed to create transport worker (id=%s, name=%s): %v. "+
					"Ensure ChannelProvider is set before supervisor starts.",
					id.ID, id.Name, err))
			}

			return worker
		},
		// Supervisor factory function
		func(cfg interface{}) interface{} {
			return supervisor.NewSupervisor[snapshot.TransportObservedState, *snapshot.TransportDesiredState](
				cfg.(supervisor.Config))
		},
	); err != nil {
		panic(fmt.Sprintf("failed to register transport worker: %v", err))
	}
}
