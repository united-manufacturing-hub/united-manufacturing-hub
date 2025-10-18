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

package container

import (
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// IsFullyHealthy returns true when all container health categories are Active.
// Used by Active and Degraded states for health-based transitions.
func IsFullyHealthy(observed *ContainerObservedState) bool {
	return observed.OverallHealth == models.Active &&
		observed.CPUHealth == models.Active &&
		observed.MemoryHealth == models.Active &&
		observed.DiskHealth == models.Active
}

// BuildDegradedReason creates a detailed reason string showing which metrics are unhealthy.
// Following UX Standards: show real values, not generic messages.
func BuildDegradedReason(observed *ContainerObservedState) string {
	thresholds := observed.ObservedThresholds

	var reasons []string

	if observed.CPUHealth != models.Active {
		if observed.IsThrottled {
			throttlePercent := observed.ThrottleRatio * 100
			reasons = append(reasons, fmt.Sprintf("CPU throttled (%.1f%% periods)", throttlePercent))
		} else {
			if observed.CgroupCores > 0 {
				cpuPercent := (observed.CPUUsageMCores / 1000.0) / observed.CgroupCores * 100
				reasons = append(reasons, fmt.Sprintf("CPU at %.0f%% (threshold %.0f%%)", cpuPercent, thresholds.CPUHighPercent))
			} else {
				reasons = append(reasons, "CPU metrics unavailable")
			}
		}
	}

	if observed.MemoryHealth != models.Active {
		if observed.MemoryTotalBytes > 0 {
			memPercent := float64(observed.MemoryUsedBytes) / float64(observed.MemoryTotalBytes) * 100
			reasons = append(reasons, fmt.Sprintf("Memory at %.0f%% (threshold %.0f%%)", memPercent, thresholds.MemoryHighPercent))
		} else {
			reasons = append(reasons, "Memory metrics unavailable")
		}
	}

	if observed.DiskHealth != models.Active {
		if observed.DiskTotalBytes > 0 {
			diskPercent := float64(observed.DiskUsedBytes) / float64(observed.DiskTotalBytes) * 100
			reasons = append(reasons, fmt.Sprintf("Disk at %.0f%% (threshold %.0f%%)", diskPercent, thresholds.DiskHighPercent))
		} else {
			reasons = append(reasons, "Disk metrics unavailable")
		}
	}

	if len(reasons) == 0 {
		return "Overall health degraded"
	}

	result := ""

	for i, reason := range reasons {
		if i > 0 {
			result += ", "
		}

		result += reason
	}

	return result
}
