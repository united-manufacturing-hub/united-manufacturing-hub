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

package constants

import (
	"context"
	"time"
)

const (
	// DefaultTickerTime is the default time between ticks
	DefaultTickerTime = 400 * time.Millisecond

	// Time budget percentages for parallel execution
	// These percentages are applied to whatever context time budget is available,
	// naturally creating a hierarchy without hardcoded absolute timeouts
	ControlLoopReservePercent  = 0.25 // 25% overhead for control loop coordination
	ManagerReservePercent      = 0.05 // 5% overhead per manager execution
	UpdateObservedStatePercent = 0.80 // 80% of manager time for I/O operations (parsing logs, health checks, metrics)
	// Remaining 20% automatically goes to reconciliation logic (FSM transitions, event sending)

	// Legacy constants - to be removed
	LoopControlLoopTimeFactor = 1.0 - ControlLoopReservePercent

	StarvationLimit = 5
	CoolDownTicks   = 5

	// starvationThreshold defines when to consider the control loop starved.
	// If no reconciliation has happened for this duration, the starvation
	// detector will log warnings and record metrics.
	// Starvation will take place for example when adding hundreds of new services
	// at once.
	StarvationThreshold = 60 * time.Second

	// Default names
	DefaultManagerName  = "Core"
	DefaultInstanceName = "Core"

	RingBufferCapacity = 3

	// MaxConcurrentFSMOperations defines the maximum number of concurrent FSM operations
	// This applies to both manager-level and instance-level parallel execution
	// Set high for I/O-bound operations like filesystem access, health checks, and network calls
	MaxConcurrentFSMOperations = 1000
)

// CreateSubContext applies a percentage to the parent context's remaining time.
// This enables hierarchical time budgets where each level respects its parent's constraints.
func CreateSubContext(parentCtx context.Context, percentage float64) (context.Context, context.CancelFunc) {
	if percentage < 0 || percentage > 1.0 {
		// Invalid percentage, return parent context to avoid breaking the chain
		return parentCtx, func() {}
	}

	deadline, ok := parentCtx.Deadline()
	if !ok {
		// No deadline to split - return parent context
		return parentCtx, func() {}
	}

	remainingTime := time.Until(deadline)
	if remainingTime <= 0 {
		// Parent context already expired
		return parentCtx, func() {}
	}

	subTime := time.Duration(float64(remainingTime) * percentage)
	return context.WithTimeout(parentCtx, subTime)
}

// CreateUpdateObservedStateContext creates a context for UpdateObservedState operations.
// Takes 80% of the manager's remaining time for I/O operations like parsing logs,
// health checks, and metrics collection.
func CreateUpdateObservedStateContext(managerCtx context.Context) (context.Context, context.CancelFunc) {
	return CreateSubContext(managerCtx, UpdateObservedStatePercent)
}

// CreateUpdateObservedStateContextWithMinimum creates a context for UpdateObservedState operations
// with a guaranteed minimum timeout. This ensures that even when the percentage-based allocation
// is smaller than the minimum required time, the operation gets enough time to complete safely.
//
// It takes the larger of:
// - 80% of the manager's remaining time (percentage-based allocation)
// - The specified minimum timeout (safety guarantee)
//
// This prevents context deadline exceeded errors when the instance execution time
// approaches the time budget limits while still respecting the hierarchical time allocation.
func CreateUpdateObservedStateContextWithMinimum(managerCtx context.Context, minimumTimeout time.Duration) (context.Context, context.CancelFunc) {
	deadline, ok := managerCtx.Deadline()
	if !ok {
		// No deadline to split - create context with minimum timeout
		return context.WithTimeout(managerCtx, minimumTimeout)
	}

	remainingTime := time.Until(deadline)
	if remainingTime <= 0 {
		// Parent context already expired
		return managerCtx, func() {}
	}

	// Calculate percentage-based allocation (80% of remaining time)
	percentageTime := time.Duration(float64(remainingTime) * UpdateObservedStatePercent)

	// Use the larger of percentage allocation or minimum guarantee
	var timeoutDuration time.Duration
	if percentageTime > minimumTimeout {
		timeoutDuration = percentageTime
	} else {
		timeoutDuration = minimumTimeout
	}

	return context.WithTimeout(managerCtx, timeoutDuration)
}

// CreateReconciliationContext creates a context for reconciliation logic.
// Takes 20% of the manager's remaining time for FSM transitions and event sending.
func CreateReconciliationContext(managerCtx context.Context) (context.Context, context.CancelFunc) {
	return CreateSubContext(managerCtx, 1.0-UpdateObservedStatePercent)
}

// CreateManagerContext creates a context for manager execution.
// Reserves 5% of available time for manager overhead.
func CreateManagerContext(controlLoopCtx context.Context) (context.Context, context.CancelFunc) {
	return CreateSubContext(controlLoopCtx, 1.0-ManagerReservePercent)
}

// Legacy functions for backward compatibility - these calculate absolute timeouts
// and should be gradually replaced with percentage-based contexts

// FilesAndDirectoriesToIgnore is a list of files and directories that we will not read.
// All older archived logs begin with @40000000
// As we retain up to 20 logs, this will otherwise lead to reading a lot of logs
var FilesAndDirectoriesToIgnore = []string{".s6-svscan", "s6-linux-init-shutdown", "s6rc-fdholder", "s6rc-oneshot-runner", "syslogd", "syslogd-log", "/control", "/lock", "@40000000"}
