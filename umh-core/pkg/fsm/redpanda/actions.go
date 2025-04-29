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

package redpanda

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	redpandaserviceconfig "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/redpandaserviceconfig"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	logger "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	redpanda_service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	"golang.org/x/sync/errgroup"
)

// The functions in this file define heavier, possibly fail-prone operations
// (for example, network or file I/O) that the Redpanda FSM might need to perform.
// They are intended to be called from Reconcile.
//
// IMPORTANT:
//   - Each action is expected to be idempotent, since it may be retried
//     multiple times due to transient failures.
//   - Each action takes a context.Context and can return an error if the operation fails.
//   - If an error occurs, the Reconcile function must handle
//     setting error state and scheduling a retry/backoff.

// CreateInstance attempts to add the Redpanda to the S6 manager.
func (r *RedpandaInstance) CreateInstance(ctx context.Context, filesystemService filesystem.Service) error {
	r.baseFSMInstance.GetLogger().Debugf("Starting Action: Adding Redpanda service %s to S6 manager ...", r.baseFSMInstance.GetID())

	err := r.service.AddRedpandaToS6Manager(ctx, &r.config, filesystemService)
	if err != nil {
		if err == redpanda_service.ErrServiceAlreadyExists {
			r.baseFSMInstance.GetLogger().Debugf("Redpanda service %s already exists in S6 manager", r.baseFSMInstance.GetID())
			return nil // do not throw an error, as each action is expected to be idempotent
		}
		return fmt.Errorf("failed to add Redpanda service %s to S6 manager: %w", r.baseFSMInstance.GetID(), err)
	}

	r.baseFSMInstance.GetLogger().Debugf("Redpanda service %s added to S6 manager", r.baseFSMInstance.GetID())
	return nil
}

// RemoveInstance attempts to remove the Redpanda from the S6 manager.
// It requires the service to be stopped before removal.
func (r *RedpandaInstance) RemoveInstance(ctx context.Context, filesystemService filesystem.Service) error {
	r.baseFSMInstance.GetLogger().Debugf("Starting Action: Removing Redpanda service %s from S6 manager ...", r.baseFSMInstance.GetID())

	// Remove the Redpanda from the S6 manager
	err := r.service.RemoveRedpandaFromS6Manager(ctx)
	if err != nil {
		if err == redpanda_service.ErrServiceNotExist {
			r.baseFSMInstance.GetLogger().Debugf("Redpanda service %s not found in S6 manager", r.baseFSMInstance.GetID())
			return nil // do not throw an error, as each action is expected to be idempotent
		}
		return fmt.Errorf("failed to remove Redpanda service %s from S6 manager: %w", r.baseFSMInstance.GetID(), err)
	}

	r.baseFSMInstance.GetLogger().Debugf("Redpanda service %s removed from S6 manager", r.baseFSMInstance.GetID())
	return nil
}

// StartInstance attempts to start the redpanda by setting the desired state to running for the given instance
func (r *RedpandaInstance) StartInstance(ctx context.Context, filesystemService filesystem.Service) error {
	r.baseFSMInstance.GetLogger().Debugf("Starting Action: Starting Redpanda service %s ...", r.baseFSMInstance.GetID())

	// TODO: Add pre-start validation

	// Set the desired state to running for the given instance
	err := r.service.StartRedpanda(ctx)
	if err != nil {
		// if the service is not there yet but we attempt to start it, we need to throw an error
		return fmt.Errorf("failed to start Redpanda service %s: %w", r.baseFSMInstance.GetID(), err)
	}

	r.baseFSMInstance.GetLogger().Debugf("Redpanda service %s start command executed", r.baseFSMInstance.GetID())
	return nil
}

// StopInstance attempts to stop the Redpanda by setting the desired state to stopped for the given instance
func (r *RedpandaInstance) StopInstance(ctx context.Context, filesystemService filesystem.Service) error {
	r.baseFSMInstance.GetLogger().Debugf("Starting Action: Stopping Redpanda service %s ...", r.baseFSMInstance.GetID())

	// Set the desired state to stopped for the given instance
	err := r.service.StopRedpanda(ctx)
	if err != nil {
		// if the service is not there yet but we attempt to stop it, we need to throw an error
		return fmt.Errorf("failed to stop Redpanda service %s: %w", r.baseFSMInstance.GetID(), err)
	}

	r.baseFSMInstance.GetLogger().Debugf("Redpanda service %s stop command executed", r.baseFSMInstance.GetID())
	return nil
}

// getServiceStatus gets the status of the Redpanda service
// its main purpose is to habdle the edge cases where the service is not yet created or not yet running
func (r *RedpandaInstance) GetServiceStatus(ctx context.Context, filesystemService filesystem.Service, tick uint64, loopStartTime time.Time) (redpanda_service.ServiceInfo, error) {

	info, err := r.service.Status(ctx, filesystemService, tick, loopStartTime)
	if err != nil {
		// If there's an error getting the service status, we need to distinguish between cases

		if errors.Is(err, redpanda_service.ErrServiceNotExist) {
			// If the service is being created, we don't want to count this as an error
			// The instance is likely in Creating or ToBeCreated state, so service doesn't exist yet
			// This will be handled in the reconcileStateTransition where the service gets created
			if r.baseFSMInstance.GetCurrentFSMState() == internalfsm.LifecycleStateCreating ||
				r.baseFSMInstance.GetCurrentFSMState() == internalfsm.LifecycleStateToBeCreated {
				return redpanda_service.ServiceInfo{}, redpanda_service.ErrServiceNotExist
			}

			// Log the warning but don't treat it as a fatal error
			r.baseFSMInstance.GetLogger().Debugf("Service not found, will be created during reconciliation")
			return redpanda_service.ServiceInfo{}, nil
		} else if errors.Is(err, redpanda_service.ErrServiceNoLogFile) {
			// This is only an error, if redpanda is already running, otherwise there are simply no logs
			// This includes degraded states, as he can only go from active/idle to degraded, and therefore there should be logs
			if IsRunningState(r.baseFSMInstance.GetCurrentFSMState()) {
				r.baseFSMInstance.GetLogger().Debugf("Health check had no logs to process for service %s, returning ServiceInfo with failed health checks", r.baseFSMInstance.GetID())
				infoWithFailedHealthChecks := info
				infoWithFailedHealthChecks.RedpandaStatus.HealthCheck.IsLive = false
				infoWithFailedHealthChecks.RedpandaStatus.HealthCheck.IsReady = false
				// Return ServiceInfo with health checks failed but preserve S6FSMState if available
				return infoWithFailedHealthChecks, nil
			} else {
				// We have no running service, therefore we can simply return nil as error
				return info, nil
			}
		} else if errors.Is(err, redpanda_monitor.ErrServiceConnectionRefused) {
			// If we are in the starting phase or stopped, ..., we can ignore this error, as it is expected
			if !IsRunningState(r.baseFSMInstance.GetCurrentFSMState()) {
				infoWithFailedHealthChecks := info
				infoWithFailedHealthChecks.RedpandaStatus.HealthCheck.IsLive = false
				infoWithFailedHealthChecks.RedpandaStatus.HealthCheck.IsReady = false
				return infoWithFailedHealthChecks, nil
			}
		} else if errors.Is(err, redpanda_service.ErrRedpandaMonitorNotRunning) {
			// If the metrics service is not running, we are unable to get the logs/metrics, therefore we must return an empty status
			if !IsRunningState(r.baseFSMInstance.GetCurrentFSMState()) {
				return info, nil
			}
		}

		// For other errors, log them and return
		r.baseFSMInstance.GetLogger().Errorf("error updating observed state for %s: %s (current state: %s, desired state: %s)", r.baseFSMInstance.GetID(), err, r.baseFSMInstance.GetCurrentFSMState(), r.baseFSMInstance.GetDesiredFSMState())
		return redpanda_service.ServiceInfo{}, err
	}

	return info, nil
}

// UpdateObservedStateOfInstance updates the observed state of the service
func (r *RedpandaInstance) UpdateObservedStateOfInstance(ctx context.Context, services serviceregistry.Provider, tick uint64, loopStartTime time.Time) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	currentState := r.baseFSMInstance.GetCurrentFSMState()
	desiredState := r.baseFSMInstance.GetDesiredFSMState()

	// Skip health checks if the desired state or current state indicates stopped/stopping
	if desiredState == OperationalStateStopped || currentState == OperationalStateStopped || currentState == OperationalStateStopping {
		// For stopped instances, just check if the S6 service exists but don't do health checks
		// This minimal information is sufficient for reconciliation
		exists := r.service.ServiceExists(ctx, services.GetFileSystem())
		if !exists {
			// If the service doesn't exist, nothing more to do
			r.ObservedState = RedpandaObservedState{}
			return nil
		}
	}

	// If both desired and current state are stopped, we can return immediately
	// There wont be any logs, metrics, etc. to check
	if desiredState == OperationalStateStopped && currentState == OperationalStateStopped {
		return nil
	}

	// Start an errgroup with the **same context** so if one sub-task
	// fails or the context is canceled, all sub-tasks are signaled to stop.
	g, gctx := errgroup.WithContext(ctx)
	observedStateMu := sync.Mutex{}

	g.Go(func() error {
		start := time.Now()
		info, err := r.GetServiceStatus(gctx, services.GetFileSystem(), tick, loopStartTime)
		metrics.ObserveReconcileTime(logger.ComponentRedpandaInstance, r.baseFSMInstance.GetID()+".getServiceStatus", time.Since(start))
		if err == nil {
			// Store the raw service info
			observedStateMu.Lock()
			r.ObservedState.ServiceInfo = info
			observedStateMu.Unlock()
		} else if strings.Contains(err.Error(), redpanda_service.ErrServiceNotExist.Error()) || strings.Contains(err.Error(), redpanda_service.ErrServiceNoLogFile.Error()) || strings.Contains(err.Error(), redpanda_service.ErrRedpandaMonitorInstanceNotFound.Error()) {
			return nil
		}

		return err
	})

	g.Go(func() error {
		start := time.Now()
		// This GetConfig requires the tick parameter, which will be used to calculate the metrics state
		observedConfig, err := r.service.GetConfig(gctx, services.GetFileSystem(), tick, loopStartTime)
		metrics.ObserveReconcileTime(logger.ComponentRedpandaInstance, r.baseFSMInstance.GetID()+".getConfig", time.Since(start))

		if err == nil {
			// Only update if we successfully got the config
			observedStateMu.Lock()
			r.ObservedState.ObservedRedpandaServiceConfig = observedConfig
			observedStateMu.Unlock()
			return nil
		} else {
			if strings.Contains(err.Error(), redpanda_service.ErrServiceNotExist.Error()) || strings.Contains(err.Error(), redpanda_service.ErrServiceNoLogFile.Error()) || strings.Contains(err.Error(), redpanda_service.ErrRedpandaMonitorInstanceNotFound.Error()) {
				// Log the error but don't fail - this might happen during creation when the config file doesn't exist yet
				// Note: as we use the logs of the underlying redpanda_monitor service, we need to ignore ErrServiceNoLogFile here.
				r.baseFSMInstance.GetLogger().Debugf("Service not found, will be created during reconciliation: %v", err)
				return nil
			}
			if strings.Contains(err.Error(), redpanda_monitor.ErrServiceConnectionRefused.Error()) {
				// This is expected during the startup phase of the redpanda service, when the service is not yet ready to receive connections
				r.baseFSMInstance.GetLogger().Debugf("Service not yet ready: %v", err)
				return nil
			}

			return fmt.Errorf("failed to get observed Redpanda config: %w", err)
		}
	})

	// Create a buffered channel to receive the result from g.Wait().
	// The channel is buffered so that the goroutine sending on it doesn't block.
	errc := make(chan error, 1)

	// Run g.Wait() in a separate goroutine.
	// This allows us to use a select statement to return early if the context is canceled.
	go func() {
		// g.Wait() blocks until all goroutines launched with g.Go() have returned.
		// It returns the first non-nil error, if any.
		errc <- g.Wait()
	}()

	// Use a select statement to wait for either the g.Wait() result or the context's cancellation.
	select {
	case err := <-errc:
		// g.Wait() has finished, so check if any goroutine returned an error.
		if err != nil {
			// If there was an error in any sub-call, return that error.
			return err
		}
		// If err is nil, all goroutines completed successfully.
	case <-ctx.Done():
		// The context was canceled or its deadline was exceeded before all goroutines finished.
		// Although some goroutines might still be running in the background,
		// they use a context (gctx) that should cause them to terminate promptly.
		return ctx.Err()
	}

	// Detect a config change - but let the S6 manager handle the actual reconciliation
	// Use new ConfigsEqual function that handles Redpanda defaults properly
	if !redpandaserviceconfig.ConfigsEqual(r.config, r.ObservedState.ObservedRedpandaServiceConfig) {
		// Check if the service exists before attempting to update
		if r.service.ServiceExists(ctx, services.GetFileSystem()) {
			r.baseFSMInstance.GetLogger().Debugf("Observed Redpanda config is different from desired config, updating S6 configuration")

			// Use the new ConfigDiff function for better debug output
			diffStr := redpandaserviceconfig.ConfigDiff(r.config, r.ObservedState.ObservedRedpandaServiceConfig)
			r.baseFSMInstance.GetLogger().Debugf("Configuration differences: %s", diffStr)

			// Update the config in the S6 manager
			err := r.service.UpdateRedpandaInS6Manager(ctx, &r.config)
			if err != nil {
				return fmt.Errorf("failed to update Redpanda service configuration: %w", err)
			}
		} else {
			r.baseFSMInstance.GetLogger().Debugf("Config differences detected but service does not exist yet, skipping update")
		}
	}

	return nil
}

// IsRedpandaS6Running determines if the Redpanda S6 FSM is in running state.
// Architecture Decision: We intentionally rely only on the FSM state, not the underlying
// service implementation details. This maintains a clean separation of concerns where:
// 1. The FSM is the source of truth for service state
// 2. We trust the FSM's state management completely
// 3. Implementation details of how S6 determines running state are encapsulated away
//
// Note: This function requires the S6FSMState to be updated in the ObservedState.
func (r *RedpandaInstance) IsRedpandaS6Running() bool {
	return r.ObservedState.ServiceInfo.S6FSMState == s6fsm.OperationalStateRunning
}

// IsRedpandaS6Stopped determines if the Redpanda S6 FSM is in stopped state.
// We follow the same architectural principle as IsRedpandaS6Running - relying solely
// on the FSM state to maintain clean separation of concerns.
//
// Note: This function requires the S6FSMState to be updated in the ObservedState.
func (r *RedpandaInstance) IsRedpandaS6Stopped() bool {
	return r.ObservedState.ServiceInfo.S6FSMState == s6fsm.OperationalStateStopped
}

// IsRedpandaConfigLoaded determines if the Redpanda service has successfully loaded its configuration.
// Implementation: We check if the service has been running for at least 5 seconds without crashing.
// This works because Redpanda performs config validation at startup and immediately panics
// if there are any configuration errors, causing the service to restart.
// Therefore, if the service stays up for >= 5 seconds, we can be confident the config is valid.
func (r *RedpandaInstance) IsRedpandaConfigLoaded() bool {
	currentUptime := r.ObservedState.ServiceInfo.S6ObservedState.ServiceInfo.Uptime
	return currentUptime >= 5
}

// IsRedpandaHealthchecksPassed determines if the Redpanda service has passed its healthchecks.
func (r *RedpandaInstance) IsRedpandaHealthchecksPassed() bool {
	return r.ObservedState.ServiceInfo.RedpandaStatus.HealthCheck.IsLive &&
		r.ObservedState.ServiceInfo.RedpandaStatus.HealthCheck.IsReady
}

// AnyRestartsSinceCreation determines if the Redpanda service has restarted since its creation.
func (r *RedpandaInstance) AnyRestartsSinceCreation() bool {
	// We can analyse the S6 ExitHistory to determine if the service has restarted in the last seconds
	// We need to check if any of the exit codes are 0 (which means a restart)
	// and if the time of the restart is within the last seconds
	if len(r.ObservedState.ServiceInfo.S6ObservedState.ServiceInfo.ExitHistory) == 0 {
		return false
	}

	return true
}

// IsRedpandaRunningForSomeTimeWithoutErrors determines if the Redpanda service has been running for some time.
func (r *RedpandaInstance) IsRedpandaRunningForSomeTimeWithoutErrors(currentTime time.Time, logWindow time.Duration) bool {
	currentUptime := r.ObservedState.ServiceInfo.S6ObservedState.ServiceInfo.Uptime
	if currentUptime < 10 {
		return false
	}

	// Check if there are any issues in the Redpanda logs
	if !r.IsRedpandaLogsFine(currentTime, logWindow) {
		return false
	}

	// Check if there are any errors in the Redpanda metrics
	if !r.IsRedpandaMetricsErrorFree() {
		return false
	}

	return true
}

// IsRedpandaLogsFine determines if there are any issues in the Redpanda logs
func (r *RedpandaInstance) IsRedpandaLogsFine(currentTime time.Time, logWindow time.Duration) bool {
	return r.service.IsLogsFine(r.ObservedState.ServiceInfo.RedpandaStatus.Logs, currentTime, logWindow)
}

// IsRedpandaMetricsErrorFree determines if the Redpanda service has no errors in the metrics
func (r *RedpandaInstance) IsRedpandaMetricsErrorFree() bool {
	return r.service.IsMetricsErrorFree(r.ObservedState.ServiceInfo.RedpandaStatus.RedpandaMetrics.Metrics)
}

// IsRedpandaDegraded determines if the Redpanda service is degraded.
// These check everything that is checked during the starting phase
// But it means that it once worked, and then degraded
func (r *RedpandaInstance) IsRedpandaDegraded(currentTime time.Time, logWindow time.Duration) bool {
	if r.IsRedpandaS6Running() && r.IsRedpandaConfigLoaded() && r.IsRedpandaHealthchecksPassed() && r.IsRedpandaRunningForSomeTimeWithoutErrors(currentTime, logWindow) {
		return false
	}
	return true
}

// IsRedpandaWithProcessingActivity determines if the Redpanda instance has active data processing
// based on metrics data and possibly other observed state information
func (r *RedpandaInstance) IsRedpandaWithProcessingActivity() bool {
	if r.ObservedState.ServiceInfo.RedpandaStatus.RedpandaMetrics.MetricsState == nil {
		return false
	}
	return r.service.HasProcessingActivity(r.ObservedState.ServiceInfo.RedpandaStatus)
}

// IsRedpandaStarted checks if "Successfully started Redpanda!" is found in logs
// indicating that Redpanda has successfully started.
func (r *RedpandaInstance) IsRedpandaStarted() bool {
	if r.ObservedState.ServiceInfo.RedpandaStatus.Logs == nil {
		return false
	}

	// Check if the success message is in the logs
	for _, log := range r.ObservedState.ServiceInfo.RedpandaStatus.Logs {
		if strings.Contains(log.Content, "Successfully started Redpanda!") {
			return true
		}
	}
	return false
}

func (r *RedpandaInstance) GetBaseFSMInstanceForTest() *internalfsm.BaseFSMInstance {
	return r.baseFSMInstance
}
