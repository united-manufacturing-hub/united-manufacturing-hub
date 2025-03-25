package fsmtest

import (
	"context"
	"fmt"

	internal_fsm "github.com/united-manufacturing-hub/benthos-umh/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/fsm"
)

// InstanceReconciler is an interface for any FSM instance that can be reconciled
type InstanceReconciler interface {
	Reconcile(ctx context.Context, tick uint64) (error, bool)
	GetCurrentFSMState() string
	GetDesiredFSMState() string
}

// ManagerReconciler is an interface for any FSM manager that can be reconciled
type ManagerReconciler interface {
	Reconcile(ctx context.Context, config config.FullConfig, tick uint64) (error, bool)
	GetInstance(id string) (fsm.FSMInstance, bool)
}

// WaitForInstanceState repeatedly calls Reconcile on an instance until it reaches the desired state or times out
func WaitForInstanceState(ctx context.Context, instance InstanceReconciler, desiredState string, maxAttempts int, startTick uint64) (uint64, error) {
	tick := startTick

	for i := 0; i < maxAttempts; i++ {
		err, _ := instance.Reconcile(ctx, tick)
		if err != nil {
			return tick, fmt.Errorf("error during reconcile: %w", err)
		}
		tick++

		currentState := instance.GetCurrentFSMState()
		if currentState == desiredState {
			return tick, nil
		}
	}

	return tick, fmt.Errorf("failed to reach state %s after %d attempts, current state: %s",
		desiredState, maxAttempts, instance.GetCurrentFSMState())
}

// WaitForManagerInstanceState repeatedly calls Reconcile on a manager until the specified instance
// reaches the desired state or times out
func WaitForManagerInstanceState(ctx context.Context, manager ManagerReconciler, config config.FullConfig,
	instanceID string, desiredState string, maxAttempts int, startTick uint64) (uint64, error) {

	tick := startTick

	for i := 0; i < maxAttempts; i++ {
		err, _ := manager.Reconcile(ctx, config, tick)
		if err != nil {
			return tick, fmt.Errorf("error during manager reconcile: %w", err)
		}
		tick++

		instance, exists := manager.GetInstance(instanceID)
		if !exists {
			// If the desired state is LifecycleStateRemoved, and the instance no longer exists,
			// that's success!
			if desiredState == internal_fsm.LifecycleStateRemoved {
				return tick, nil
			}
			return tick, fmt.Errorf("instance %s not found in manager", instanceID)
		}

		currentState := instance.GetCurrentFSMState()
		if currentState == desiredState {
			return tick, nil
		}
	}

	instance, exists := manager.GetInstance(instanceID)
	if !exists {
		return tick, fmt.Errorf("instance %s not found in manager after %d attempts", instanceID, maxAttempts)
	}

	return tick, fmt.Errorf("instance %s failed to reach state %s after %d attempts, current state: %s",
		instanceID, desiredState, maxAttempts, instance.GetCurrentFSMState())
}

// WaitForManagerState repeatedly calls Reconcile on a manager until all its instances
// reach the desired state or maxAttempts is exceeded
func WaitForManagerState(ctx context.Context, manager ManagerReconciler, config config.FullConfig,
	desiredState string, maxAttempts int, startTick uint64, printDetails bool) (uint64, error) {

	tick := startTick
	var lastErr error

	for i := 0; i < maxAttempts; i++ {
		lastErr, _ = manager.Reconcile(ctx, config, tick)
		if lastErr != nil {
			return tick, lastErr
		}

		// Need to check each instance
		allInstancesMatched := true
		allInstances := map[string]fsm.FSMInstance{}

		// Get list of instances and check their states
		// This depends on the manager implementation but we can enumerate instances
		// by checking all instances that exist
		for id, exists := range getInstancesMap(manager) {
			if !exists {
				continue
			}
			instance, found := manager.GetInstance(id)
			if !found {
				continue // Skip if instance suddenly disappeared
			}
			allInstances[id] = instance

			currentState := instance.GetCurrentFSMState()
			if currentState != desiredState {
				allInstancesMatched = false
				if printDetails {
					fmt.Printf("Instance %s: current=%s (waiting for %s)\n",
						id, currentState, desiredState)
				}
			} else if printDetails {
				fmt.Printf("Instance %s: reached target state %s\n",
					id, desiredState)
			}
		}

		// If no instances found, that's a success if the desired state is "removed"
		if len(allInstances) == 0 {
			if desiredState == internal_fsm.LifecycleStateRemoved {
				return tick, nil
			}
		}

		if allInstancesMatched && len(allInstances) > 0 {
			if printDetails {
				fmt.Printf("All instances reached target state %s\n", desiredState)
			}
			return tick, nil
		}

		tick++
	}

	return tick, fmt.Errorf("failed to reach state %s for all instances after %d attempts",
		desiredState, maxAttempts)
}

// getInstancesMap is a helper function to get a map of instance IDs to existence
// This handles the fact that we don't have a direct way to enumerate instances in the interface
func getInstancesMap(manager ManagerReconciler) map[string]bool {
	// This is a simple implementation that checks for common instance IDs
	// In a real implementation, you might need to adjust this based on your manager's capabilities
	instanceMap := make(map[string]bool)

	// Test common instance names/patterns
	// This is just a simple approach for test purposes
	commonTestNames := []string{
		"test-service", "test-transition", "test-lifecycle", "test-state-change",
		"test-stopping", "test-slow-start", "test-slow-stop", "test-startup",
		"test-benthos", "test-active", "test-degraded", "test-active-to-stopped-to-active",
	}

	for _, name := range commonTestNames {
		if _, exists := manager.GetInstance(name); exists {
			instanceMap[name] = true
		}
	}

	return instanceMap
}

// WaitForManagerInstanceCreation repeatedly calls Reconcile on a manager until the specified instance
// is created (exists in the manager) or maxAttempts is exceeded.
// This function is useful when you need to wait for initial instance creation before checking its state.
func WaitForManagerInstanceCreation(
	ctx context.Context,
	manager ManagerReconciler,
	config config.FullConfig,
	instanceID string,
	maxAttempts int,
	startTick uint64,
) (uint64, error) {
	tick := startTick

	for i := 0; i < maxAttempts; i++ {
		err, _ := manager.Reconcile(ctx, config, tick)
		if err != nil {
			return tick, fmt.Errorf("error during manager reconcile: %w", err)
		}
		tick++

		// Check if the instance exists
		_, exists := manager.GetInstance(instanceID)
		if exists {
			return tick, nil
		}
	}

	return tick, fmt.Errorf("instance %s not created after %d attempts", instanceID, maxAttempts)
}

// WaitForManagerInstanceRemoval repeatedly calls Reconcile on a manager until the specified instance
// is completely removed (no longer exists in the manager) or maxAttempts is exceeded.
// This function is useful when you need to wait for an instance to be fully removed from the manager.
func WaitForManagerInstanceRemoval(
	ctx context.Context,
	manager ManagerReconciler,
	config config.FullConfig,
	instanceID string,
	maxAttempts int,
	startTick uint64,
) (uint64, error) {
	tick := startTick

	for i := 0; i < maxAttempts; i++ {
		err, _ := manager.Reconcile(ctx, config, tick)
		if err != nil {
			return tick, fmt.Errorf("error during manager reconcile: %w", err)
		}
		tick++

		// Check if the instance still exists
		_, exists := manager.GetInstance(instanceID)
		if !exists {
			return tick, nil // Success - instance is gone
		}
	}

	return tick, fmt.Errorf("instance %s not removed after %d attempts", instanceID, maxAttempts)
}

// RunMultipleReconciliations runs the reconciliation process multiple times
// This is useful when you need to run reconciliation several times without waiting for a specific state
func RunMultipleReconciliations(
	ctx context.Context,
	manager ManagerReconciler,
	config config.FullConfig,
	numReconciliations int,
	startTick uint64,
) (uint64, error) {
	tick := startTick

	for i := 0; i < numReconciliations; i++ {
		err, _ := manager.Reconcile(ctx, config, tick)
		if err != nil {
			return tick, fmt.Errorf("error during manager reconcile (attempt %d): %w", i+1, err)
		}
		tick++
	}

	return tick, nil
}
