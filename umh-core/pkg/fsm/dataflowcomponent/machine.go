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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/sentry"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/storage"
)

// NewDataflowComponentInstance creates a new DataflowComponentInstance with a given ID and service path
func NewDataflowComponentInstance(
	s6BaseDir string,
	config config.DataFlowComponentConfig,
	archiveStorage storage.ArchiveStorer) *DataflowComponentInstance {

	cfg := internal_fsm.BaseFSMInstanceConfig{
		ID:                           config.Name,
		DesiredFSMState:              OperationalStateStopped,
		OperationalStateAfterCreate:  OperationalStateStopped,
		OperationalStateBeforeRemove: OperationalStateStopped,
		OperationalTransitions: []fsm.EventDesc{
			// Basic lifecycle transitions
			// Stopped is the initial state
			// stopped -> starting
			{Name: EventStart, Src: []string{OperationalStateStopped}, Dst: OperationalStateStarting},

			// starting -> idle
			{Name: EventStartDone, Src: []string{OperationalStateStarting}, Dst: OperationalStateIdle},

			// idle -> active
			{Name: EventBenthosDataReceived, Src: []string{OperationalStateIdle}, Dst: OperationalStateActive},

			//	active/idle -> degraded
			{Name: EventBenthosDegraded, Src: []string{OperationalStateActive, OperationalStateIdle}, Dst: OperationalStateDegraded},

			// active -> idle
			{Name: EventBenthosNoDataReceived, Src: []string{OperationalStateActive}, Dst: OperationalStateIdle},

			// degraded -> idle
			{Name: EventBenthosRecovered, Src: []string{OperationalStateDegraded}, Dst: OperationalStateIdle},

			// starting -> startingFailed
			{Name: EventStartFailed, Src: []string{OperationalStateStarting}, Dst: OperationalStateStartingFailed},

			// everywhere to stopping
			{
				Name: EventStop,
				Src: []string{
					OperationalStateStarting,
					OperationalStateStartingFailed,
					OperationalStateIdle,
					OperationalStateActive,
					OperationalStateDegraded,
				},
				Dst: OperationalStateStopping,
			},

			// stopping to stopped
			{Name: EventStopDone, Src: []string{OperationalStateStopping}, Dst: OperationalStateStopped},
		},
	}

	instance := &DataflowComponentInstance{
		baseFSMInstance: internal_fsm.NewBaseFSMInstance(cfg, logger.For(config.Name)),
		service:         dataflowcomponent.NewDefaultDataFlowComponentService(config.Name, archiveStorage),
		config:          config.DataFlowComponentConfig,
		ObservedState:   DataflowComponentObservedState{},
		archiveStorage:  archiveStorage,
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
func (d *DataflowComponentInstance) GetCurrentFSMState() string {
	return d.baseFSMInstance.GetCurrentFSMState()
}

// GetDesiredFSMState returns the desired state of the FSM
func (d *DataflowComponentInstance) GetDesiredFSMState() string {
	return d.baseFSMInstance.GetDesiredFSMState()
}

// Remove starts the removal process, it is idempotent and can be called multiple times
// Note: it is only removed once IsRemoved returns true
func (d *DataflowComponentInstance) Remove(ctx context.Context) error {
	return d.baseFSMInstance.Remove(ctx)
}

// IsRemoved returns true if the instance has been removed
func (d *DataflowComponentInstance) IsRemoved() bool {
	return d.baseFSMInstance.IsRemoved()
}

// IsRemoving returns true if the instance is in the removing state
func (d *DataflowComponentInstance) IsRemoving() bool {
	return d.baseFSMInstance.IsRemoving()
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
	return constants.DataflowComponentExpectedMaxP95ExecutionTimePerInstance
}
