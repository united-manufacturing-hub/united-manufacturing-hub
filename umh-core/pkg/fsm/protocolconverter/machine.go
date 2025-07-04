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

package protocolconverter

import (
	"context"
	"fmt"
	"time"

	"github.com/looplab/fsm"
	internal_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	protocolconverterconfig "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	protocolconvertersvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/protocolconverter"
)

// NewProtocolConverterInstance creates a new ProtocolConverterInstance with a given ID and service path
func NewProtocolConverterInstance(
	s6BaseDir string,
	config config.ProtocolConverterConfig,
) *ProtocolConverterInstance {

	var degradedStates = []string{
		OperationalStateDegradedConnection,
		OperationalStateDegradedRedpanda,
		OperationalStateDegradedDFC,
		OperationalStateDegradedOther,
	}

	var runningStates = []string{
		OperationalStateActive,
		OperationalStateIdle,
	}
	runningStates = append(runningStates, degradedStates...)

	var startingStates = []string{
		OperationalStateStartingConnection,
		OperationalStateStartingRedpanda,
		OperationalStateStartingDFC,
	}

	var startingStatesWithFailed = append(startingStates, append(
		[]string{OperationalStateStartingFailedDFC},
		[]string{OperationalStateStartingFailedDFCMissing}...,
	)...)

	var startingAndRunningStates = append(startingStatesWithFailed, runningStates...)

	cfg := internal_fsm.BaseFSMInstanceConfig{
		ID:                           config.Name,
		DesiredFSMState:              OperationalStateStopped,
		OperationalStateAfterCreate:  OperationalStateStopped,
		OperationalStateBeforeRemove: OperationalStateStopped,
		OperationalTransitions: []fsm.EventDesc{
			// Basic lifecycle transitions
			// Stopped is the initial state
			// stopped -> starting
			{Name: EventStart, Src: []string{OperationalStateStopped}, Dst: OperationalStateStartingConnection},
			{Name: EventStartConnectionUp, Src: []string{OperationalStateStartingConnection}, Dst: OperationalStateStartingRedpanda},
			{Name: EventStartRedpandaUp, Src: []string{OperationalStateStartingRedpanda}, Dst: OperationalStateStartingDFC},

			// EventStartRetry allows to reset the starting phase, e.g., if an intermediary check failed
			// So it will cause the start to happen again
			// In benthos, we simply call a EventStartFailed, and then let the system retry
			// But the protocol converter, an EventStartFailed means an unrecoverable error requiring human intervention
			// So this is why there is a separate EventStartRetry
			{Name: EventStartRetry, Src: startingStatesWithFailed, Dst: OperationalStateStartingConnection},

			// starting -> idle
			{Name: EventStartDFCUp, Src: []string{OperationalStateStartingDFC}, Dst: OperationalStateIdle},

			// idle -> active
			{Name: EventDFCActive, Src: []string{OperationalStateIdle}, Dst: OperationalStateActive},

			//	active/idle -> degraded
			{Name: EventConnectionUnhealthy, Src: runningStates, Dst: OperationalStateDegradedConnection},
			{Name: EventRedpandaDegraded, Src: runningStates, Dst: OperationalStateDegradedRedpanda},
			{Name: EventDFCDegraded, Src: runningStates, Dst: OperationalStateDegradedDFC},
			{Name: EventDegradedOther, Src: runningStates, Dst: OperationalStateDegradedOther},

			// active -> idle
			{Name: EventDFCIdle, Src: []string{OperationalStateActive}, Dst: OperationalStateIdle},

			// degraded -> idle
			{Name: EventRecovered, Src: degradedStates, Dst: OperationalStateIdle},

			// starting -> startingFailed
			{Name: EventStartFailedDFCMissing, Src: startingStates, Dst: OperationalStateStartingFailedDFCMissing},
			{Name: EventStartFailedDFC, Src: startingStates, Dst: OperationalStateStartingFailedDFC},

			// everywhere to stopping
			{
				Name: EventStop,
				Src:  startingAndRunningStates,
				Dst:  OperationalStateStopping,
			},

			// stopping to stopped
			{Name: EventStopDone, Src: []string{OperationalStateStopping}, Dst: OperationalStateStopped},
		},
	}

	logger := logger.For(config.Name)
	backoffConfig := backoff.DefaultConfig(cfg.ID, logger)

	instance := &ProtocolConverterInstance{
		baseFSMInstance: internal_fsm.NewBaseFSMInstance(cfg, backoffConfig, logger),
		service:         protocolconvertersvc.NewDefaultProtocolConverterService(config.Name),
		specConfig:      config.ProtocolConverterServiceConfig,
		ObservedState:   ProtocolConverterObservedState{},
		runtimeConfig:   protocolconverterconfig.ProtocolConverterServiceConfigRuntime{},
	}

	instance.registerCallbacks()

	metrics.InitErrorCounter(metrics.ComponentProtocolConverterInstance, config.Name)

	return instance
}

// SetDesiredFSMState safely updates the desired state
// But ensures that the desired state is a valid state and that it is also a reasonable state
// e.g., nobody wants to have an instance in the "starting" state, that is just intermediate
func (d *ProtocolConverterInstance) SetDesiredFSMState(state string) error {
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

// GetCurrentFSMState returns the current state of the FSM
func (d *ProtocolConverterInstance) GetCurrentFSMState() string {
	return d.baseFSMInstance.GetCurrentFSMState()
}

// GetDesiredFSMState returns the desired state of the FSM
func (d *ProtocolConverterInstance) GetDesiredFSMState() string {
	return d.baseFSMInstance.GetDesiredFSMState()
}

// Remove starts the removal process, it is idempotent and can be called multiple times
// Note: it is only removed once IsRemoved returns true
func (d *ProtocolConverterInstance) Remove(ctx context.Context) error {
	return d.baseFSMInstance.Remove(ctx)
}

// IsRemoved returns true if the instance has been removed
func (d *ProtocolConverterInstance) IsRemoved() bool {
	return d.baseFSMInstance.IsRemoved()
}

// IsRemoving returns true if the instance is in the removing state
func (d *ProtocolConverterInstance) IsRemoving() bool {
	return d.baseFSMInstance.IsRemoving()
}

// IsStopping returns true if the instance is in the stopping state
func (d *ProtocolConverterInstance) IsStopping() bool {
	return d.baseFSMInstance.GetCurrentFSMState() == OperationalStateStopping
}

// IsStopped returns true if the instance is in the stopped state
func (d *ProtocolConverterInstance) IsStopped() bool {
	return d.baseFSMInstance.GetCurrentFSMState() == OperationalStateStopped
}

// PrintState prints the current state of the FSM for debugging
func (d *ProtocolConverterInstance) PrintState() {
	d.baseFSMInstance.GetLogger().Debugf("Current state: %s", d.baseFSMInstance.GetCurrentFSMState())
	d.baseFSMInstance.GetLogger().Debugf("Desired state: %s", d.baseFSMInstance.GetDesiredFSMState())
	d.baseFSMInstance.GetLogger().Debugf("Connection: %s, Read DFC: %s, Write DFC: %s, Status: %s",
		d.ObservedState.ServiceInfo.ConnectionFSMState,
		d.ObservedState.ServiceInfo.DataflowComponentReadFSMState,
		d.ObservedState.ServiceInfo.DataflowComponentWriteFSMState,
		d.ObservedState.ServiceInfo.StatusReason)
}

// GetExpectedMaxP95ExecutionTimePerInstance returns the expected max p95 execution time of the instance
func (d *ProtocolConverterInstance) GetExpectedMaxP95ExecutionTimePerInstance() time.Duration {
	return constants.ProtocolConverterExpectedMaxP95ExecutionTimePerInstance
}
