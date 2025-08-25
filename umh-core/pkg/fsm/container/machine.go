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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/container_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// NewContainerInstance creates a new ContainerInstance with the standard transitions.
func NewContainerInstance(config config.ContainerConfig) *ContainerInstance {
	return NewContainerInstanceWithService(config, container_monitor.NewContainerMonitorService(filesystem.NewDefaultService()))
}

// NewContainerInstanceWithService creates a new ContainerInstance with a custom monitor service.
func NewContainerInstanceWithService(config config.ContainerConfig, service container_monitor.Service) *ContainerInstance {
	// Build the config for the base FSM
	fsmCfg := internal_fsm.BaseFSMInstanceConfig{
		ID: config.Name,
		// The user has said they only allow "active" or "stopped" as desired states
		DesiredFSMState:              config.DesiredFSMState,  // "active" or "stopped"
		OperationalStateAfterCreate:  OperationalStateStopped, // upon creation, start in stopped
		OperationalStateBeforeRemove: OperationalStateStopped, // must be stopped before removal
		OperationalTransitions: []fsm.EventDesc{
			// from stopped -> start -> starting
			{Name: EventStart, Src: []string{OperationalStateStopped}, Dst: OperationalStateStarting},

			// from starting -> degraded,
			{Name: EventStartDone, Src: []string{OperationalStateStarting}, Dst: OperationalStateDegraded},

			// from active -> metrics_not_ok -> degraded
			{Name: EventMetricsNotOK, Src: []string{OperationalStateActive}, Dst: OperationalStateDegraded},
			// from degraded -> metrics_all_ok -> active
			{Name: EventMetricsAllOK, Src: []string{OperationalStateDegraded}, Dst: OperationalStateActive},

			// from active/degraded/starting -> stop -> stopping
			{Name: EventStop, Src: []string{OperationalStateActive, OperationalStateDegraded, OperationalStateStarting}, Dst: OperationalStateStopping},

			// Final transition for stopping
			{Name: EventStopDone, Src: []string{OperationalStateStopping}, Dst: OperationalStateStopped},
		},
	}

	// Construct the base instance
	logger := logger.For(config.Name)
	backoffConfig := backoff.DefaultConfig(fsmCfg.ID, logger)
	baseFSM := internal_fsm.NewBaseFSMInstance(fsmCfg, backoffConfig, logger)

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

// SetDesiredFSMState is how external code updates the desired state at runtime.
func (c *ContainerInstance) SetDesiredFSMState(state string) error {
	// We only allow "active" or "stopped"
	if state != OperationalStateActive && state != OperationalStateStopped {
		return fmt.Errorf("invalid desired state: %s (only '%s' or '%s' allowed)",
			state, OperationalStateActive, OperationalStateStopped)
	}

	c.baseFSMInstance.SetDesiredFSMState(state)

	return nil
}

// GetCurrentFSMState returns the current operational or lifecycle state.
func (c *ContainerInstance) GetCurrentFSMState() string {
	return c.baseFSMInstance.GetCurrentFSMState()
}

// GetDesiredFSMState returns what we want operationally.
func (c *ContainerInstance) GetDesiredFSMState() string {
	return c.baseFSMInstance.GetDesiredFSMState()
}

// Remove initiates the removal lifecycle.
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
	return c.baseFSMInstance.GetCurrentFSMState() == OperationalStateStopped
}

func (c *ContainerInstance) IsStopping() bool {
	return c.baseFSMInstance.GetCurrentFSMState() == OperationalStateStopping
}

// WantsToBeStopped returns true if the instance wants to be stopped.
func (c *ContainerInstance) WantsToBeStopped() bool {
	return c.baseFSMInstance.GetDesiredFSMState() == OperationalStateStopped
}

// PrintState is a helper for debugging.
func (c *ContainerInstance) PrintState() {
	c.baseFSMInstance.GetLogger().Infof("ContainerInstance %s - Current state: %s, Desired: %s",
		c.baseFSMInstance.GetID(), c.GetCurrentFSMState(), c.GetDesiredFSMState())
}

// GetMinimumRequiredTime returns the minimum required time for this instance.
func (c *ContainerInstance) GetMinimumRequiredTime() time.Duration {
	return constants.ContainerUpdateObservedStateTimeout
}
