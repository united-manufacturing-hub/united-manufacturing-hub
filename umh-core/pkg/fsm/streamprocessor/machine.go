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

package streamprocessor

import (
	"context"
	"fmt"
	"time"

	"github.com/looplab/fsm"
	internal_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/streamprocessorserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	spsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/streamprocessor"
)

// NewInstance creates a new StreamProcessorInstance with a given ID and service path
func NewInstance(
	s6BaseDir string,
	config config.StreamProcessorConfig,
) *Instance {
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
				Name: EventStart, Src: []string{
					OperationalStateStopped,
				}, Dst: OperationalStateStartingRedpanda,
			},

			// starting redpanda -> starting dfc
			{
				Name: EventStartRedpandaUp, Src: []string{
					OperationalStateStartingRedpanda,
				}, Dst: OperationalStateStartingDFC,
			},

			// retry starting for failed
			{
				Name: EventStartRetry, Src: []string{
					OperationalStateStartingDFC,
					OperationalStateStartingRedpanda,
					OperationalStateStartingFailedDFC,
				}, Dst: OperationalStateStartingRedpanda,
			},

			// starting -> idle
			{
				Name: EventStartDFCUp, Src: []string{
					OperationalStateStartingDFC,
				}, Dst: OperationalStateIdle,
			},

			// idle -> active
			{
				Name: EventDFCActive, Src: []string{
					OperationalStateIdle,
				}, Dst: OperationalStateActive,
			},

			// active -> idle
			{
				Name: EventDFCIdle, Src: []string{
					OperationalStateActive,
				}, Dst: OperationalStateIdle,
			},

			//	active/idle -> degraded rp/dfc/other
			{
				Name: EventRedpandaDegraded, Src: []string{
					OperationalStateActive,
					OperationalStateIdle,
				}, Dst: OperationalStateDegradedRedpanda,
			},

			{
				Name: EventDFCDegraded, Src: []string{
					OperationalStateActive,
					OperationalStateIdle,
				}, Dst: OperationalStateDegradedDFC,
			},

			{
				Name: EventDegradedOther, Src: []string{
					OperationalStateActive,
					OperationalStateIdle,
				}, Dst: OperationalStateDegradedOther,
			},

			// degraded -> idle
			{
				Name: EventRecovered, Src: []string{
					OperationalStateDegradedRedpanda,
					OperationalStateDegradedDFC,
					OperationalStateDegradedOther,
				}, Dst: OperationalStateIdle,
			},

			// starting -> startingFailed
			{
				Name: EventStartFailedDFC, Src: []string{
					OperationalStateStartingRedpanda,
					OperationalStateStartingDFC,
				}, Dst: OperationalStateStartingFailedDFC,
			},

			// everywhere to stopping
			{
				Name: EventStop,
				Src: []string{
					OperationalStateStartingRedpanda,
					OperationalStateStartingDFC,
					OperationalStateStartingFailedDFC,
					OperationalStateIdle,
					OperationalStateActive,
					OperationalStateDegradedRedpanda,
					OperationalStateDegradedDFC,
					OperationalStateDegradedOther,
				},
				Dst: OperationalStateStopping,
			},

			// stopping to stopped
			{
				Name: EventStopDone, Src: []string{
					OperationalStateStopping,
				}, Dst: OperationalStateStopped,
			},
		},
	}

	logger := logger.For(config.Name)
	backoffConfig := backoff.DefaultConfig(cfg.ID, logger)

	instance := &Instance{
		baseFSMInstance: internal_fsm.NewBaseFSMInstance(cfg, backoffConfig, logger),
		service:         spsvc.NewDefaultService(config.Name),
		specConfig:      config.StreamProcessorServiceConfig,
		ObservedState:   ObservedState{},
		runtimeConfig:   streamprocessorserviceconfig.StreamProcessorServiceConfigRuntime{},
	}

	instance.registerCallbacks()

	metrics.InitErrorCounter(metrics.ComponentStreamProcessorInstance, config.Name)

	return instance
}

// SetDesiredFSMState safely updates the desired state
// But ensures that the desired state is a valid state and that it is also a reasonable state
// e.g., nobody wants to have an instance in the "starting" state, that is just intermediate
func (i *Instance) SetDesiredFSMState(state string) error {
	if state != OperationalStateStopped &&
		state != OperationalStateActive {
		return fmt.Errorf("invalid desired state: %s. valid states are %s and %s",
			state,
			OperationalStateStopped,
			OperationalStateActive)
	}

	i.baseFSMInstance.SetDesiredFSMState(state)
	return nil
}

// GetCurrentFSMState returns the current state of the FSM
func (i *Instance) GetCurrentFSMState() string {
	return i.baseFSMInstance.GetCurrentFSMState()
}

// GetDesiredFSMState returns the desired state of the FSM
func (i *Instance) GetDesiredFSMState() string {
	return i.baseFSMInstance.GetDesiredFSMState()
}

// Remove starts the removal process, it is idempotent and can be called multiple times
// Note: it is only removed once IsRemoved returns true
func (i *Instance) Remove(ctx context.Context) error {
	return i.baseFSMInstance.Remove(ctx)
}

// IsRemoved returns true if the instance has been removed
func (i *Instance) IsRemoved() bool {
	return i.baseFSMInstance.IsRemoved()
}

// IsRemoving returns true if the instance is in the removing state
func (i *Instance) IsRemoving() bool {
	return i.baseFSMInstance.IsRemoving()
}

// IsStopping returns true if the instance is in the stopping state
func (i *Instance) IsStopping() bool {
	return i.baseFSMInstance.GetCurrentFSMState() == OperationalStateStopping
}

// IsStopped returns true if the instance is in the stopped state
func (i *Instance) IsStopped() bool {
	return i.baseFSMInstance.GetCurrentFSMState() == OperationalStateStopped
}

// PrintState prints the current state of the FSM for debugging
func (i *Instance) PrintState() {
	i.baseFSMInstance.GetLogger().Debugf("Current state: %s", i.baseFSMInstance.GetCurrentFSMState())
	i.baseFSMInstance.GetLogger().Debugf("Desired state: %s", i.baseFSMInstance.GetDesiredFSMState())
	i.baseFSMInstance.GetLogger().Debugf("Observed state: %+v", i.ObservedState)
}

// GetExpectedMaxP95ExecutionTimePerInstance returns the expected max p95 execution time of the instance
func (i *Instance) GetExpectedMaxP95ExecutionTimePerInstance() time.Duration {
	return constants.StreamProcessorExpectedMaxP95ExecutionTimePerInstance
}

// GetMinimumRequiredTime returns the minimum required time for this instance
func (i *Instance) GetMinimumRequiredTime() time.Duration {
	return constants.StreamProcessorUpdateObservedStateTimeout
}
