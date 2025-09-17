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

package constants

import "time"

// Container monitor constants.
const (
	// Hardware information sources.
	HWIDFilePath  = "/data/hwid"
	DataMountPath = "/data"
)

// Container Operation Timeouts - Level 1 Service (depends on S6)
// ContainerUpdateObservedStateTimeout is the timeout for updating the observed state.
const ContainerUpdateObservedStateTimeout = 10 * time.Millisecond

// Health assessment thresholds.
const (
	CPUHighThresholdPercent   = 70.0 // Degraded if above this - lowered from 90% to prevent throttling
	CPUMediumThresholdPercent = 60.0 // Warning level (but still Active)

	MemoryHighThresholdPercent   = 80.0 // Degraded if above this - lowered from 90% for earlier detection
	MemoryMediumThresholdPercent = 70.0 // Warning level (but still Active)

	DiskHighThresholdPercent   = 85.0 // Degraded if above this - lowered from 90% for safety margin
	DiskMediumThresholdPercent = 75.0 // Warning level (but still Active)
)

// Resource scaling limits.
const (
	// MaxBridgesPerCPUCore defines the maximum number of protocol converter bridges per CPU core.
	// This limit ensures stable performance and prevents resource exhaustion.
	// See docs/production/sizing-guide.md for more details on resource planning.
	MaxBridgesPerCPUCore = 5

	// CPUThrottleRatioThreshold defines when CPU throttling is considered significant.
	// If more than 5% of periods are throttled, the CPU is considered throttled.
	CPUThrottleRatioThreshold = 0.05
)
