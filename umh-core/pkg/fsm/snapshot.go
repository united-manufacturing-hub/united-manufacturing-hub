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

package fsm

import (
	"fmt"
	"sync"
	"time"

	"github.com/tiendc/go-deepcopy"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
)

// These are the names of the managers and instances that are part of the system snapshot
// They are quite inconsistent and need to be cleaned up
const (
	// Manager name constants
	ContainerManagerName         = logger.ComponentContainerManager + "_" + constants.DefaultManagerName
	BenthosManagerName           = logger.ComponentBenthosManager + "_" + constants.DefaultManagerName
	AgentManagerName             = logger.ComponentAgentManager + "_" + constants.DefaultManagerName
	RedpandaManagerName          = logger.ComponentRedpandaManager + constants.DefaultManagerName
	DataflowcomponentManagerName = constants.DataflowcomponentManagerName

	// Instance name constants
	CoreInstanceName     = "Core"
	AgentInstanceName    = "agent"
	RedpandaInstanceName = "redpanda"
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
	// Warning: treat the returned snapshots as read-only and do not modify them. If you do that within the core loop, you will change the state of the system
	// If you do it in the communicator, it is "fine" as the communicator got only a deep copy of the snapshot
	// The pointers are needed to avoid unexported fields when doing deep copies
	GetInstances() map[string]*FSMInstanceSnapshot
	// GetSnapshotTime returns the time the snapshot was created
	GetSnapshotTime() time.Time
	// GetManagerTick returns the current manager-specific tick
	GetManagerTick() uint64
}

// BaseManagerSnapshot contains the basic immutable state common to all manager types
type BaseManagerSnapshot struct {
	Name           string
	Instances      map[string]*FSMInstanceSnapshot // this needs to be a pointer to avoid unexported fields when doing deep copies
	ManagerTick    uint64
	NextAddTick    uint64
	NextUpdateTick uint64
	NextRemoveTick uint64
	NextStateTick  uint64
	SnapshotTime   time.Time
}

// GetName returns the name of the manager
func (s *BaseManagerSnapshot) GetName() string {
	return s.Name
}

// GetInstances returns the snapshots of all instances
// Warning: treat the returned snapshots as read-only and do not modify them
// Warning: treat the returned snapshots as read-only and do not modify them. If you do that within the core loop, you will change the state of the system
// If you do it in the communicator, it is "fine" as the communicator got only a deep copy of the snapshot
// The pointers are needed to avoid unexported fields when doing deep copies
func (s *BaseManagerSnapshot) GetInstances() map[string]*FSMInstanceSnapshot {
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
	CurrentConfig config.FullConfig
	Managers      map[string]ManagerSnapshot
	SnapshotTime  time.Time
	ConfigHash    string
	Tick          uint64
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

// GetDeepCopySnapshot returns a deep copy of the most recent system snapshot
func (s *SnapshotManager) GetDeepCopySnapshot() SystemSnapshot {
	if s == nil {
		return SystemSnapshot{} // Safety check for nil receiver
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var snapshotCopy SystemSnapshot
	err := deepcopy.Copy(&snapshotCopy, s.lastSnapshot)
	if err != nil {
		// If deep copy fails, return nil to indicate failure
		return SystemSnapshot{}
	}
	return snapshotCopy
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
		Instances:    make(map[string]*FSMInstanceSnapshot),
		SnapshotTime: time.Now(),
	}

	// Try type assertion for additional fields
	baseManager, ok := manager.(*BaseFSMManager[any])
	if ok {
		snapshot.ManagerTick = baseManager.GetManagerTick()
		snapshot.NextAddTick = baseManager.GetNextAddTick()
		snapshot.NextUpdateTick = baseManager.GetNextUpdateTick()
		snapshot.NextRemoveTick = baseManager.GetNextRemoveTick()
		snapshot.NextStateTick = baseManager.GetNextStateTick()
	}

	// Get instances and their states
	instances := manager.GetInstances()
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

		snapshot.Instances[name] = &instanceSnapshot
	}

	return snapshot
}

// FindManager finds a manager in the system snapshot.
// Returns nil and false if the manager is not found.
func FindManager(
	snap SystemSnapshot,
	managerName string,
) (ManagerSnapshot, bool) {
	mgr, ok := snap.Managers[managerName]
	if !ok || mgr == nil {
		return nil, false
	}
	return mgr, true
}

// FindInstance finds an instance in the system snapshot.
// This is useful if we want to fetch the data from a manager that always has one instance (e.g., core, agent, container, redpanda).
// Returns nil and false if the instance is not found.
func FindInstance(
	snap SystemSnapshot,
	managerName, instanceName string,
) (*FSMInstanceSnapshot, bool) {
	mgr, ok := FindManager(snap, managerName)
	if !ok {
		return nil, false
	}
	inst, ok := mgr.GetInstances()[instanceName]
	return inst, ok
}

// FindDfcInstanceByUUID finds a dataflow component instance with the given UUID.
// It returns the instance if found, otherwise an error is returned.
func FindDfcInstanceByUUID(systemSnapshot SystemSnapshot, dfcUUID string) (*FSMInstanceSnapshot, error) {
	dfcManager, ok := FindManager(systemSnapshot, constants.DataflowcomponentManagerName)
	if !ok {
		return nil, fmt.Errorf("dfc manager not found")
	}

	dfcInstances := dfcManager.GetInstances()

	for _, instance := range dfcInstances {
		if instance == nil {
			continue
		}

		currentUUID := dataflowcomponentserviceconfig.GenerateUUIDFromName(instance.ID).String()
		if currentUUID == dfcUUID {
			return instance, nil
		}
	}

	return nil, fmt.Errorf("the requested DFC with UUID %s was not found", dfcUUID)
}
