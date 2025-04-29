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

package redpanda_monitor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	redpanda_monitor_service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/standarderrors"
)

// CreateInstance is called when the FSM transitions from to_be_created -> creating.
func (r *RedpandaMonitorInstance) CreateInstance(ctx context.Context, filesystemService filesystem.Service) error {
	r.baseFSMInstance.GetLogger().Debugf("Starting Action: Adding Redpanda Monitor service %s to S6 manager ...", r.baseFSMInstance.GetID())

	r.baseFSMInstance.GetLogger().Debugf("Adding Redpanda Monitor service %s to S6 manager", r.baseFSMInstance.GetID())
	err := r.monitorService.AddRedpandaMonitorToS6Manager(ctx)
	if err != nil {
		if err == redpanda_monitor_service.ErrServiceAlreadyExists {
			r.baseFSMInstance.GetLogger().Debugf("Redpanda Monitor service %s already exists in S6 manager", r.baseFSMInstance.GetID())
			return nil // do not throw an error, as each action is expected to be idempotent
		}
		return fmt.Errorf("failed to add Redpanda Monitor service %s to S6 manager: %w", r.baseFSMInstance.GetID(), err)
	}

	r.baseFSMInstance.GetLogger().Debugf("Redpanda Monitor service %s added to S6 manager", r.baseFSMInstance.GetID())

	return nil
}

// RemoveInstance is called when the FSM transitions to removing.
// It requires the service to be stopped before removal.
func (r *RedpandaMonitorInstance) RemoveInstance(ctx context.Context, filesystemService filesystem.Service) error {
	r.baseFSMInstance.GetLogger().Debugf("Starting Action: Removing Redpanda Monitor service %s from S6 manager ...", r.baseFSMInstance.GetID())

	// Remove the Redpanda from the S6 manager
	err := r.monitorService.RemoveRedpandaMonitorFromS6Manager(ctx)
	switch {
	// ---------------------------------------------------------------
	// happy paths
	// ---------------------------------------------------------------
	case err == nil: // fully removed
		r.baseFSMInstance.GetLogger().Debugf("Redpanda Monitor service %s removed from S6 manager", r.baseFSMInstance.GetID())
		return nil

	case err == redpanda_monitor_service.ErrServiceNotExist:
		r.baseFSMInstance.GetLogger().Debugf("Redpanda Monitor service %s not found in S6 manager", r.baseFSMInstance.GetID())
		// idempotent: was already gone
		return nil

	// ---------------------------------------------------------------
	// transient path – keep retrying
	// ---------------------------------------------------------------
	case errors.Is(err, standarderrors.ErrRemovalPending):
		r.baseFSMInstance.GetLogger().Infof("Redpanda monitor service %s removal still in progress", r.baseFSMInstance.GetID())
		// not an error from the FSM's perspective – just means "try again"
		return err

	// ---------------------------------------------------------------
	// real error – escalate
	// ---------------------------------------------------------------
	default:
		r.baseFSMInstance.GetLogger().Errorf("failed to remove Redpanda Monitor service %s: %s", r.baseFSMInstance.GetID(), err)
		return fmt.Errorf("failed to remove Redpanda Monitor service %s: %w", r.baseFSMInstance.GetID(), err)
	}
}

// StartInstance is called when the agent monitoring should be enabled.
// Currently this is a no-op as the monitoring service runs independently.
func (r *RedpandaMonitorInstance) StartInstance(ctx context.Context, filesystemService filesystem.Service) error {
	r.baseFSMInstance.GetLogger().Debugf("Starting Action: Starting Redpanda Monitor service %s ...", r.baseFSMInstance.GetID())

	// TODO: Add pre-start validation

	// Set the desired state to running for the given instance
	err := r.monitorService.StartRedpandaMonitor(ctx)
	if err != nil {
		// if the service is not there yet but we attempt to start it, we need to throw an error
		return fmt.Errorf("failed to start Redpanda Monitor service %s: %w", r.baseFSMInstance.GetID(), err)
	}

	r.baseFSMInstance.GetLogger().Debugf("Redpanda Monitor service %s start command executed", r.baseFSMInstance.GetID())
	return nil
}

// StopInstance is called when the agent monitoring should be disabled.
// Currently this is a no-op as the monitoring service runs independently.
func (r *RedpandaMonitorInstance) StopInstance(ctx context.Context, filesystemService filesystem.Service) error {
	r.baseFSMInstance.GetLogger().Debugf("Starting Action: Stopping Redpanda Monitor service %s ...", r.baseFSMInstance.GetID())

	// Set the desired state to stopped for the given instance
	err := r.monitorService.StopRedpandaMonitor(ctx)
	if err != nil {
		return fmt.Errorf("failed to stop Redpanda Monitor service %s: %w", r.baseFSMInstance.GetID(), err)
	}

	r.baseFSMInstance.GetLogger().Debugf("Redpanda Monitor service %s stop command executed", r.baseFSMInstance.GetID())
	return nil
}

// UpdateObservedStateOfInstance is called when the FSM transitions to updating.
func (r *RedpandaMonitorInstance) UpdateObservedStateOfInstance(ctx context.Context, services serviceregistry.Provider, tick uint64, loopStartTime time.Time) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	start := time.Now()
	info, err := r.monitorService.Status(ctx, services.GetFileSystem(), tick)
	if err != nil {
		return err
	}
	metrics.ObserveReconcileTime(logger.ComponentRedpandaMonitorInstance, r.baseFSMInstance.GetID()+".getServiceStatus", time.Since(start))

	// Store the raw service info
	r.ObservedState.ServiceInfo = &info
	return nil
}

// isMonitorHealthy checks if the last scan was successful and the service is running.
func (r *RedpandaMonitorInstance) isMonitorHealthy(loopStartTime time.Time) bool {
	if r.ObservedState.ServiceInfo == nil {
		return false
	}

	if r.ObservedState.ServiceInfo.RedpandaStatus.LastScan == nil {
		return false
	}

	// Check that the last scan is not older then RedpandaMaxMetricsAndConfigAge
	if loopStartTime.Sub(r.ObservedState.ServiceInfo.RedpandaStatus.LastScan.LastUpdatedAt) > constants.RedpandaMaxMetricsAndConfigAge {
		r.baseFSMInstance.GetLogger().Warnf("last scan is %s old, returning empty status", loopStartTime.Sub(r.ObservedState.ServiceInfo.RedpandaStatus.LastScan.LastUpdatedAt))
		return false
	}

	// if the metrics are not available, the service is not healthy
	if r.ObservedState.ServiceInfo.RedpandaStatus.LastScan.RedpandaMetrics == nil {
		return false
	}

	return true
}

// CheckForCreation checks if the Redpanda Monitor service should be created
func (r *RedpandaMonitorInstance) CheckForCreation(ctx context.Context, filesystemService filesystem.Service) bool {
	return true
}
