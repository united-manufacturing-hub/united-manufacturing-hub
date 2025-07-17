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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/process_manager/ipm/constants"
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
	deadline, hasDeadline := ctx.Deadline()
	if !hasDeadline {
		pm.Logger.Error("Context has no deadline, this should never happen")
		return fmt.Errorf("context has no deadline")
	} else if time.Until(deadline) <= constants.StepTimeThreshold {
		pm.Logger.Info("No time left, stopping")
		return nil // No time left, stop
	}

	// Before processing tasks, check and rotate logs if needed
	if pm.logManager != nil {
		if err := pm.logManager.CheckAndRotate(ctx, fsService); err != nil {
			pm.Logger.Errorf("Error during log rotation: %v", err)
			// Continue with task processing even if log rotation fails
		}
	} else {
		pm.Logger.Warn("LogManager is nil - skipping log rotation. This should not happen in production.")
	}

	// Check if there are any tasks to process
	if len(pm.TaskQueue) > 0 {
		task := pm.TaskQueue[0]
		pm.Logger.Infof("Processing task: %s for %s", task.Operation.String(), task.Identifier)

		var err error
		switch task.Operation {
		case OperationCreate:
			err = pm.createService(ctx, task.Identifier, fsService)
		case OperationRemove:
			err = pm.removeService(ctx, task.Identifier, fsService)
		case OperationStart:
			err = pm.startService(ctx, task.Identifier, fsService)
		case OperationStop:
			err = pm.stopService(ctx, task.Identifier, fsService)
		case OperationRestart:
			// Try to stop the service first
			if err = pm.stopService(ctx, task.Identifier, fsService); err != nil {
				pm.Logger.Errorf("Restart: stop failed, not queuing start operation for %s: %v", task.Identifier, err)
			} else {
				pm.Logger.Infof("Restart: stop succeeded, queued start operation for %s", task.Identifier)
				// If stop succeeded, queue a start operation for later execution
				startTask := Task{
					Identifier: task.Identifier,
					Operation:  OperationStart,
				}
				pm.TaskQueue = append(pm.TaskQueue, startTask)
			}
		default:
			err = fmt.Errorf("unknown operation type: %v", task.Operation)
		}

		if err != nil {
			pm.Logger.Errorf("Error processing task %s: %v", task.Operation.String(), err)
		}

		// Remove the processed task from the queue
		// We always dequeue, as the task will be re-enqued by the caller
		pm.TaskQueue = pm.TaskQueue[1:]
	}

	// Only recurse if there are more tasks to process
	if len(pm.TaskQueue) > 0 {
		return pm.step(ctx, fsService)
	}

	return nil
}

// GenerateContext creates a new context with a deadline that reserves the specified timeout duration.
// This function is essential for implementing proper timeout management in nested operations.
// By creating a context that expires before the parent context, it ensures that calling functions
// have sufficient time to perform cleanup operations after the nested operation completes or times out.
// This prevents cascading timeout failures and ensures that the overall operation can complete
// within its allocated time budget. The function validates that sufficient time remains before
// creating the new context, preventing operations from starting when they cannot complete successfully.
func GenerateContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc, error) {
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
