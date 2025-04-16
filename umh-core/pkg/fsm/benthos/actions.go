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

package benthos

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	benthosserviceconfig "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/benthosserviceconfig"
	s6fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/s6"
	logger "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	benthos_service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// The functions in this file define heavier, possibly fail-prone operations
// (for example, network or file I/O) that the Benthos FSM might need to perform.
// They are intended to be called from Reconcile.
//
// IMPORTANT:
//   - Each action is expected to be idempotent, since it may be retried
//     multiple times due to transient failures.
//   - Each action takes a context.Context and can return an error if the operation fails.
//   - If an error occurs, the Reconcile function must handle
//     setting error state and scheduling a retry/backoff.

// CreateInstance attempts to add the Benthos to the S6 manager.
func (b *BenthosInstance) CreateInstance(ctx context.Context, filesystemService filesystem.Service) error {
	b.baseFSMInstance.GetLogger().Debugf("Starting Action: Adding Benthos service %s to S6 manager ...", b.baseFSMInstance.GetID())

	err := b.service.AddBenthosToS6Manager(ctx, filesystemService, &b.config, b.baseFSMInstance.GetID())
	if err != nil {
		if err == benthos_service.ErrServiceAlreadyExists {
			b.baseFSMInstance.GetLogger().Debugf("Benthos service %s already exists in S6 manager", b.baseFSMInstance.GetID())
			return nil // do not throw an error, as each action is expected to be idempotent
		}
		return fmt.Errorf("failed to add Benthos service %s to S6 manager: %w", b.baseFSMInstance.GetID(), err)
	}

	b.baseFSMInstance.GetLogger().Debugf("Benthos service %s added to S6 manager", b.baseFSMInstance.GetID())
	return nil
}

// RemoveInstance attempts to remove the Benthos from the S6 manager.
// It requires the service to be stopped before removal.
func (b *BenthosInstance) RemoveInstance(ctx context.Context, filesystemService filesystem.Service) error {
	b.baseFSMInstance.GetLogger().Debugf("Starting Action: Removing Benthos service %s from S6 manager ...", b.baseFSMInstance.GetID())

	// Remove the Benthos from the S6 manager
	err := b.service.RemoveBenthosFromS6Manager(ctx, filesystemService, b.baseFSMInstance.GetID())
	if err != nil {
		if err == benthos_service.ErrServiceNotExist {
			b.baseFSMInstance.GetLogger().Debugf("Benthos service %s not found in S6 manager", b.baseFSMInstance.GetID())
			return nil // do not throw an error, as each action is expected to be idempotent
		}
		return fmt.Errorf("failed to remove Benthos service %s from S6 manager: %w", b.baseFSMInstance.GetID(), err)
	}

	b.baseFSMInstance.GetLogger().Debugf("Benthos service %s removed from S6 manager", b.baseFSMInstance.GetID())
	return nil
}

// StartInstance attempts to start the benthos by setting the desired state to running for the given instance
func (b *BenthosInstance) StartInstance(ctx context.Context, filesystemService filesystem.Service) error {
	b.baseFSMInstance.GetLogger().Debugf("Starting Action: Starting Benthos service %s ...", b.baseFSMInstance.GetID())

	// TODO: Add pre-start validation

	// Set the desired state to running for the given instance
	err := b.service.StartBenthos(ctx, filesystemService, b.baseFSMInstance.GetID())
	if err != nil {
		// if the service is not there yet but we attempt to start it, we need to throw an error
		return fmt.Errorf("failed to start Benthos service %s: %w", b.baseFSMInstance.GetID(), err)
	}

	b.baseFSMInstance.GetLogger().Debugf("Benthos service %s start command executed", b.baseFSMInstance.GetID())
	return nil
}

// StopInstance attempts to stop the Benthos by setting the desired state to stopped for the given instance
func (b *BenthosInstance) StopInstance(ctx context.Context, filesystemService filesystem.Service) error {
	b.baseFSMInstance.GetLogger().Debugf("Starting Action: Stopping Benthos service %s ...", b.baseFSMInstance.GetID())

	// Set the desired state to stopped for the given instance
	err := b.service.StopBenthos(ctx, filesystemService, b.baseFSMInstance.GetID())
	if err != nil {
		// if the service is not there yet but we attempt to stop it, we need to throw an error
		return fmt.Errorf("failed to stop Benthos service %s: %w", b.baseFSMInstance.GetID(), err)
	}

	b.baseFSMInstance.GetLogger().Debugf("Benthos service %s stop command executed", b.baseFSMInstance.GetID())
	return nil
}

// getServiceStatus gets the status of the Benthos service
// its main purpose is to handle the edge cases where the service is not yet created or not yet running
func (b *BenthosInstance) getServiceStatus(ctx context.Context, filesystemService filesystem.Service, tick uint64) (benthos_service.ServiceInfo, error) {
	info, err := b.service.Status(ctx, filesystemService, b.baseFSMInstance.GetID(), b.config.MetricsPort, tick)
	if err != nil {
		// If there's an error getting the service status, we need to distinguish between cases

		if errors.Is(err, benthos_service.ErrServiceNotExist) {
			// If the service is being created, we don't want to count this as an error
			// The instance is likely in Creating or ToBeCreated state, so service doesn't exist yet
			// This will be handled in the reconcileStateTransition where the service gets created
			if b.baseFSMInstance.GetCurrentFSMState() == internalfsm.LifecycleStateCreating ||
				b.baseFSMInstance.GetCurrentFSMState() == internalfsm.LifecycleStateToBeCreated {
				return benthos_service.ServiceInfo{}, benthos_service.ErrServiceNotExist
			}

			// Log the warning but don't treat it as a fatal error
			b.baseFSMInstance.GetLogger().Debugf("Service not found, will be created during reconciliation")
			return benthos_service.ServiceInfo{}, nil
		} else if errors.Is(err, benthos_service.ErrHealthCheckConnectionRefused) {
			// Instead of conditional state checking, always return a ServiceInfo with failed health checks
			// This allows the FSM to continue reconciliation and make proper state transition decisions
			if b.baseFSMInstance.GetCurrentFSMState() != OperationalStateStopped { // no need to spam the logs if the service is already stopped
				b.baseFSMInstance.GetLogger().Debugf("Health check refused connection for service %s, returning ServiceInfo with failed health checks", b.baseFSMInstance.GetID())
			}
			infoWithFailedHealthChecks := info
			infoWithFailedHealthChecks.BenthosStatus.HealthCheck.IsLive = false
			infoWithFailedHealthChecks.BenthosStatus.HealthCheck.IsReady = false
			// Return ServiceInfo with health checks failed but preserve S6FSMState if available
			return infoWithFailedHealthChecks, nil
		}

		// For other errors, log them and return
		b.baseFSMInstance.GetLogger().Errorf("error updating observed state for %s: %s", b.baseFSMInstance.GetID(), err)
		return benthos_service.ServiceInfo{}, err
	}

	return info, nil
}

// UpdateObservedStateOfInstance updates the observed state of the service
func (b *BenthosInstance) UpdateObservedStateOfInstance(ctx context.Context, filesystemService filesystem.Service, tick uint64, loopStartTime time.Time) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	currentState := b.baseFSMInstance.GetCurrentFSMState()
	desiredState := b.baseFSMInstance.GetDesiredFSMState()

	if currentState == OperationalStateStopped && desiredState == OperationalStateStopped {
		return nil
	}

	start := time.Now()
	info, err := b.getServiceStatus(ctx, filesystemService, tick)
	if err != nil {
		return err
	}
	metrics.ObserveReconcileTime(logger.ComponentBenthosInstance, b.baseFSMInstance.GetID()+".getServiceStatus", time.Since(start))
	// Store the raw service info
	b.ObservedState.ServiceInfo = info

	// Fetch the actual Benthos config from the service
	start = time.Now()
	observedConfig, err := b.service.GetConfig(ctx, filesystemService, b.baseFSMInstance.GetID())
	metrics.ObserveReconcileTime(logger.ComponentBenthosInstance, b.baseFSMInstance.GetID()+".getConfig", time.Since(start))
	if err == nil {
		// Only update if we successfully got the config
		b.ObservedState.ObservedBenthosServiceConfig = observedConfig
	} else {
		if strings.Contains(err.Error(), benthos_service.ErrServiceNotExist.Error()) {
			// Log the error but don't fail - this might happen during creation when the config file doesn't exist yet
			b.baseFSMInstance.GetLogger().Debugf("Service not found, will be created during reconciliation: %v", err)
			return nil
		} else {
			return fmt.Errorf("failed to get observed Benthos config: %w", err)
		}
	}

	// Detect a config change - but let the S6 manager handle the actual reconciliation
	// Use new ConfigsEqual function that handles Benthos defaults properly
	if !benthosserviceconfig.ConfigsEqual(b.config, b.ObservedState.ObservedBenthosServiceConfig) {
		// Check if the service exists before attempting to update
		if b.service.ServiceExists(ctx, filesystemService, b.baseFSMInstance.GetID()) {
			b.baseFSMInstance.GetLogger().Debugf("Observed Benthos config is different from desired config, updating S6 configuration")

			// Use the new ConfigDiff function for better debug output
			diffStr := benthosserviceconfig.ConfigDiff(b.config, b.ObservedState.ObservedBenthosServiceConfig)
			b.baseFSMInstance.GetLogger().Debugf("Configuration differences: %s", diffStr)

			// Update the config in the S6 manager
			err := b.service.UpdateBenthosInS6Manager(ctx, filesystemService, &b.config, b.baseFSMInstance.GetID())
			if err != nil {
				return fmt.Errorf("failed to update Benthos service configuration: %w", err)
			}
		} else {
			b.baseFSMInstance.GetLogger().Debugf("Config differences detected but service does not exist yet, skipping update")
		}
	}

	return nil
}

// IsBenthosS6Running determines if the Benthos S6 FSM is in running state.
// Architecture Decision: We intentionally rely only on the FSM state, not the underlying
// service implementation details. This maintains a clean separation of concerns where:
// 1. The FSM is the source of truth for service state
// 2. We trust the FSM's state management completely
// 3. Implementation details of how S6 determines running state are encapsulated away
//
// Note: This function requires the S6FSMState to be updated in the ObservedState.
func (b *BenthosInstance) IsBenthosS6Running() bool {
	return b.ObservedState.ServiceInfo.S6FSMState == s6fsm.OperationalStateRunning
}

// IsBenthosS6Stopped determines if the Benthos S6 FSM is in stopped state.
// We follow the same architectural principle as IsBenthosS6Running - relying solely
// on the FSM state to maintain clean separation of concerns.
//
// Note: This function requires the S6FSMState to be updated in the ObservedState.
func (b *BenthosInstance) IsBenthosS6Stopped() bool {
	return b.ObservedState.ServiceInfo.S6FSMState == s6fsm.OperationalStateStopped
}

// IsBenthosConfigLoaded determines if the Benthos service has successfully loaded its configuration.
// Implementation: We check if the service has been running for at least 5 seconds without crashing.
// This works because Benthos performs config validation at startup and immediately panics
// if there are any configuration errors, causing the service to restart.
// Therefore, if the service stays up for >= 5 seconds, we can be confident the config is valid.
func (b *BenthosInstance) IsBenthosConfigLoaded() bool {
	currentUptime := b.ObservedState.ServiceInfo.S6ObservedState.ServiceInfo.Uptime
	return currentUptime >= 5
}

// IsBenthosHealthchecksPassed determines if the Benthos service has passed its healthchecks.
func (b *BenthosInstance) IsBenthosHealthchecksPassed() bool {
	return b.ObservedState.ServiceInfo.BenthosStatus.HealthCheck.IsLive &&
		b.ObservedState.ServiceInfo.BenthosStatus.HealthCheck.IsReady
}

// AnyRestartsSinceCreation determines if the Benthos service has restarted since its creation.
func (b *BenthosInstance) AnyRestartsSinceCreation() bool {
	// We can analyse the S6 ExitHistory to determine if the service has restarted in the last seconds
	// We need to check if any of the exit codes are 0 (which means a restart)
	// and if the time of the restart is within the last seconds
	if len(b.ObservedState.ServiceInfo.S6ObservedState.ServiceInfo.ExitHistory) == 0 {
		return false
	}

	return true
}

// IsBenthosRunningForSomeTimeWithoutErrors determines if the Benthos service has been running for some time.
func (b *BenthosInstance) IsBenthosRunningForSomeTimeWithoutErrors(currentTime time.Time, logWindow time.Duration) bool {
	currentUptime := b.ObservedState.ServiceInfo.S6ObservedState.ServiceInfo.Uptime
	if currentUptime < 10 {
		return false
	}

	// Check if there are any issues in the Benthos logs
	if !b.IsBenthosLogsFine(currentTime, logWindow) {
		return false
	}

	// Check if there are any errors in the Benthos metrics
	if !b.IsBenthosMetricsErrorFree() {
		return false
	}

	return true
}

// IsBenthosLogsFine determines if there are any issues in the Benthos logs
func (b *BenthosInstance) IsBenthosLogsFine(currentTime time.Time, logWindow time.Duration) bool {
	return b.service.IsLogsFine(b.ObservedState.ServiceInfo.BenthosStatus.Logs, currentTime, logWindow)
}

// IsBenthosMetricsErrorFree determines if the Benthos service has no errors in the metrics
func (b *BenthosInstance) IsBenthosMetricsErrorFree() bool {
	return b.service.IsMetricsErrorFree(b.ObservedState.ServiceInfo.BenthosStatus.Metrics)
}

// IsBenthosDegraded determines if the Benthos service is degraded.
// These check everything that is checked during the starting phase
// But it means that it once worked, and then degraded
func (b *BenthosInstance) IsBenthosDegraded(currentTime time.Time, logWindow time.Duration) bool {
	if b.IsBenthosS6Running() && b.IsBenthosConfigLoaded() && b.IsBenthosHealthchecksPassed() && b.IsBenthosRunningForSomeTimeWithoutErrors(currentTime, logWindow) {
		return false
	}
	return true
}

// IsBenthosWithProcessingActivity determines if the Benthos instance has active data processing
// based on metrics data and possibly other observed state information
func (b *BenthosInstance) IsBenthosWithProcessingActivity() bool {
	if b.ObservedState.ServiceInfo.BenthosStatus.MetricsState == nil {
		return false
	}
	return b.service.HasProcessingActivity(b.ObservedState.ServiceInfo.BenthosStatus)
}
