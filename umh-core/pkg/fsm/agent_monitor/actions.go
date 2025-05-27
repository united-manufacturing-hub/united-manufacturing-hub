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

package agent_monitor

import (
	"context"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/serviceregistry"
)

// In Benthos, actions.go contained idempotent operations (like starting/stopping a service).
// For the agent monitor, we technically don't "start/stop" the agent itself—we're only
// enabling or disabling the monitoring. We'll keep placeholder actions for consistency.

// CreateInstance is called when the FSM transitions from to_be_created -> creating.
// For agent monitoring, this is a no-op as there's no actual agent to create.
// This function is present for structural consistency with other FSM packages.
func (a *AgentInstance) CreateInstance(ctx context.Context, filesystemService filesystem.Service) error {
	a.baseFSMInstance.GetLogger().Debugf("Creating agent monitor instance %s (no-op)", a.baseFSMInstance.GetID())
	return nil
}

// RemoveInstance is called when the FSM transitions to removing.
// For agent monitoring, this is a no-op as we don't need to remove any resources.
// This function is present for structural consistency with other FSM packages.
func (a *AgentInstance) RemoveInstance(ctx context.Context, filesystemService filesystem.Service) error {
	a.baseFSMInstance.GetLogger().Debugf("Removing agent monitor instance %s (no-op)", a.baseFSMInstance.GetID())
	return nil
}

// optionally, we might have something like "enableMonitoring" / "disableMonitoring" if
// you want actual side effects. For now, do no-ops or just logs.

// StartInstance is called when the agent monitoring should be enabled.
// Currently this is a no-op as the monitoring service runs independently.
func (a *AgentInstance) StartInstance(ctx context.Context, filesystemService filesystem.Service) error {
	a.baseFSMInstance.GetLogger().Infof("Enabling agent monitoring for %s (no-op)", a.baseFSMInstance.GetID())
	return nil
}

// StopInstance is called when the agent monitoring should be disabled.
// Currently this is a no-op as the monitoring service runs independently.
func (a *AgentInstance) StopInstance(ctx context.Context, filesystemService filesystem.Service) error {
	a.baseFSMInstance.GetLogger().Infof("Disabling agent monitoring for %s (no-op)", a.baseFSMInstance.GetID())
	return nil
}

// CheckForCreation is called when the FSM transitions to creating.
// For agent monitoring, this is a no-op as we don't need to check anything
func (a *AgentInstance) CheckForCreation(ctx context.Context, filesystemService filesystem.Service) bool {
	return true
}

// UpdateObservedStateOfInstance is called when the FSM transitions to updating.
// For agent monitoring, this is a no-op as we don't need to update any resources.
func (a *AgentInstance) UpdateObservedStateOfInstance(ctx context.Context, services serviceregistry.Provider, snapshot fsm.SystemSnapshot, tick uint64, loopStartTime time.Time) error {
	a.baseFSMInstance.GetLogger().Debugf("Updating observed state for %s (no-op)", a.baseFSMInstance.GetID())
	return nil
}

// areAllMetricsHealthy decides if the agent health is Active
func (a *AgentInstance) areAllMetricsHealthy() bool {
	status := a.ObservedState.ServiceInfo
	if status == nil {
		// If we have no data, let's consider it not healthy
		return false
	}

	// Only consider agent healthy if the overall health category is Active
	return status.OverallHealth == models.Active
}
