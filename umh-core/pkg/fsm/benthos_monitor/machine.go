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
	"fmt"
	"time"

	"github.com/looplab/fsm"
	internal_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	benthos_monitor "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

// NewBenthosMonitorInstance creates a new BenthosMonitorInstance with the standard transitions.
func NewBenthosMonitorInstance(config config.BenthosMonitorConfig) *BenthosMonitorInstance {
	return NewBenthosMonitorInstanceWithService(config, benthos_monitor.NewBenthosMonitorService(config.Name,
		benthos_monitor.WithS6Service(s6.NewDefaultService()),
	))
}

// NewBenthosMonitorInstanceWithService creates a new BenthosMonitorInstance with a custom monitor service.
func NewBenthosMonitorInstanceWithService(config config.BenthosMonitorConfig, service benthos_monitor.IBenthosMonitorService) *BenthosMonitorInstance {
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
	baseFSM := internal_fsm.NewBaseFSMInstance(fsmCfg, backoff.DefaultConfig(metrics.ComponentAgentMonitor, logger.For(config.Name)), logger.For(config.Name))

	// Create our instance
	instance := &BenthosMonitorInstance{
		baseFSMInstance: baseFSM,
		monitorService:  service,
		config:          config,
	}

	// Register any state-entry callbacks
	instance.registerCallbacks()

	// Initialize error counter
	metrics.InitErrorCounter(metrics.ComponentAgentMonitor, config.Name)

	return instance
}

// SetDesiredFSMState is how external code updates the desired state at runtime
func (a *BenthosMonitorInstance) SetDesiredFSMState(state string) error {
	// We only allow "active" or "stopped"
	if state != OperationalStateActive && state != OperationalStateStopped {
		return fmt.Errorf("invalid desired state: %s (only '%s' or '%s' allowed)",
			state, OperationalStateActive, OperationalStateStopped)
	}
	a.baseFSMInstance.SetDesiredFSMState(state)
	return nil
}

// GetCurrentFSMState returns the current operational or lifecycle state
func (a *BenthosMonitorInstance) GetCurrentFSMState() string {
	return a.baseFSMInstance.GetCurrentFSMState()
}

// GetDesiredFSMState returns what we want operationally
func (a *BenthosMonitorInstance) GetDesiredFSMState() string {
	return a.baseFSMInstance.GetDesiredFSMState()
}

// Remove initiates the removal lifecycle
func (a *BenthosMonitorInstance) Remove(ctx context.Context) error {
	return a.baseFSMInstance.Remove(ctx)
}

func (a *BenthosMonitorInstance) IsRemoved() bool {
	return a.baseFSMInstance.IsRemoved()
}

func (a *BenthosMonitorInstance) IsRemoving() bool {
	return a.baseFSMInstance.IsRemoving()
}

func (a *BenthosMonitorInstance) IsStopped() bool {
	return a.baseFSMInstance.GetCurrentFSMState() == OperationalStateStopped
}

func (a *BenthosMonitorInstance) IsStopping() bool {
	return a.baseFSMInstance.GetCurrentFSMState() == OperationalStateStopping
}

// PrintState is a helper for debugging
func (a *BenthosMonitorInstance) PrintState() {
	a.baseFSMInstance.GetLogger().Infof("BenthosMonitorInstance %s - Current state: %s, Desired: %s",
		a.baseFSMInstance.GetID(), a.GetCurrentFSMState(), a.GetDesiredFSMState())
}

// GetExpectedMaxP95ExecutionTimePerInstance returns the expected max p95 execution time of the instance
func (a *BenthosMonitorInstance) GetExpectedMaxP95ExecutionTimePerInstance() time.Duration {
	return constants.BenthosMonitorExpectedMaxP95ExecutionTimePerInstance
}
