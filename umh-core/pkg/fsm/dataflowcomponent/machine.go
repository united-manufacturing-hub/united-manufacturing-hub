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
	"fmt"
	"time"

	"github.com/looplab/fsm"
	internal_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
)

// NewDataflowComponentInstance creates a new DataflowComponentInstance with a given ID and service path
func NewDataflowComponentInstance(
	s6BaseDir string,
	config config.DataFlowComponentConfig,
) *DataflowComponentInstance {

	cfg := internal_fsm.BaseFSMInstanceConfig{
		ID:                           config.Name,
		DesiredFSMState:              OperationalStateStopped,
		OperationalStateAfterCreate:  OperationalStateStopped,
		OperationalStateBeforeRemove: OperationalStateStopped,
		OperationalTransitions: []fsm.EventDesc{
			// Basic lifecycle transitions
			// Stopped is the initial state
			{Name: EventStart, Src: []string{OperationalStateStopped}, Dst: OperationalStateStarting},

			// Starting phase transitions
			{Name: EventBenthosCreated, Src: []string{OperationalStateStarting}, Dst: OperationalStateStartingConfigLoading},
			{Name: EventBenthosConfigLoaded, Src: []string{OperationalStateStartingConfigLoading}, Dst: OperationalStateStartingConfigLoading},
			{Name: EventHealthchecksPassed, Src: []string{OperationalStateStartingWaitingForHealthchecks}, Dst: OperationalStateStartingWaitingForServiceToRemainRunning},
			{Name: EventStartDone, Src: []string{OperationalStateStartingWaitingForServiceToRemainRunning}, Dst: OperationalStateIdle},
			{Name: EventStop, Src: []string{OperationalStateStarting, OperationalStateStartingConfigLoading, OperationalStateStartingWaitingForHealthchecks, OperationalStateStartingWaitingForServiceToRemainRunning}, Dst: OperationalStateStopping},

			// From any starting state, we can either go back to OperationalStateStarting (e.g: If there is an error)
			{Name: EventStartFailed, Src: []string{OperationalStateStarting, OperationalStateStartingConfigLoading, OperationalStateStartingWaitingForHealthchecks, OperationalStateStartingWaitingForServiceToRemainRunning}, Dst: OperationalStateStarting},

			// Running phase transitions
			// From Idle, we can go to Active when data is processed or to Stopping
			{Name: EventDataReceived, Src: []string{OperationalStateIdle}, Dst: OperationalStateActive},
			{Name: EventNoDataTimeout, Src: []string{OperationalStateIdle}, Dst: OperationalStateIdle},
			{Name: EventStop, Src: []string{OperationalStateIdle}, Dst: OperationalStateStopping},

			// From Active, we can go to Idle when there's no data, to Degraded when there are issues, or to Stopping
			{Name: EventNoDataTimeout, Src: []string{OperationalStateActive}, Dst: OperationalStateIdle},
			{Name: EventDegraded, Src: []string{OperationalStateActive}, Dst: OperationalStateDegraded},
			{Name: EventStop, Src: []string{OperationalStateActive}, Dst: OperationalStateStopping},

			// From Degraded, we can recover to Active, go to Idle, or to Stopping
			{Name: EventRecovered, Src: []string{OperationalStateDegraded}, Dst: OperationalStateIdle},
			{Name: EventStop, Src: []string{OperationalStateDegraded}, Dst: OperationalStateStopping},

			// Final transition for stopping
			{Name: EventStopDone, Src: []string{OperationalStateStopping}, Dst: OperationalStateStopped},

			// Add degraded transition from Idle
			{Name: EventDegraded, Src: []string{OperationalStateIdle}, Dst: OperationalStateDegraded},
		},
	}

	logger := logger.For(config.Name)
	backoffConfig := backoff.DefaultConfig(cfg.ID, logger)

	instance := &DataflowComponentInstance{
		baseFSMInstance: internal_fsm.NewBaseFSMInstance(cfg, backoffConfig, logger),
		service:         dataflowcomponent.NewDefaultDataFlowComponentService(config.Name),
		config:          config.DataFlowComponentConfig,
		ObservedState:   DataflowComponentObservedState{},
	}

	instance.registerCallbacks()
	metrics.InitErrorCounter(metrics.ComponentDataflowComponentInstance, config.Name)

	return instance
}

// SetDesiredFSMState safely updates the desired state
// But ensures that the desired state is a valid state and that it is also a reasonable state
// e.g., nobody wants to have an instance in the "starting" state, that is just intermediate
func (d *DataflowComponentInstance) SetDesiredFSMState(state string) error {
	sentry.ReportFSMFatal(d.baseFSMInstance.GetLogger(), d.baseFSMInstance.GetID(), "DataflowComponentInstance", "SetDesiredFSMState", fmt.Errorf("not implemented"))
	panic("not implemented")
}

// GetCurrentFSMState returns the current state of the FSM
func (b *DataflowComponentInstance) GetCurrentFSMState() string {
	return b.baseFSMInstance.GetCurrentFSMState()
}

// GetDesiredFSMState returns the desired state of the FSM
func (b *DataflowComponentInstance) GetDesiredFSMState() string {
	return b.baseFSMInstance.GetDesiredFSMState()
}

// Remove starts the removal process, it is idempotent and can be called multiple times
// Note: it is only removed once IsRemoved returns true
func (b *DataflowComponentInstance) Remove(ctx context.Context) error {
	return b.baseFSMInstance.Remove(ctx)
}

// IsRemoved returns true if the instance has been removed
func (b *DataflowComponentInstance) IsRemoved() bool {
	return b.baseFSMInstance.IsRemoved()
}

// IsRemoving returns true if the instance is in the removing state
func (b *DataflowComponentInstance) IsRemoving() bool {
	return b.baseFSMInstance.IsRemoving()
}

// IsStopping returns true if the instance is in the stopping state
func (b *DataflowComponentInstance) IsStopping() bool {
	sentry.ReportFSMFatal(b.baseFSMInstance.GetLogger(), b.baseFSMInstance.GetID(), "DataflowComponentInstance", "IsStopping", fmt.Errorf("not implemented"))
	panic("not implemented")
}

// IsStopped returns true if the instance is in the stopped state
func (b *DataflowComponentInstance) IsStopped() bool {
	sentry.ReportFSMFatal(b.baseFSMInstance.GetLogger(), b.baseFSMInstance.GetID(), "DataflowComponentInstance", "IsStopped", fmt.Errorf("not implemented"))
	panic("not implemented")
}

// PrintState prints the current state of the FSM for debugging
func (b *DataflowComponentInstance) PrintState() {
	b.baseFSMInstance.GetLogger().Debugf("Current state: %s", b.baseFSMInstance.GetCurrentFSMState())
	b.baseFSMInstance.GetLogger().Debugf("Desired state: %s", b.baseFSMInstance.GetDesiredFSMState())
	b.baseFSMInstance.GetLogger().Debugf("Observed state: %+v", b.ObservedState)
}

// GetExpectedMaxP95ExecutionTimePerInstance returns the expected max p95 execution time of the instance
func (b *DataflowComponentInstance) GetExpectedMaxP95ExecutionTimePerInstance() time.Duration {
	return constants.BenthosExpectedMaxP95ExecutionTimePerInstance
}
