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
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// validWorkerStates lists the state names each worker type the runner runs may
// report. It duplicates the integration battery's list in
// integration/scenarios_test.go until the v1 integration specs are migrated
// (ENG-5114); no registry of a type's states exists to read instead. "unknown"
// is allowed for every type, because the supervisor reports it until a
// worker's first tick. A worker type missing from the map is skipped: its
// stored states cannot be validated.
var validWorkerStates = map[string]map[string]bool{
	"application": {
		"Running": true, "Degraded": true, "Stopped": true,
		"unknown": true,
	},
	"configworker": {
		"Running": true, "Stopped": true,
		"unknown": true,
	},
	"helloworld": {
		"TryingToStart": true, "Running": true,
		"Degraded": true, "Stopped": true,
		"unknown": true,
	},
}

// checkStoredWorkerStates reads every stored worker the way the scenario dump
// does and returns an error naming the first worker whose stored observed
// state is not a state name its type may report, or nil when every stored
// state is valid. The check runs after the supervisor has stopped, so no
// supervisor write races the read.
func checkStoredWorkerStates(ctx context.Context, store storage.TriangularStoreInterface, logger deps.FSMLogger) error {
	dump, err := DumpScenario(ctx, store, 0)
	if err != nil {
		return fmt.Errorf("read stored workers for the state check: %w", err)
	}

	for _, w := range dump.Workers {
		if w.Observed == nil {
			continue
		}

		state, ok := w.Observed["state"].(string)
		if !ok || state == "" {
			continue
		}

		validStates, known := validWorkerStates[w.WorkerType]
		if !known {
			continue
		}

		if !validStates[state] {
			return fmt.Errorf("worker %s (type %s) holds stored state %q, which is not a state name its worker type may report",
				w.WorkerID, w.WorkerType, state)
		}
	}

	return nil
}
