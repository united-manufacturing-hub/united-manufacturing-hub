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

package nmap

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
	nmap_service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/nmap"
)

// NewNmapInstance creates a new NmapInstance with the standard transitions.
func NewNmapInstance(config config.NmapConfig) *NmapInstance {
	return NewNmapInstanceWithService(config, nmap_service.NewDefaultNmapService(config.Name))
}

// NewNmapInstanceWithService creates a new NmapInstance with a custom monitor service.
func NewNmapInstanceWithService(config config.NmapConfig, service nmap_service.INmapService) *NmapInstance {
	// Build the config for the base FSM
	fsmCfg := internal_fsm.BaseFSMInstanceConfig{
		ID: config.Name,
		// The user can set "open/closed/filtered/degraded" or "stopped" as desired states
		// but it's supposed to be "open" since you want an opened port here.
		DesiredFSMState:              config.DesiredFSMState,  // should be "open"
		OperationalStateAfterCreate:  OperationalStateStopped, // upon creation, start in stopped
		OperationalStateBeforeRemove: OperationalStateStopped, // must be stopped before removal
		OperationalTransitions: []fsm.EventDesc{
			// from stopped -> starting when start monitoring
			{
				Name: EventStart,
				Src:  []string{OperationalStateStopped},
				Dst:  OperationalStateStarting,
			},

			// from starting -> degraded when start monitoring
			// Degraded is the state from where the monitoring is supposed to be running
			// and shift into open/filtered/closed whenever no execution error is faced.
			{
				Name: EventStartDone,
				Src:  []string{OperationalStateStarting},
				Dst:  OperationalStateDegraded,
			},

			// from active (open/filtered/closed/degraded) ->  degraded
			// This state transition happens, whenever monitoring is supposed to run
			// but failes due to e.g. Nmap-error
			{
				Name: EventNmapExecutionFailed,
				Src: []string{
					OperationalStateOpen,
					OperationalStateFiltered,
					OperationalStateClosed,
					OperationalStateUnfiltered,
					OperationalStateOpenFiltered,
					OperationalStateClosedFiltered,
				},
				Dst: OperationalStateDegraded,
			},

			// from filtered/closed -> open
			{
				Name: EventPortOpen,
				Src: []string{
					OperationalStateFiltered,
					OperationalStateClosed,
					OperationalStateDegraded,
					OperationalStateUnfiltered,
					OperationalStateOpenFiltered,
					OperationalStateClosedFiltered,
				},
				Dst: OperationalStateOpen,
			},

			// from open/closed -> filtered
			{
				Name: EventPortFiltered,
				Src: []string{
					OperationalStateOpen,
					OperationalStateClosed,
					OperationalStateDegraded,
					OperationalStateUnfiltered,
					OperationalStateOpenFiltered,
					OperationalStateClosedFiltered,
				},
				Dst: OperationalStateFiltered,
			},

			// from open/filtered -> closed
			{
				Name: EventPortClosed,
				Src: []string{
					OperationalStateOpen,
					OperationalStateFiltered,
					OperationalStateDegraded,
					OperationalStateUnfiltered,
					OperationalStateOpenFiltered,
					OperationalStateClosedFiltered,
				},
				Dst: OperationalStateClosed,
			},

			// from open/closed/degraded/open_filtered/closed_filtered -> unfiltered
			{
				Name: EventPortUnfiltered,
				Src: []string{
					OperationalStateOpen,
					OperationalStateClosed,
					OperationalStateDegraded,
					OperationalStateFiltered,
					OperationalStateOpenFiltered,
					OperationalStateClosedFiltered,
				},
				Dst: OperationalStateUnfiltered,
			},

			// from open/closed/degraded/unfiltered/closed_filtered -> open_filtered
			{
				Name: EventPortOpenFiltered,
				Src: []string{
					OperationalStateOpen,
					OperationalStateClosed,
					OperationalStateDegraded,
					OperationalStateFiltered,
					OperationalStateUnfiltered,
					OperationalStateClosedFiltered,
				},
				Dst: OperationalStateOpenFiltered,
			},

			// from open/closed/degraded/unfiltered/open_filtered -> closed_filtered
			{
				Name: EventPortClosedFiltered,
				Src: []string{
					OperationalStateOpen,
					OperationalStateClosed,
					OperationalStateDegraded,
					OperationalStateFiltered,
					OperationalStateUnfiltered,
					OperationalStateOpenFiltered,
				},
				Dst: OperationalStateClosedFiltered,
			},

			// from all states -> stopped
			{
				Name: EventStop,
				Src: []string{
					OperationalStateOpen,
					OperationalStateFiltered,
					OperationalStateClosed,
					OperationalStateDegraded,
					OperationalStateStarting,
					OperationalStateUnfiltered,
					OperationalStateOpenFiltered,
					OperationalStateClosedFiltered,
				},
				Dst: OperationalStateStopping,
			},

			// from stopping -> stopped
			{
				Name: EventStopDone,
				Src: []string{
					OperationalStateStopping,
				},
				Dst: OperationalStateStopped,
			},
		},
	}

	// Construct the base instance
	baseFSM := internal_fsm.NewBaseFSMInstance(fsmCfg, logger.For(config.Name))

	// Create our instance
	instance := &NmapInstance{
		baseFSMInstance: baseFSM,
		monitorService:  service,
		config:          config,
	}

	// Register any state-entry callbacks
	instance.registerCallbacks()

	// Initialize error counter
	metrics.InitErrorCounter(metrics.ComponentNmapInstance, config.Name)

	return instance
}

// SetDesiredFSMState safely updates the desired state
// But ensures that the desired state is a valid state and that it is also a reasonable state
// e.g., nobody wants to have an instance in the "starting" state, that is just intermediate
func (n *NmapInstance) SetDesiredFSMState(state string) error {
	if state != OperationalStateOpen &&
		state != OperationalStateStopped {
		return fmt.Errorf("invalid desired state: %s (only '%s' or '%s' allowed)",
			state, OperationalStateOpen, OperationalStateStopped)
	}
	n.baseFSMInstance.SetDesiredFSMState(state)
	return nil
}

// GetCurrentFSMState returns the current operational or lifecycle state
func (n *NmapInstance) GetCurrentFSMState() string {
	return n.baseFSMInstance.GetCurrentFSMState()
}

// GetDesiredFSMState returns what we want operationally
func (n *NmapInstance) GetDesiredFSMState() string {
	return n.baseFSMInstance.GetDesiredFSMState()
}

// Remove initiates the removal lifecycle
func (n *NmapInstance) Remove(ctx context.Context) error {
	return n.baseFSMInstance.Remove(ctx)
}

func (n *NmapInstance) IsRemoved() bool {
	return n.baseFSMInstance.IsRemoved()
}

func (n *NmapInstance) IsRemoving() bool {
	return n.baseFSMInstance.IsRemoving()
}

func (n *NmapInstance) IsStopped() bool {
	return n.baseFSMInstance.GetCurrentFSMState() == OperationalStateStopped
}

func (n *NmapInstance) IsStopping() bool {
	return n.baseFSMInstance.GetCurrentFSMState() == OperationalStateStopping
}

// PrintState is a helper for debugging
func (n *NmapInstance) PrintState() {
	n.baseFSMInstance.GetLogger().Infof("NmapInstance %s - Current state: %s, Desired: %s",
		n.baseFSMInstance.GetID(), n.GetCurrentFSMState(), n.GetDesiredFSMState())
}

// GetExpectedMaxP95ExecutionTimePerInstance returns the expected max p95 execution time of the instance
func (n *NmapInstance) GetExpectedMaxP95ExecutionTimePerInstance() time.Duration {
	return constants.NmapExpectedMaxP95ExecutionTimePerInstance
}
