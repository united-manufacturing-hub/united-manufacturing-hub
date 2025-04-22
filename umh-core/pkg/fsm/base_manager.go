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
	"go.uber.org/zap"
)

// Rate limiting is implemented using manager-specific ticks (managerTick) instead of global ticks.
// This enables multiple managers to operate independently without affecting each other's rate limiting.
// Each manager maintains its own tick counter that increments on each reconciliation cycle.

// ObservedState is a marker interface for type safety of state implementations
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
	// Reconcile moves the instance toward its desired state
	// Returns an error if reconciliation fails, and a boolean indicating
	// whether a change was made to the instance's state
	// The filesystemService parameter is used to read and write to the filesystem.
	// Specifically it is used so that we only need to read in the entire file system once, and then can pass it to all the managers and instances, who can then save on I/O operations.
	Reconcile(ctx context.Context, snapshot SystemSnapshot, filesystemService filesystem.Service) (error, bool)
	// Remove initiates the removal process for this instance
	Remove(ctx context.Context) error
	// GetLastObservedState returns the last known state of the instance
	// This is cached data from the last reconciliation cycle
	GetLastObservedState() ObservedState
	// GetExpectedMaxP95ExecutionTimePerInstance returns the expected max p95 execution time of the instance
	GetExpectedMaxP95ExecutionTimePerInstance() time.Duration

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
	Reconcile(ctx context.Context, snapshot SystemSnapshot, filesystemService filesystem.Service) (error, bool)
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
// - Error handling: Standardized error reporting and metrics collection
type BaseFSMManager[C any] struct {
	instances   map[string]FSMInstance
	logger      *zap.SugaredLogger
	managerName string

	// Manager-specific tick counter
	managerTick uint64

	// Tick tracking for rate limiting (relative to managerTick)
	nextAddTick    uint64 // Earliest tick on which another instance may be added
	nextUpdateTick uint64 // Earliest tick an instance config may be updated again
	nextRemoveTick uint64 // Earliest tick another instance may begin removal
	nextStateTick  uint64 // Earliest tick another desired‑state change may happen

	// These methods are implemented by each concrete manager
	extractConfigs                            func(config config.FullConfig) ([]C, error)
	getName                                   func(C) (string, error)
	getDesiredState                           func(C) (string, error)
	createInstance                            func(C) (FSMInstance, error)
	compareConfig                             func(FSMInstance, C) (bool, error)
	setConfig                                 func(FSMInstance, C) error
	getExpectedMaxP95ExecutionTimePerInstance func(FSMInstance) (time.Duration, error)
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
func NewBaseFSMManager[C any](
	managerName string,
	baseDir string,
	extractConfigs func(config config.FullConfig) ([]C, error),
	getName func(C) (string, error),
	getDesiredState func(C) (string, error),
	createInstance func(C) (FSMInstance, error),
	compareConfig func(FSMInstance, C) (bool, error),
	setConfig func(FSMInstance, C) error,
	getExpectedMaxP95ExecutionTimePerInstance func(FSMInstance) (time.Duration, error),
) *BaseFSMManager[C] {

	metrics.InitErrorCounter(metrics.ComponentBaseFSMManager, managerName)
	return &BaseFSMManager[C]{
		instances:       make(map[string]FSMInstance),
		logger:          logger.For(managerName),
		managerName:     managerName,
		managerTick:     0,
		nextAddTick:     0,
		nextUpdateTick:  0,
		nextRemoveTick:  0,
		nextStateTick:   0,
		extractConfigs:  extractConfigs,
		getName:         getName,
		getDesiredState: getDesiredState,
		createInstance:  createInstance,
		compareConfig:   compareConfig,
		setConfig:       setConfig,
		getExpectedMaxP95ExecutionTimePerInstance: getExpectedMaxP95ExecutionTimePerInstance,
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
// - A boolean indicating whether the instance exists
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
// - instance: The FSMInstance to add to the manager
func (m *BaseFSMManager[C]) AddInstanceForTest(name string, instance FSMInstance) {
	m.instances[name] = instance
}

// GetManagerName returns the name of the manager.
// This is used for metrics reporting and logging to identify
// which manager generated a particular metric or log entry.
func (m *BaseFSMManager[C]) GetManagerName() string {
	return m.managerName
}

// GetManagerTick returns the current manager-specific tick count
func (m *BaseFSMManager[C]) GetManagerTick() uint64 {
	return m.managerTick
}

// GetNextAddTick returns the earliest tick on which another instance may be added
func (m *BaseFSMManager[C]) GetNextAddTick() uint64 {
	return m.nextAddTick
}

// GetNextUpdateTick returns the earliest tick on which an instance config may be updated again
func (m *BaseFSMManager[C]) GetNextUpdateTick() uint64 {
	return m.nextUpdateTick
}

// GetNextRemoveTick returns the earliest tick on which another instance may begin removal
func (m *BaseFSMManager[C]) GetNextRemoveTick() uint64 {
	return m.nextRemoveTick
}

// GetNextStateTick returns the earliest tick on which another desired‑state change may happen
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
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - config: The full configuration to reconcile against
//   - tick: Current tick count for rate limiting operations
//
// Returns:
//   - error: Any error encountered during reconciliation
//   - bool: True if a change was made, indicating the control loop should not
//     run another manager and instead should wait for the next tick
func (m *BaseFSMManager[C]) Reconcile(
	ctx context.Context,
	snapshot SystemSnapshot,
	filesystemService filesystem.Service,
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
		return ctx.Err(), false
	}

	// Step 1: Extract the specific configs from the full config
	extractStart := time.Now()
	desiredState, err := m.extractConfigs(snapshot.CurrentConfig)
	if err != nil {
		metrics.IncErrorCount(metrics.ComponentBaseFSMManager, m.managerName)
		return fmt.Errorf("failed to extract configs: %w", err), false
	}
	metrics.ObserveReconcileTime(metrics.ComponentBaseFSMManager, m.managerName+".extract_configs", time.Since(extractStart))

	// Step 2: Create or update instances
	for _, cfg := range desiredState {
		name, err := m.getName(cfg)
		if err != nil {
			metrics.IncErrorCount(metrics.ComponentBaseFSMManager, m.managerName)
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
				metrics.IncErrorCount(metrics.ComponentBaseFSMManager, m.managerName)
				return fmt.Errorf("failed to create instance: %w", err), false
			}
			metrics.ObserveReconcileTime(metrics.ComponentBaseFSMManager, m.managerName+".create_instance", time.Since(createStart))

			desiredState, err := m.getDesiredState(cfg)
			if err != nil {
				metrics.IncErrorCount(metrics.ComponentBaseFSMManager, m.managerName)
				return fmt.Errorf("failed to get desired state: %w", err), false
			}
			err = instance.SetDesiredFSMState(desiredState)
			if err != nil {
				metrics.IncErrorCount(metrics.ComponentBaseFSMManager, m.managerName)
				m.logger.Errorf("failed to set desired state: %v for instance %s", err, name)
				return fmt.Errorf("failed to set desired state: %w", err), false
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
			metrics.IncErrorCount(metrics.ComponentBaseFSMManager, m.managerName)
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
				metrics.IncErrorCount(metrics.ComponentBaseFSMManager, m.managerName)
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
			metrics.IncErrorCount(metrics.ComponentBaseFSMManager, m.managerName)
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
				metrics.IncErrorCount(metrics.ComponentBaseFSMManager, m.managerName)
				m.logger.Errorf("failed to set desired state: %w for instance %s", err, name)
				return fmt.Errorf("failed to set desired state: %w", err), false
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
				metrics.IncErrorCount(metrics.ComponentBaseFSMManager, m.managerName)
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

			// Temporary logging
			if instanceName == "golden-service" {
				sentry.ReportIssuef(sentry.IssueTypeError, m.logger, "m.instances: %#v, desiredState: %+v", m.instances, desiredState)
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

	// Reconcile instances
	for name, instance := range m.instances {
		reconcileStart := time.Now()

		// Check whether we have enough time left to reconcile the instance
		// This is another fallback to prevent high p99 spikes
		// If maybe a couple of previous instances were slow, we don't want to
		// have a ripple effect on the whole control loop
		expectedMaxP95ExecutionTime, err := m.getExpectedMaxP95ExecutionTimePerInstance(instance)
		if err != nil {
			metrics.IncErrorCount(metrics.ComponentBaseFSMManager, m.managerName)
			return fmt.Errorf("failed to get expected max p95 execution time: %w", err), false
		}
		remaining, sufficient, err := ctxutil.HasSufficientTime(ctx, expectedMaxP95ExecutionTime)
		if err != nil {
			if errors.Is(err, ctxutil.ErrNoDeadline) {
				return fmt.Errorf("no deadline set in context"), false
			}
			// For any other error, log and abort reconciliation
			metrics.IncErrorCount(metrics.ComponentBaseFSMManager, m.managerName)
			return fmt.Errorf("deadline check error: %w", err), false
		}

		if !sufficient {
			m.logger.Warnf("not enough time left to reconcile instance %s (only %v remaining, needed %v), skipping",
				name, remaining, expectedMaxP95ExecutionTime)
			return nil, true // return true to indicate that we should not run another manager and instead should wait for the next tick
		}

		instanceCtx, instanceCancel := context.WithTimeout(ctx, expectedMaxP95ExecutionTime)
		defer instanceCancel()

		// Pass manager-specific tick to instance.Reconcile
		// Update the snapshot tick to the manager tick
		snapshot.Tick = m.managerTick
		err, reconciled := instance.Reconcile(instanceCtx, snapshot, filesystemService)
		reconcileTime := time.Since(reconcileStart)
		metrics.ObserveReconcileTime(metrics.ComponentBaseFSMManager, m.managerName+".instances."+name, reconcileTime)

		if err != nil {
			metrics.IncErrorCount(metrics.ComponentBaseFSMManager, m.managerName+".instances."+name)

			// If the error is a permanent failure, remove the instance from the manager
			// so that it can be recreated in further ticks
			if backoff.IsPermanentFailureError(err) {
				sentry.ReportFSMErrorf(
					m.logger,
					name,
					m.managerName,
					"reconcile_permanent_failure",
					"Permanent failure reconciling instance: %v",
					err,
				)

				delete(m.instances, name)
				return nil, true
			}
			sentry.ReportFSMErrorf(
				m.logger,
				name,
				m.managerName,
				"reconcile_error",
				"Error reconciling instance: %v",
				err,
			)
			return fmt.Errorf("error reconciling instance: %w", err), false
		}

		if reconciled {
			return nil, true
		}
	}

	// Return nil if no errors occurred
	return nil, false
}

// GetLastObservedStates returns the last known states of all instances
func (m *BaseFSMManager[C]) GetLastObservedStates() map[string]ObservedState {
	states := make(map[string]ObservedState)
	for name, instance := range m.instances {
		states[name] = instance.GetLastObservedState()
	}
	return states
}

// GetLastObservedState returns the last known state of a specific instance
func (m *BaseFSMManager[C]) GetLastObservedState(serviceName string) (ObservedState, error) {
	if instance, exists := m.instances[serviceName]; exists {
		return instance.GetLastObservedState(), nil
	}
	return nil, fmt.Errorf("instance %s not found", serviceName)
}

// GetCurrentFSMState returns the current state of a specific instance
func (m *BaseFSMManager[C]) GetCurrentFSMState(serviceName string) (string, error) {
	if instance, exists := m.instances[serviceName]; exists {
		return instance.GetCurrentFSMState(), nil
	}
	return "", fmt.Errorf("instance %s not found", serviceName)
}

// CreateSnapshot creates a ManagerSnapshot from the current manager state
func (m *BaseFSMManager[C]) CreateSnapshot() ManagerSnapshot {
	snapshot := &BaseManagerSnapshot{
		Name:           m.managerName,
		Instances:      make(map[string]FSMInstanceSnapshot),
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

		snapshot.Instances[name] = instanceSnapshot
	}

	return snapshot
}

// ObservedStateConverter is an interface for objects that can convert their observed state to a snapshot
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
