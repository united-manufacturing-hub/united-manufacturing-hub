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

import "go.uber.org/zap"

// Dependencies provides access to worker-specific tools for actions.
// All worker dependencies embed BaseDependencies and extend with worker-specific tools.
type Dependencies interface {
	GetLogger() *zap.SugaredLogger
	// ActionLogger returns a logger enriched with action context.
	// Use this when logging from within an action for consistent structured logs.
	ActionLogger(actionType string) *zap.SugaredLogger
}

// BaseDependencies provides common tools for all workers.
// Worker-specific dependencies should embed this struct.
type BaseDependencies struct {
	logger     *zap.SugaredLogger
	workerType string
	workerID   string
}

// NewBaseDependencies creates a new base dependencies with common tools.
// The logger is automatically enriched with worker_type and worker_id context.
func NewBaseDependencies(logger *zap.SugaredLogger, workerType, workerID string) *BaseDependencies {
	if logger == nil {
		panic("NewBaseDependencies: logger cannot be nil")
	}

	return &BaseDependencies{
		logger:     logger.With("worker_type", workerType, "worker_id", workerID),
		workerType: workerType,
		workerID:   workerID,
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
