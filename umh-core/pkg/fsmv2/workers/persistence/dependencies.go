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

	store             storage.TriangularStoreInterface
	lastCompactionAt  time.Time
	lastMaintenanceAt time.Time
	mu                sync.RWMutex
}

var _ snapshot.PersistenceDependencies = (*PersistenceDependencies)(nil)

// NewPersistenceDependencies creates dependencies for the persistence worker.
func NewPersistenceDependencies(
	store storage.TriangularStoreInterface,
	logger deps.FSMLogger,
	stateReader deps.StateReader,
	identity deps.Identity,
) *PersistenceDependencies {
	if store == nil {
		panic("NewPersistenceDependencies: store cannot be nil")
	}

	return &PersistenceDependencies{
		BaseDependencies: deps.NewBaseDependencies(logger, stateReader, identity),
		store:            store,
	}
}

func (d *PersistenceDependencies) GetStore() storage.TriangularStoreInterface {
	return d.store
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
