package fsm

import (
	"fmt"
	"sync"
	"time"

	"github.com/united-manufacturing-hub/benthos-umh/umh-core/pkg/config"
)

// ObservedStateSnapshot represents a deep copy of an observed state
type ObservedStateSnapshot interface {
	// IsObservedStateSnapshot is a marker method to ensure type safety
	IsObservedStateSnapshot()
}

// FSMInstanceSnapshot contains the immutable state of an FSM instance
type FSMInstanceSnapshot struct {
	ID                string
	CurrentState      string
	DesiredState      string
	LastError         string
	LastErrorTime     time.Time
	LastObservedState ObservedStateSnapshot
	CreatedAt         time.Time
	LastUpdatedAt     time.Time
}

// ManagerSnapshot defines the interface for manager-specific snapshots
type ManagerSnapshot interface {
	// GetName returns the name of the manager
	GetName() string
	// GetInstances returns the snapshots of all instances
	GetInstances() map[string]FSMInstanceSnapshot
	// GetSnapshotTime returns the time the snapshot was created
	GetSnapshotTime() time.Time
	// GetManagerTick returns the current manager-specific tick
	GetManagerTick() uint64
}

// BaseManagerSnapshot contains the basic immutable state common to all manager types
type BaseManagerSnapshot struct {
	Name            string
	Instances       map[string]FSMInstanceSnapshot
	ManagerTick     uint64
	LastAddTick     uint64
	LastUpdateTick  uint64
	LastRemoveTick  uint64
	LastStateChange uint64
	SnapshotTime    time.Time
}

// GetName returns the name of the manager
func (s *BaseManagerSnapshot) GetName() string {
	return s.Name
}

// GetInstances returns the snapshots of all instances
func (s *BaseManagerSnapshot) GetInstances() map[string]FSMInstanceSnapshot {
	return s.Instances
}

// GetSnapshotTime returns the time the snapshot was created
func (s *BaseManagerSnapshot) GetSnapshotTime() time.Time {
	return s.SnapshotTime
}

// GetManagerTick returns the current manager-specific tick
func (s *BaseManagerSnapshot) GetManagerTick() uint64 {
	return s.ManagerTick
}

// SystemSnapshot contains a thread-safe snapshot of the entire system state
type SystemSnapshot struct {
	Managers     map[string]ManagerSnapshot
	SnapshotTime time.Time
	ConfigHash   string
	Tick         uint64
}

// SnapshotManager manages thread-safe creation, storage, and retrieval of system snapshots
type SnapshotManager struct {
	mu           sync.RWMutex
	lastSnapshot *SystemSnapshot
}

// NewSnapshotManager creates a new snapshot manager
func NewSnapshotManager() *SnapshotManager {
	return &SnapshotManager{
		lastSnapshot: &SystemSnapshot{
			Managers:     make(map[string]ManagerSnapshot),
			SnapshotTime: time.Now(),
		},
	}
}

// UpdateSnapshot creates a new system snapshot from the current state
func (s *SnapshotManager) UpdateSnapshot(snapshot *SystemSnapshot) {
	if s == nil {
		return // Safety check for nil receiver
	}
	if snapshot == nil {
		return // Don't update with nil snapshot
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastSnapshot = snapshot
}

// GetSnapshot returns the most recent system snapshot
func (s *SnapshotManager) GetSnapshot() *SystemSnapshot {
	if s == nil {
		return nil // Safety check for nil receiver
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastSnapshot
}

// GetManagerSnapshots extracts snapshots from all FSM managers
func GetManagerSnapshots(managers []FSMManager[any], currentTick uint64, cfg config.FullConfig) (*SystemSnapshot, error) {
	if managers == nil {
		return nil, fmt.Errorf("managers list is nil")
	}

	snapshot := &SystemSnapshot{
		Managers:     make(map[string]ManagerSnapshot),
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

		var managerSnapshot ManagerSnapshot

		if snapshotCreator, ok := manager.(ManagerSnapshotCreator); ok {
			// If the manager implements ManagerSnapshotCreator, use that
			managerSnapshot = snapshotCreator.CreateSnapshot()
		} else {
			// Fall back to the generic implementation
			managerSnapshot = getManagerSnapshot(manager)
		}

		if managerSnapshot != nil {
			snapshot.Managers[managerName] = managerSnapshot
		}
	}

	return snapshot, nil
}

// ManagerSnapshotCreator is an interface for managers that can create their own snapshots
type ManagerSnapshotCreator interface {
	CreateSnapshot() ManagerSnapshot
}

// Helper function to extract a snapshot from a single manager
func getManagerSnapshot(manager FSMManager[any]) ManagerSnapshot {
	if manager == nil {
		return nil // Safety check
	}

	// Create base snapshot with required fields
	snapshot := &BaseManagerSnapshot{
		Name:         manager.GetManagerName(),
		Instances:    make(map[string]FSMInstanceSnapshot),
		SnapshotTime: time.Now(),
	}

	// Try type assertion for additional fields
	baseManager, ok := manager.(*BaseFSMManager[any])
	if ok {
		snapshot.ManagerTick = baseManager.GetManagerTick()
		snapshot.LastAddTick = baseManager.GetLastAddTick()
		snapshot.LastUpdateTick = baseManager.GetLastUpdateTick()
		snapshot.LastRemoveTick = baseManager.GetLastRemoveTick()
		snapshot.LastStateChange = baseManager.GetLastStateChange()
	}

	// Get instances and their states
	instances := manager.GetInstances()
	if instances != nil {
		for name, instance := range instances {
			if instance == nil {
				continue // Skip nil instances
			}

			instanceSnapshot := FSMInstanceSnapshot{
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

			snapshot.Instances[name] = instanceSnapshot
		}
	}

	return snapshot
}
