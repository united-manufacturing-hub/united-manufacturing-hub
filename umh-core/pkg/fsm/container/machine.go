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
	"fmt"
	"time"

	"github.com/looplab/fsm"
	internal_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/container_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// ContainerConfig is the portion of your FullConfig for container monitoring.
type ContainerConfig struct {
	Name            string `json:"name"`            // name of the container instance
	DesiredFSMState string `json:"desiredFSMState"` // e.g. "active" or "stopped"
}

// ContainerInstance holds the FSM instance and references to the container monitor service.
type ContainerInstance struct {
	// This embeds the "BaseFSMInstance" which handles lifecycle states,
	// desired state, removal, etc.
	baseFSMInstance *internal_fsm.BaseFSMInstance

	// ObservedState: last known container metrics, updated in reconcile
	ObservedState ContainerObservedState

	// The container monitor service used to gather metrics
	monitorService container_monitor.Service

	// Possibly store config needed for the container monitor
	config ContainerConfig
}

// Verify at compile time that we implement fsm.FSMInstance
var _ ContainerMonitorInstance = (*ContainerInstance)(nil)

// NewContainerInstance creates a new ContainerInstance with the standard transitions.
func NewContainerInstance(config ContainerConfig) *ContainerInstance {
	return NewContainerInstanceWithService(config, container_monitor.NewContainerMonitorService(filesystem.NewDefaultService()))
}

// NewContainerInstanceWithService creates a new ContainerInstance with a custom monitor service.
func NewContainerInstanceWithService(config ContainerConfig, service container_monitor.Service) *ContainerInstance {
	// Build the config for the base FSM
	fsmCfg := internal_fsm.BaseFSMInstanceConfig{
		ID: config.Name,
		// The user has said they only allow "active" or "stopped" as desired states
		DesiredFSMState:              config.DesiredFSMState, // "active" or "stopped"
		OperationalStateAfterCreate:  MonitoringStateStopped, // upon creation, start in stopped
		OperationalStateBeforeRemove: MonitoringStateStopped, // must be stopped before removal
		OperationalTransitions: []fsm.EventDesc{
			// from monitoring_stopped -> start -> degraded
			{Name: EventStart, Src: []string{MonitoringStateStopped}, Dst: MonitoringStateDegraded},

			// from active -> metrics_not_ok -> degraded
			{Name: EventMetricsNotOK, Src: []string{MonitoringStateActive}, Dst: MonitoringStateDegraded},
			// from degraded -> metrics_all_ok -> active
			{Name: EventMetricsAllOK, Src: []string{MonitoringStateDegraded}, Dst: MonitoringStateActive},

			// from active -> stop -> monitoring_stopped
			{Name: EventStop, Src: []string{MonitoringStateActive}, Dst: MonitoringStateStopped},
			// from degraded -> stop -> monitoring_stopped
			{Name: EventStop, Src: []string{MonitoringStateDegraded}, Dst: MonitoringStateStopped},
		},
	}

	// Construct the base instance
	baseFSM := internal_fsm.NewBaseFSMInstance(fsmCfg, logger.For(config.Name))

	// Create our instance
	instance := &ContainerInstance{
		baseFSMInstance: baseFSM,
		monitorService:  service,
		config:          config,
	}

	// Register any state-entry callbacks
	instance.registerCallbacks()

	// Initialize error counter
	metrics.InitErrorCounter(metrics.ComponentContainerMonitor, config.Name)

	return instance
}

// SetDesiredFSMState is how external code updates the desired state at runtime
func (c *ContainerInstance) SetDesiredFSMState(state string) error {
	// We only allow "active" or "stopped"
	if state != MonitoringStateActive && state != MonitoringStateStopped {
		return fmt.Errorf("invalid desired state: %s (only '%s' or '%s' allowed)",
			state, MonitoringStateActive, MonitoringStateStopped)
	}
	c.baseFSMInstance.SetDesiredFSMState(state)
	return nil
}

// GetCurrentFSMState returns the current operational or lifecycle state
func (c *ContainerInstance) GetCurrentFSMState() string {
	return c.baseFSMInstance.GetCurrentFSMState()
}

// GetDesiredFSMState returns what we want operationally
func (c *ContainerInstance) GetDesiredFSMState() string {
	return c.baseFSMInstance.GetDesiredFSMState()
}

// Remove initiates the removal lifecycle
func (c *ContainerInstance) Remove(ctx context.Context) error {
	return c.baseFSMInstance.Remove(ctx)
}

func (c *ContainerInstance) IsRemoved() bool {
	return c.baseFSMInstance.IsRemoved()
}

func (c *ContainerInstance) IsRemoving() bool {
	return c.baseFSMInstance.IsRemoving()
}

func (c *ContainerInstance) IsStopped() bool {
	return c.baseFSMInstance.GetCurrentFSMState() == MonitoringStateStopped
}

// PrintState is a helper for debugging
func (c *ContainerInstance) PrintState() {
	c.baseFSMInstance.GetLogger().Infof("ContainerInstance %s - Current state: %s, Desired: %s",
		c.baseFSMInstance.GetID(), c.GetCurrentFSMState(), c.GetDesiredFSMState())
}

// GetExpectedMaxP95ExecutionTimePerInstance returns the expected max p95 execution time of the instance
func (c *ContainerInstance) GetExpectedMaxP95ExecutionTimePerInstance() time.Duration {
	return constants.ContainerExpectedMaxP95ExecutionTimePerInstance
}
