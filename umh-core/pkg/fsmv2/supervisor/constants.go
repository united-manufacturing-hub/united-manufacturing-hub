// Copyright 2025 UMH Systems GmbH
package supervisor

import "time"

const (
	// DefaultTickInterval is the default interval between FSM reconciliation ticks.
	DefaultTickInterval = 1 * time.Second

	// DefaultStaleThreshold is the default age threshold for detecting stale observation data.
	DefaultStaleThreshold = 10 * time.Second

	// DefaultCollectorTimeout is the default age threshold for detecting critically old data requiring restart.
	DefaultCollectorTimeout = 20 * time.Second

	// DefaultMaxRestartAttempts is the default maximum number of collector restart attempts before panic.
	DefaultMaxRestartAttempts = 3

	// DefaultObservationInterval is the default interval between observation collection attempts.
	DefaultObservationInterval = 1 * time.Second

	// MaxCgroupThrottlePeriod is the maximum CPU throttling period for Docker/Kubernetes cgroups.
	// Default cpu.cfs_period_us = 100ms. Conservative estimate: 2x for consecutive throttles.
	MaxCgroupThrottlePeriod = 200 * time.Millisecond

	// DefaultObservationTimeout is the default timeout for observation operations.
	// Calculated as: observation interval + cgroup throttle buffer + safety margin.
	DefaultObservationTimeout = DefaultObservationInterval + MaxCgroupThrottlePeriod + 1*time.Second
)
