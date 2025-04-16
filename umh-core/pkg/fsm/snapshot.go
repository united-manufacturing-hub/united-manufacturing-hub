package fsm

import (
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/snapshot"
)

// GetManagerSnapshots extracts snapshots from all FSM managers
func GetManagerSnapshots(managers []FSMManager[any], currentTick uint64, cfg config.FullConfig) (*snapshot.SystemSnapshot, error) {
	if managers == nil {
		return nil, fmt.Errorf("managers list is nil")
	}

	currentSnapshot := &snapshot.SystemSnapshot{
		Managers:     make(map[string]snapshot.ManagerSnapshot),
		SnapshotTime: time.Now(),
		Tick:         currentTick,
	}

	for _, manager := range managers {
		if manager == nil {
			continue // Skip nil managers
		}

		managerName := manager.GetManagerName()
		if managerName == "" {
			continue // Skip managers with empty names
		}

		var managerSnapshot snapshot.ManagerSnapshot

		if snapshotCreator, ok := manager.(snapshot.ManagerSnapshotCreator); ok {
			// If the manager implements ManagerSnapshotCreator, use that
			managerSnapshot = snapshotCreator.CreateSnapshot()
		} else {
			// Fall back to the generic implementation
			managerSnapshot = getManagerSnapshot(manager)
		}

		if managerSnapshot != nil {
			currentSnapshot.Managers[managerName] = managerSnapshot
		}
	}

	return currentSnapshot, nil
}

// Helper function to extract a snapshot from a single manager
func getManagerSnapshot(manager FSMManager[any]) snapshot.ManagerSnapshot {
	if manager == nil {
		return nil // Safety check
	}

	// Create base snapshot with required fields
	currentSnapshot := &snapshot.BaseManagerSnapshot{
		Name:         manager.GetManagerName(),
		Instances:    make(map[string]snapshot.FSMInstanceSnapshot),
		SnapshotTime: time.Now(),
	}

	// Try type assertion for additional fields
	baseManager, ok := manager.(*BaseFSMManager[any])
	if ok {
		currentSnapshot.ManagerTick = baseManager.GetManagerTick()
		currentSnapshot.LastAddTick = baseManager.GetLastAddTick()
		currentSnapshot.LastUpdateTick = baseManager.GetLastUpdateTick()
		currentSnapshot.LastRemoveTick = baseManager.GetLastRemoveTick()
		currentSnapshot.LastStateChange = baseManager.GetLastStateChange()
	}

	// Get instances and their states
	instances := manager.GetInstances()
	for name, instance := range instances {
		if instance == nil {
			continue // Skip nil instances
		}

		instanceSnapshot := snapshot.FSMInstanceSnapshot{
			ID:           name,
			CurrentState: instance.GetCurrentFSMState(),
			DesiredState: instance.GetDesiredFSMState(),
			// Other fields would be populated if we could access them
		}

		// Add observed state if available
		if observedState := instance.GetLastObservedState(); observedState != nil {
			// Check if instance implements ObservedStateConverter
			if converter, ok := instance.(ObservedStateConverter); ok {
				instanceSnapshot.LastObservedState = converter.CreateObservedStateSnapshot()
			}
		}

		currentSnapshot.Instances[name] = instanceSnapshot
	}

	return currentSnapshot
}

// ObservedStateConverter is an interface for objects that can convert their observed state to a snapshot
type ObservedStateConverter interface {
	CreateObservedStateSnapshot() snapshot.ObservedStateSnapshot
}
