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

package state

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence/action"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence/snapshot"
)

func emitActionIfDue(
	currentState fsmv2.State[any, any],
	snap fsmv2.WorkerSnapshot[snapshot.PersistenceConfig, snapshot.PersistenceStatus],
) fsmv2.NextResult[any, any] {
	timeSinceCompaction := snap.CollectedAt.Sub(snap.Status.LastCompactionAt)
	if timeSinceCompaction >= snap.Config.CompactionInterval {
		return fsmv2.Transition(currentState, fsmv2.SignalNone,
			action.NewCompactDeltasAction(snap.Config.RetentionWindow), "Running compaction", nil)
	}

	if isMaintenanceDue(snap) {
		return fsmv2.Transition(currentState, fsmv2.SignalNone,
			action.NewRunMaintenanceAction(), "Running maintenance", nil)
	}

	return fsmv2.Transition(currentState, fsmv2.SignalNone, nil, "Monitoring for cleanup needs", nil)
}

const shortIntervalThreshold = 3 * 24 * time.Hour

func isMaintenanceDue(
	snap fsmv2.WorkerSnapshot[snapshot.PersistenceConfig, snapshot.PersistenceStatus],
) bool {
	interval := snap.Config.MaintenanceInterval
	timeSince := snap.CollectedAt.Sub(snap.Status.LastMaintenanceAt)

	if interval < shortIntervalThreshold {
		return timeSince >= interval
	}

	lateDeadline := interval + 2*24*time.Hour
	earlyStart := interval - 2*24*time.Hour

	if timeSince >= lateDeadline {
		return true
	}

	if timeSince >= interval && snap.Status.IsAcceptableMaintenanceWindow {
		return true
	}

	if timeSince >= earlyStart && snap.Status.IsPreferredMaintenanceWindow {
		return true
	}

	return false
}
