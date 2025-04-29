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

package benthos_monitor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	benthos_monitor_service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	standarderrors "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/standarderrors"
)

// CreateInstance is called when the FSM transitions from to_be_created -> creating.
func (b *BenthosMonitorInstance) CreateInstance(ctx context.Context, filesystemService filesystem.Service) error {
	b.baseFSMInstance.GetLogger().Debugf("Starting Action: Adding Benthos Monitor service %s to S6 manager ...", b.baseFSMInstance.GetID())

	b.baseFSMInstance.GetLogger().Debugf("Adding Benthos Monitor service %s to S6 manager with port %d", b.baseFSMInstance.GetID(), b.config.MetricsPort)
	err := b.monitorService.AddBenthosMonitorToS6Manager(ctx, b.config.MetricsPort)
	if err != nil {
		if err == benthos_monitor_service.ErrServiceAlreadyExists {
			b.baseFSMInstance.GetLogger().Debugf("Benthos Monitor service %s already exists in S6 manager", b.baseFSMInstance.GetID())
			return nil // do not throw an error, as each action is expected to be idempotent
		}
		return fmt.Errorf("failed to add Benthos Monitor service %s to S6 manager: %w", b.baseFSMInstance.GetID(), err)
	}

	b.baseFSMInstance.GetLogger().Debugf("Benthos Monitor service %s added to S6 manager", b.baseFSMInstance.GetID())

	return nil
}

// RemoveInstance is called when the FSM transitions to removing.
// It requires the service to be stopped before removal.
func (b *BenthosMonitorInstance) RemoveInstance(ctx context.Context, filesystemService filesystem.Service) error {
	b.baseFSMInstance.GetLogger().Debugf("Starting Action: Removing Benthos Monitor service %s from S6 manager ...", b.baseFSMInstance.GetID())

	// Remove the Benthos from the S6 manager
	err := b.monitorService.RemoveBenthosMonitorFromS6Manager(ctx)
	switch {
	// ---------------------------------------------------------------
	// happy paths
	// ---------------------------------------------------------------
	case err == nil: // fully removed
		b.baseFSMInstance.GetLogger().
			Infof("Benthos monitor service %s removed from S6 manager",
				b.baseFSMInstance.GetID())
		return nil

	case errors.Is(err, benthos_monitor_service.ErrServiceNotExist):
		b.baseFSMInstance.GetLogger().
			Infof("Benthos monitor service %s already removed from S6 manager",
				b.baseFSMInstance.GetID())
		// idempotent: was already gone
		return nil

	// ---------------------------------------------------------------
	// transient path – keep retrying
	// ---------------------------------------------------------------
	case errors.Is(err, standarderrors.ErrRemovalPending):
		b.baseFSMInstance.GetLogger().
			Infof("Benthos monitor service %s removal still in progress",
				b.baseFSMInstance.GetID())
		// not an error from the FSM’s perspective – just means “try again”
		return err

	// ---------------------------------------------------------------
	// real error – escalate
	// ---------------------------------------------------------------
	default:
		b.baseFSMInstance.GetLogger().
			Errorf("failed to remove Benthos Monitor service %s: %s",
				b.baseFSMInstance.GetID(), err)
		return fmt.Errorf("failed to remove Benthos Monitor service %s: %w",
			b.baseFSMInstance.GetID(), err)
	}
}

// StartInstance is called when the agent monitoring should be enabled.
// Currently this is a no-op as the monitoring service runs independently.
func (b *BenthosMonitorInstance) StartInstance(ctx context.Context, filesystemService filesystem.Service) error {
	b.baseFSMInstance.GetLogger().Debugf("Starting Action: Starting Benthos Monitor service %s ...", b.baseFSMInstance.GetID())

	// TODO: Add pre-start validation

	// Set the desired state to running for the given instance
	err := b.monitorService.StartBenthosMonitor(ctx)
	if err != nil {
		// if the service is not there yet but we attempt to start it, we need to throw an error
		return fmt.Errorf("failed to start Benthos Monitor service %s: %w", b.baseFSMInstance.GetID(), err)
	}

	b.baseFSMInstance.GetLogger().Debugf("Benthos Monitor service %s start command executed", b.baseFSMInstance.GetID())
	return nil
}

// StopInstance is called when the agent monitoring should be disabled.
// Currently this is a no-op as the monitoring service runs independently.
func (b *BenthosMonitorInstance) StopInstance(ctx context.Context, filesystemService filesystem.Service) error {
	b.baseFSMInstance.GetLogger().Debugf("Starting Action: Stopping Benthos Monitor service %s ...", b.baseFSMInstance.GetID())

	// Set the desired state to stopped for the given instance
	err := b.monitorService.StopBenthosMonitor(ctx)
	if err != nil {
		return fmt.Errorf("failed to stop Benthos Monitor service %s: %w", b.baseFSMInstance.GetID(), err)
	}

	b.baseFSMInstance.GetLogger().Debugf("Benthos Monitor service %s stop command executed", b.baseFSMInstance.GetID())
	return nil
}

// CheckForCreation checks if the Benthos Monitor service should be created
func (b *BenthosMonitorInstance) CheckForCreation(ctx context.Context, filesystemService filesystem.Service) bool {
	return true
}

// UpdateObservedStateOfInstance is called when the FSM transitions to updating.
func (b *BenthosMonitorInstance) UpdateObservedStateOfInstance(ctx context.Context, filesystemService filesystem.Service, tick uint64, loopStartTime time.Time) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	start := time.Now()
	info, err := b.monitorService.Status(ctx, filesystemService, tick)
	if err != nil {
		return err
	}
	metrics.ObserveReconcileTime(logger.ComponentBenthosMonitorInstance, b.baseFSMInstance.GetID()+".getServiceStatus", time.Since(start))

	// Store the raw service info
	b.ObservedState.ServiceInfo = &info
	return nil
}

// isMonitorHealthy checks if the last scan was successful and the service is running.
func (b *BenthosMonitorInstance) isMonitorHealthy(loopStartTime time.Time) bool {
	if b.ObservedState.ServiceInfo == nil {
		return false
	}

	if b.ObservedState.ServiceInfo.BenthosStatus.LastScan == nil {
		return false
	}

	// Check that the last scan is not older then BenthosMaxMetricsAndConfigAge
	if loopStartTime.Sub(b.ObservedState.ServiceInfo.BenthosStatus.LastScan.LastUpdatedAt) > constants.BenthosMaxMetricsAndConfigAge {
		b.baseFSMInstance.GetLogger().Warnf("last scan is %s old, returning empty status", loopStartTime.Sub(b.ObservedState.ServiceInfo.BenthosStatus.LastScan.LastUpdatedAt))
		return false
	}

	// if the metrics are not available, the service is not healthy
	if b.ObservedState.ServiceInfo.BenthosStatus.LastScan.BenthosMetrics == nil {
		return false
	}

	return true
}
