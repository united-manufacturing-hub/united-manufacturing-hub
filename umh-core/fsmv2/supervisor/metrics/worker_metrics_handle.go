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

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
)

// WorkerMetricsHandle exports one worker's metrics under the worker's hierarchy path while it owns that path.
// Only the newest handle for a path exports, so a collection that returns after its worker was
// removed cannot bring the series back or overwrite a worker added again at the same path.
type WorkerMetricsHandle struct {
	exporter      *WorkerMetricsExporter
	hierarchyPath string
}

// ForWorker makes a newly added worker the only one that exports and deletes hierarchyPath's series.
func ForWorker(hierarchyPath string) *WorkerMetricsHandle {
	return workerMetricsExporter.claim(hierarchyPath)
}

// Export exports observed's worker metrics while h still owns its hierarchy path. No-op if observed does not implement MetricsHolder.
func (h *WorkerMetricsHandle) Export(observed fsmv2.ObservedState) {
	if h == nil {
		return
	}

	holder, ok := observed.(deps.MetricsHolder)
	if !ok {
		return
	}

	metrics := holder.GetWorkerMetrics()
	if metrics.Counters == nil && metrics.Gauges == nil {
		return
	}

	h.exporter.export(h, &metrics)
}

// Release deletes the worker-metric series of h's hierarchy path while h still owns it.
// After Release, Export does nothing.
func (h *WorkerMetricsHandle) Release() {
	if h == nil {
		return
	}

	h.exporter.release(h)
}

func (e *WorkerMetricsExporter) claim(hierarchyPath string) *WorkerMetricsHandle {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, claimedBefore := e.owners[hierarchyPath]; claimedBefore {
		e.deleteSeries(hierarchyPath)
	}

	handle := &WorkerMetricsHandle{exporter: e, hierarchyPath: hierarchyPath}
	e.owners[hierarchyPath] = handle

	return handle
}

func (e *WorkerMetricsExporter) release(handle *WorkerMetricsHandle) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.owners[handle.hierarchyPath] != handle {
		return
	}

	delete(e.owners, handle.hierarchyPath)
	e.deleteSeries(handle.hierarchyPath)
}

func (e *WorkerMetricsExporter) deleteSeries(hierarchyPath string) {
	labels := prometheus.Labels{
		"hierarchy_path": hierarchyPath,
	}

	for _, counter := range e.counters {
		counter.Delete(labels)
	}

	delete(e.previousCounterValuesByPath, hierarchyPath)

	for _, gauge := range e.gauges {
		gauge.Delete(labels)
	}
}
