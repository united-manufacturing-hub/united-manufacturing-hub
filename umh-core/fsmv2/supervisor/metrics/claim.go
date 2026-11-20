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

// WorkerMetricsClaim is one worker's ownership of the worker-metric series of its hierarchy path.
// Only the newest claim on a path exports, so a collection that returns after its worker was
// removed cannot bring the series back or overwrite a worker added again at the same path.
type WorkerMetricsClaim struct {
	exporter      *WorkerMetricsExporter
	hierarchyPath string
}

// ClaimWorkerMetrics makes a newly added worker the only one that exports and deletes hierarchyPath's series.
func ClaimWorkerMetrics(hierarchyPath string) *WorkerMetricsClaim {
	return workerMetricsExporter.claim(hierarchyPath)
}

// Export exports observed's worker metrics while c still owns its hierarchy path.
func (c *WorkerMetricsClaim) Export(observed fsmv2.ObservedState) {
	if c == nil {
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

	c.exporter.export(c, &metrics)
}

// Release deletes the worker-metric series of c's hierarchy path while c still owns it.
// After Release, Export does nothing.
func (c *WorkerMetricsClaim) Release() {
	if c == nil {
		return
	}

	c.exporter.release(c)
}

func (e *WorkerMetricsExporter) claim(hierarchyPath string) *WorkerMetricsClaim {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, claimedBefore := e.owners[hierarchyPath]; claimedBefore {
		e.deleteSeries(hierarchyPath)
	}

	claim := &WorkerMetricsClaim{exporter: e, hierarchyPath: hierarchyPath}
	e.owners[hierarchyPath] = claim

	return claim
}

func (e *WorkerMetricsExporter) release(claim *WorkerMetricsClaim) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.owners[claim.hierarchyPath] != claim {
		return
	}

	delete(e.owners, claim.hierarchyPath)
	e.deleteSeries(claim.hierarchyPath)
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
