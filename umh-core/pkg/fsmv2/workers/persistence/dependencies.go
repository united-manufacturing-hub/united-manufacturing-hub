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

package persistence

import (
	"sync"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence/snapshot"
)

// PersistenceDependencies provides store access for persistence worker actions.
type PersistenceDependencies struct {
	*deps.BaseDependencies

	lastCompactionAt    time.Time
	lastMaintenanceAt   time.Time
	store               storage.TriangularStoreInterface
	scheduler           deps.Scheduler
	mu                  sync.RWMutex
	observedStateLoaded bool
}

var _ snapshot.PersistenceDependencies = (*PersistenceDependencies)(nil)

// NewPersistenceDependencies creates dependencies for the persistence worker.
func NewPersistenceDependencies(
	store storage.TriangularStoreInterface,
	scheduler deps.Scheduler,
	logger deps.FSMLogger,
	stateReader deps.StateReader,
	identity deps.Identity,
) *PersistenceDependencies {
	if store == nil {
		panic("NewPersistenceDependencies: store cannot be nil")
	}

	if scheduler == nil {
		panic("NewPersistenceDependencies: scheduler cannot be nil")
	}

	return &PersistenceDependencies{
		BaseDependencies: deps.NewBaseDependencies(logger, stateReader, identity),
		store:            store,
		scheduler:        scheduler,
	}
}

// NewStoreOnlyDependencies builds a seed PersistenceDependencies carrying only
// the triangular store, for parent wiring at cmd/main.go to publish via
// register.SetDeps. The persistence worker constructor detects the missing
// BaseDependencies and rebuilds a full dependencies struct with the worker's
// own identity, logger, and stateReader. Panics on nil store.
func NewStoreOnlyDependencies(store storage.TriangularStoreInterface) *PersistenceDependencies {
	if store == nil {
		panic("NewStoreOnlyDependencies: store cannot be nil")
	}

	return &PersistenceDependencies{store: store}
}

func (d *PersistenceDependencies) GetStore() storage.TriangularStoreInterface {
	return d.store
}

func (d *PersistenceDependencies) GetScheduler() deps.Scheduler {
	return d.scheduler
}

func (d *PersistenceDependencies) SetLastCompactionAt(t time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.lastCompactionAt = t
}

func (d *PersistenceDependencies) GetLastCompactionAt() time.Time {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.lastCompactionAt
}

func (d *PersistenceDependencies) SetLastMaintenanceAt(t time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.lastMaintenanceAt = t
}

func (d *PersistenceDependencies) GetLastMaintenanceAt() time.Time {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.lastMaintenanceAt
}

// SetObservedStateLoaded marks that observed state was successfully loaded at least once.
// It is safe for concurrent use.
func (d *PersistenceDependencies) SetObservedStateLoaded() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.observedStateLoaded = true
}

// IsObservedStateLoaded reports whether observed state was successfully loaded at least once.
// It is safe for concurrent use.
func (d *PersistenceDependencies) IsObservedStateLoaded() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.observedStateLoaded
}
