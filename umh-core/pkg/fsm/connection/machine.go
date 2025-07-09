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

package connection

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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/connection"
)

// NewConnectionInstance creates a new ConnectionInstance with a given ID and service path
func NewConnectionInstance(
	s6BaseDir string,
	config config.ConnectionConfig) *ConnectionInstance {

	cfg := internal_fsm.BaseFSMInstanceConfig{
		ID:                           config.Name,
		DesiredFSMState:              OperationalStateStopped,
		OperationalStateAfterCreate:  OperationalStateStopped,
		OperationalStateBeforeRemove: OperationalStateStopped,
		OperationalTransitions: []fsm.EventDesc{
			// Basic lifecycle transitions
			// Stopped is the initial state
			// stopped -> starting
			{
				Name: EventStart,
				Src:  []string{OperationalStateStopped},
				Dst:  OperationalStateStarting,
			},

			// starting -> up
			{
				Name: EventStartDone,
				Src:  []string{OperationalStateStarting},
				Dst:  OperationalStateUp,
			},

			// up/degraded -> down
			{
				Name: EventProbeDown,
				Src: []string{
					OperationalStateUp,
					OperationalStateDegraded,
				},
				Dst: OperationalStateDown,
			},

			//	up/down -> degraded
			{
				Name: EventProbeFlaky,
				Src: []string{
					OperationalStateUp,
					OperationalStateDown,
				},
				Dst: OperationalStateDegraded,
			},

			// down/degraded -> up
			{
				Name: EventProbeUp,
				Src: []string{
					OperationalStateDown,
					OperationalStateDegraded,
				},
				Dst: OperationalStateUp,
			},

			// everywhere to stopping
			{
				Name: EventStop,
				Src: []string{
					OperationalStateStarting,
					OperationalStateDown,
					OperationalStateUp,
					OperationalStateDegraded,
				},
				Dst: OperationalStateStopping,
			},

			// stopping to stopped
			{
				Name: EventStopDone,
				Src:  []string{OperationalStateStopping},
				Dst:  OperationalStateStopped,
			},
		},
	}

	logger := logger.For(config.Name)
	backoffConfig := backoff.DefaultConfig(cfg.ID, logger)

	instance := &ConnectionInstance{
		baseFSMInstance: internal_fsm.NewBaseFSMInstance(cfg, backoffConfig, logger),
		service:         connection.NewDefaultConnectionService(config.Name),
		config:          config.ConnectionServiceConfig,
		ObservedState:   ConnectionObservedState{},
	}

	instance.registerCallbacks()

	metrics.InitErrorCounter(metrics.ComponentConnectionInstance, config.Name)

	return instance
}

// SetDesiredFSMState safely updates the desired state
// But ensures that the desired state is a valid state and that it is also a reasonable state
// e.g., nobody wants to have an instance in the "starting" state, that is just intermediate
func (c *ConnectionInstance) SetDesiredFSMState(state string) error {
	if state != OperationalStateStopped &&
		state != OperationalStateUp {
		return fmt.Errorf("invalid desired state: %s. valid states are %s and %s",
			state,
			OperationalStateStopped,
			OperationalStateUp)
	}

	c.baseFSMInstance.SetDesiredFSMState(state)
	return nil
}

// GetCurrentFSMState returns the current state of the FSM
func (c *ConnectionInstance) GetCurrentFSMState() string {
	return c.baseFSMInstance.GetCurrentFSMState()
}

// GetDesiredFSMState returns the desired state of the FSM
func (c *ConnectionInstance) GetDesiredFSMState() string {
	return c.baseFSMInstance.GetDesiredFSMState()
}

// Remove starts the removal process, it is idempotent and can be called multiple times
// Note: it is only removed once IsRemoved returns true
func (c *ConnectionInstance) Remove(ctx context.Context) error {
	return c.baseFSMInstance.Remove(ctx)
}

// IsRemoved returns true if the instance has been removed
func (c *ConnectionInstance) IsRemoved() bool {
	return c.baseFSMInstance.IsRemoved()
}

// IsRemoving returns true if the instance is in the removing state
func (c *ConnectionInstance) IsRemoving() bool {
	return c.baseFSMInstance.IsRemoving()
}

// IsStopping returns true if the instance is in the stopping state
func (c *ConnectionInstance) IsStopping() bool {
	return c.baseFSMInstance.GetCurrentFSMState() == OperationalStateStopping
}

// IsStopped returns true if the instance is in the stopped state
func (c *ConnectionInstance) IsStopped() bool {
	return c.baseFSMInstance.GetCurrentFSMState() == OperationalStateStopped
}

// PrintState prints the current state of the FSM for debugging
func (c *ConnectionInstance) PrintState() {
	c.baseFSMInstance.GetLogger().Debugf("Current state: %s", c.baseFSMInstance.GetCurrentFSMState())
	c.baseFSMInstance.GetLogger().Debugf("Desired state: %s", c.baseFSMInstance.GetDesiredFSMState())
	c.baseFSMInstance.GetLogger().Debugf("Observed state: %+v", c.ObservedState)
}

// GetExpectedMaxP95ExecutionTimePerInstance returns the minimum required time for this instance
func (c *ConnectionInstance) GetExpectedMaxP95ExecutionTimePerInstance() time.Duration {
	return constants.ConnectionUpdateObservedStateTimeout
}
