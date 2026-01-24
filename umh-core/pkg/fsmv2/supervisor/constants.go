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
	// Invariant I12: TickInterval <= ObservationInterval to prevent tick queue buildup.
	DefaultTickInterval = 1 * time.Second

	// DefaultStaleThreshold is the default age threshold for detecting stale observation data.
	DefaultStaleThreshold = 10 * time.Second

	// DefaultCollectorTimeout is the default age threshold for detecting stale data requiring restart.
	DefaultCollectorTimeout = 20 * time.Second

	// DefaultMaxRestartAttempts is the default maximum number of collector restart attempts before panic.
	DefaultMaxRestartAttempts = 3

	// DefaultObservationInterval is the default interval between observation collection attempts.
	// Invariant I11: ObservationInterval < StaleThreshold so multiple attempts occur before data is marked stale.
	DefaultObservationInterval = 1 * time.Second

	// MaxCgroupThrottlePeriod is the maximum CPU throttling period for Docker/Kubernetes cgroups.
	// 200ms buffer (2x default cfs_period_us) accounts for consecutive throttles.
	// Reference: https://docs.docker.com/config/containers/resource_constraints/
	MaxCgroupThrottlePeriod = 200 * time.Millisecond

	// DefaultObservationTimeout is the default timeout for observation operations.
	// Invariant I7: ObservationTimeout < StaleThreshold < CollectorTimeout.
	DefaultObservationTimeout = DefaultObservationInterval + MaxCgroupThrottlePeriod + 1*time.Second

	// DefaultGracefulShutdownTimeout is the default time to wait for workers to complete graceful shutdown.
	DefaultGracefulShutdownTimeout = 5 * time.Second

	// DefaultGracefulRestartTimeout is how long to wait for graceful restart before force resetting.
	DefaultGracefulRestartTimeout = 30 * time.Second
)
