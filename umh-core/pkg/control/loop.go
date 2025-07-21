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

package control

// Package control implements the central control system for UMH.
//
// This package is responsible for:
// - Creating and coordinating FSM managers for different service types (S6, Benthos)
// - Executing the single-threaded control loop that drives the system
// - Managing the reconciliation process to maintain desired system state
// - Handling errors and ensuring system stability
// - Monitoring performance metrics and detecting starvation conditions
// - Creating and maintaining snapshots of system state for external consumers
//
// The control loop architecture follows established patterns from Kubernetes controllers,
// where a continuous reconciliation approach gradually moves the system toward its desired state.
//
// The main components are:
// - ControlLoop: Coordinates the entire system's operation
// - FSMManagers: Type-specific managers that handle individual services (S6, Benthos)
// - ConfigManager: Provides the desired system state from configuration
// - StarvationChecker: Monitors system health and detects control loop problems
// - SnapshotManager: Maintains thread-safe snapshots of the system state

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/tiendc/go-deepcopy"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/ctxutil"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/agent_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/connection"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/container"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/nmap"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/protocolconverter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/redpanda"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/streamprocessor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/starvationchecker"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// ControlLoop is the central orchestration component of the UMH Core.
// It implements the primary reconciliation loop that drives the entire system
// toward its desired state by coordinating multiple FSM managers.
//
// The control loop follows a "desired state" pattern where:
// 1. Configuration defines what the system should look like
// 2. Managers continuously reconcile actual state with desired state
// 3. Changes propagate in sequence until the system stabilizes
//
// This single-threaded design ensures deterministic behavior while the
// time-sliced approach allows responsive handling of multiple components.
type ControlLoop struct {
	tickerTime        time.Duration
	managers          []fsm.FSMManager[any]
	configManager     config.ConfigManager
	logger            *zap.SugaredLogger
	starvationChecker *starvationchecker.StarvationChecker
	currentTick       uint64
	snapshotManager   *fsm.SnapshotManager
	managerTimes      map[string]time.Duration // Tracks execution time for each manager
	managerTimesMutex sync.RWMutex
	services          *serviceregistry.Registry
}

// NewControlLoop creates a new control loop with all necessary managers.
// It initializes the complete orchestration system with all required components:
// - S6 and Benthos managers for service instance management
// - Config manager for tracking desired system state
// - Starvation checker for detecting loop health issues
// - Snapshot manager for sharing system state with external components
//
// The control loop runs at a fixed interval (defaultTickerTime) and orchestrates
// all components according to the configuration.
func NewControlLoop(configManager config.ConfigManager) *ControlLoop {
	// Get a component-specific logger
	log := logger.For(logger.ComponentControlLoop)
	if log == nil {
		// If logger initialization failed somehow, create a no-op logger to avoid nil panics
		log = zap.NewNop().Sugar()
	}

	// Create the service registry. The service registry will contain all services like filesystem, and portmanager
	servicesRegistry, err := serviceregistry.NewRegistry()
	if err != nil || servicesRegistry == nil {
		sentry.ReportIssuef(sentry.IssueTypeFatal, log, "Failed to create service registry: %s", err)
	}

	// Create the managers
	managers := []fsm.FSMManager[any]{
		s6.NewS6Manager(constants.DefaultManagerName),
		benthos.NewBenthosManager(constants.DefaultManagerName),
		container.NewContainerManager(constants.DefaultManagerName),
		redpanda.NewRedpandaManager(constants.DefaultManagerName),
		agent_monitor.NewAgentManager(constants.DefaultManagerName),
		nmap.NewNmapManager(constants.DefaultManagerName),
		dataflowcomponent.NewDataflowComponentManager(constants.DefaultManagerName),
		connection.NewConnectionManager(constants.DefaultManagerName),
		protocolconverter.NewProtocolConverterManager(constants.DefaultManagerName),
		topicbrowser.NewTopicBrowserManager(constants.DefaultManagerName),
		streamprocessor.NewManager(constants.DefaultManagerName),
	}

	// Create a starvation checker
	starvationChecker := starvationchecker.NewStarvationChecker(constants.StarvationThreshold)

	// Create a snapshot manager
	snapshotManager := fsm.NewSnapshotManager()

	metrics.InitErrorCounter(metrics.ComponentControlLoop, "main")

	return &ControlLoop{
		managers:          managers,
		tickerTime:        constants.DefaultTickerTime,
		configManager:     configManager,
		logger:            log,
		starvationChecker: starvationChecker,
		snapshotManager:   snapshotManager,
		managerTimes:      make(map[string]time.Duration),
		managerTimesMutex: sync.RWMutex{},
		services:          servicesRegistry,
	}
}

// Execute runs the control loop until the context is cancelled.
// This is the main entry point that starts the continuous reconciliation process.
// The loop follows a simple pattern:
// 1. Wait for the next tick interval
// 2. Fetch latest configuration
// 3. Reconcile each manager in sequence
// 4. Update metrics and monitor for starvation
// 5. Handle any errors appropriately
//
// Critical error handling patterns:
// - Deadline exceeded: Log warning and continue (temporary slowness indicating the ticker is too fast or the managers are slow)
// - Context cancelled: Clean shutdown
// - Other errors: Abort the loop
func (c *ControlLoop) Execute(ctx context.Context) error {
	ticker := time.NewTicker(c.tickerTime)
	defer ticker.Stop()

	// Initialize tick counter
	c.currentTick = 0

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			// Increment tick counter on each iteration
			c.currentTick++

			// Measure reconcile time
			start := time.Now()

			// Create a timeout context for the reconcile
			timeoutCtx, cancel := context.WithTimeout(ctx, c.tickerTime)
			// Reconcile the managers
			err := c.Reconcile(timeoutCtx, c.currentTick)
			cancel()

			// Record metrics for the reconcile cycle
			cycleTime := time.Since(start)

			// If cycleTime is greater than tickerTime, log a warning
			if cycleTime > c.tickerTime {
				c.logger.Warnf("Control loop reconcile cycle time is greater then ticker time: %v", cycleTime)
				// If cycleTime is greater than 2*tickerTime, log an error
				if cycleTime > 2*c.tickerTime {
					c.logger.Errorf("Control loop reconcile cycle time is greater then 2*ticker time: %v", cycleTime)
				}
			}

			metrics.ObserveReconcileTime(metrics.ComponentControlLoop, "main", cycleTime)

			// Handle errors differently based on type
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					// For timeouts, log warning but continue
					sentry.ReportIssuef(sentry.IssueTypeWarning, c.logger, "Control loop reconcile timed out: %v", err)
				} else if errors.Is(err, context.Canceled) {
					// For cancellation, exit the loop
					c.logger.Infof("Control loop cancelled")
					return nil
				} else {
					metrics.IncErrorCountAndLog(metrics.ComponentControlLoop, "main", err, c.logger)
					// Any other unhandled error will result in the control loop stopping
					sentry.ReportIssuef(sentry.IssueTypeError, c.logger, "Control loop error: %v", err)
					return err
				}
			}
		}
	}
}

// Reconcile performs a single reconciliation cycle across all managers using parallel execution.
// This is the core algorithm that drives the system toward its desired state:
// 1. Fetch the latest configuration
// 2. Create time budget hierarchy with LoopControlLoopTimeFactor (80% of available time)
// 3. Execute all managers in parallel using errgroup for coordination
// 4. Aggregate results and handle errors from parallel execution
// 5. Create a snapshot of the current system state for external consumers
//
// Parallel Execution Strategy:
//   - Uses errgroup.WithContext for coordinated parallel execution
//   - Each manager runs in its own goroutine with shared error handling
//   - Thread-safe tracking of executed managers and reconciliation status
//   - Time budget enforcement prevents individual managers from consuming excessive time
//   - Early termination if any manager encounters critical errors
//
// Time Budget Management:
//   - Inner context created with LoopControlLoopTimeFactor (80%) of remaining time
//   - Ensures 20% buffer for error handling, cleanup, and snapshot creation
//   - Individual managers checked for sufficient time before execution
//   - Performance monitoring and warnings for time budget violations
//
// Error Handling:
//   - Parallel execution errors are aggregated and wrapped with context
//   - Timeout scenarios handled gracefully with partial success logging
//   - Manager-specific errors include manager name for debugging
//   - System continues operation even if individual managers fail
func (c *ControlLoop) Reconcile(ctx context.Context, ticker uint64) error {
	// Get the config
	if c.configManager == nil {
		return fmt.Errorf("config manager is not set")
	}

	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// 1) Retrieve or create the "previous" snapshot
	prevSnapshot := c.snapshotManager.GetSnapshot()
	var newSnapshot fsm.SystemSnapshot

	// If there is no previous snapshot, create a new one
	// Note: should fetching the config in step 2 fail, the snapshot will not be updated
	// Hence, once we have a snapshot, we will always have a config
	if prevSnapshot == nil {
		// If none existed, create an empty one
		newSnapshot = fsm.SystemSnapshot{
			Managers:     make(map[string]fsm.ManagerSnapshot),
			SnapshotTime: time.Now(),
			Tick:         ticker,
		}
	} else {
		// the new snapshot is a deep copy of the previous snapshot
		err := deepcopy.Copy(&newSnapshot, prevSnapshot)
		if err != nil {
			sentry.ReportIssuef(sentry.IssueTypeError, c.logger, "Failed to deep copy snapshot: %v", err)
			return fmt.Errorf("failed to deep copy snapshot: %w", err)
		}
		newSnapshot.Tick = ticker
		newSnapshot.SnapshotTime = time.Now()
	}

	// 2) Get the config, this can fail for example through filesystem errors
	// Therefore we need a backoff here
	// GetConfig returns a temporary backoff error or a permanent failure error
	cfg, err := c.configManager.GetConfig(ctx, ticker)
	if err != nil {
		// Handle temporary backoff errors --> we want to continue reconciling
		if backoff.IsTemporaryBackoffError(err) {
			c.logger.Debugf("Skipping reconcile cycle due to temporary config backoff: %v", err)
			return nil
		} else if backoff.IsPermanentFailureError(err) { // Handle permanent failure errors --> we want to stop the control loop
			originalErr := backoff.ExtractOriginalError(err)
			sentry.ReportIssuef(sentry.IssueTypeError, c.logger, "Config manager has permanently failed after max retries: %v (original error: %v)",
				err, originalErr)
			metrics.IncErrorCountAndLog(metrics.ComponentControlLoop, "config_permanent_failure", err, c.logger)

			// Propagate the error to the parent component so it can potentially restart the system
			return fmt.Errorf("config permanently failed, system needs intervention: %w", err)
		} else {
			// Handle other errors --> we want to continue reconciling
			sentry.ReportIssuef(sentry.IssueTypeError, c.logger, "Config manager error: %v", err)
			return nil
		}
	}

	// 3) Place the newly fetched config into the snapshot
	newSnapshot.CurrentConfig = cfg

	if c == nil {
		return fmt.Errorf("service registry is nil, possible initialization failure")
	}
	// 4) If your filesystem service is a buffered FS, sync once per loop:
	bufferedFs, ok := c.services.GetFileSystem().(*filesystem.BufferedService)
	if ok {
		// Step 1: Flush all pending writes to disk
		err = bufferedFs.SyncToDisk(ctx)
		if err != nil {
			sentry.ReportIssuef(sentry.IssueTypeError, c.logger, "Failed to sync S6 filesystem to disk: %v", err)
			return fmt.Errorf("failed to sync S6 filesystem to disk: %w", err)
		}

		// Step 2: Read the filesystem from disk
		err = bufferedFs.SyncFromDisk(ctx)
		if err != nil {
			sentry.ReportIssuef(sentry.IssueTypeError, c.logger, "Failed to sync S6 filesystem from disk: %v", err)
			return fmt.Errorf("failed to sync S6 filesystem from disk: %w", err)
		}
	}

	// <factor>% ctx to ensure we finish in time.
	deadline, ok := ctx.Deadline()
	if !ok {
		return ctxutil.ErrNoDeadline
	}
	remainingTime := time.Until(deadline)
	timeToAdd := time.Duration(float64(remainingTime) * constants.LoopControlLoopTimeFactor)
	newDeadline := time.Now().Add(timeToAdd)
	innerCtx, cancel := context.WithDeadline(ctx, newDeadline)
	defer cancel()

	// 5) Reconcile each manager with the current tick count and passing in the newSnapshot
	// Reset manager times for this reconciliation cycle
	c.managerTimesMutex.Lock()
	c.managerTimes = make(map[string]time.Duration)
	c.managerTimesMutex.Unlock()

	// Track executed managers for logging purposes
	var executedManagers []string
	executedManagersMutex := sync.RWMutex{}

	// Track if any managers were reconciled
	hasAnyReconciles := false
	hasAnyReconcilesMutex := sync.Mutex{}

	errorgroup, _ := errgroup.WithContext(innerCtx)
	// Limit concurrent manager operations for I/O-bound workloads
	errorgroup.SetLimit(constants.MaxConcurrentFSMOperations)

	// If we have more managers than the concurrency limit, schedule only a subset
	// and rotate based on tick to ensure all managers get scheduled over time
	startIdx := 0
	endIdx := len(c.managers)

	if len(c.managers) > constants.MaxConcurrentFSMOperations {
		// Calculate rotation based on current tick
		totalBatches := (len(c.managers) + constants.MaxConcurrentFSMOperations - 1) / constants.MaxConcurrentFSMOperations
		currentBatch := int(c.currentTick % uint64(totalBatches))

		startIdx = currentBatch * constants.MaxConcurrentFSMOperations
		endIdx = startIdx + constants.MaxConcurrentFSMOperations
		if endIdx > len(c.managers) {
			endIdx = len(c.managers)
		}

		c.logger.Debugf("Scheduling manager batch %d/%d: managers %d-%d (tick %d)",
			currentBatch+1, totalBatches, startIdx, endIdx-1, c.currentTick)
	}

	// Schedule the selected batch of managers
	for i := startIdx; i < endIdx; i++ {
		capturedManager := c.managers[i]

		started := errorgroup.TryGo(func() error {
			// It might be that .Go is blocked until the ctx is already cancelled, in that case we just return
			if innerCtx.Err() != nil {
				c.logger.Debugf("Context is already cancelled, skipping manager %s", capturedManager.GetManagerName())
				return nil
			}

			reconciled, err := c.reconcileManager(innerCtx, capturedManager, &executedManagers, &executedManagersMutex, newSnapshot)
			if err != nil {
				return err
			}
			if reconciled {
				hasAnyReconcilesMutex.Lock()
				hasAnyReconciles = true
				hasAnyReconcilesMutex.Unlock()
			}
			return nil
		})
		if !started {
			c.logger.Debugf("To many running managers, skipping remaining")
			break
		}
	}
	waitErrorChannel := make(chan error, 1)
	go func() {
		waitErrorChannel <- errorgroup.Wait()
	}()

	select {
	case wgErr := <-waitErrorChannel:
		err = wgErr
	case <-innerCtx.Done():
		err = innerCtx.Err()
	}

	// If any managers were reconciled, create a snapshot
	hasAnyReconcilesMutex.Lock()
	defer hasAnyReconcilesMutex.Unlock()
	if hasAnyReconciles {
		// Create a snapshot after any successful reconciliation
		c.updateSystemSnapshot(ctx, cfg)
		return nil
	}

	// If there was an error during any of the managers, return it
	if err != nil {
		return err
	}

	if c.starvationChecker != nil {
		// Check for starvation
		err, _ := c.starvationChecker.Reconcile(ctx, cfg)
		if err != nil {
			return fmt.Errorf("starvation checker reconciliation failed: %w", err)
		}
	} else {
		return fmt.Errorf("starvation checker is not set")
	}

	// 6) Finally, persist the updated snapshot
	c.updateSystemSnapshot(ctx, cfg)

	// Return nil if no errors occurred
	return nil
}

// updateSystemSnapshot creates a snapshot of the current system state
func (c *ControlLoop) updateSystemSnapshot(ctx context.Context, cfg config.FullConfig) {
	// Check if logger is nil to prevent panic
	if c.logger == nil {
		// If logger is nil, initialize it with a default logger
		c.logger = logger.For(logger.ComponentControlLoop)
	}

	if c.snapshotManager == nil {
		sentry.ReportIssuef(sentry.IssueTypeWarning, c.logger, "[updateSystemSnapshot] Cannot create system snapshot: snapshot manager is not set")
		return
	}

	snapshot, err := fsm.GetManagerSnapshots(c.managers, c.currentTick, cfg)
	if err != nil {
		c.logger.Errorf("Failed to create system snapshot: %v", err)
		sentry.ReportIssuef(sentry.IssueTypeError, c.logger, "[updateSystemSnapshot] Failed to create system snapshot: %v", err)
		return
	}

	c.snapshotManager.UpdateSnapshot(snapshot)
	c.logger.Debugf("Updated system snapshot at tick %d", c.currentTick)
}

// GetSystemSnapshot returns the current snapshot of the system state
// This is thread-safe and can be called from any goroutine
func (c *ControlLoop) GetSystemSnapshot() *fsm.SystemSnapshot {
	// Check if logger is nil to prevent panic
	if c.logger == nil {
		// If logger is nil, initialize it with a default logger
		c.logger = logger.For(logger.ComponentControlLoop)
	}

	if c.snapshotManager == nil {
		sentry.ReportIssuef(sentry.IssueTypeWarning, c.logger, "[GetSystemSnapshot] Cannot get system snapshot: snapshot manager is not set")
		return nil
	}
	return c.snapshotManager.GetSnapshot()
}

// GetConfigManager returns the config manager
// This can be used by components that need direct access to the current configuration
func (c *ControlLoop) GetConfigManager() config.ConfigManager {
	return c.configManager
}

// GetSnapshotManager returns the snapshot manager
func (c *ControlLoop) GetSnapshotManager() *fsm.SnapshotManager {
	return c.snapshotManager
}

// Stop gracefully terminates the control loop and its components.
// This provides clean shutdown of all managed resources:
// - Stops the starvation checker background goroutine
// - Signals cancellation to the main loop
//
// This should be called as part of system shutdown to prevent
// resource leaks and ensure clean termination.
func (c *ControlLoop) Stop(ctx context.Context) error {
	if c.starvationChecker != nil {
		// Stop the starvation checker
		c.starvationChecker.Stop()
	} else {
		return fmt.Errorf("starvation checker is not set")
	}

	// Signal the control loop to stop
	ctx.Done()
	return nil
}

// reconcileManager executes a single manager within the parallel execution context.
// It demonstrates proper error handling, timeout management, and performance monitoring
// in a concurrent environment.
//
// The function handles three critical aspects:
// 1. Time Budget Validation - Ensures sufficient time before starting work
// 2. Concurrent State Management - Thread-safe updates to shared execution tracking
// 3. Performance Monitoring - Detailed timing and efficiency metrics
//
// Error scenarios and handling:
//   - ErrNoDeadline: Context missing deadline, indicates improper setup
//   - ErrInsufficientTime: Not enough time remaining, manager execution skipped
//   - Manager-specific errors: Wrapped with manager context for debugging
//
// Performance considerations:
//   - Execution time is measured and logged for performance monitoring
//   - Time budget warnings are issued when managers approach limits (using LoopControlLoopTimeFactor)
//   - Thread-safe manager time tracking for system-wide visibility
//
// The function returns:
//   - bool: whether the manager performed reconciliation work
//   - error: any error encountered during execution, wrapped with manager context
func (c *ControlLoop) reconcileManager(ctx context.Context, manager fsm.FSMManager[any], executedManagers *[]string, executedManagersMutex *sync.RWMutex, newSnapshot fsm.SystemSnapshot) (bool, error) {
	managerName := manager.GetManagerName()

	executedManagersMutex.Lock()
	*executedManagers = append(*executedManagers, managerName)
	executedManagersMutex.Unlock()

	// Record manager execution time
	managerStart := time.Now()
	err, reconciled := manager.Reconcile(ctx, newSnapshot, c.services)
	executionTime := time.Since(managerStart)

	c.managerTimesMutex.Lock()
	c.managerTimes[managerName] = executionTime
	c.managerTimesMutex.Unlock()

	if err != nil {
		metrics.IncErrorCountAndLog(metrics.ComponentControlLoop, managerName, err, c.logger)
		return false, fmt.Errorf("manager %s reconciliation failed: %w", managerName, err)
	}

	return reconciled, nil
}
