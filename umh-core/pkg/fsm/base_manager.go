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

package fsm

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"time"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/ctxutil"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// Rate limiting is implemented using manager-specific ticks (managerTick) instead of global ticks.
// This enables multiple managers to operate independently without affecting each other's rate limiting.
// Each manager maintains its own tick counter that increments on each reconciliation cycle.

// ObservedState is a marker interface for type safety of state implementations.
type ObservedState interface {
	// IsObservedState is a marker method to ensure type safety
	IsObservedState()
}

// FSMInstance defines the interface for a finite state machine instance.
// Each instance has a current state and a desired state, and can be reconciled
// to move toward the desired state.
type FSMInstance interface {
	// GetCurrentFSMState returns the current state of the instance
	GetCurrentFSMState() string
	// GetDesiredFSMState returns the desired state of the instance
	GetDesiredFSMState() string
	// SetDesiredFSMState sets the desired state of the instance
	SetDesiredFSMState(desiredState string) error
	// IsTransientStreakCounterMaxed returns whether the transient streak counter
	// has reached the maximum number of ticks, which means that the FSM is stuck in a state
	// and should be removed
	IsTransientStreakCounterMaxed() bool
	// Reconcile moves the instance toward its desired state
	// Returns an error if reconciliation fails, and a boolean indicating
	// whether a change was made to the instance's state
	// The filesystemService parameter is used to read and write to the filesystem.
	// Specifically it is used so that we only need to read in the entire file system once, and then can pass it to all the managers and instances, who can then save on I/O operations.
	Reconcile(ctx context.Context, snapshot SystemSnapshot, services serviceregistry.Provider) (error, bool)
	// Remove initiates the removal process for this instance
	Remove(ctx context.Context) error
	// GetLastObservedState returns the last known state of the instance
	// This is cached data from the last reconciliation cycle
	GetLastObservedState() ObservedState
	// GetMinimumRequiredTime returns the minimum required time for this instance
	GetMinimumRequiredTime() time.Duration

	// FSMInstanceActions defines the actions that can be performed on an FSM instance
	FSMInstanceActions
}

// FSMManager defines the interface for managing multiple FSM instances.
// It provides methods for retrieving and reconciling instances.
type FSMManager[C any] interface {
	// GetInstances returns all instances managed by this manager
	GetInstances() map[string]FSMInstance
	// GetInstance returns an instance by name
	GetInstance(name string) (FSMInstance, bool)
	// Reconcile ensures that all instances are moving toward their desired state
	// The tick parameter provides a counter to track operation rate limiting
	// The filesystemService parameter is used to read and write to the filesystem.
	// Specifically it is used so that we only need to read in the entire file system once, and then can pass it to all the managers and instances, who can then save on I/O operations.
	Reconcile(ctx context.Context, snapshot SystemSnapshot, services serviceregistry.Provider) (error, bool)
	// GetManagerName returns the name of this manager for logging and metrics
	GetManagerName() string
}

// BaseFSMManager provides a generic, reusable implementation of the FSM management pattern.
// It serves as the foundation for specialized managers like S6Manager and BenthosManager,
// eliminating code duplication while allowing type-safe specialization through Go generics.
//
// Why it matters:
// - DRY (Don't Repeat Yourself): Implements common reconciliation logic once, shared across managers
// - Separation of concerns: Concrete managers only need to implement domain-specific logic
// - Standardization: Ensures consistent behavior for instance lifecycle management
// - Metrics: Provides uniform performance tracking and error reporting
// - Safety: Type parameters ensure type-safe operations while still sharing core logic
//
// How it works with generics:
// - Uses type parameter C to represent the specific configuration type (S6Config, BenthosConfig, etc.)
// - Dependency injection pattern with function callbacks for type-specific operations
// - Embedding in concrete managers through composition (S6Manager embeds BaseFSMManager[S6Config])
//
// Key responsibilities:
// - Lifecycle management: Creating, updating, and removing FSM instances
// - State reconciliation: Ensuring instances match their desired state
// - Configuration updates: Detecting and applying configuration changes
// - Error handling: Standardized error reporting and metrics collection.
type BaseFSMManager[C any] struct {
	instances map[string]FSMInstance
	logger    *zap.SugaredLogger

	// These methods are implemented by each concrete manager
	extractConfigs         func(config config.FullConfig) ([]C, error)
	getName                func(C) (string, error)
	getDesiredState        func(C) (string, error)
	createInstance         func(C) (FSMInstance, error)
	compareConfig          func(FSMInstance, C) (bool, error)
	setConfig              func(FSMInstance, C) error
	getMinimumRequiredTime func(FSMInstance) (time.Duration, error)
	managerName            string

	// Manager-specific tick counter
	managerTick uint64

	// Tick tracking for rate limiting (relative to managerTick)
	nextAddTick    uint64 // Earliest tick on which another instance may be added
	nextUpdateTick uint64 // Earliest tick an instance config may be updated again
	nextRemoveTick uint64 // Earliest tick another instance may begin removal
	nextStateTick  uint64 // Earliest tick another desired‑state change may happen

}

// NewBaseFSMManager creates a new base manager with dependencies injected.
// It follows the dependency injection pattern, where type-specific operations
// are provided as function parameters, allowing for code reuse while maintaining type safety.
//
// Parameters:
// - managerName: Identifier for metrics and logging
// - baseDir: Base directory for FSM instance files
// - extractConfigs: Extracts configuration objects of type C from the full config
// - getName: Extracts the unique name from a config object
// - getDesiredState: Determines the target state from a config object
// - createInstance: Factory function that creates appropriate FSM instances
// - compareConfig: Determines if a config change requires an update
// - setConfig: Updates an instance with new configuration
// - getMinimumRequiredTime: Returns the minimum time required for an instance to complete its UpdateObservedState operation.
func NewBaseFSMManager[C any](
	managerName string,
	baseDir string,
	extractConfigs func(config config.FullConfig) ([]C, error),
	getName func(C) (string, error),
	getDesiredState func(C) (string, error),
	createInstance func(C) (FSMInstance, error),
	compareConfig func(FSMInstance, C) (bool, error),
	setConfig func(FSMInstance, C) error,
	getMinimumRequiredTime func(FSMInstance) (time.Duration, error),
) *BaseFSMManager[C] {
	metrics.InitErrorCounter(metrics.ComponentBaseFSMManager, managerName)

	return &BaseFSMManager[C]{
		instances:              make(map[string]FSMInstance),
		logger:                 logger.For(managerName),
		managerName:            managerName,
		managerTick:            0,
		nextAddTick:            0,
		nextUpdateTick:         0,
		nextRemoveTick:         0,
		nextStateTick:          0,
		extractConfigs:         extractConfigs,
		getName:                getName,
		getDesiredState:        getDesiredState,
		createInstance:         createInstance,
		compareConfig:          compareConfig,
		setConfig:              setConfig,
		getMinimumRequiredTime: getMinimumRequiredTime,
	}
}

// GetInstances returns all instances managed by the manager.
// This provides access to the current set of running FSM instances,
// which can be useful for debugging or monitoring purposes.
func (m *BaseFSMManager[C]) GetInstances() map[string]FSMInstance {
	return m.instances
}

// GetInstance returns an instance by name.
// This allows direct access to a specific FSM instance for operations
// outside the normal reconciliation cycle.
//
// Parameters:
// - name: The unique identifier of the instance to retrieve
//
// Returns:
// - The FSMInstance if found
// - A boolean indicating whether the instance exists.
func (m *BaseFSMManager[C]) GetInstance(name string) (FSMInstance, bool) {
	instance, ok := m.instances[name]

	return instance, ok
}

// AddInstanceForTest adds an instance to the manager for testing purposes.
// This method exists solely to support unit testing of managers and
// should not be used in production code.
//
// Parameters:
// - name: The unique identifier for the instance
// - instance: The FSMInstance to add to the manager.
func (m *BaseFSMManager[C]) AddInstanceForTest(name string, instance FSMInstance) {
	m.instances[name] = instance
}

// GetManagerName returns the name of the manager.
// This is used for metrics reporting and logging to identify
// which manager generated a particular metric or log entry.
func (m *BaseFSMManager[C]) GetManagerName() string {
	return m.managerName
}

// GetManagerTick returns the current manager-specific tick count.
func (m *BaseFSMManager[C]) GetManagerTick() uint64 {
	return m.managerTick
}

// GetNextAddTick returns the earliest tick on which another instance may be added.
func (m *BaseFSMManager[C]) GetNextAddTick() uint64 {
	return m.nextAddTick
}

// GetNextUpdateTick returns the earliest tick on which an instance config may be updated again.
func (m *BaseFSMManager[C]) GetNextUpdateTick() uint64 {
	return m.nextUpdateTick
}

// GetNextRemoveTick returns the earliest tick on which another instance may begin removal.
func (m *BaseFSMManager[C]) GetNextRemoveTick() uint64 {
	return m.nextRemoveTick
}

// GetNextStateTick returns the earliest tick on which another desired‑state change may happen.
func (m *BaseFSMManager[C]) GetNextStateTick() uint64 {
	return m.nextStateTick
}

// Reconcile implements the core FSM management algorithm that powers the control loop.
// This method is called repeatedly by the control loop to ensure the system state
// converges toward the desired state defined in configuration.
//
// The reconciliation process follows a clear sequence:
// 1. Extract configurations for this specific manager type from the full config
// 2. For each configured instance:
//   - Create new instances if they don't exist
//   - Update configuration of existing instances if changed
//   - Update desired state if changed
//
// 3. Clean up instances that are no longer in the configuration
// 4. Reconcile each managed instance's internal state
//
// Performance metrics are collected for each phase of reconciliation,
// enabling fine-grained monitoring of system behavior.
//
// Reconcile ensures that all instances are moving toward their desired state using parallel execution.
// This method implements the core FSM manager reconciliation loop with concurrent instance processing.
//
// PARALLEL EXECUTION DESIGN:
//   - Uses errgroup.WithContext for coordinated parallel execution of all instances
//   - Limits concurrent goroutines to runtime.NumCPU() to prevent CPU starvation
//   - Thread-safe coordination using mutexes for shared state updates
//   - Time budget validation before spawning goroutines to prevent deadline violations
//   - Graceful handling of context cancellation during parallel execution
//
// TIME BUDGET MANAGEMENT:
//   - Creates inner context with BaseManagerControlLoopTimeFactor (95%) of available time
//   - Reserves 5% for error aggregation, cleanup, and instance removal operations
//   - Pre-validates execution time budget using getMinimumRequiredTime
//   - Skips instances without sufficient time rather than causing timeout failures
//
// CONCURRENT SAFETY GUARANTEES:
//   - hasAnyReconcilesMutex protects the reconciliation status flag
//   - instancesToRemoveMutex guards the instance removal list
//   - Instance map operations are serialized after parallel execution completes
//   - Error collection and aggregation through errgroup pattern
//
// OPERATIONAL BEHAVIOR:
//   - Continues processing other instances even if individual instances fail
//   - Logs warnings for instances skipped due to insufficient time
//   - Maintains rate limiting through manager tick scheduling
//   - Supports graceful shutdown through context cancellation
//
// ERROR HANDLING:
//   - Timeout scenarios handled with context cancellation
//   - Individual instance errors aggregated through errgroup
//   - Metrics collection for error rates and timing violations
//   - Detailed logging for debugging parallel execution issues
//
// PERFORMANCE CONSIDERATIONS:
//   - Parallel execution reduces overall reconciliation time
//   - CPU-bound operations limited to prevent system overload
//   - I/O operations optimized through shared filesystem service
//   - Memory usage controlled through goroutine limits
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - snapshot: System snapshot containing current configuration and state
//   - services: Service registry provider for accessing system services
//
// Returns:
//   - error: Any error encountered during reconciliation, nil if all succeeded
//   - bool: True if any instance performed reconciliation work
func (m *BaseFSMManager[C]) Reconcile(
	ctx context.Context,
	snapshot SystemSnapshot,
	services serviceregistry.Provider,
) (error, bool) {
	// Increment manager-specific tick counter
	m.managerTick++

	// Start tracking metrics for the manager
	start := time.Now()

	defer func() {
		// Record total reconcile time at the end
		metrics.ObserveReconcileTime(metrics.ComponentBaseFSMManager, m.managerName, time.Since(start))
	}()

	// Check if context is already cancelled
	if ctx.Err() != nil {
		// If it is a ctx context exceeded, return no error
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			m.logger.Debugf("Context is already cancelled, skipping manager %s", m.managerName)

			return nil, false
		}

		return ctx.Err(), false
	}

	// Step 1: Extract the specific configs from the full config
	extractStart := time.Now()

	desiredState, err := m.extractConfigs(snapshot.CurrentConfig)
	if err != nil {
		metrics.IncErrorCountAndLog(metrics.ComponentBaseFSMManager, m.managerName, err, m.logger)

		return fmt.Errorf("failed to extract configs: %w", err), false
	}

	metrics.ObserveReconcileTime(metrics.ComponentBaseFSMManager, m.managerName+".extract_configs", time.Since(extractStart))

	// Step 2: Create or update instances
	for _, cfg := range desiredState {
		name, err := m.getName(cfg)
		if err != nil {
			metrics.IncErrorCountAndLog(metrics.ComponentBaseFSMManager, m.managerName, err, m.logger)

			return fmt.Errorf("failed to get name: %w", err), false
		}

		// If the instance does not exist, create it and set it to the desired state
		if _, ok := m.instances[name]; !ok {
			// Using manager-specific ticks for rate limiting
			if m.managerTick < m.nextAddTick {
				m.logger.Debugf(
					"Rate limiting: Skipping creation of %s (next add @%d, now %d)",
					name, m.nextAddTick, m.managerTick,
				)

				continue // Skip this instance for now, will be created on a future tick
			}

			createStart := time.Now()

			instance, err := m.createInstance(cfg)
			if err != nil || instance == nil {
				metrics.IncErrorCountAndLog(metrics.ComponentBaseFSMManager, m.managerName, err, m.logger)

				return fmt.Errorf("failed to create instance: %w", err), false
			}

			metrics.ObserveReconcileTime(metrics.ComponentBaseFSMManager, m.managerName+".create_instance", time.Since(createStart))

			desiredState, err := m.getDesiredState(cfg)
			if err != nil {
				metrics.IncErrorCountAndLog(metrics.ComponentBaseFSMManager, m.managerName, err, m.logger)

				return fmt.Errorf("failed to get desired state: %w", err), false
			}

			err = instance.SetDesiredFSMState(desiredState)
			if err != nil {
				metrics.IncErrorCountAndLog(metrics.ComponentBaseFSMManager, m.managerName, err, m.logger)
				m.logger.Errorf("failed to set desired state: %v for newly to be created instance %s", err, name)

				continue // Skip this instance for now, we should not create it
			}

			m.instances[name] = instance

			// Update last add tick using manager-specific tick
			m.nextAddTick = m.schedule(constants.TicksBeforeNextAdd)

			return nil, true
		}

		// If the instance exists, but the config is different, update it
		compareStart := time.Now()

		equal, err := m.compareConfig(m.instances[name], cfg)
		if err != nil {
			metrics.IncErrorCountAndLog(metrics.ComponentBaseFSMManager, m.managerName, err, m.logger)

			return fmt.Errorf("failed to compare config: %w", err), false
		}

		metrics.ObserveReconcileTime(metrics.ComponentBaseFSMManager, m.managerName+".compare_config", time.Since(compareStart))

		if !equal {
			// Using manager-specific ticks for rate limiting
			if m.managerTick < m.nextUpdateTick {
				m.logger.Debugf(
					"Rate limiting: Skipping update of %s (next update @%d, now %d)",
					name, m.nextUpdateTick, m.managerTick,
				)

				continue // Skip this update for now, will be updated on a future tick
			}

			updateStart := time.Now()

			err := m.setConfig(m.instances[name], cfg)
			if err != nil {
				metrics.IncErrorCountAndLog(metrics.ComponentBaseFSMManager, m.managerName, err, m.logger)

				return fmt.Errorf("failed to set config: %w", err), false
			}

			metrics.ObserveReconcileTime(metrics.ComponentBaseFSMManager, m.managerName+".set_config", time.Since(updateStart))

			m.logger.Infof("Updated config of instance %s", name)
			// Update last update tick using manager-specific tick
			m.nextUpdateTick = m.schedule(constants.TicksBeforeNextUpdate)

			return nil, true
		}

		// If the instance exists, but the desired state is different, update it
		desiredState, err := m.getDesiredState(cfg)
		if err != nil {
			metrics.IncErrorCountAndLog(metrics.ComponentBaseFSMManager, m.managerName, err, m.logger)

			return fmt.Errorf("failed to get desired state: %w", err), false
		}

		if m.instances[name].GetDesiredFSMState() != desiredState {
			// Using manager-specific ticks for rate limiting
			if m.managerTick < m.nextStateTick {
				m.logger.Debugf(
					"Rate limiting: Skipping state change of %s (next state @%d, now %d)",
					name, m.nextStateTick, m.managerTick,
				)

				continue // Skip this state change for now, will be updated on a future tick
			}

			m.logger.Infof("Updated desired state of instance %s from %s to %s",
				name, m.instances[name].GetDesiredFSMState(), desiredState)

			err := m.instances[name].SetDesiredFSMState(desiredState)
			if err != nil {
				metrics.IncErrorCountAndLog(metrics.ComponentBaseFSMManager, m.managerName, err, m.logger)
				m.logger.Errorf("failed to set desired state: %w for instance %s", err, name)

				continue // Skip this state change for now, it is broken, we should not update it
			}

			// Update last state change tick using manager-specific tick
			m.nextStateTick = m.schedule(constants.TicksBeforeNextState)

			return nil, true
		}
	}
	// Step 3: Clean up any instances that are not in desiredState, or are in the removed state
	// Before deletion, they need to be gracefully stopped and we need to wait until they are in the state removed

	// Collect instances to delete to avoid modifying map during iteration
	instancesToDelete := make([]string, 0)

	for instanceName := range m.instances {
		// If the instance is not in desiredState, start its removal process
		found := false

		for _, desired := range desiredState {
			name, err := m.getName(desired)
			if err != nil {
				metrics.IncErrorCountAndLog(metrics.ComponentBaseFSMManager, m.managerName, err, m.logger)

				return fmt.Errorf("failed to get name: %w", err), false
			}

			if name == instanceName {
				found = true

				break
			}
		}

		instance := m.instances[instanceName]
		if instance == nil {
			m.logger.Debugf("instance %s is nil, will be deleted from the manager", instanceName)
			// TODO: Check if we need to do anything else here
			continue
		}

		switch instance.GetCurrentFSMState() {
		case internalfsm.LifecycleStateRemoving:
			m.logger.Debugf("instance %s is already in removing state, waiting until it is removed", instanceName)

			continue
		case internalfsm.LifecycleStateRemoved:
			m.logger.Debugf("instance %s is in removed state, will be deleted from the manager", instanceName)
			instancesToDelete = append(instancesToDelete, instanceName)

			continue
		default:
			// If the instance is in desiredState, we don't need to remove it
			if found {
				continue
			}

			// Using manager-specific ticks for rate limiting
			if m.managerTick < m.nextRemoveTick {
				// Currently uncommented to prevent log spam (the integration tests will create a lot of services)
				// m.logger.Debugf("Rate limiting: Skipping removal of instance %s (waiting %d more ticks)",
				// 	instanceName, TicksBeforeNextRemove-(m.managerTick-m.lastRemoveTick))
				continue // Skip this removal for now, will be removed on a future tick
			}

			// Otherwise, we need to remove the instance
			m.logger.Debugf("instance %s is in state %s, starting the removing process", instanceName, instance.GetCurrentFSMState())

			err := instance.Remove(ctx)
			if err != nil {
				return err, false
			}

			// Update last remove tick using manager-specific tick
			m.nextRemoveTick = m.schedule(constants.TicksBeforeNextRemove)

			return nil, true
		}
	}

	// Find first instance in "removed" state
	for _, instanceName := range instancesToDelete {
		m.logger.Debugf("deleting instance %s from the manager", instanceName)
		delete(m.instances, instanceName)
	}

	// <factor>% ctx to ensure we finish in time.
	deadline, ok := ctx.Deadline()
	if !ok {
		return ctxutil.ErrNoDeadline, false
	}

	remainingTime := time.Until(deadline)
	timeToAdd := time.Duration(float64(remainingTime) * constants.BaseManagerControlLoopTimeFactor)
	newDeadline := time.Now().Add(timeToAdd)

	innerCtx, cancel := context.WithDeadline(ctx, newDeadline)
	defer cancel()

	// Reconcile instances
	// We do not use the returned ctx, as it cancels once any of the reconciles returns either an error or finishes (And the 2nd behaviour is undesired.)

	errorgroup, _ := errgroup.WithContext(innerCtx)
	// Limit concurrent FSM operations for I/O-bound workloads
	errorgroup.SetLimit(constants.MaxConcurrentFSMOperations)

	hasAnyReconciles := false
	hasAnyReconcilesMutex := sync.Mutex{}

	instancesToRemove := make([]string, 0)
	instancesToRemoveMutex := sync.Mutex{}

	// Update the snapshot tick to the manager tick
	snapshot.Tick = m.managerTick

	// Convert map to slice for deterministic ordering and rotation
	instanceNames := make([]string, 0, len(m.instances))
	for name := range m.instances {
		instanceNames = append(instanceNames, name)
	}

	// If we have more instances than the concurrency limit, schedule only a subset
	// and rotate based on tick to ensure all instances get scheduled over time
	startIdx := 0
	endIdx := len(instanceNames)

	if len(instanceNames) > constants.MaxConcurrentFSMOperations {
		// Calculate rotation based on manager tick
		totalBatches := (len(instanceNames) + constants.MaxConcurrentFSMOperations - 1) / constants.MaxConcurrentFSMOperations
		currentBatch := int(m.managerTick % uint64(totalBatches))

		startIdx = currentBatch * constants.MaxConcurrentFSMOperations

		endIdx = startIdx + constants.MaxConcurrentFSMOperations
		if endIdx > len(instanceNames) {
			endIdx = len(instanceNames)
		}

		m.logger.Debugf("Scheduling batch %d/%d: instances %d-%d (tick %d)",
			currentBatch+1, totalBatches, startIdx, endIdx-1, m.managerTick)
	}

	// Schedule the selected batch of instances
	for i := startIdx; i < endIdx; i++ {
		name := instanceNames[i]
		instance := m.instances[name]
		// If the ctx is already expired, we can skip adding new goroutines
		if innerCtx.Err() != nil {
			m.logger.Debugf("context expired, skipping reconciliation of instance %s", name)

			break
		}

		// Check time budget before spawning goroutine to prevent concurrent execution
		// from exceeding the control loop's deadline
		minimumRequiredTime, execTimeErr := m.getMinimumRequiredTime(instance)
		if execTimeErr != nil {
			metrics.IncErrorCountAndLog(metrics.ComponentBaseFSMManager, m.managerName, execTimeErr, m.logger)

			return fmt.Errorf("failed to get minimum required time for instance %s: %w", name, execTimeErr), false
		}

		remaining, sufficient, timeErr := ctxutil.HasSufficientTime(innerCtx, minimumRequiredTime)
		if timeErr != nil {
			if errors.Is(timeErr, ctxutil.ErrNoDeadline) {
				return errors.New("no deadline set in context"), false
			}
			// For any other error, log and abort reconciliation
			metrics.IncErrorCountAndLog(metrics.ComponentBaseFSMManager, m.managerName, timeErr, m.logger)

			return fmt.Errorf("deadline check error for instance %s: %w", name, timeErr), false
		}

		if !sufficient {
			m.logger.Warnf("not enough time left to reconcile instance %s (only %v remaining, needed %v), skipping",
				name, remaining, minimumRequiredTime)
			hasAnyReconcilesMutex.Lock()

			hasAnyReconciles = true

			hasAnyReconcilesMutex.Unlock()

			continue // Skip this instance but continue with others (who might need less time)
		}

		nameCaptured := name
		instanceCaptured := instance

		started := errorgroup.TryGo(func() error {
			if innerCtx.Err() != nil {
				m.logger.Debugf("Context is already cancelled, skipping instance %s", nameCaptured)

				return nil
			}

			reconciled, shallBeRemoved, err := m.reconcileInstanceWithTimeout(innerCtx, instanceCaptured, services, nameCaptured, snapshot, minimumRequiredTime)
			if err != nil {
				return err
			}

			if reconciled {
				hasAnyReconcilesMutex.Lock()

				hasAnyReconciles = true

				hasAnyReconcilesMutex.Unlock()
			}

			if shallBeRemoved {
				instancesToRemoveMutex.Lock()

				instancesToRemove = append(instancesToRemove, nameCaptured)

				instancesToRemoveMutex.Unlock()
			}

			return nil
		})
		if !started {
			m.logger.Debugf("To many running managers, skipping remaining")

			break
		}
	}

	waitErrorChannel := make(chan error, 1)

	go func() {
		waitErrorChannel <- errorgroup.Wait()
	}()

	// Wait for either the error group to finish or the context to be done
	select {
	case wgErr := <-waitErrorChannel:
		err = wgErr
	case <-innerCtx.Done():
		err = innerCtx.Err()
	}

	instancesToRemoveMutex.Lock()

	for _, name := range instancesToRemove {
		delete(m.instances, name)
	}

	instancesToRemoveMutex.Unlock()

	// Ignore context deadline issues
	if errors.Is(err, context.DeadlineExceeded) {
		hasAnyReconcilesMutex.Lock()
		defer hasAnyReconcilesMutex.Unlock()

		return nil, hasAnyReconciles
	}

	// Return nil if no errors occurred
	hasAnyReconcilesMutex.Lock()
	defer hasAnyReconcilesMutex.Unlock()

	return err, hasAnyReconciles
}

// GetLastObservedStates returns the last known states of all instances.
func (m *BaseFSMManager[C]) GetLastObservedStates() map[string]ObservedState {
	states := make(map[string]ObservedState)
	for name, instance := range m.instances {
		states[name] = instance.GetLastObservedState()
	}

	return states
}

// GetLastObservedState returns the last known state of a specific instance.
func (m *BaseFSMManager[C]) GetLastObservedState(serviceName string) (ObservedState, error) {
	if instance, exists := m.instances[serviceName]; exists {
		return instance.GetLastObservedState(), nil
	}

	m.logger.Debugf("instance %s not found: %v", serviceName, m.instances)

	return nil, fmt.Errorf("instance %s not found", serviceName)
}

// GetCurrentFSMState returns the current state of a specific instance.
func (m *BaseFSMManager[C]) GetCurrentFSMState(serviceName string) (string, error) {
	if instance, exists := m.instances[serviceName]; exists {
		return instance.GetCurrentFSMState(), nil
	}

	return "", fmt.Errorf("instance %s not found", serviceName)
}

// CreateSnapshot creates a ManagerSnapshot from the current manager state.
func (m *BaseFSMManager[C]) CreateSnapshot() ManagerSnapshot {
	snapshot := &BaseManagerSnapshot{
		Name:           m.managerName,
		Instances:      make(map[string]*FSMInstanceSnapshot),
		ManagerTick:    m.managerTick,
		NextAddTick:    m.nextAddTick,
		NextUpdateTick: m.nextUpdateTick,
		NextRemoveTick: m.nextRemoveTick,
		NextStateTick:  m.nextStateTick,
		SnapshotTime:   time.Now(),
	}

	for name, instance := range m.instances {
		instanceSnapshot := FSMInstanceSnapshot{
			ID:           name,
			CurrentState: instance.GetCurrentFSMState(),
			DesiredState: instance.GetDesiredFSMState(),
		}

		// Add observed state if available
		if observedState := instance.GetLastObservedState(); observedState != nil {
			// This requires proper implementation of ObservedStateSnapshot conversion
			// in each concrete instance type
			if converter, ok := instance.(ObservedStateConverter); ok {
				instanceSnapshot.LastObservedState = converter.CreateObservedStateSnapshot()
			}
		}

		snapshot.Instances[name] = &instanceSnapshot
	}

	return snapshot
}

// ObservedStateConverter is an interface for objects that can convert their observed state to a snapshot.
type ObservedStateConverter interface {
	CreateObservedStateSnapshot() ObservedStateSnapshot
}

// schedule returns the *absolute* control‑loop tick on (or after) which the
// next rate‑limited operation may be executed.
//
// The delay that is added on top of the current manager tick is
//
//	δ = base * (1 ± JitterFraction)
//
// Where `base` is the deterministic cooldown (e.g. TicksBeforeNextAdd)
// and `constants.JitterFraction` ‑ a value in the open unit interval (0 < j < 1) ‑
// controls how much randomness is introduced:
//
//   - j = 0.25  →  delay is uniformly distributed in [0.75 · base, 1.25 · base]
//     With `base == 10` this means 7 – 13 ticks.
//
//   - j = 0      →  no jitter at all (always exactly `base` ticks)
//
//   - j → 1    →  almost full spread, up to 0–2 · base.
//
// The jitter is *symmetric*: on average the mean delay stays equal to `base`,
// the random spread only prevents many managers from waking up on the very
// same tick and thus evens out load spikes.
//
// Example
//
//	// constants.JitterFraction = 0.25
//	next := m.schedule(10)   // 10 ticks ± 25 %  →  7 … 13
func (m *BaseFSMManager[C]) schedule(base uint64) uint64 {
	// Pick a random factor in [1‑j, 1+j]
	factor := 1 + (rand.Float64()*2-1)*constants.JitterFraction
	delta := float64(base) * factor

	// Round to nearest integer tick and add to the current manager tick
	return m.managerTick + uint64(delta+0.5)
}

// maybeEscalateRemoval is the **watch-dog** that prevents an FSM from being
// stuck forever in one of the *transient* lifecycle states.
//
// It is invoked by the manager **once per tick** *after* an
// `inst.Reconcile(…)` call has completed.
// It checks if an instance has been in a transient state for too long
// and takes action to prevent it from remaining stuck.
//
// # Inputs
//
//   - inst  – any FSMInstance that was just reconciled.
//   - filesystemService - service used for file operations if a force remove is needed
//
// # Logic
//
//  1. **Check if the instance has been in a transient state too long**
//     by using GetTransientStreakCounter() and comparing to a threshold
//
//  2. **Escalate** depending on the *current* state:
//
//     * **creating / to_be_created**
//     → initiate normal removal. This asks the normal "graceful remove"
//     path to start.
//
//     * **removing**
//     → attempt a hard, unconditional teardown.
//     If the concrete instance implements the ForceRemover interface,
//     the manager calls the method in a detached goroutine.
//
// # Side effects
//
//   - May initiate instance removal
//   - May launch a goroutine that calls `ForceRemove`
//   - No error is returned from this function
//
// # Errors
//
// This helper never returns an error. All failures in operations
// are handled through the normal FSM reconciliation mechanisms.
func (m *BaseFSMManager[C]) maybeEscalateRemoval(ctx context.Context, inst FSMInstance, services serviceregistry.Provider) {
	// Check if the instance has been in a transient state for too long
	if !inst.IsTransientStreakCounterMaxed() {
		return // Not stuck yet
	}

	currentState := inst.GetCurrentFSMState()

	// Find the instance name from the instances map
	var instanceName string

	for name, i := range m.instances {
		if i == inst {
			instanceName = name

			break
		}
	}

	switch currentState {
	case internalfsm.LifecycleStateRemoving:
		// Instance is stuck in removing state - try force removal if possible
		if forceRemover, ok := inst.(interface {
			ForceRemove(context.Context, filesystem.Service) error
		}); ok {
			// Run force removal in a detached goroutine to avoid blocking the main reconciliation loop
			go func() {
				err := forceRemover.ForceRemove(ctx, services.GetFileSystem())
				if err != nil {
					sentry.ReportFSMErrorf(
						m.logger,
						instanceName,
						m.managerName,
						"force_remove_error",
						"Error force removing instance: %v",
						err,
					)
				}

				sentry.ReportIssuef(sentry.IssueTypeWarning, m.logger, "force removing instance %s: %v", instanceName, err)
			}()
		} else {
			sentry.ReportIssuef(sentry.IssueTypeWarning, m.logger, "no force remover for instance %s, cannot force remove", instanceName)
		}

		return
	default:
		// Instance is stuck in creating or to_be_created state - try normal removal
		err := inst.Remove(ctx)
		if err != nil {
			sentry.ReportFSMErrorf(
				m.logger,
				instanceName,
				m.managerName,
				"remove_error",
				"Error removing instance: %v",
				err,
			)

			if forceRemover, ok := inst.(interface {
				ForceRemove(context.Context, filesystem.Service) error
			}); ok {
				// Run force removal in a detached goroutine to avoid blocking the main reconciliation loop
				go func() {
					err := forceRemover.ForceRemove(ctx, services.GetFileSystem())
					if err != nil {
						sentry.ReportFSMErrorf(
							m.logger,
							instanceName,
							m.managerName,
							"force_remove_error",
							"Error force removing instance after normal removal failed: %v",
							err,
						)
					}

					sentry.ReportIssuef(sentry.IssueTypeWarning, m.logger, "force removing instance %s after normal removal failed: %v", instanceName, err)
				}()
			} else {
				sentry.ReportIssuef(sentry.IssueTypeWarning, m.logger, "no force remover for instance %s, cannot force remove", instanceName)
			}
		}

		return
	}
}

func (m *BaseFSMManager[C]) reconcileInstanceWithTimeout(ctx context.Context, instance FSMInstance, services serviceregistry.Provider, name string, snapshot SystemSnapshot, minimumRequiredTime time.Duration) (reconciled bool, shallBeRemoved bool, err error) {
	reconcileStart := time.Now()

	// Time budget check is now done upfront before goroutine creation
	// This method assumes sufficient time has already been verified
	// Use the full manager context instead of artificially limiting to minimumRequiredTime
	// The time budget check is the primary gating mechanism, not context timeout
	instanceCtx := ctx

	// Pass manager-specific tick to instance.Reconcile
	reconcileErr, instanceReconciled := instance.Reconcile(instanceCtx, snapshot, services)
	reconcileTime := time.Since(reconcileStart)
	metrics.ObserveReconcileTime(metrics.ComponentBaseFSMManager, m.managerName+".instances."+name, reconcileTime)

	if reconcileErr != nil {
		metrics.IncErrorCountAndLog(metrics.ComponentBaseFSMManager, m.managerName+".instances."+name, reconcileErr, m.logger)

		// If the error is a permanent failure, remove the instance from the manager
		// so that it can be recreated in further ticks
		if backoff.IsPermanentFailureError(reconcileErr) {
			sentry.ReportFSMErrorf(
				m.logger,
				name,
				m.managerName,
				"reconcile_permanent_failure",
				"Permanent failure reconciling instance: %v",
				reconcileErr,
			)

			return true, true, nil
		}

		sentry.ReportFSMErrorf(
			m.logger,
			name,
			m.managerName,
			"reconcile_error",
			"Error reconciling instance: %v",
			reconcileErr,
		)

		return false, false, fmt.Errorf("error reconciling instance: %w", reconcileErr)
	}

	// Check if the instance has been stuck in a transient state for too long
	m.maybeEscalateRemoval(ctx, instance, services)

	return instanceReconciled, false, nil
}
