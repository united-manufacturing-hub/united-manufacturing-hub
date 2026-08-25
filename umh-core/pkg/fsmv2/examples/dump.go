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
	"sort"
	"strings"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// ANSI color codes for terminal output.
const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorCyan   = "\033[36m"
)

// ScenarioDump captures delta history and final state of a scenario run.
type ScenarioDump struct {
	Deltas      []storage.Delta
	Workers     []WorkerSnapshot
	StartSyncID int64
	EndSyncID   int64
}

// WorkerSnapshot represents the final triangular state of a worker.
type WorkerSnapshot struct {
	Identity   persistence.Document
	Desired    persistence.Document
	Observed   persistence.Document
	WorkerType string
	WorkerID   string
}

// DumpScenario captures deltas and final state. Use startSyncID=0 for all history.
func DumpScenario(ctx context.Context, store storage.TriangularStoreInterface, startSyncID int64) (*ScenarioDump, error) {
	resp, err := store.GetDeltas(ctx, storage.Subscription{LastSyncID: startSyncID})
	if err != nil {
		return nil, fmt.Errorf("failed to get deltas: %w", err)
	}

	// Use LatestSyncID from response to avoid race between GetDeltas and GetLatestSyncID
	endSyncID := resp.LatestSyncID

	var workers []WorkerSnapshot

	// Handle bootstrap case: when client is too far behind, use Bootstrap.Workers
	if resp.RequiresBootstrap && resp.Bootstrap != nil {
		workers = convertStorageWorkers(resp.Bootstrap.Workers)
	} else {
		workers = extractAndLoadWorkers(ctx, store, resp.Deltas)
	}

	return &ScenarioDump{
		StartSyncID: startSyncID,
		EndSyncID:   endSyncID,
		Deltas:      resp.Deltas,
		Workers:     workers,
	}, nil
}

// convertStorageWorkers converts storage.WorkerSnapshot to examples.WorkerSnapshot.
func convertStorageWorkers(storageWorkers []storage.WorkerSnapshot) []WorkerSnapshot {
	workers := make([]WorkerSnapshot, len(storageWorkers))

	for i, sw := range storageWorkers {
		workers[i] = WorkerSnapshot{
			WorkerType: sw.WorkerType,
			WorkerID:   sw.WorkerID,
			Identity:   sw.Identity,
			Desired:    sw.Desired,
			Observed:   sw.Observed,
		}
	}

	return workers
}

// extractAndLoadWorkers loads snapshots for unique workers found in deltas.
func extractAndLoadWorkers(ctx context.Context, store storage.TriangularStoreInterface, deltas []storage.Delta) []WorkerSnapshot {
	seen := make(map[string]bool)

	var workers []WorkerSnapshot

	for _, delta := range deltas {
		key := delta.WorkerType + "/" + delta.WorkerID
		if seen[key] {
			continue
		}

		seen[key] = true

		snapshot, err := store.LoadSnapshot(ctx, delta.WorkerType, delta.WorkerID)
		if err != nil {
			continue
		}

		var observedDoc persistence.Document
		if doc, ok := snapshot.Observed.(persistence.Document); ok {
			observedDoc = doc
		}

		workers = append(workers, WorkerSnapshot{
			WorkerType: delta.WorkerType,
			WorkerID:   delta.WorkerID,
			Identity:   snapshot.Identity,
			Desired:    snapshot.Desired,
			Observed:   observedDoc,
		})
	}

	sort.Slice(workers, func(i, j int) bool {
		if workers[i].WorkerType != workers[j].WorkerType {
			return workers[i].WorkerType < workers[j].WorkerType
		}

		return workers[i].WorkerID < workers[j].WorkerID
	})

	return workers
}

// FormatHuman returns delta history followed by final state grouped by worker type.
func (d *ScenarioDump) FormatHuman() string {
	var sb strings.Builder

	// Header
	sb.WriteString("\n")
	sb.WriteString("═══════════════════════════════════════════════════════════════════════\n")
	sb.WriteString("CSE SCENARIO DUMP\n")
	sb.WriteString("═══════════════════════════════════════════════════════════════════════\n")
	sb.WriteString(fmt.Sprintf("SyncID Range: %d → %d\n", d.StartSyncID, d.EndSyncID))
	sb.WriteString(fmt.Sprintf("Workers: %d\n", len(d.Workers)))
	sb.WriteString(fmt.Sprintf("Deltas: %d\n", len(d.Deltas)))

	// Delta History
	sb.WriteString("\n")
	sb.WriteString("═══════════════════════════════════════════════════════════════════════\n")
	sb.WriteString(fmt.Sprintf("DELTA HISTORY (%d changes)\n", len(d.Deltas)))
	sb.WriteString("═══════════════════════════════════════════════════════════════════════\n")
	sb.WriteString("\n")

	for i, delta := range d.Deltas {
		ts := time.UnixMilli(delta.TimestampMs).Format("15:04:05.000")
		sb.WriteString(fmt.Sprintf("[%d] %s | %s/%s | %s\n",
			i+1, ts, delta.WorkerType, delta.WorkerID, delta.Role))

		if delta.Changes != nil {
			var addedKeys []string
			for field := range delta.Changes.Added {
				addedKeys = append(addedKeys, field)
			}

			sort.Strings(addedKeys)

			for _, field := range addedKeys {
				sb.WriteString(fmt.Sprintf("    %s+%s %s: %v\n", colorGreen, colorReset, field, formatValue(delta.Changes.Added[field])))
			}

			var modifiedKeys []string
			for field := range delta.Changes.Modified {
				modifiedKeys = append(modifiedKeys, field)
			}

			sort.Strings(modifiedKeys)

			for _, field := range modifiedKeys {
				mod := delta.Changes.Modified[field]
				sb.WriteString(fmt.Sprintf("    %s~%s %s: %v → %v\n", colorYellow, colorReset, field, formatValue(mod.Old), formatValue(mod.New)))
			}

			sortedRemoved := make([]string, len(delta.Changes.Removed))
			copy(sortedRemoved, delta.Changes.Removed)
			sort.Strings(sortedRemoved)

			for _, field := range sortedRemoved {
				sb.WriteString(fmt.Sprintf("    %s-%s %s\n", colorRed, colorReset, field))
			}
		}

		sb.WriteString("\n")
	}

	// Final State
	sb.WriteString("═══════════════════════════════════════════════════════════════════════\n")
	sb.WriteString(fmt.Sprintf("FINAL STATE (%d workers)\n", len(d.Workers)))
	sb.WriteString("═══════════════════════════════════════════════════════════════════════\n")

	workersByType := make(map[string][]WorkerSnapshot)
	for _, w := range d.Workers {
		workersByType[w.WorkerType] = append(workersByType[w.WorkerType], w)
	}

	var workerTypes []string
	for wt := range workersByType {
		workerTypes = append(workerTypes, wt)
	}

	sort.Strings(workerTypes)

	for _, wt := range workerTypes {
		sb.WriteString(fmt.Sprintf("\nWorker Type: %s\n", wt))
		sb.WriteString("───────────────────────────────────────────────────────────────────────\n")

		for _, w := range workersByType[wt] {
			sb.WriteString(fmt.Sprintf("\n  [%s]\n", w.WorkerID))

			sb.WriteString("    IDENTITY:\n")
			formatDocument(&sb, w.Identity, "      ")

			sb.WriteString("    DESIRED:\n")
			formatDocument(&sb, w.Desired, "      ")

			sb.WriteString("    OBSERVED:\n")
			formatDocument(&sb, w.Observed, "      ")
		}
	}

	// Summary
	sb.WriteString("\n")
	sb.WriteString("═══════════════════════════════════════════════════════════════════════\n")
	sb.WriteString(fmt.Sprintf("Summary: %d worker types, %d workers, %d deltas\n",
		len(workerTypes), len(d.Workers), len(d.Deltas)))
	sb.WriteString("═══════════════════════════════════════════════════════════════════════\n")

	return sb.String()
}

// formatValue formats a value, truncating at 80 characters.
func formatValue(v interface{}) string {
	s := fmt.Sprintf("%v", v)
	if len(s) > 80 {
		return s[:77] + "..."
	}

	return s
}

// formatDocument writes document fields to the string builder, skipping CSE metadata.
func formatDocument(sb *strings.Builder, doc persistence.Document, indent string) {
	if len(doc) == 0 {
		sb.WriteString(indent + "(empty)\n")

		return
	}

	var keys []string

	for k := range doc {
		if strings.HasPrefix(k, "_") {
			continue
		}

		keys = append(keys, k)
	}

	sort.Strings(keys)

	for _, k := range keys {
		fmt.Fprintf(sb, "%s%s: %v\n", indent, k, formatValue(doc[k]))
	}

	if len(keys) == 0 {
		sb.WriteString(indent + "(only CSE metadata)\n")
	}
}
