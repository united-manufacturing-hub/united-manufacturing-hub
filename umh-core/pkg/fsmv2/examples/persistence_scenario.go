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

package examples

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/configworker/dynamicchildren"
	persistenceworker "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence"
	persistencesnapshot "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence/snapshot"
)

// PersistenceScenarioV2 runs one persistence worker against an in-memory
// store held in the dependency map: the worker runs its startup maintenance,
// reaches Running, and compacts the store.
var PersistenceScenarioV2 = ScenarioV2{
	Name:        "persistence",
	Description: "Compacts and maintains an in-memory store through the dependency map",

	Dependencies: func() (map[string]any, func(), error) {
		// This store is separate from RunConfig.Store, which this function
		// cannot reach. The worker compacts and maintains this one. The
		// collector writes the worker's observations to RunConfig.Store.
		var store storage.TriangularStoreInterface = SetupStore(deps.NewNopFSMLogger())

		m := map[string]any{}

		config.SetDependency(m, persistenceworker.StoreKey, store)

		return m, nil, nil
	},

	Run: func(ctx context.Context, env Env) error {
		ref := dynamicchildren.Ref{WorkerType: "persistence", Name: "persistence-1"}

		env.Step("create the persistence worker")

		// No state reads the state key: StoppedState moves to TryingToStart
		// unless a shutdown is requested.
		if err := env.Client.Upsert(ref, map[string]any{"state": "running"}); err != nil {
			return err
		}

		// Startup maintenance runs in TryingToStart, so its counter and
		// timestamp are set once Running is visible. Running stays healthy
		// to the end: only a shutdown or a failed action leaves it, and
		// both actions succeed against the in-memory store. Both counters
		// in this scenario only rise, and neither timestamp is cleared.
		if err := env.WaitFor(ctx, "store shows Running after startup maintenance",
			func(ctx context.Context) (bool, string, error) {
				obs, err := fsmv2client.Get[persistencesnapshot.PersistenceStatus](ctx, env.Client, ref)
				if err != nil {
					if errors.Is(err, fsmv2client.ErrNotObserved) {
						return false, "the worker has not published an observation yet", nil
					}

					return false, "", err
				}

				maintenanceCycles := obs.Metrics.Worker.Counters[string(deps.CounterMaintenanceCyclesTotal)]

				done := obs.State == "Running" &&
					obs.Status.IsHealthy() &&
					maintenanceCycles >= 1 &&
					!obs.Status.LastMaintenanceAt.IsZero()

				seen := fmt.Sprintf("state=%s maintenance_cycles=%d last_maintenance_at=%s",
					obs.State, maintenanceCycles, obs.Status.LastMaintenanceAt.Format(time.RFC3339))

				return done, seen, nil
			}); err != nil {
			return err
		}

		// A zero LastCompactionAt is due at once, so compaction runs on the
		// first Running tick. Its counter and timestamp then hold to the end.
		return env.WaitFor(ctx, "compaction has run",
			func(ctx context.Context) (bool, string, error) {
				obs, err := fsmv2client.Get[persistencesnapshot.PersistenceStatus](ctx, env.Client, ref)
				if err != nil {
					if errors.Is(err, fsmv2client.ErrNotObserved) {
						return false, "the worker has not published an observation yet", nil
					}

					return false, "", err
				}

				compactionCycles := obs.Metrics.Worker.Counters[string(deps.CounterCompactionCyclesTotal)]

				done := compactionCycles >= 1 &&
					!obs.Status.LastCompactionAt.IsZero()

				seen := fmt.Sprintf("compaction_cycles=%d last_compaction_at=%s",
					compactionCycles, obs.Status.LastCompactionAt.Format(time.RFC3339))

				return done, seen, nil
			})
	},
}
