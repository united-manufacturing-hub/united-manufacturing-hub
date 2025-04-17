package fsm

import (
	"context"
	"errors"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/backoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/errorhandling"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/snapshot"
)

// Reconcile examines the BenthosInstance and, in three steps:
//  1. Check if a previous transition failed or if fetching external state failed; if so, verify whether the backoff has elapsed.
//  2. Detect any external changes (e.g., a new configuration or external signals).
//  3. Attempt the required state transition by sending the appropriate event.
//
// This function is intended to be called repeatedly (e.g. in a periodic control loop).
// Over multiple calls, it converges the actual state to the desired state. Transitions
// that fail are retried in subsequent reconcile calls after a backoff period.
func (b *BaseFSMInstance) Reconcile(ctx context.Context, currentSnapshot snapshot.SystemSnapshot, filesystemService filesystem.Service) (err error, reconciled bool) {
	start := time.Now()
	instanceName := b.GetID()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentBaseFSMInstance, instanceName, time.Since(start))
		if err != nil {
			b.GetLogger().Errorf("error reconciling container instance %s: %s", instanceName, err)
			b.PrintState()
			// Add metrics for error
			metrics.IncErrorCount(metrics.ComponentBaseFSMInstance, instanceName)
		}
	}()

	// Check if context is already cancelled
	if ctx.Err() != nil {
		return ctx.Err(), false
	}

	// 1) If we have a "transient error" with time left in backoff, skip.
	// TODO: Update with ForceRemovalLogic from Janik
	// TODO: Update the creation of all base fsms with a definition for IsStopping (OperationalStateBeforeBeforeRemove)
	if b.ShouldSkipReconcileBecauseOfError(currentSnapshot.Tick) {
		backErr := b.GetBackoffError(currentSnapshot.Tick)
		b.GetLogger().Debugf("Skipping reconcile for %s: %v", instanceName, backErr)

		// If backoff says it's a permanent failure, we forcibly remove the instance.
		if backoff.IsPermanentFailureError(backErr) {
			if b.IsRemoved() || b.IsRemoving() || b.IsStopping() || b.IsStopped() {
				// Already in terminal or removing state => do a forced remove
				b.GetLogger().Errorf("Instance %s is in terminal state; force removing", instanceName)
				forceErr := b.ForceRemoveInstance(ctx, filesystemService)
				if forceErr != nil {
					b.GetLogger().Errorf("Force remove also failed: %s", forceErr)
					return forceErr, false
				}
				return backErr, false
			} else {
				// Attempt to do a graceful remove
				b.GetLogger().Errorf("Instance %s not in terminal; resetting & removing", instanceName)
				b.ResetState()
				err = b.Remove(ctx)
				if err != nil {
					b.GetLogger().Errorf("remove failed; force removing: %s", err)
					forceErr := b.ForceRemoveInstance(ctx, filesystemService)
					if forceErr != nil {
						b.GetLogger().Errorf("Force remove also failed: %s", forceErr)
						return forceErr, false
					}
					return err, false
				}
				return nil, false
			}
		}
		// If it's not a permanent failure, just skip this tick
		return nil, false
	}

	// 2) Detect external changes => calls UpdateObservedStateOfInstance
	if err := b.reconcileExternalChanges(ctx, filesystemService, currentSnapshot.Tick, start); err != nil {
		catErr := backoff.CategorizeError(err) // ensure we have a known category
		switch {
		case backoff.IsIgnoredError(catErr):
			// E.g. "healthcheck refused" while we are in "starting."
			b.GetLogger().Debugf("Ignoring ephemeral error for %s: %s", instanceName, err)
			// proceed with no changes
		case backoff.IsTransientError(catErr):
			// Something that might be recoverable => we record it in backoff
			b.GetLogger().Debugf("Transient error in external changes for %s: %s", instanceName, err)
			b.SetError(catErr, currentSnapshot.Tick)
			return catErr, false

		case backoff.IsPermanentError(catErr):
			// Immediately remove the instance forcibly or do a graceful remove
			b.GetLogger().Errorf("Permanent error in external changes for %s: %s", instanceName, err)
			b.SetError(catErr, currentSnapshot.Tick)
			// Potentially we do a graceful remove:
			// but since it's "permanent," we might do forced:
			return catErr, false

		default:
			// If we somehow didn't categorize => fallback to transient
			b.SetError(err, currentSnapshot.Tick)
			return catErr, false
		}
	}

	// 3) Attempt to reconcile the state
	err, reconciled = b.reconcileStateTransition(ctx, filesystemService, currentSnapshot.SnapshotTime)
	if err != nil {
		// If the instance is removed, we don't want to return an error here, because we want to continue reconciling
		if errors.Is(err, errorhandling.ErrInstanceRemoved) {
			return nil, false
		}

		b.SetError(err, currentSnapshot.Tick)
		b.GetLogger().Errorf("error reconciling state: %s", err)
		return nil, false // We don't want to return an error here, because we want to continue reconciling
	}

	// 4) Reconcile the underlying managers
	// If any manager fails, we return an error
	// If any manager was reconciled, we set reconciled to true
	for _, manager := range b.cfg.ManagersToReconcile {
		err, managerReconciled := manager.Reconcile(ctx, currentSnapshot, filesystemService)
		if err != nil {
			b.GetLogger().Errorf("error reconciling manager %s: %s", manager, err)
			return err, false
		}
		if managerReconciled {
			reconciled = true
		}
	}

	return
}

// reconcileExternalChanges checks if the instance has changed
// externally (e.g., if someone manually stopped or started it, or if it crashed)
// It calls UpdateObservedStateOfInstance to check if the instance has changed
// and returns an error if it has.
func (b *BaseFSMInstance) reconcileExternalChanges(ctx context.Context, filesystemService filesystem.Service, tick uint64, loopStartTime time.Time) error {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentBaseFSMInstance, b.GetID()+".reconcileExternalChanges", time.Since(start))
	}()

	// Fetching the observed state can sometimes take longer, but we need to ensure when reconciling a lot of instances
	// that a single status of a single instance does not block the whole reconciliation
	observedStateCtx, cancel := context.WithTimeout(ctx, b.cfg.UpdateObservedStateTimeout)
	defer cancel()

	err := b.UpdateObservedStateOfInstance(observedStateCtx, filesystemService, tick, loopStartTime)
	if err != nil {
		// If the current state has a reason to ignore certain errors:
		currentState := b.GetCurrentFSMState()

		ignoredList := b.cfg.IgnoreErrorsInStates[currentState]
		for _, ign := range ignoredList {
			// If the "type" of error matches, we ignore.
			// (In practice, you might do reflect.TypeOf or check substrings.)
			if errors.Is(err, ign) {
				b.GetLogger().Debugf("Ignore error in state %s: %s", currentState, err)
				return backoff.NewIgnoredError(err)
			}
		}

		permanentList := b.cfg.PermanentErrorsInStates[currentState]
		for _, perm := range permanentList {
			if errors.Is(err, perm) {
				b.GetLogger().Errorf("Permanent error in state %s: %s", currentState, err)
				return backoff.NewPermanentError(err)
			}
		}

		// If not in the ignore or permanent lists => default to transient
		return backoff.NewTransientError(err)
	}
	return nil
}

// reconcileStateTransition compares the current state with the desired state
// and, if necessary, sends events to drive the FSM from the current to the desired state.
// Any functions that fetch information are disallowed here and must be called in reconcileExternalChanges
// and exist in ExternalState.
// This is to ensure full testability of the FSM.
func (b *BaseFSMInstance) reconcileStateTransition(ctx context.Context, filesystemService filesystem.Service, currentTime time.Time) (err error, reconciled bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentBaseFSMInstance, b.GetID()+".reconcileStateTransition", time.Since(start))
	}()

	currentState := b.GetCurrentFSMState()
	desiredState := b.GetDesiredFSMState()

	// Handle lifecycle states first - these take precedence over operational states
	if IsLifecycleState(currentState) {
		err, reconciled := b.reconcileLifecycleStates(ctx, filesystemService, currentState)
		if err != nil {
			return err, false
		}
		return nil, reconciled
	}

	// For everything else, we use the ReconcileOperationalStates method
	err, reconciled = b.ReconcileOperationalStates(ctx, currentState, desiredState, filesystemService, currentTime)
	if err != nil {
		return err, false
	}
	return nil, reconciled
}

// reconcileLifecycleStates handles to_be_created, creating, removing, removed
func (b *BaseFSMInstance) reconcileLifecycleStates(ctx context.Context, filesystemService filesystem.Service, currentState string) (error, bool) {
	start := time.Now()
	defer func() {
		metrics.ObserveReconcileTime(metrics.ComponentBaseFSMInstance, b.GetID()+".reconcileLifecycleStates", time.Since(start))
	}()

	switch currentState {
	case LifecycleStateToBeCreated:
		// do creation
		if err := b.CreateInstance(ctx, filesystemService); err != nil {
			return err, false
		}
		return b.SendEvent(ctx, LifecycleEventCreate), true

	case LifecycleStateCreating:
		// We can assume creation is done immediately (no real action)
		return b.SendEvent(ctx, LifecycleEventCreateDone), true

	case LifecycleStateRemoving:
		if err := b.RemoveInstance(ctx, filesystemService); err != nil {
			return err, false
		}
		return b.SendEvent(ctx, LifecycleEventRemoveDone), true

	case LifecycleStateRemoved:
		// The manager will clean this up eventually
		return errorhandling.ErrInstanceRemoved, true

	default:
		return nil, false
	}
}

func (b *BaseFSMInstance) PrintState() {
	b.GetLogger().Infof("BaseFSMInstance %s - Current state: %s, Desired: %s",
		b.GetID(), b.GetCurrentFSMState(), b.GetDesiredFSMState())
}
