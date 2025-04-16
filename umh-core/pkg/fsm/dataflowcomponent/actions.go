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

package dataflowcomponent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	benthosfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	dataflowcomponentservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// The functions in this file define heavier, possibly fail-prone operations
// (for example, network or file I/O) that the DataflowComponent's Benthos manager might need to perform.
// They are intended to be called from Reconcile.
//
// IMPORTANT:
//   - Each action is expected to be idempotent, since it may be retried
//     multiple times due to transient failures.
//   - Each action takes a context.Context and can return an error if the operation fails.
//   - If an error occurs, the Reconcile function must handle
//     setting error state and scheduling a retry/backoff.

// CreateInstance attempts to add the DataflowComponent to the Benthos manager.
func (d *DataflowComponentInstance) CreateInstance(ctx context.Context, filesystemService filesystem.Service) error {
	d.baseFSMInstance.GetLogger().Debugf("Starting Action: Adding DataflowComponent service %s to Benthos manager ...", d.baseFSMInstance.GetID())

	err := d.service.AddDataFlowComponentToBenthosManager(ctx, filesystemService, &d.config, d.baseFSMInstance.GetID())
	if err != nil {
		if err == dataflowcomponentservice.ErrServiceAlreadyExists {
			d.baseFSMInstance.GetLogger().Debugf("DataflowComponent service %s already exists in Benthos manager", d.baseFSMInstance.GetID())
			return nil // do not throw an error, as each action is expected to be idempotent
		}
		return fmt.Errorf("failed to add DataflowComponent service %s to Benthos manager: %w", d.baseFSMInstance.GetID(), err)
	}

	d.baseFSMInstance.GetLogger().Debugf("DataflowComponent service %s added to Benthos manager", d.baseFSMInstance.GetID())
	return nil
}

// RemoveInstance attempts to remove the DataflowComponent from the Benthos manager.
// It requires the service to be stopped before removal.
func (b *DataflowComponentInstance) RemoveInstance(ctx context.Context, filesystemService filesystem.Service) error {
	b.baseFSMInstance.GetLogger().Debugf("Starting Action: Removing DataflowComponent service %s from Benthos manager ...", b.baseFSMInstance.GetID())

	// Remove the initiateDataflowComponent from the Benthos manager
	err := b.service.RemoveDataFlowComponentFromBenthosManager(ctx, filesystemService, b.baseFSMInstance.GetID())
	if err != nil {
		if err == dataflowcomponentservice.ErrServiceNotExist {
			b.baseFSMInstance.GetLogger().Debugf("DataflowComponent service %s not found in Benthos manager", b.baseFSMInstance.GetID())
			return nil // do not throw an error, as each action is expected to be idempotent
		}
		return fmt.Errorf("failed to remove DataflowComponent service %s from Benthos manager: %w", b.baseFSMInstance.GetID(), err)
	}

	b.baseFSMInstance.GetLogger().Debugf("DataflowComponent service %s removed from Benthos manager", b.baseFSMInstance.GetID())
	return nil
}

// StartInstance to start the DataflowComponent by setting the desired state to running for the given instance
func (d *DataflowComponentInstance) StartInstance(ctx context.Context, filesystemService filesystem.Service) error {
	d.baseFSMInstance.GetLogger().Debugf("Starting Action: Starting DataflowComponent service %s ...", d.baseFSMInstance.GetID())

	// TODO: Add pre-start validation

	// Set the desired state to running for the given instance
	err := d.service.StartDataFlowComponent(ctx, filesystemService, d.baseFSMInstance.GetID())
	if err != nil {
		// if the service is not there yet but we attempt to start it, we need to throw an error
		return fmt.Errorf("failed to start DataflowComponent service %s: %w", d.baseFSMInstance.GetID(), err)
	}

	d.baseFSMInstance.GetLogger().Debugf("DataflowComponent service %s start command executed", d.baseFSMInstance.GetID())
	return nil
}

// StopInstance attempts to stop the DataflowComponent by setting the desired state to stopped for the given instance
func (d *DataflowComponentInstance) StopInstance(ctx context.Context, filesystemService filesystem.Service) error {
	d.baseFSMInstance.GetLogger().Debugf("Starting Action: Stopping DataflowComponent service %s ...", d.baseFSMInstance.GetID())

	// Set the desired state to stopped for the given instance
	err := d.service.StopDataFlowComponent(ctx, filesystemService, d.baseFSMInstance.GetID())
	if err != nil {
		// if the service is not there yet but we attempt to stop it, we need to throw an error
		return fmt.Errorf("failed to stop DataflowComponent service %s: %w", d.baseFSMInstance.GetID(), err)
	}

	d.baseFSMInstance.GetLogger().Debugf("DataflowComponent service %s stop command executed", d.baseFSMInstance.GetID())
	return nil
}

// getServiceStatus gets the status of the DataflowComponent service
// its main purpose is to handle the edge cases where the service is not yet created or not yet running
func (d *DataflowComponentInstance) getServiceStatus(ctx context.Context, filesystemService filesystem.Service, tick uint64) (dataflowcomponentservice.ServiceInfo, error) {
	info, err := d.service.Status(ctx, filesystemService, d.baseFSMInstance.GetID(), tick)
	if err != nil {
		// If there's an error getting the service status, we need to distinguish between cases

		if errors.Is(err, dataflowcomponentservice.ErrServiceNotExist) {
			// If the service is being created, we don't want to count this as an error
			// The instance is likely in Creating or ToBeCreated state, so service doesn't exist yet
			// This will be handled in the reconcileStateTransition where the service gets created
			if d.baseFSMInstance.GetCurrentFSMState() == internalfsm.LifecycleStateCreating ||
				d.baseFSMInstance.GetCurrentFSMState() == internalfsm.LifecycleStateToBeCreated {
				return dataflowcomponentservice.ServiceInfo{}, dataflowcomponentservice.ErrServiceNotExist
			}

			// Log the warning but don't treat it as a fatal error
			d.baseFSMInstance.GetLogger().Debugf("Service not found, will be created during reconciliation")
			return dataflowcomponentservice.ServiceInfo{}, nil
		}

		// For other errors, log them and return
		d.baseFSMInstance.GetLogger().Errorf("error updating observed state for %s: %s", d.baseFSMInstance.GetID(), err)
		infoWithFailedHealthChecks := info
		infoWithFailedHealthChecks.BenthosObservedState.ServiceInfo.BenthosStatus.HealthCheck.IsLive = false
		infoWithFailedHealthChecks.BenthosObservedState.ServiceInfo.BenthosStatus.HealthCheck.IsReady = false
		// return the info with healthchecks failed
		return infoWithFailedHealthChecks, err
	}

	return info, nil
}

// UpdateObservedStateOfInstance updates the observed state of the service
func (d *DataflowComponentInstance) UpdateObservedStateOfInstance(ctx context.Context, filesystemService filesystem.Service, tick uint64) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	start := time.Now()
	info, err := d.getServiceStatus(ctx, filesystemService, tick)
	if err != nil {
		return fmt.Errorf("error while getting service status: %w", err)
	}
	metrics.ObserveReconcileTime(logger.ComponentDataFlowComponentInstance, d.baseFSMInstance.GetID()+".getServiceStatus", time.Since(start))
	// Store the raw service info
	d.ObservedState.ServiceInfo = info

	// Fetch the actual Benthos config from the service
	start = time.Now()
	observedConfig, err := d.service.GetConfig(ctx, filesystemService, d.baseFSMInstance.GetID())
	metrics.ObserveReconcileTime(logger.ComponentDataFlowComponentInstance, d.baseFSMInstance.GetID()+".getConfig", time.Since(start))
	if err == nil {
		// Only update if we successfully got the config
		d.ObservedState.ObservedDataflowComponentConfig = observedConfig
	} else {
		if strings.Contains(err.Error(), dataflowcomponentservice.ErrServiceNotExist.Error()) {
			// Log the error but don't fail - this might happen during creation when the config file doesn't exist yet
			d.baseFSMInstance.GetLogger().Debugf("Service not found, will be created during reconciliation: %v", err)
			return nil
		} else {
			return fmt.Errorf("failed to get observed DataflowComponent config: %w", err)
		}
	}

	if !dataflowcomponentconfig.ConfigsEqual(&d.config, &d.ObservedState.ObservedDataflowComponentConfig) {
		// Check if the service exists before attempting to update
		if d.service.ServiceExists(ctx, filesystemService, d.baseFSMInstance.GetID()) {
			d.baseFSMInstance.GetLogger().Debugf("Observed DataflowComponent config is different from desired config, updating Benthos configuration")

			diffStr := dataflowcomponentconfig.ConfigDiff(&d.config, &d.ObservedState.ObservedDataflowComponentConfig)
			d.baseFSMInstance.GetLogger().Debugf("Configuration differences: %s", diffStr)

			// Update the config in the Benthos manager
			err := d.service.UpdateDataFlowComponentInBenthosManager(ctx, filesystemService, &d.config, d.baseFSMInstance.GetID())
			if err != nil {
				return fmt.Errorf("failed to update DataflowComponent service configuration: %w", err)
			}
		} else {
			d.baseFSMInstance.GetLogger().Debugf("Config differences detected but service does not exist yet, skipping update")
		}
	}

	return nil
}

// IsDataflowComponentBenthosRunning determines if the DataflowComponent's Benthos FSM is in running state.
// Architecture Decision: We intentionally rely only on the FSM state, not the underlying
// service implementation details. This maintains a clean separation of concerns where:
// 1. The FSM is the source of truth for service state
// 2. We trust the FSM's state management completely
// 3. Implementation details of how Benthos determines running state are encapsulated away
//
// Note: This function requires the BenthosFSMState to be updated in the ObservedState.
func (d *DataflowComponentInstance) IsDataflowComponentBenthosRunning() bool {
	switch d.ObservedState.ServiceInfo.BenthosFSMState {
	// Consider Active and Idle as running states
	case benthosfsm.OperationalStateActive, benthosfsm.OperationalStateIdle, benthosfsm.OperationalStateDegraded:
		return true
	}
	return false
}

// IsDataflowComponentBenthosStopped determines if the Dataflowcomponent's Benthos FSM is in the stopped state.
// Note: This function requires the BenthosFSMState to be updated in the ObservedState.
func (d *DataflowComponentInstance) IsDataflowComponentBenthosStopped() bool {
	return d.ObservedState.ServiceInfo.BenthosFSMState == benthosfsm.OperationalStateStopped
}

// IsDataflowComponentDegraded determines if the DataflowComponent service is degraded.
// These check everything that is checked during the starting phase
// But it means that it once worked, and then degraded
func (d *DataflowComponentInstance) IsDataflowComponentDegraded() bool {
	return d.ObservedState.ServiceInfo.BenthosFSMState == benthosfsm.OperationalStateDegraded
}

// IsDataflowComponentWithProcessingActivity determines if the Benthos instance has active data processing
// based on metrics data and possibly other observed state information
func (d *DataflowComponentInstance) IsDataflowComponentWithProcessingActivity() bool {
	benthosStatus := d.ObservedState.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus

	return benthosStatus.MetricsState != nil && benthosStatus.MetricsState.IsActive
}

func (d *DataflowComponentInstance) IsStartupGracePeriodExpired(currentTime time.Time, state string) bool {

	// Get the time when the DataflowComponentInstance entered the given state
	stateEntryTime, err := d.GetStateEntryTime(state)
	if err != nil {
		// If the stateEntryntime for the Starting state is not set, use a conservative  approach and assume we just entered the state
		stateEntryTime = currentTime
	}

	// Check if we have exceeded the grace period since entering the Starting state
	if currentTime.Sub(stateEntryTime) > constants.WaitTimeBeforeMarkingStartFailed {
		return true
	}
	return false
}
