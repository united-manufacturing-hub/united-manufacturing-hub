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
	CPUHighThresholdPercent   = 90.0 // Degraded if above this
	CPUMediumThresholdPercent = 70.0 // Warning level (but still Active)

	MemoryHighThresholdPercent   = 90.0 // Degraded if above this
	MemoryMediumThresholdPercent = 80.0 // Warning level (but still Active)

	DiskHighThresholdPercent   = 90.0 // Degraded if above this
	DiskMediumThresholdPercent = 80.0 // Warning level (but still Active)
)
