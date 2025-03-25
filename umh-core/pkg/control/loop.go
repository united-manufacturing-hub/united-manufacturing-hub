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
	"time"

	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/backoff"
	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/fsm/benthos"
	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/fsm/s6"
	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/metrics"
	s6svc "github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/service/s6"
	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/starvationchecker"
	"go.uber.org/zap"
)

const (
	// defaultTickerTime is the interval between reconciliation cycles.
	// This value balances responsiveness with resource utilization:
	// - Too small: could mean that the managers do not have enough time to complete their work
	// - Too high: Delayed response to configuration changes
	defaultTickerTime = 100 * time.Millisecond

	// starvationThreshold defines when to consider the control loop starved.
	// If no reconciliation has happened for this duration, the starvation
	// detector will log warnings and record metrics.
	// Starvation will take place for example when adding hundreds of new services
	// at once.
	starvationThreshold = 15 * time.Second
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
func NewControlLoop() *ControlLoop {
	// Get a component-specific logger
	log := logger.For(logger.ComponentControlLoop)
	if log == nil {
		// If logger initialization failed somehow, create a no-op logger to avoid nil panics
		log = zap.NewNop().Sugar()
	}

	// Create the managers
	managers := []fsm.FSMManager[any]{
		s6.NewS6Manager("Core"),
		benthos.NewBenthosManager("Core"),
	}

	// Create the config manager with backoff support
	configManager := config.NewFileConfigManagerWithBackoff()

	// Create a starvation checker
	starvationChecker := starvationchecker.NewStarvationChecker(starvationThreshold)

	// Create a snapshot manager
	snapshotManager := fsm.NewSnapshotManager()

	metrics.InitErrorCounter(metrics.ComponentControlLoop, "main")

	// Now clean the S6 service directory except for the known services
	s6Service := s6svc.NewDefaultService()
	log.Debugf("Cleaning S6 service directory: %s", constants.S6BaseDir)
	err := s6Service.CleanS6ServiceDirectory(context.Background(), constants.S6BaseDir)
	if err != nil {
		log.Errorf("Failed to clean S6 service directory: %s", err)
	}
	log.Debugf("S6 service directory cleaned: %s", constants.S6BaseDir)

	return &ControlLoop{
		managers:          managers,
		tickerTime:        defaultTickerTime,
		configManager:     configManager,
		logger:            log,
		starvationChecker: starvationChecker,
		snapshotManager:   snapshotManager,
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

			// Create a timeout context for the reconcile
			timeoutCtx, cancel := context.WithTimeout(ctx, c.tickerTime)
			defer cancel()

			// Measure reconcile time
			start := time.Now()

			// Reconcile the managers
			err := c.Reconcile(timeoutCtx, c.currentTick)

			// Record metrics for the reconcile cycle
			cycleTime := time.Since(start)
			metrics.ObserveReconcileTime(metrics.ComponentControlLoop, "main", cycleTime)

			// Handle errors differently based on type
			if err != nil {
				metrics.IncErrorCount(metrics.ComponentControlLoop, "main")

				if errors.Is(err, context.DeadlineExceeded) {
					// For timeouts, log warning but continue
					c.logger.Warnf("Control loop reconcile timed out: %v", err)
				} else if errors.Is(err, context.Canceled) {
					// For cancellation, exit the loop
					c.logger.Infof("Control loop cancelled")
					return nil
				} else {
					// Any other unhandled error will result in the control loop stopping
					c.logger.Errorf("Control loop error: %v", err)
					return err
				}
			}
		}
	}
}

// Reconcile performs a single reconciliation cycle across all managers.
// This is the core algorithm that drives the system toward its desired state:
// 1. Fetch the latest configuration
// 2. For each manager in sequence:
//   - Call its Reconcile method with the configuration
//   - If error occurs, propagate it upward
//   - If reconciliation occurred (bool=true), skip the reconcilation of the next managers to avoid reaching the ticker interval
//
// 3. Create a snapshot of the current system state for external consumers
func (c *ControlLoop) Reconcile(ctx context.Context, ticker uint64) error {
	// Get the config
	if c.configManager == nil {
		return fmt.Errorf("config manager is not set")
	}

	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Get the config, this can fail for example through filesystem errors
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
			c.logger.Errorf("Config manager has permanently failed after max retries: %v (original error: %v)",
				err, originalErr)
			metrics.IncErrorCount(metrics.ComponentControlLoop, "config_permanent_failure")

			// Propagate the error to the parent component so it can potentially restart the system
			return fmt.Errorf("config permanently failed, system needs intervention: %w", err)
		} else {
			// Handle other errors --> we want to continue reconciling
			c.logger.Errorf("Config manager error: %v", err)
			return nil
		}
	}

	// Reconcile each manager with the current tick count
	for _, manager := range c.managers {
		err, reconciled := manager.Reconcile(ctx, cfg, c.currentTick)
		if err != nil {
			metrics.IncErrorCount(metrics.ComponentControlLoop, manager.GetManagerName())
			return err
		}

		// If the manager was reconciled, skip the reconcilation of the next managers
		if reconciled {
			// Create a snapshot after any successful reconciliation
			c.updateSystemSnapshot(ctx, cfg)
			return nil
		}
	}

	if c.starvationChecker != nil {
		// Check for starvation
		c.starvationChecker.Reconcile(ctx, cfg)
	} else {
		return fmt.Errorf("starvation checker is not set")
	}

	// Create a snapshot after the entire reconciliation cycle
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
		c.logger.Warnf("Cannot create system snapshot: snapshot manager is not set")
		return
	}

	snapshot, err := fsm.GetManagerSnapshots(c.managers, c.currentTick, cfg)
	if err != nil {
		c.logger.Errorf("Failed to create system snapshot: %v", err)
		metrics.IncErrorCount(metrics.ComponentControlLoop, "snapshot_creation")
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
		c.logger.Warnf("Cannot get system snapshot: snapshot manager is not set")
		return nil
	}
	return c.snapshotManager.GetSnapshot()
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
