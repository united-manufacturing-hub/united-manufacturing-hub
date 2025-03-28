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

package core

import (
	"context"

	"github.com/looplab/fsm"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
)

// NewCoreInstance creates a new CoreInstance with the given ID
func NewCoreInstance(id string, componentName string) *CoreInstance {
	cfg := internalfsm.BaseFSMInstanceConfig{
		ID:                           id,
		DesiredFSMState:              OperationalStateMonitoringStopped,
		OperationalStateAfterCreate:  OperationalStateMonitoringStopped,
		OperationalStateBeforeRemove: OperationalStateMonitoringStopped,
		OperationalTransitions: []fsm.EventDesc{
			// Lifecycle transitions (based on the diagram)
			{Name: EventCreate, Src: []string{LifecycleStateCreate}, Dst: OperationalStateMonitoringStopped},

			// Operational state transitions (based on the diagram)
			// From monitoring_stopped, we can start monitoring and go to active
			{Name: EventStart, Src: []string{OperationalStateMonitoringStopped}, Dst: OperationalStateActive},

			// From active, we can go to degraded or stop monitoring
			{Name: EventDegraded, Src: []string{OperationalStateActive}, Dst: OperationalStateDegraded},
			{Name: EventStop, Src: []string{OperationalStateActive}, Dst: OperationalStateMonitoringStopped},

			// From degraded, we can recover to active or stop monitoring
			{Name: EventStart, Src: []string{OperationalStateDegraded}, Dst: OperationalStateActive},
			{Name: EventStop, Src: []string{OperationalStateDegraded}, Dst: OperationalStateMonitoringStopped},

			// From monitoring_stopped, we can remove the instance
			{Name: EventRemove, Src: []string{OperationalStateMonitoringStopped}, Dst: LifecycleStateRemove},
		},
	}

	instance := &CoreInstance{
		baseFSMInstance: internalfsm.NewBaseFSMInstance(cfg, logger.For(id)),
		componentName:   componentName,
		componentID:     id,
		ObservedState: CoreObservedState{
			IsMonitoring: false,
			ErrorCount:   0,
		},
	}

	instance.registerCallbacks()

	metrics.InitErrorCounter(metrics.ComponentCoreInstance, id)

	return instance
}

// registerCallbacks registers callback functions for FSM state transitions
func (c *CoreInstance) registerCallbacks() {
	// Register callbacks for state transitions
	c.baseFSMInstance.AddCallback("enter_"+OperationalStateMonitoringStopped, func(ctx context.Context, e *fsm.Event) {
		c.baseFSMInstance.GetLogger().Infof("Entering monitoring stopped state for %s", c.baseFSMInstance.GetID())
		c.ObservedState.IsMonitoring = false
	})

	c.baseFSMInstance.AddCallback("enter_"+OperationalStateActive, func(ctx context.Context, e *fsm.Event) {
		c.baseFSMInstance.GetLogger().Infof("Entering active state for %s", c.baseFSMInstance.GetID())
		c.ObservedState.IsMonitoring = true
	})

	c.baseFSMInstance.AddCallback("enter_"+OperationalStateDegraded, func(ctx context.Context, e *fsm.Event) {
		c.baseFSMInstance.GetLogger().Warnf("Entering degraded state for %s", c.baseFSMInstance.GetID())
		c.ObservedState.IsMonitoring = true
	})

	c.baseFSMInstance.AddCallback("enter_"+LifecycleStateCreate, func(ctx context.Context, e *fsm.Event) {
		c.baseFSMInstance.GetLogger().Infof("Creating core instance %s", c.baseFSMInstance.GetID())
	})

	c.baseFSMInstance.AddCallback("enter_"+LifecycleStateRemove, func(ctx context.Context, e *fsm.Event) {
		c.baseFSMInstance.GetLogger().Infof("Removing core instance %s", c.baseFSMInstance.GetID())
	})
}
