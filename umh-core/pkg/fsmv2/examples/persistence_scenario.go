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
// reaches Running, and compacts the store on its first Running tick.
var PersistenceScenarioV2 = ScenarioV2{
	Name:        "persistence",
	Description: "Compacts and maintains an in-memory store through the dependency map",

	Dependencies: func() (map[string]any, func(), error) {
		// A second store: the supervisor still writes to RunConfig.Store,
		// which Dependencies cannot reach. The worker compacts and
		// maintains this one; the collector writes its observations to the
		// run's.
		var store storage.TriangularStoreInterface = SetupStore(deps.NewNopFSMLogger())

		m := map[string]any{}

		config.SetDependency(m, persistenceworker.StoreKey, store)

		return m, nil, nil
	},

	Run: func(ctx context.Context, env Env) error {
		ref := dynamicchildren.Ref{WorkerType: "persistence", Name: "persistence-1"}

		env.Step("create the persistence worker")

		// The state key is accepted and ignored: StoppedState leaves on
		// IsShutdownRequested only, so the worker starts regardless. It is
		// sent for the same shape as the other scenarios.
		if err := env.Client.Upsert(ref, map[string]any{"state": "running"}); err != nil {
			return err
		}

		// The startup maintenance runs in TryingToStart, before Running,
		// so its counter and timestamp are set by the time this wait sees
		// Running. Running with IsHealthy then holds for the rest of the
		// run: RunningState leaves only on a shutdown or a failed action,
		// and both actions succeed against the in-memory store. The
		// counter only rises and the timestamp is never cleared.
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

		// Compaction runs on the first Running tick, because a zero
		// LastCompactionAt is due at once. The counter only rises (the
		// collector adds each drained delta to the stored value) and the
		// timestamp is never cleared, so both hold from the first
		// compaction to the end of the run.
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
