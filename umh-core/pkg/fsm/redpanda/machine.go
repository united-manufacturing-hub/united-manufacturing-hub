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

package redpanda

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
	redpanda_service "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda"
)

// NewRedpandaInstance creates a new RedpandaInstance with the given ID and service path
func NewRedpandaInstance(
	config config.RedpandaConfig) *RedpandaInstance {

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
			{Name: EventStartFailed, Src: []string{OperationalStateStarting}, Dst: OperationalStateStopped},
			{Name: EventStartDone, Src: []string{OperationalStateStarting}, Dst: OperationalStateIdle},
			{Name: EventStop, Src: []string{OperationalStateStarting}, Dst: OperationalStateStopping},

			// Running phase transitions
			// From Idle, we can go to Active when data is processed, to Degraded when there are issues, or to Stopping
			{Name: EventDataReceived, Src: []string{OperationalStateIdle}, Dst: OperationalStateActive},
			{Name: EventNoDataTimeout, Src: []string{OperationalStateIdle}, Dst: OperationalStateIdle},
			{Name: EventDegraded, Src: []string{OperationalStateIdle}, Dst: OperationalStateDegraded},
			{Name: EventStop, Src: []string{OperationalStateIdle}, Dst: OperationalStateStopping},

			// From Active, we can go to Idle when there's no data, to Degraded when there are issues, or to Stopping
			{Name: EventNoDataTimeout, Src: []string{OperationalStateActive}, Dst: OperationalStateIdle},
			{Name: EventDegraded, Src: []string{OperationalStateActive}, Dst: OperationalStateDegraded},
			{Name: EventStop, Src: []string{OperationalStateActive}, Dst: OperationalStateStopping},

			// From Degraded, we can recover to Idle, or to Stopping
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

	instance := &RedpandaInstance{
		baseFSMInstance:       internal_fsm.NewBaseFSMInstance(cfg, backoffConfig, logger),
		service:               redpanda_service.NewDefaultRedpandaService(config.Name),
		config:                config.RedpandaServiceConfig,
		schemaRegistry:        redpanda_service.NewSchemaRegistry(),
		PreviousObservedState: RedpandaObservedState{},
	}

	// Note: We intentionally do NOT initialize the S6 service here.
	// Service creation happens during state reconciliation via initiateRedpandaCreate.
	// This maintains separation of concerns and follows the pattern used by S6.
	// The reconcile loop will properly handle "service not found" errors.

	instance.registerCallbacks()

	metrics.InitErrorCounter(metrics.ComponentRedpandaInstance, config.Name)

	return instance
}

// SetDesiredFSMState safely updates the desired state
// But ensures that the desired state is a valid state and that it is also a reasonable state
// e.g., nobody wants to have an instance in the "starting" state, that is just intermediate
func (r *RedpandaInstance) SetDesiredFSMState(state string) error {
	// For Redpanda, we only allow setting Stopped or Active as desired states
	if state != OperationalStateStopped &&
		state != OperationalStateActive {
		return fmt.Errorf("invalid desired state: %s. valid states are %s and %s",
			state,
			OperationalStateStopped,
			OperationalStateActive)
	}

	r.baseFSMInstance.SetDesiredFSMState(state)
	return nil
}

// GetCurrentFSMState returns the current state of the FSM
func (r *RedpandaInstance) GetCurrentFSMState() string {
	return r.baseFSMInstance.GetCurrentFSMState()
}

// GetDesiredFSMState returns the desired state of the FSM
func (r *RedpandaInstance) GetDesiredFSMState() string {
	return r.baseFSMInstance.GetDesiredFSMState()
}

// Remove starts the removal process, it is idempotent and can be called multiple times
// Note: it is only removed once IsRemoved returns true
func (r *RedpandaInstance) Remove(ctx context.Context) error {
	return r.baseFSMInstance.Remove(ctx)
}

// IsRemoved returns true if the instance has been removed
func (r *RedpandaInstance) IsRemoved() bool {
	return r.baseFSMInstance.IsRemoved()
}

// IsRemoving returns true if the instance is in the removing state
func (r *RedpandaInstance) IsRemoving() bool {
	return r.baseFSMInstance.IsRemoving()
}

// IsStopping returns true if the instance is in the stopping state
func (r *RedpandaInstance) IsStopping() bool {
	return r.baseFSMInstance.GetCurrentFSMState() == OperationalStateStopping
}

// IsStopped returns true if the instance is in the stopped state
func (r *RedpandaInstance) IsStopped() bool {
	return r.baseFSMInstance.GetCurrentFSMState() == OperationalStateStopped
}

// IsRunning returns true if the instance is in any running state (active, idle, or degraded)
func (r *RedpandaInstance) IsRunning() bool {
	currentState := r.baseFSMInstance.GetCurrentFSMState()
	return currentState == OperationalStateActive ||
		currentState == OperationalStateIdle ||
		currentState == OperationalStateDegraded
}

// WantsToBeStopped returns true if the instance wants to be stopped
func (r *RedpandaInstance) WantsToBeStopped() bool {
	return r.baseFSMInstance.GetDesiredFSMState() == OperationalStateStopped
}

// PrintState prints the current state of the FSM for debugging
func (r *RedpandaInstance) PrintState() {
	r.baseFSMInstance.GetLogger().Debugf("Current state: %s", r.baseFSMInstance.GetCurrentFSMState())
	r.baseFSMInstance.GetLogger().Debugf("Desired state: %s", r.baseFSMInstance.GetDesiredFSMState())
	r.baseFSMInstance.GetLogger().Debugf("S6: %s, Status: %s",
		r.PreviousObservedState.ServiceInfo.S6FSMState,
		r.PreviousObservedState.ServiceInfo.StatusReason)
}

// TODO: Add Redpanda-specific health check methods
// Examples:
// - IsProcessingData() - Checks if Redpanda is actively processing data
// - HasWarnings() - Checks if Redpanda is reporting warnings
// - HasErrors() - Checks if Redpanda is reporting errors

// GetMinimumRequiredTime returns the minimum required time for this instance
func (r *RedpandaInstance) GetMinimumRequiredTime() time.Duration {
	return constants.RedpandaUpdateObservedStateTimeout
}
