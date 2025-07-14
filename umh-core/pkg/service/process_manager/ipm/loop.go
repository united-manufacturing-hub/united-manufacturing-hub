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

package ipm

import (
	"context"
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"go.uber.org/zap"
)

const (
	// Directory structure constants
	logDirectoryName    = "log"
	configDirectoryName = "config"
	pidFileName         = "run.pid"

	// File permissions
	configFilePermission = 0644

	// Process termination constants
	cleanupTimeReserve = 20 * time.Millisecond
	stepTimeThreshold  = 10 * time.Millisecond
)

// step processes queued service operations (create, remove, restart, etc.) in a time-bounded manner.
// It operates under the assumption that the ProcessManager mutex is already locked by the caller.
// The function processes one operation at a time and recursively calls itself if there's sufficient
// time remaining in the context. This design ensures that service operations are handled promptly
// while respecting context deadlines and preventing infinite loops that could block the system.
func (pm *ProcessManager) step(ctx context.Context, fsService filesystem.Service) error {
	// Check if we still got time to run (from the context)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	// If we have time, we will step
	deadline, ok := ctx.Deadline()
	if !ok {
		pm.Logger.Error("Context has no deadline, this should never happen")
		return fmt.Errorf("context has no deadline")
	} else if time.Until(deadline) <= stepTimeThreshold {
		pm.Logger.Info("No time left, stopping")
		return nil // No time left, stop
	}

	// Check if there are any tasks to process
	if len(pm.taskQueue) > 0 {
		task := pm.taskQueue[0]
		pm.Logger.Info("Processing task", zap.String("operation", task.Operation.String()), zap.String("identifier", string(task.Identifier)))

		var err error
		switch task.Operation {
		case OperationCreate:
			err = pm.createService(ctx, task.Identifier, fsService)
		case OperationRemove:
			err = pm.removeService(ctx, task.Identifier, fsService)
		case OperationStart:
			err = pm.startService(ctx, task.Identifier, fsService)
		case OperationStop:
			// TODO: Implement stop functionality
			err = fmt.Errorf("stop operation not yet implemented")
		case OperationRestart:
			// TODO: Implement restart functionality
			err = fmt.Errorf("restart operation not yet implemented")
		default:
			err = fmt.Errorf("unknown operation type: %v", task.Operation)
		}

		if err != nil {
			pm.Logger.Error("Error processing task", zap.String("operation", task.Operation.String()), zap.Error(err))
			return err
		}

		// Remove the processed task from the queue
		pm.taskQueue = pm.taskQueue[1:]
	}

	// Only recurse if there are more tasks to process
	if len(pm.taskQueue) > 0 {
		// Step again (the callee will check if there is time left)
		return pm.step(ctx, fsService)
	}

	// No more tasks to process
	return nil
}

// generateContext creates a new context with a deadline that reserves the specified timeout duration.
// This function is essential for implementing proper timeout management in nested operations.
// By creating a context that expires before the parent context, it ensures that calling functions
// have sufficient time to perform cleanup operations after the nested operation completes or times out.
// This prevents cascading timeout failures and ensures that the overall operation can complete
// within its allocated time budget. The function validates that sufficient time remains before
// creating the new context, preventing operations from starting when they cannot complete successfully.
func generateContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc, error) {
	// Check if we have enough time on the ctx (if not, we will return an error)
	if ctx.Err() != nil {
		return nil, nil, ctx.Err()
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		return nil, nil, fmt.Errorf("context has no deadline")
	}

	// Check if we have enough time on the ctx (if not, we will return an error)
	if time.Until(deadline) < timeout {
		return nil, nil, fmt.Errorf("context has no enough time")
	}

	// Create a new context with deadline-timeout
	ctx, cancel := context.WithDeadline(ctx, deadline.Add(-timeout))
	return ctx, cancel, nil
}
