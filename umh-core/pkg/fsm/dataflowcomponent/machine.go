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

	"github.com/looplab/fsm"

	internal_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
)

// NewDataFlowComponent creates a new DataFlowComponent instance with the given configuration
func NewDataFlowComponent(config DataFlowComponentConfig, benthosConfigManager BenthosConfigManager) *DataFlowComponent {
	cfg := internal_fsm.BaseFSMInstanceConfig{
		ID:                           config.Name,
		DesiredFSMState:              OperationalStateStopped,
		OperationalStateAfterCreate:  OperationalStateStopped,
		OperationalStateBeforeRemove: OperationalStateStopped,
		OperationalTransitions: []fsm.EventDesc{
			// Basic lifecycle transitions
			// Stopped is the initial state
			{Name: EventStart, Src: []string{OperationalStateStopped}, Dst: OperationalStateStarting},
			{Name: EventStartDone, Src: []string{OperationalStateStarting}, Dst: OperationalStateActive},

			// From Active, we can go to Degraded or Stopping
			{Name: EventDegraded, Src: []string{OperationalStateActive}, Dst: OperationalStateDegraded},
			{Name: EventStop, Src: []string{OperationalStateActive}, Dst: OperationalStateStopping},

			// From Degraded, we can go to Active or Stopping
			{Name: EventRecovered, Src: []string{OperationalStateDegraded}, Dst: OperationalStateActive},
			{Name: EventStop, Src: []string{OperationalStateDegraded}, Dst: OperationalStateStopping},

			// Final transition for stopping
			{Name: EventStopDone, Src: []string{OperationalStateStopping}, Dst: OperationalStateStopped},
		},
	}

	instance := &DataFlowComponent{
		baseFSMInstance:      internal_fsm.NewBaseFSMInstance(cfg, logger.For(config.Name)),
		Config:               config,
		BenthosConfigManager: benthosConfigManager,
		ObservedState:        DFCObservedState{},
	}

	instance.registerCallbacks()

	return instance
}

// SetDesiredFSMState safely updates the desired state
func (d *DataFlowComponent) SetDesiredFSMState(state string) error {
	// For DataFlowComponent, we only allow setting Stopped or Active as desired states
	if state != OperationalStateStopped &&
		state != OperationalStateActive {
		return fmt.Errorf("invalid desired state: %s. valid states are %s and %s",
			state,
			OperationalStateStopped,
			OperationalStateActive)
	}

	d.baseFSMInstance.SetDesiredFSMState(state)
	return nil
}

// Remove starts the removal process
func (d *DataFlowComponent) Remove(ctx context.Context) error {
	return d.baseFSMInstance.Remove(ctx)
}

// IsRemoved returns true if the instance has been removed
func (d *DataFlowComponent) IsRemoved() bool {
	return d.baseFSMInstance.IsRemoved()
}

// IsRemoving returns true if the instance is in the removing state
func (d *DataFlowComponent) IsRemoving() bool {
	return d.baseFSMInstance.IsRemoving()
}

// IsStopping returns true if the instance is in the stopping state
func (d *DataFlowComponent) IsStopping() bool {
	return d.baseFSMInstance.GetCurrentFSMState() == OperationalStateStopping
}

// IsStopped returns true if the instance is in the stopped state
func (d *DataFlowComponent) IsStopped() bool {
	return d.baseFSMInstance.GetCurrentFSMState() == OperationalStateStopped
}
