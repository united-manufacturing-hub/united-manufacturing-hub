package benthos

import (
	"context"
	"fmt"

	"github.com/looplab/fsm"

	internal_fsm "github.com/united-manufacturing-hub/benthos-umh/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/metrics"
	benthos_service "github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/service/benthos"
)

// NewBenthosInstance creates a new BenthosInstance with the given ID and service path
func NewBenthosInstance(
	s6BaseDir string,
	config config.BenthosConfig) *BenthosInstance {

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
			{Name: EventS6Started, Src: []string{OperationalStateStarting}, Dst: OperationalStateStartingConfigLoading},
			{Name: EventConfigLoaded, Src: []string{OperationalStateStartingConfigLoading}, Dst: OperationalStateStartingWaitingForHealthchecks},
			{Name: EventHealthchecksPassed, Src: []string{OperationalStateStartingWaitingForHealthchecks}, Dst: OperationalStateStartingWaitingForServiceToRemainRunning},
			{Name: EventStartDone, Src: []string{OperationalStateStartingWaitingForServiceToRemainRunning}, Dst: OperationalStateIdle},
			{Name: EventStop, Src: []string{OperationalStateStarting, OperationalStateStartingConfigLoading, OperationalStateStartingWaitingForHealthchecks, OperationalStateStartingWaitingForServiceToRemainRunning}, Dst: OperationalStateStopping},

			// From any starting state, we can either go back to OperationalStateStarting (e.g., if there was an error)
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

	instance := &BenthosInstance{
		baseFSMInstance: internal_fsm.NewBaseFSMInstance(cfg, logger.For(config.Name)),
		service:         benthos_service.NewDefaultBenthosService(config.Name),
		config:          config.BenthosServiceConfig,
		ObservedState:   BenthosObservedState{},
	}

	// Note: We intentionally do NOT initialize the S6 service here.
	// Service creation happens during state reconciliation via initiateBenthosCreate.
	// This maintains separation of concerns and follows the pattern used by S6.
	// The reconcile loop will properly handle "service not found" errors.

	instance.registerCallbacks()

	metrics.InitErrorCounter(metrics.ComponentBenthosInstance, config.Name)

	return instance
}

// SetDesiredFSMState safely updates the desired state
// But ensures that the desired state is a valid state and that it is also a reasonable state
// e.g., nobody wants to have an instance in the "starting" state, that is just intermediate
func (b *BenthosInstance) SetDesiredFSMState(state string) error {
	// For Benthos, we only allow setting Stopped or Active as desired states
	if state != OperationalStateStopped &&
		state != OperationalStateActive {
		return fmt.Errorf("invalid desired state: %s. valid states are %s and %s",
			state,
			OperationalStateStopped,
			OperationalStateActive)
	}

	b.baseFSMInstance.SetDesiredFSMState(state)
	return nil
}

// GetCurrentFSMState returns the current state of the FSM
func (b *BenthosInstance) GetCurrentFSMState() string {
	return b.baseFSMInstance.GetCurrentFSMState()
}

// GetDesiredFSMState returns the desired state of the FSM
func (b *BenthosInstance) GetDesiredFSMState() string {
	return b.baseFSMInstance.GetDesiredFSMState()
}

// Remove starts the removal process, it is idempotent and can be called multiple times
// Note: it is only removed once IsRemoved returns true
func (b *BenthosInstance) Remove(ctx context.Context) error {
	return b.baseFSMInstance.Remove(ctx)
}

// IsRemoved returns true if the instance has been removed
func (b *BenthosInstance) IsRemoved() bool {
	return b.baseFSMInstance.IsRemoved()
}

// IsRemoving returns true if the instance is in the removing state
func (b *BenthosInstance) IsRemoving() bool {
	return b.baseFSMInstance.IsRemoving()
}

// IsStopping returns true if the instance is in the stopping state
func (b *BenthosInstance) IsStopping() bool {
	return b.baseFSMInstance.GetCurrentFSMState() == OperationalStateStopping
}

// IsStopped returns true if the instance is in the stopped state
func (b *BenthosInstance) IsStopped() bool {
	return b.baseFSMInstance.GetCurrentFSMState() == OperationalStateStopped
}

// PrintState prints the current state of the FSM for debugging
func (b *BenthosInstance) PrintState() {
	b.baseFSMInstance.GetLogger().Debugf("Current state: %s", b.baseFSMInstance.GetCurrentFSMState())
	b.baseFSMInstance.GetLogger().Debugf("Desired state: %s", b.baseFSMInstance.GetDesiredFSMState())
	b.baseFSMInstance.GetLogger().Debugf("Observed state: %+v", b.ObservedState)
}

// TODO: Add Benthos-specific health check methods
// Examples:
// - IsProcessingData() - Checks if Benthos is actively processing data
// - HasWarnings() - Checks if Benthos is reporting warnings
// - HasErrors() - Checks if Benthos is reporting errors
