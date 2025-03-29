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
	"time"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	redpandaserviceconfig "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/redpandaserviceconfig"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	logger "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	redpanda_service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda"
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

// initiateRedpandaCreate attempts to add the Redpanda to the S6 manager.
func (b *RedpandaInstance) initiateRedpandaCreate(ctx context.Context) error {
	b.baseFSMInstance.GetLogger().Debugf("Starting Action: Adding Redpanda service %s to S6 manager ...", b.baseFSMInstance.GetID())

	err := b.service.AddRedpandaToS6Manager(ctx, &b.config)
	if err != nil {
		if err == redpanda_service.ErrServiceAlreadyExists {
			b.baseFSMInstance.GetLogger().Debugf("Redpanda service %s already exists in S6 manager", b.baseFSMInstance.GetID())
			return nil // do not throw an error, as each action is expected to be idempotent
		}
		return fmt.Errorf("failed to add Redpanda service %s to S6 manager: %w", b.baseFSMInstance.GetID(), err)
	}

	b.baseFSMInstance.GetLogger().Debugf("Redpanda service %s added to S6 manager", b.baseFSMInstance.GetID())
	return nil
}

// initiateRedpandaRemove attempts to remove the Redpanda from the S6 manager.
// It requires the service to be stopped before removal.
func (b *RedpandaInstance) initiateRedpandaRemove(ctx context.Context) error {
	b.baseFSMInstance.GetLogger().Debugf("Starting Action: Removing Redpanda service %s from S6 manager ...", b.baseFSMInstance.GetID())

	// Remove the Redpanda from the S6 manager
	err := b.service.RemoveRedpandaFromS6Manager(ctx)
	if err != nil {
		if err == redpanda_service.ErrServiceNotExist {
			b.baseFSMInstance.GetLogger().Debugf("Redpanda service %s not found in S6 manager", b.baseFSMInstance.GetID())
			return nil // do not throw an error, as each action is expected to be idempotent
		}
		return fmt.Errorf("failed to remove Redpanda service %s from S6 manager: %w", b.baseFSMInstance.GetID(), err)
	}

	b.baseFSMInstance.GetLogger().Debugf("Redpanda service %s removed from S6 manager", b.baseFSMInstance.GetID())
	return nil
}

// initiateRedpandaStart attempts to start the redpanda by setting the desired state to running for the given instance
func (b *RedpandaInstance) initiateRedpandaStart(ctx context.Context) error {
	b.baseFSMInstance.GetLogger().Debugf("Starting Action: Starting Redpanda service %s ...", b.baseFSMInstance.GetID())

	// TODO: Add pre-start validation

	// Set the desired state to running for the given instance
	err := b.service.StartRedpanda(ctx)
	if err != nil {
		// if the service is not there yet but we attempt to start it, we need to throw an error
		return fmt.Errorf("failed to start Redpanda service %s: %w", b.baseFSMInstance.GetID(), err)
	}

	b.baseFSMInstance.GetLogger().Debugf("Redpanda service %s start command executed", b.baseFSMInstance.GetID())
	return nil
}

// initiateRedpandaStop attempts to stop the Redpanda by setting the desired state to stopped for the given instance
func (b *RedpandaInstance) initiateRedpandaStop(ctx context.Context) error {
	b.baseFSMInstance.GetLogger().Debugf("Starting Action: Stopping Redpanda service %s ...", b.baseFSMInstance.GetID())

	// Set the desired state to stopped for the given instance
	err := b.service.StopRedpanda(ctx)
	if err != nil {
		// if the service is not there yet but we attempt to stop it, we need to throw an error
		return fmt.Errorf("failed to stop Redpanda service %s: %w", b.baseFSMInstance.GetID(), err)
	}

	b.baseFSMInstance.GetLogger().Debugf("Redpanda service %s stop command executed", b.baseFSMInstance.GetID())
	return nil
}

// getServiceStatus gets the status of the Redpanda service
// its main purpose is to habdle the edge cases where the service is not yet created or not yet running
func (b *RedpandaInstance) getServiceStatus(ctx context.Context, tick uint64) (redpanda_service.ServiceInfo, error) {
	info, err := b.service.Status(ctx, tick)
	if err != nil {
		// If there's an error getting the service status, we need to distinguish between cases

		if errors.Is(err, redpanda_service.ErrServiceNotExist) {
			// If the service is being created, we don't want to count this as an error
			// The instance is likely in Creating or ToBeCreated state, so service doesn't exist yet
			// This will be handled in the reconcileStateTransition where the service gets created
			if b.baseFSMInstance.GetCurrentFSMState() == internalfsm.LifecycleStateCreating ||
				b.baseFSMInstance.GetCurrentFSMState() == internalfsm.LifecycleStateToBeCreated {
				return redpanda_service.ServiceInfo{}, redpanda_service.ErrServiceNotExist
			}

			// Log the warning but don't treat it as a fatal error
			b.baseFSMInstance.GetLogger().Debugf("Service not found, will be created during reconciliation")
			return redpanda_service.ServiceInfo{}, nil
		} else if errors.Is(err, redpanda_service.ErrHealthCheckConnectionRefused) {
			// Instead of conditional state checking, always return a ServiceInfo with failed health checks
			// This allows the FSM to continue reconciliation and make proper state transition decisions
			if b.baseFSMInstance.GetCurrentFSMState() != OperationalStateStopped { // no need to spam the logs if the service is already stopped
				b.baseFSMInstance.GetLogger().Debugf("Health check refused connection for service %s, returning ServiceInfo with failed health checks", b.baseFSMInstance.GetID())
			}
			infoWithFailedHealthChecks := info
			infoWithFailedHealthChecks.RedpandaStatus.HealthCheck.IsLive = false
			infoWithFailedHealthChecks.RedpandaStatus.HealthCheck.IsReady = false
			// Return ServiceInfo with health checks failed but preserve S6FSMState if available
			return infoWithFailedHealthChecks, nil
		}

		// For other errors, log them and return
		b.baseFSMInstance.GetLogger().Errorf("error updating observed state for %s: %s", b.baseFSMInstance.GetID(), err)
		return redpanda_service.ServiceInfo{}, err
	}

	return info, nil
}

// updateObservedState updates the observed state of the service
func (b *RedpandaInstance) updateObservedState(ctx context.Context, tick uint64) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	start := time.Now()
	info, err := b.getServiceStatus(ctx, tick)
	if err != nil {
		return err
	}
	metrics.ObserveReconcileTime(logger.ComponentRedpandaInstance, b.baseFSMInstance.GetID()+".getServiceStatus", time.Since(start))
	// Store the raw service info
	b.ObservedState.ServiceInfo = info

	// Fetch the actual Redpanda config from the service
	start = time.Now()
	observedConfig, err := b.service.GetConfig(ctx)
	metrics.ObserveReconcileTime(logger.ComponentRedpandaInstance, b.baseFSMInstance.GetID()+".getConfig", time.Since(start))
	if err == nil {
		// Only update if we successfully got the config
		b.ObservedState.ObservedRedpandaServiceConfig = observedConfig
	} else {
		if strings.Contains(err.Error(), redpanda_service.ErrServiceNotExist.Error()) {
			// Log the error but don't fail - this might happen during creation when the config file doesn't exist yet
			b.baseFSMInstance.GetLogger().Debugf("Service not found, will be created during reconciliation: %v", err)
			return nil
		} else {
			return fmt.Errorf("failed to get observed Redpanda config: %w", err)
		}
	}

	// Detect a config change - but let the S6 manager handle the actual reconciliation
	// Use new ConfigsEqual function that handles Redpanda defaults properly
	if !redpandaserviceconfig.ConfigsEqual(b.config, b.ObservedState.ObservedRedpandaServiceConfig) {
		// Check if the service exists before attempting to update
		if b.service.ServiceExists(ctx) {
			b.baseFSMInstance.GetLogger().Debugf("Observed Redpanda config is different from desired config, updating S6 configuration")

			// Use the new ConfigDiff function for better debug output
			diffStr := redpandaserviceconfig.ConfigDiff(b.config, b.ObservedState.ObservedRedpandaServiceConfig)
			b.baseFSMInstance.GetLogger().Debugf("Configuration differences: %s", diffStr)

			// Update the config in the S6 manager
			err := b.service.UpdateRedpandaInS6Manager(ctx, &b.config)
			if err != nil {
				return fmt.Errorf("failed to update Redpanda service configuration: %w", err)
			}
		} else {
			b.baseFSMInstance.GetLogger().Debugf("Config differences detected but service does not exist yet, skipping update")
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
func (b *RedpandaInstance) IsRedpandaS6Running() bool {
	return b.ObservedState.ServiceInfo.S6FSMState == s6fsm.OperationalStateRunning
}

// IsRedpandaS6Stopped determines if the Redpanda S6 FSM is in stopped state.
// We follow the same architectural principle as IsRedpandaS6Running - relying solely
// on the FSM state to maintain clean separation of concerns.
//
// Note: This function requires the S6FSMState to be updated in the ObservedState.
func (b *RedpandaInstance) IsRedpandaS6Stopped() bool {
	return b.ObservedState.ServiceInfo.S6FSMState == s6fsm.OperationalStateStopped
}

// IsRedpandaConfigLoaded determines if the Redpanda service has successfully loaded its configuration.
// Implementation: We check if the service has been running for at least 5 seconds without crashing.
// This works because Redpanda performs config validation at startup and immediately panics
// if there are any configuration errors, causing the service to restart.
// Therefore, if the service stays up for >= 5 seconds, we can be confident the config is valid.
func (b *RedpandaInstance) IsRedpandaConfigLoaded() bool {
	currentUptime := b.ObservedState.ServiceInfo.S6ObservedState.ServiceInfo.Uptime
	return currentUptime >= 5
}

// IsRedpandaHealthchecksPassed determines if the Redpanda service has passed its healthchecks.
func (b *RedpandaInstance) IsRedpandaHealthchecksPassed() bool {
	return b.ObservedState.ServiceInfo.RedpandaStatus.HealthCheck.IsLive &&
		b.ObservedState.ServiceInfo.RedpandaStatus.HealthCheck.IsReady
}

// AnyRestartsSinceCreation determines if the Redpanda service has restarted since its creation.
func (b *RedpandaInstance) AnyRestartsSinceCreation() bool {
	// We can analyse the S6 ExitHistory to determine if the service has restarted in the last seconds
	// We need to check if any of the exit codes are 0 (which means a restart)
	// and if the time of the restart is within the last seconds
	if len(b.ObservedState.ServiceInfo.S6ObservedState.ServiceInfo.ExitHistory) == 0 {
		return false
	}

	return true
}

// IsRedpandaRunningForSomeTimeWithoutErrors determines if the Redpanda service has been running for some time.
func (b *RedpandaInstance) IsRedpandaRunningForSomeTimeWithoutErrors(currentTime time.Time, logWindow time.Duration) bool {
	currentUptime := b.ObservedState.ServiceInfo.S6ObservedState.ServiceInfo.Uptime
	if currentUptime < 10 {
		return false
	}

	// Check if there are any issues in the Redpanda logs
	if !b.IsRedpandaLogsFine(currentTime, logWindow) {
		return false
	}

	// Check if there are any errors in the Redpanda metrics
	if !b.IsRedpandaMetricsErrorFree() {
		return false
	}

	return true
}

// IsRedpandaLogsFine determines if there are any issues in the Redpanda logs
func (b *RedpandaInstance) IsRedpandaLogsFine(currentTime time.Time, logWindow time.Duration) bool {
	return b.service.IsLogsFine(b.ObservedState.ServiceInfo.RedpandaStatus.Logs, currentTime, logWindow)
}

// IsRedpandaMetricsErrorFree determines if the Redpanda service has no errors in the metrics
func (b *RedpandaInstance) IsRedpandaMetricsErrorFree() bool {
	return b.service.IsMetricsErrorFree(b.ObservedState.ServiceInfo.RedpandaStatus.Metrics)
}

// IsRedpandaDegraded determines if the Redpanda service is degraded.
// These check everything that is checked during the starting phase
// But it means that it once worked, and then degraded
func (b *RedpandaInstance) IsRedpandaDegraded(currentTime time.Time, logWindow time.Duration) bool {
	if b.IsRedpandaS6Running() && b.IsRedpandaConfigLoaded() && b.IsRedpandaHealthchecksPassed() && b.IsRedpandaRunningForSomeTimeWithoutErrors(currentTime, logWindow) {
		return false
	}
	return true
}

// IsRedpandaWithProcessingActivity determines if the Redpanda instance has active data processing
// based on metrics data and possibly other observed state information
func (b *RedpandaInstance) IsRedpandaWithProcessingActivity() bool {
	if b.ObservedState.ServiceInfo.RedpandaStatus.MetricsState == nil {
		return false
	}
	return b.service.HasProcessingActivity(b.ObservedState.ServiceInfo.RedpandaStatus)
}
