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

package topicbrowser

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
	tbsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/topicbrowser"
)

// NewInstance creates a new Instance with a given ID and service path
func NewInstance(
	s6BaseDir string,
	config config.TopicBrowserConfig,
) *TopicBrowserInstance {
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

			// starting -> starting_benthos
			{
				Name: EventBenthosStarted,
				Src:  []string{OperationalStateStarting},
				Dst:  OperationalStateStartingBenthos,
			},

			// starting_benthos -> starting_redpanda
			{
				Name: EventRedpandaStarted,
				Src:  []string{OperationalStateStartingBenthos},
				Dst:  OperationalStateStartingRedpanda,
			},

			// starting_redpanda -> idle
			{
				Name: EventStartDone,
				Src:  []string{OperationalStateStartingRedpanda},
				Dst:  OperationalStateIdle,
			},

			// idle -> active (when data processing starts)
			{
				Name: EventDataReceived,
				Src:  []string{OperationalStateIdle},
				Dst:  OperationalStateActive,
			},

			// active -> idle (when no data activity)
			{
				Name: EventNoDataTimeout,
				Src:  []string{OperationalStateActive},
				Dst:  OperationalStateIdle,
			},

			// idle/active -> degraded benthos
			{
				Name: EventBenthosDegraded,
				Src: []string{
					OperationalStateIdle,
					OperationalStateActive,
				},
				Dst: OperationalStateDegradedBenthos,
			},

			// idle/active -> degraded redpanda
			{
				Name: EventRedpandaDegraded,
				Src: []string{
					OperationalStateIdle,
					OperationalStateActive,
				},
				Dst: OperationalStateDegradedRedpanda,
			},

			// starting states -> starting (restart startup on failure)
			{
				Name: EventStartupFailed,
				Src: []string{
					OperationalStateStartingBenthos,
					OperationalStateStartingRedpanda,
				},
				Dst: OperationalStateStarting,
			},

			// degraded -> idle (recovery)
			{
				Name: EventRecovered,
				Src: []string{
					OperationalStateDegradedBenthos,
					OperationalStateDegradedRedpanda,
				},
				Dst: OperationalStateIdle,
			},

			// everywhere to stopping
			{
				Name: EventStop,
				Src: []string{
					OperationalStateStarting,
					OperationalStateStartingBenthos,
					OperationalStateStartingRedpanda,
					OperationalStateIdle,
					OperationalStateActive,
					OperationalStateDegradedBenthos,
					OperationalStateDegradedRedpanda,
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

	instance := &TopicBrowserInstance{
		baseFSMInstance: internal_fsm.NewBaseFSMInstance(cfg, backoffConfig, logger),
		service:         tbsvc.NewDefaultService(config.Name),
		config:          config.TopicBrowserServiceConfig,
		ObservedState:   ObservedState{},
	}

	instance.registerCallbacks()

	metrics.InitErrorCounter(metrics.ComponentTopicBrowserInstance, config.Name)

	return instance
}

// SetDesiredFSMState safely updates the desired state
// But ensures that the desired state is a valid state and that it is also a reasonable state
// e.g., nobody wants to have an instance in the "starting" state, that is just intermediate
func (i *TopicBrowserInstance) SetDesiredFSMState(state string) error {
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
func (i *TopicBrowserInstance) GetCurrentFSMState() string {
	return i.baseFSMInstance.GetCurrentFSMState()
}

// GetDesiredFSMState returns the desired state of the FSM
func (i *TopicBrowserInstance) GetDesiredFSMState() string {
	return i.baseFSMInstance.GetDesiredFSMState()
}

// Remove starts the removal process, it is idempotent and can be called multiple times
// Note: it is only removed once IsRemoved returns true
func (i *TopicBrowserInstance) Remove(ctx context.Context) error {
	return i.baseFSMInstance.Remove(ctx)
}

// IsRemoved returns true if the instance has been removed
func (i *TopicBrowserInstance) IsRemoved() bool {
	return i.baseFSMInstance.IsRemoved()
}

// IsRemoving returns true if the instance is in the removing state
func (i *TopicBrowserInstance) IsRemoving() bool {
	return i.baseFSMInstance.IsRemoving()
}

// IsStopping returns true if the instance is in the stopping state
func (i *TopicBrowserInstance) IsStopping() bool {
	return i.baseFSMInstance.GetCurrentFSMState() == OperationalStateStopping
}

// IsStopped returns true if the instance is in the stopped state
func (i *TopicBrowserInstance) IsStopped() bool {
	return i.baseFSMInstance.GetCurrentFSMState() == OperationalStateStopped
}

// WantsToBeStopped returns true if the instance wants to be stopped
func (i *TopicBrowserInstance) WantsToBeStopped() bool {
	return i.baseFSMInstance.GetDesiredFSMState() == OperationalStateStopped
}

// PrintState prints the current state of the FSM for debugging
func (i *TopicBrowserInstance) PrintState() {
	i.baseFSMInstance.GetLogger().Debugf("Current state: %s", i.baseFSMInstance.GetCurrentFSMState())
	i.baseFSMInstance.GetLogger().Debugf("Desired state: %s", i.baseFSMInstance.GetDesiredFSMState())
	i.baseFSMInstance.GetLogger().Debugf("Observed state: %+v", i.ObservedState)
}

// GetExpectedMaxP95ExecutionTimePerInstance returns the expected max p95 execution time of the instance
func (i *TopicBrowserInstance) GetExpectedMaxP95ExecutionTimePerInstance() time.Duration {
	return constants.TopicBrowserExpectedMaxP95ExecutionTimePerInstance
}
