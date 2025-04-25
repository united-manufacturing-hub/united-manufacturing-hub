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

package container

import (
	"context"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// In Benthos, actions.go contained idempotent operations (like starting/stopping a service).
// For the container monitor, we technically don't "start/stop" the container itself—we're only
// enabling or disabling the monitoring. We'll keep placeholder actions for consistency.

// CreateInstance is called when the FSM transitions from to_be_created -> creating.
// For container monitoring, this is a no-op as there's no actual container to create.
// This function is present for structural consistency with other FSM packages.
func (c *ContainerInstance) CreateInstance(ctx context.Context, filesystemService filesystem.Service) error {
	c.baseFSMInstance.GetLogger().Debugf("Creating container monitor instance %s (no-op)", c.baseFSMInstance.GetID())
	return nil
}

// RemoveInstance is called when the FSM transitions to removing.
// For container monitoring, this is a no-op as we don't need to remove any resources.
// This function is present for structural consistency with other FSM packages.
func (c *ContainerInstance) RemoveInstance(ctx context.Context, filesystemService filesystem.Service) error {
	c.baseFSMInstance.GetLogger().Debugf("Removing container monitor instance %s (no-op)", c.baseFSMInstance.GetID())
	return nil
}

// optionally, we might have something like "enableMonitoring" / "disableMonitoring" if
// you want actual side effects. For now, do no-ops or just logs.

// StartInstance is called when the container monitoring should be enabled.
// Currently this is a no-op as the monitoring service runs independently.
func (c *ContainerInstance) StartInstance(ctx context.Context, filesystemService filesystem.Service) error {
	c.baseFSMInstance.GetLogger().Infof("Enabling monitoring for %s (no-op)", c.baseFSMInstance.GetID())
	return nil
}

// StopInstance is called when the container monitoring should be disabled.
// Currently this is a no-op as the monitoring service runs independently.
func (c *ContainerInstance) StopInstance(ctx context.Context, filesystemService filesystem.Service) error {
	c.baseFSMInstance.GetLogger().Infof("Disabling monitoring for %s (no-op)", c.baseFSMInstance.GetID())
	return nil
}

// UpdateObservedStateOfInstance is called when the FSM transitions to updating.
// For container monitoring, this is a no-op as we don't need to update any resources.
func (c *ContainerInstance) UpdateObservedStateOfInstance(ctx context.Context, filesystemService filesystem.Service, tick uint64, loopStartTime time.Time) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	currentState := c.baseFSMInstance.GetCurrentFSMState()
	desiredState := c.baseFSMInstance.GetDesiredFSMState()
	// If both desired and current state are stopped, we can return immediately
	// There wont be any logs, metrics, etc. to check
	if desiredState == OperationalStateStopped && currentState == OperationalStateStopped {
		return nil
	}

	c.baseFSMInstance.GetLogger().Debugf("Updating observed state for %s (no-op)", c.baseFSMInstance.GetID())
	return nil
}

// areAllMetricsHealthy decides if the container health is Active
func (c *ContainerInstance) areAllMetricsHealthy() bool {
	status := c.ObservedState.ServiceInfo
	if status == nil {
		// If we have no data, let's consider it not healthy
		return false
	}

	// Only consider container healthy if the overall health category is Active
	return status.OverallHealth == models.Active
}
