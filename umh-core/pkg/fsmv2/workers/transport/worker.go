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
// # Control Loop
//
// Sensor (CollectObservedState): reads JWT token, error counters, auth timing from deps
// Controller (state machine): Stopped → Starting → Running ⇄ Degraded
// Actuator (Actions): AuthenticateAction, ResetTransportAction
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

	"reflect"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

// defaultAuthenticateTimeout is the fallback timeout for authentication when not specified in config.
// Defined locally to avoid import cycle with the action package.
const defaultAuthenticateTimeout = 10 * time.Second

// Compile-time interface check: TransportWorker implements fsmv2.Worker.
var _ fsmv2.Worker = (*TransportWorker)(nil)

// TransportWorker implements the FSM Worker interface for HTTP transport.
// It manages authentication and coordinates PushWorker/PullWorker children
// for bidirectional message exchange with the backend relay server.
type TransportWorker struct {
	fsmv2.WorkerBase[TransportConfig, TransportStatus]
	deps *TransportDependencies
}

// NewTransportWorker creates a new Transport worker.
// Returns an error if required dependencies are missing.
func NewTransportWorker(
	identity deps.Identity,
	logger deps.FSMLogger,
	stateReader deps.StateReader,
) (*TransportWorker, error) {
	if logger == nil {
		return nil, errors.New("logger must not be nil")
	}

	if identity.WorkerType == "" {
		identity.WorkerType = workerType
	}

	w := &TransportWorker{}
	w.InitBase(identity, logger, stateReader)

	// Create dependencies (will panic if ChannelProvider not set)
	w.deps = NewTransportDependencies(nil, logger, stateReader, identity)

	// Validation hook: required fields when running.
	// When all fields are empty (nil spec startup path), skip validation —
	// transport will attempt auth with empty credentials, fail, and retry with backoff.
	// This enables self-healing when spec delivery is delayed during startup.
	w.SetPostParseHook(func(cfg *TransportConfig) error {
		if cfg.GetState() == config.DesiredStateRunning {
			if cfg.RelayURL == "" && cfg.InstanceUUID == "" && cfg.AuthToken == "" {
				return nil
			}
			if cfg.RelayURL == "" {
				return fmt.Errorf("relayURL is required when state is running")
			}
			if cfg.InstanceUUID == "" {
				return fmt.Errorf("instanceUUID is required when state is running")
			}
			if cfg.AuthToken == "" {
				return fmt.Errorf("authToken is required when state is running")
			}
			if cfg.Timeout == 0 {
				cfg.Timeout = defaultAuthenticateTimeout
			}
		}
		return nil
	})

	// Child specs factory: Push and Pull children
	w.SetChildSpecsFactory(func(_ TransportConfig, rawSpec config.UserSpec) []config.ChildSpec {
		return append(makePushChildSpec(rawSpec), makePullChildSpec(rawSpec)...)
	})

	return w, nil
}

// GetDependenciesAny returns the custom TransportDependencies.
// Overrides WorkerBase's default which returns *BaseDependencies.
// Required by architecture test: custom deps must be visible to the supervisor.
func (w *TransportWorker) GetDependenciesAny() any {
	return w.deps
}

// CollectObservedState returns the current observed state of the transport worker.
// Returns NewObservation — the collector handles CollectedAt, framework metrics,
// action history, and metric accumulation automatically after COS returns.
func (w *TransportWorker) CollectObservedState(ctx context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return fsmv2.NewObservation(TransportStatus{
		JWTToken:          w.deps.GetJWTToken(),
		JWTExpiry:         w.deps.GetJWTExpiry(),
		AuthenticatedUUID: w.deps.GetAuthenticatedUUID(),
		ConsecutiveErrors: w.deps.GetConsecutiveErrors(),
		LastErrorType:     w.deps.GetLastErrorType(),
		LastAuthAttemptAt: w.deps.GetLastAuthAttemptAt(),
		LastRetryAfter:    w.deps.GetLastRetryAfter(),
	}), nil
}

const workerType = "transport"

// init registers the transport worker and supervisor factory.
// Uses factory.RegisterWorkerAndSupervisorFactoryByType (explicit workerType) instead of
// factory.RegisterWorkerType (derived from type name) because Observation[TransportStatus]
// doesn't match the legacy "FooObservedState" naming convention.
func init() {
	// Step 1: Register worker + supervisor factories.
	if err := factory.RegisterWorkerAndSupervisorFactoryByType(
		workerType,
		// Worker factory function
		func(id deps.Identity, logger deps.FSMLogger, stateReader deps.StateReader, extraDeps map[string]any) fsmv2.Worker {
			worker, err := NewTransportWorker(id, logger, stateReader)
			if err != nil {
				panic(fmt.Sprintf("failed to create transport worker (id=%s, name=%s): %v. "+
					"Ensure ChannelProvider is set before supervisor starts.",
					id.ID, id.Name, err))
			}

			extraDeps["transport_deps"] = worker.deps

			return worker
		},
		// Supervisor factory function
		func(cfg interface{}) interface{} {
			return supervisor.NewSupervisor[fsmv2.Observation[TransportStatus], *fsmv2.WrappedDesiredState[TransportConfig]](
				cfg.(supervisor.Config))
		},
	); err != nil {
		panic(fmt.Sprintf("failed to register transport worker: %v", err))
	}

	// Step 2: Register with CSE TypeRegistry for storage.
	observedType := reflect.TypeOf(fsmv2.Observation[TransportStatus]{})
	desiredType := reflect.TypeOf(fsmv2.WrappedDesiredState[TransportConfig]{})

	if err := storage.GlobalRegistry().RegisterWorkerType(workerType, observedType, desiredType); err != nil {
		panic(fmt.Sprintf("failed to register transport CSE types: %v", err))
	}
}

// makePushChildSpec creates the ChildSpec for the PushWorker child.
// PushWorker runs when TransportWorker is in "Running" or "Degraded" states.
// Including "Degraded" prevents an oscillation loop where Push stops on parent
// degradation (caused by Push being unhealthy), parent recovers (no unhealthy children),
// Push restarts, and the cycle repeats.
func makePushChildSpec(parentSpec config.UserSpec) []config.ChildSpec {
	return []config.ChildSpec{
		{
			Name:             "push",
			WorkerType:       "push",
			UserSpec:         config.UserSpec{Config: parentSpec.Config, Variables: parentSpec.Variables},
			ChildStartStates: []string{"Running", "Degraded"},
		},
	}
}

// makePullChildSpec creates the ChildSpec for the PullWorker child.
// PullWorker runs when TransportWorker is in "Running" or "Degraded" states,
// mirroring PushWorker's lifecycle to prevent the same oscillation issue.
func makePullChildSpec(parentSpec config.UserSpec) []config.ChildSpec {
	return []config.ChildSpec{
		{
			Name:             "pull",
			WorkerType:       "pull",
			UserSpec:         config.UserSpec{Config: parentSpec.Config, Variables: parentSpec.Variables},
			ChildStartStates: []string{"Running", "Degraded"},
		},
	}
}
