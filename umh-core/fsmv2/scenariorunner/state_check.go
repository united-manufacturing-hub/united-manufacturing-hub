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

package scenariorunner

import (
	"context"
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
)

// checkStoredWorkerStates returns an error for the first stored worker whose
// type is in validWorkerStates and whose state is not. Call it only after the
// supervisor stopped, so no supervisor write races the read.
func checkStoredWorkerStates(ctx context.Context, store storage.TriangularStoreInterface, validWorkerStates map[string]map[string]bool, logger deps.FSMLogger) error {
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
