package benthos

import (
	"context"

	"github.com/looplab/fsm"
)

// registerCallbacks registers common callbacks for state transitions
// These callbacks are executed synchronously and should not have any network calls or other operations that could fail
func (instance *BenthosInstance) registerCallbacks() {
	// Basic operational state callbacks
	instance.baseFSMInstance.AddCallback("enter_"+OperationalStateStarting, func(ctx context.Context, e *fsm.Event) {
		instance.baseFSMInstance.GetLogger().Infof("Entering starting state for %s", instance.baseFSMInstance.GetID())
	})

	instance.baseFSMInstance.AddCallback("enter_"+OperationalStateStopping, func(ctx context.Context, e *fsm.Event) {
		instance.baseFSMInstance.GetLogger().Infof("Entering stopping state for %s", instance.baseFSMInstance.GetID())
	})

	instance.baseFSMInstance.AddCallback("enter_"+OperationalStateStopped, func(ctx context.Context, e *fsm.Event) {
		instance.baseFSMInstance.GetLogger().Infof("Entering stopped state for %s", instance.baseFSMInstance.GetID())
	})

	instance.baseFSMInstance.AddCallback("enter_"+OperationalStateActive, func(ctx context.Context, e *fsm.Event) {
		instance.baseFSMInstance.GetLogger().Infof("Entering active state for %s", instance.baseFSMInstance.GetID())
	})

	// Starting phase state callbacks
	instance.baseFSMInstance.AddCallback("enter_"+OperationalStateStartingConfigLoading, func(ctx context.Context, e *fsm.Event) {
		instance.baseFSMInstance.GetLogger().Infof("Entering config loading state for %s", instance.baseFSMInstance.GetID())
	})

	instance.baseFSMInstance.AddCallback("enter_"+OperationalStateStartingWaitingForHealthchecks, func(ctx context.Context, e *fsm.Event) {
		instance.baseFSMInstance.GetLogger().Infof("Entering waiting for healthchecks state for %s", instance.baseFSMInstance.GetID())
	})

	instance.baseFSMInstance.AddCallback("enter_"+OperationalStateStartingWaitingForServiceToRemainRunning, func(ctx context.Context, e *fsm.Event) {
		instance.baseFSMInstance.GetLogger().Infof("Entering waiting for service to remain running state for %s", instance.baseFSMInstance.GetID())
	})

	// Running phase state callbacks
	instance.baseFSMInstance.AddCallback("enter_"+OperationalStateIdle, func(ctx context.Context, e *fsm.Event) {
		instance.baseFSMInstance.GetLogger().Infof("Entering idle state for %s", instance.baseFSMInstance.GetID())
	})

	instance.baseFSMInstance.AddCallback("enter_"+OperationalStateDegraded, func(ctx context.Context, e *fsm.Event) {
		instance.baseFSMInstance.GetLogger().Warnf("Entering degraded state for %s", instance.baseFSMInstance.GetID())
		// Additional logic for handling degraded state could be added here
	})
}
