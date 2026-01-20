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

// Package fsmv2 implements a dependency injection pattern for FSM workers.
//
// Dependencies Pattern:
//
// The Dependencies interface provides a standardized way to inject worker-specific
// tools (logger, transport, metrics, etc.) into FSM actions. This pattern:
//
//   - Avoids global state and tight coupling
//   - Makes testing easier through dependency injection
//   - Provides type-safe access to worker-specific tools
//   - Enables worker-specific extensions through embedding
//
// Example usage:
//
//	// Worker-specific dependencies
//	type CommunicatorDependencies struct {
//	    *fsmv2.BaseDependencies
//	    transport Transport
//	}
//
//	func (d *CommunicatorDependencies) GetTransport() Transport {
//	    return d.transport
//	}
//
//	// Actions receive dependencies
//	type SendHeartbeatAction struct {
//	    dependencies *CommunicatorDependencies
//	}
//
//	func (a *SendHeartbeatAction) Execute(ctx context.Context) error {
//	    logger := a.dependencies.GetLogger()
//	    transport := a.dependencies.GetTransport()
//	    // ... use injected dependencies
//	}
//
// Note: This is unrelated to storage.Registry (CSE collection metadata).
package fsmv2

import (
	"context"

	"go.uber.org/zap"
)

// StateReader provides read-only access to the TriangularStore for workers.
// Workers can query their own previous observed state (for state change detection)
// and query children's observed states (for richer parent-child coordination).
//
// IMPORTANT: This is READ-ONLY. Workers MUST NOT write to the store directly.
// All writes go through the supervisor's collector.
type StateReader interface {
	// LoadObservedTyped loads the observed state for a worker into the provided pointer.
	// Use this to:
	//   - Query your own previous observed state (for state change detection)
	//   - Query children's observed states (for parent-child coordination)
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeouts
	//   - workerType: Type of worker (e.g., "exampleparent", "examplechild")
	//   - id: Worker's unique identifier
	//   - result: Pointer to struct where result will be unmarshaled
	//
	// Returns error if the observed state doesn't exist or unmarshaling fails.
	LoadObservedTyped(ctx context.Context, workerType string, id string, result interface{}) error
}

// Dependencies provides access to worker-specific tools for actions.
// All worker dependencies embed BaseDependencies and extend with worker-specific tools.
type Dependencies interface {
	GetLogger() *zap.SugaredLogger
	// ActionLogger returns a logger enriched with action context.
	// Use this when logging from within an action for consistent structured logs.
	ActionLogger(actionType string) *zap.SugaredLogger
	// GetStateReader returns read-only access to the TriangularStore.
	// Returns nil if no StateReader was provided during construction.
	GetStateReader() StateReader
}

// BaseDependencies provides common tools for all workers.
// Worker-specific dependencies should embed this struct.
type BaseDependencies struct {
	logger      *zap.SugaredLogger
	stateReader StateReader
	metrics     *MetricsRecorder
	workerType  string
	workerID    string
}

// NewBaseDependencies creates a new base dependencies with common tools.
// The logger is automatically enriched with hierarchical worker context using identity.String().
// The stateReader can be nil if state access is not needed.
func NewBaseDependencies(logger *zap.SugaredLogger, stateReader StateReader, identity Identity) *BaseDependencies {
	if logger == nil {
		panic("NewBaseDependencies: logger cannot be nil")
	}

	return &BaseDependencies{
		logger:      logger.With("worker", identity.String()),
		stateReader: stateReader,
		metrics:     NewMetricsRecorder(),
		workerType:  identity.WorkerType,
		workerID:    identity.ID,
	}
}

// GetLogger returns the logger for this dependencies.
func (d *BaseDependencies) GetLogger() *zap.SugaredLogger {
	return d.logger
}

// ActionLogger returns a logger enriched with action context.
// Use this when logging from within an action for consistent structured logs.
func (d *BaseDependencies) ActionLogger(actionType string) *zap.SugaredLogger {
	return d.logger.With("log_source", "action", "action_type", actionType)
}

// GetStateReader returns read-only access to the TriangularStore.
// Returns nil if no StateReader was provided during construction.
func (d *BaseDependencies) GetStateReader() StateReader {
	return d.stateReader
}

// Metrics returns the MetricsRecorder for actions to record metrics.
// Actions call IncrementCounter/SetGauge with typed constants from the metrics package.
// CollectObservedState implementations should call Drain() to merge buffered metrics
// into the observed state.
//
// Example usage in an action:
//
//	deps.Metrics().IncrementCounter(metrics.CounterPullOps, 1)
//	deps.Metrics().SetGauge(metrics.GaugeLastPullLatencyMs, float64(latency.Milliseconds()))
func (d *BaseDependencies) Metrics() *MetricsRecorder {
	return d.metrics
}

// GetWorkerType returns the worker type for this dependencies.
func (d *BaseDependencies) GetWorkerType() string {
	return d.workerType
}

// GetWorkerID returns the worker ID for this dependencies.
func (d *BaseDependencies) GetWorkerID() string {
	return d.workerID
}
