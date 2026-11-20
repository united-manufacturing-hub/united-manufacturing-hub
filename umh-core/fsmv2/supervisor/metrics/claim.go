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

import "github.com/prometheus/client_golang/prometheus"

// WorkerMetricsClaim identifies the worker that owns the worker-metric series of a hierarchy path.
type WorkerMetricsClaim uint64

// ClaimWorkerMetrics makes a newly added worker the only one that exports and deletes hierarchyPath's series.
func ClaimWorkerMetrics(hierarchyPath string) WorkerMetricsClaim {
	return workerMetricsExporter.claim(hierarchyPath)
}

// CleanupWorkerMetrics deletes hierarchyPath's worker-metric series while claim still owns them.
func CleanupWorkerMetrics(hierarchyPath string, claim WorkerMetricsClaim) {
	workerMetricsExporter.cleanup(hierarchyPath, claim)
}

func (e *WorkerMetricsExporter) claim(hierarchyPath string) WorkerMetricsClaim {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, claimedBefore := e.owners[hierarchyPath]; claimedBefore {
		e.deleteSeries(hierarchyPath)
	}

	e.lastClaim++
	e.owners[hierarchyPath] = e.lastClaim

	return e.lastClaim
}

func (e *WorkerMetricsExporter) cleanup(hierarchyPath string, claim WorkerMetricsClaim) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.owners[hierarchyPath] != claim {
		return
	}

	delete(e.owners, hierarchyPath)
	e.deleteSeries(hierarchyPath)
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
