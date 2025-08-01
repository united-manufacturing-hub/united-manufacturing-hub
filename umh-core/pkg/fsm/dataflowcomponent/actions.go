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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthosfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	dataflowcomponentservice "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	standarderrors "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/standarderrors"
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
		if errors.Is(err, dataflowcomponentservice.ErrServiceAlreadyExists) {
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
	switch {
	// ---------------------------------------------------------------
	// happy paths
	// ---------------------------------------------------------------
	case err == nil: // fully removed
		b.baseFSMInstance.GetLogger().
			Debugf("Benthos service %s removed from S6 manager",
				b.baseFSMInstance.GetID())
		return nil

	case errors.Is(err, dataflowcomponentservice.ErrServiceNotExists):
		b.baseFSMInstance.GetLogger().
			Debugf("Benthos service %s already removed from S6 manager",
				b.baseFSMInstance.GetID())
		// idempotent: was already gone
		return nil

	// ---------------------------------------------------------------
	// transient path – keep retrying
	// ---------------------------------------------------------------
	case errors.Is(err, standarderrors.ErrRemovalPending):
		b.baseFSMInstance.GetLogger().
			Debugf("Benthos service %s removal still in progress",
				b.baseFSMInstance.GetID())
		// not an error from the FSM’s perspective – just means “try again”
		return err

	// ---------------------------------------------------------------
	// real error – escalate
	// ---------------------------------------------------------------
	default:
		return fmt.Errorf("failed to remove service %s: %w",
			b.baseFSMInstance.GetID(), err)
	}
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

// CheckForCreation checks whether the creation was successful
// For DataflowComponent, this is a no-op as we don't need to check anything
func (d *DataflowComponentInstance) CheckForCreation(ctx context.Context, filesystemService filesystem.Service) bool {
	return true
}

// getServiceStatus gets the status of the DataflowComponent service
// its main purpose is to handle the edge cases where the service is not yet created or not yet running
func (d *DataflowComponentInstance) getServiceStatus(ctx context.Context, filesystemService filesystem.Service, tick uint64) (dataflowcomponentservice.ServiceInfo, error) {
	info, err := d.service.Status(ctx, filesystemService, d.baseFSMInstance.GetID(), tick)
	if err != nil {
		// If there's an error getting the service status, we need to distinguish between cases

		if errors.Is(err, dataflowcomponentservice.ErrServiceNotExists) {
			// If the service is being created, we don't want to count this as an error
			// The instance is likely in Creating or ToBeCreated state, so service doesn't exist yet
			// This will be handled in the reconcileStateTransition where the service gets created
			if d.baseFSMInstance.GetCurrentFSMState() == internalfsm.LifecycleStateCreating ||
				d.baseFSMInstance.GetCurrentFSMState() == internalfsm.LifecycleStateToBeCreated {
				return dataflowcomponentservice.ServiceInfo{}, dataflowcomponentservice.ErrServiceNotExists
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
func (d *DataflowComponentInstance) UpdateObservedStateOfInstance(ctx context.Context, services serviceregistry.Provider, snapshot fsm.SystemSnapshot) error {
	if ctx.Err() != nil {
		if d.baseFSMInstance.IsDeadlineExceededAndHandle(ctx.Err(), snapshot.Tick, "UpdateObservedStateOfInstance") {
			return nil
		}
		return ctx.Err()
	}

	start := time.Now()
	info, err := d.getServiceStatus(ctx, services.GetFileSystem(), snapshot.Tick)
	if err != nil {
		return fmt.Errorf("error while getting service status: %w", err)
	}
	metrics.ObserveReconcileTime(logger.ComponentDataFlowComponentInstance, d.baseFSMInstance.GetID()+".getServiceStatus", time.Since(start))
	// Store the raw service info
	d.ObservedState.ServiceInfo = info

	currentState := d.baseFSMInstance.GetCurrentFSMState()
	desiredState := d.baseFSMInstance.GetDesiredFSMState()
	// If both desired and current state are stopped, we can return immediately
	// There wont be any logs, metrics, etc. to check
	if desiredState == OperationalStateStopped && currentState == OperationalStateStopped {
		return nil
	}

	// Fetch the actual Benthos config from the service
	start = time.Now()
	observedConfig, err := d.service.GetConfig(ctx, services.GetFileSystem(), d.baseFSMInstance.GetID())
	metrics.ObserveReconcileTime(logger.ComponentDataFlowComponentInstance, d.baseFSMInstance.GetID()+".getConfig", time.Since(start))
	if err == nil {
		// Only update if we successfully got the config
		d.ObservedState.ObservedDataflowComponentConfig = observedConfig
	} else {
		if strings.Contains(err.Error(), dataflowcomponentservice.ErrServiceNotExists.Error()) {
			// Log the error but don't fail - this might happen during creation when the config file doesn't exist yet
			d.baseFSMInstance.GetLogger().Debugf("Service not found, will be created during reconciliation: %v", err)
			return nil
		} else {
			return fmt.Errorf("failed to get observed DataflowComponent config: %w", err)
		}
	}

	if !dataflowcomponentserviceconfig.ConfigsEqual(d.config, d.ObservedState.ObservedDataflowComponentConfig) {
		// Check if the service exists before attempting to update
		if d.service.ServiceExists(ctx, services.GetFileSystem(), d.baseFSMInstance.GetID()) {
			d.baseFSMInstance.GetLogger().Debugf("Observed DataflowComponent config is different from desired config, updating Benthos configuration")

			diffStr := dataflowcomponentserviceconfig.ConfigDiff(d.config, d.ObservedState.ObservedDataflowComponentConfig)
			d.baseFSMInstance.GetLogger().Debugf("Configuration differences: %s", diffStr)

			// Update the config in the Benthos manager
			err := d.service.UpdateDataFlowComponentInBenthosManager(ctx, services.GetFileSystem(), &d.config, d.baseFSMInstance.GetID())
			if err != nil {
				return fmt.Errorf("failed to update DataflowComponent service configuration: %w", err)
			}
		} else {
			d.baseFSMInstance.GetLogger().Debugf("Config differences detected but service does not exist yet, skipping update")
		}
	}

	return nil
}

// IsDataflowComponentBenthosRunning reports true when the embedded Benthos FSM
// is in one of the running states (Active, Idle or Degraded).
func (d *DataflowComponentInstance) IsDataflowComponentBenthosRunning() bool {
	switch d.ObservedState.ServiceInfo.BenthosFSMState {
	// Consider Active , Degraded and Idle as running states
	case benthosfsm.OperationalStateActive, benthosfsm.OperationalStateIdle, benthosfsm.OperationalStateDegraded:
		return true
	}
	return false
}

// IsDataflowComponentBenthosStopped reports true when the Benthos FSM state is
// benthosfsm.OperationalStateStopped.
func (d *DataflowComponentInstance) IsDataflowComponentBenthosStopped() bool {
	return d.ObservedState.ServiceInfo.BenthosFSMState == benthosfsm.OperationalStateStopped
}

// IsDataflowComponentDegraded reports true when the Benthos FSM is already in
// OperationalStateDegraded.
//
// It returns:
//
//	degraded – true when degraded, false otherwise.
//	reason   – empty when degraded is false; otherwise the current Benthos
//	           status reason.
func (d *DataflowComponentInstance) IsDataflowComponentDegraded() (bool, string) {
	if d.ObservedState.ServiceInfo.BenthosFSMState == benthosfsm.OperationalStateDegraded {
		return true, d.ObservedState.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus.StatusReason
	}
	return false, ""
}

// IsDataflowComponentWithProcessingActivity reports true when Benthos metrics
// indicate active message processing.
//
// It returns:
//
//	ok     – true when activity is detected, false otherwise.
//	reason – empty when ok is true; otherwise the Benthos status reason.
func (d *DataflowComponentInstance) IsDataflowComponentWithProcessingActivity() (bool, string) {
	benthosStatus := d.ObservedState.ServiceInfo.BenthosObservedState.ServiceInfo.BenthosStatus

	if benthosStatus.BenthosMetrics.MetricsState != nil && benthosStatus.BenthosMetrics.MetricsState.IsActive {
		return true, ""
	}

	return false, benthosStatus.StatusReason // no need to format, as it is already formatted
}

// IsStartingPeriodGracePeriodExceeded returns true when the difference
// between `currentTime` (supplied by Reconcile for easier testing) and the
// timestamp of the **latest** EventStart/OperationalStateStarting record
// in the archive exceeds `constants.WaitTimeBeforeMarkingStartFailed`.
//
// Why archive lookup instead of a timer?
//   - Unit tests can inject an arbitrary `currentTime` without monkey-
//     patching `time.Now()`.
//   - Persistence across restarts guarantees that the grace-period is not
//     reset accidentally.
//
// Behaviour:
//   - If the archive has *no* Starting event yet, we treat the grace period
//     as **not** exceeded (return false) – Reconcile will wait for the next
//     tick.
//   - Any unexpected error results in a conservative **false** because we
//     prefer to wait rather than fail spuriously.
func (d *DataflowComponentInstance) IsStartingPeriodGracePeriodExceeded(ctx context.Context, currentTime time.Time) (bool, string) {
	// TODO
	return false, ""
}

// DidDFCAlreadyFailedBefore reports whether **any** permanent-failure event
// for this DataflowComponent (currently `OperationalStateStartingFailed`)
// has ever been written to the archive.
//
// Business rules:
//  1. Starting is considered an *irrecoverable* failure – the instance
//     must be deleted/recreated by an operator or by the manager after a
//     config change.  The FSM must therefore *never* leave
//     `StartingFailed` automatically.
//  2. To make the decision idempotent and testable we query the persistent
//     `ArchiveEventStorage` instead of using an in-memory flag.
//  3. The archive is assumed to be durable across process restarts;
//     if it is not, this function must return `false`, effectively
//     disabling the feature.
func (d *DataflowComponentInstance) DidDFCAlreadyFailedBefore(ctx context.Context) (bool, string) {
	// TODO: Implement this
	// TODO: make sure that the archive storage is persistent
	/*
		// Query for any data points with the specified failed state
		options := storage.QueryOptions{
			Limit:     1,                                                           // We only need to know if any exist, so limit to 1
			StartTime: time.Now().Add(-constants.WaitTimeBeforeMarkingStartFailed), // Get only the recent events to avoid old stale events
			SortDesc:  true,                                                        // recent events first
			States:    d.DataflowComponentBenthosFailureStates(),
		}




			if d.archiveStorage == nil {
				d.baseFSMInstance.GetLogger().Warnf("archiveStorage is not initialized- skippling failed-state detection")
				return false
			}

			dataPoints, err := d.archiveStorage.GetDataPoints(ctx, d.baseFSMInstance.GetID(), options)
			if err != nil {
				d.baseFSMInstance.GetLogger().Warnf("Failed to query state history: %v", err)
				return false // Default to false if we can't query
			}

			d.baseFSMInstance.GetLogger().Infof("Found %d data points with failed states. Data points: %v", len(dataPoints), dataPoints)
			// If we found any data points with this state, return true
			return len(dataPoints) > 0
	*/
	return false, ""
}
