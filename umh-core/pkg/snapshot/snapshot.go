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

package snapshot

import (
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
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

// ManagerSnapshotCreator is an interface for managers that can create their own snapshots
type ManagerSnapshotCreator interface {
	CreateSnapshot() ManagerSnapshot
}
