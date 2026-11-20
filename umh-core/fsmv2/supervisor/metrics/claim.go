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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
)

// WorkerMetricsClaims returns the claims on the exported worker-metric series, for a worker's dependencies to attach to.
func WorkerMetricsClaims() deps.WorkerMetricsClaims {
	return workerMetricsExporter
}

// Claim makes a newly registered worker the only one that exports and deletes hierarchyPath's series.
func (e *WorkerMetricsExporter) Claim(hierarchyPath string) deps.WorkerMetricsClaim {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, claimedBefore := e.owners[hierarchyPath]; claimedBefore {
		e.deleteSeries(hierarchyPath)
	}

	e.lastClaim++
	e.owners[hierarchyPath] = e.lastClaim

	return e.lastClaim
}

// Release deletes hierarchyPath's worker-metric series while claim still owns them.
func (e *WorkerMetricsExporter) Release(hierarchyPath string, claim deps.WorkerMetricsClaim) {
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
