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

package supervisor

import "time"

const (
	// DefaultTickInterval is the default interval between FSM reconciliation ticks.
	//
	// RECOMMENDED (Invariant I12): TickInterval <= ObservationInterval
	// This prevents tick queue buildup when observations take longer.
	// When TickInterval > ObservationInterval, multiple ticks may execute on same stale snapshot.
	//
	// Current defaults: 1s tick interval, 1s observation interval (equal, optimal).
	DefaultTickInterval = 1 * time.Second

	// DefaultStaleThreshold is the default age threshold for detecting stale observation data.
	DefaultStaleThreshold = 10 * time.Second

	// DefaultCollectorTimeout is the default age threshold for detecting critically old data requiring restart.
	DefaultCollectorTimeout = 20 * time.Second

	// DefaultMaxRestartAttempts is the default maximum number of collector restart attempts before panic.
	DefaultMaxRestartAttempts = 3

	// DefaultObservationInterval is the default interval between observation collection attempts.
	//
	// RECOMMENDED (Invariant I11): ObservationInterval < StaleThreshold
	// This ensures multiple observation attempts before data is marked stale.
	// Example: 1s observation interval + 10s stale threshold = ~10 attempts before stale.
	//
	// If violated: System still works but FSM may pause more frequently due to
	// perceived stale data from fewer observation attempts.
	DefaultObservationInterval = 1 * time.Second

	// MaxCgroupThrottlePeriod is the maximum CPU throttling period for Docker/Kubernetes cgroups.
	// Default cpu.cfs_period_us = 100ms. Conservative estimate: 2x for consecutive throttles.
	//
	// TIMING ASSUMPTION (Invariant I14): Timeout margin accounts for cgroup throttling
	// Docker and Kubernetes use cgroups to limit CPU usage. When a container exhausts
	// its CPU quota, it's throttled for the remainder of the cgroup period (default 100ms).
	//
	// This constant provides a 200ms buffer (2x the default period) to account for:
	// - One full throttle period (100ms)
	// - Potential consecutive throttle (another 100ms)
	// - Ensures timeouts don't fire prematurely due to CPU throttling
	//
	// Reference: https://docs.docker.com/config/containers/resource_constraints/
	MaxCgroupThrottlePeriod = 200 * time.Millisecond

	// DefaultObservationTimeout is the default timeout for observation operations.
	// Calculated as: observation interval + cgroup throttle buffer + safety margin.
	//
	// IMPORTANT: Timeout ordering invariant (I7)
	// ObservationTimeout < StaleThreshold < CollectorTimeout
	// This ordering ensures observation failures don't trigger stale detection,
	// and stale detection occurs before collector restart.
	DefaultObservationTimeout = DefaultObservationInterval + MaxCgroupThrottlePeriod + 1*time.Second

	// DefaultGracefulShutdownTimeout is the default time to wait for workers to complete
	// graceful shutdown during Shutdown(). Workers have this time to process their shutdown
	// state machines and emit SignalNeedsRemoval before forced shutdown proceeds.
	DefaultGracefulShutdownTimeout = 5 * time.Second
)
