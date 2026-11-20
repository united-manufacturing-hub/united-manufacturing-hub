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

package deps

// WorkerLifecycle is implemented by dependencies that hold something for as
// long as their worker is registered with a supervisor.
type WorkerLifecycle interface {
	// Attach is called when the supervisor registers the worker.
	Attach(claims WorkerMetricsClaims)
	// Detach is called when the supervisor stops the worker.
	Detach()
}

// WorkerMetricsClaim identifies the worker that owns the worker-metric series of a hierarchy path.
type WorkerMetricsClaim uint64

// WorkerMetricsClaims hands out the claims on the worker-metric series of a hierarchy path.
type WorkerMetricsClaims interface {
	// Claim makes a newly registered worker the only one that exports hierarchyPath's series.
	Claim(hierarchyPath string) WorkerMetricsClaim
	// Release deletes hierarchyPath's series while claim still owns them.
	Release(hierarchyPath string, claim WorkerMetricsClaim)
}

// Attach claims the worker-metric series of the worker's hierarchy path.
func (d *BaseDependencies) Attach(claims WorkerMetricsClaims) {
	if d == nil {
		return
	}

	claim := claims.Claim(d.hierarchyPath)

	d.mu.Lock()
	defer d.mu.Unlock()

	d.metricsClaims = claims
	d.metricsClaim = claim
}

// Detach deletes the worker-metric series of the worker's hierarchy path.
func (d *BaseDependencies) Detach() {
	if d == nil {
		return
	}

	d.mu.Lock()
	claims := d.metricsClaims
	claim := d.metricsClaim
	d.metricsClaims = nil
	d.mu.Unlock()

	if claims == nil {
		return
	}

	claims.Release(d.hierarchyPath, claim)
}

// MetricsClaim returns the claim the worker's metrics are exported under, or 0 before Attach.
func (d *BaseDependencies) MetricsClaim() WorkerMetricsClaim {
	if d == nil {
		return 0
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.metricsClaim
}
