// Copyright 2025 UMH Systems GmbH
package supervisor

import "time"

const (
	DefaultTickInterval = 1 * time.Second

	DefaultStaleThreshold = 10 * time.Second

	DefaultCollectorTimeout = 20 * time.Second

	DefaultMaxRestartAttempts = 3

	DefaultObservationInterval = 1 * time.Second
)
