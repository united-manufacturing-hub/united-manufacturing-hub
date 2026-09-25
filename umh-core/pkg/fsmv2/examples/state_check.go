package examples

import (
	"context"
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// validWorkerStates lists the state names each worker type the runner runs may
// report. It duplicates the integration battery's list in
// integration/scenarios_test.go until the v1 specs go; no registry of a type's
// states exists to read instead. "unknown" is allowed for every type, because
// the supervisor reports it until a worker's first tick. A worker type missing
// from the map is skipped: its stored states cannot be validated.
var validWorkerStates = map[string]map[string]bool{
	"application": {
		"TryingToStart": true, "Running": true,
		"TryingToStop": true, "Stopped": true,
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
		logger.SentryWarn(deps.FeatureExamples, "", "stored_state_check_dump_failed",
			deps.Err(err),
			deps.String("impact", "stored_states_not_validated"))

		return nil
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
