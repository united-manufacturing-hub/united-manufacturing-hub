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

package container_monitor

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
)

// CPUCgroupInfo contains cgroup v2 CPU metrics including throttling information.
type CPUCgroupInfo struct {
	QuotaCores    float64 // CPU quota in cores (e.g., 2.0 = 2 cores)
	PeriodMicros  int64   // CPU period in microseconds
	NrPeriods     int64   // Total number of periods
	NrThrottled   int64   // Number of throttled periods
	ThrottledUsec int64   // Total throttled time in microseconds
	ThrottleRatio float64 // Ratio of throttled periods (0.0 to 1.0)
	IsThrottled   bool    // True if throttling detected (ratio > 0.05)
}

// parseCPUMax parses the cpu.max file content and returns quota cores and period.
func parseCPUMax(data []byte) (quotaCores float64, periodMicros int64, err error) {
	// Parse cpu.max format: "quota period" or "max period"
	parts := strings.Fields(string(data))
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("unexpected cpu.max format: %s", string(data))
	}

	// Parse period (always second field)
	periodMicros, err = strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse cpu period: %w", err)
	}

	// Parse quota (first field, can be "max" for unlimited)
	if parts[0] != "max" {
		quota, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to parse cpu quota: %w", err)
		}
		// Calculate cores from quota and period
		// quota/period gives us the fraction of CPU time allowed
		if periodMicros > 0 {
			quotaCores = float64(quota) / float64(periodMicros)
		}
	}

	return quotaCores, periodMicros, nil
}

// parseCPUStats parses the cpu.stat file content and returns throttling statistics.
func parseCPUStats(data []byte) (nrPeriods, nrThrottled, throttledUsec int64, err error) {
	// Parse cpu.stat (key-value pairs)
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}

		value, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			continue
		}

		switch parts[0] {
		case "nr_periods":
			nrPeriods = value
		case "nr_throttled":
			nrThrottled = value
		case "throttled_usec":
			throttledUsec = value
		}
	}

	return nrPeriods, nrThrottled, throttledUsec, nil
}

// getCgroupCPUInfo reads cgroup v2 CPU limits and throttling statistics.
func (c *ContainerMonitorService) getCgroupCPUInfo(ctx context.Context) (*CPUCgroupInfo, error) {
	info := &CPUCgroupInfo{}

	// Read cpu.max for quota/period
	cpuMaxPath := "/sys/fs/cgroup/cpu.max"

	// TODO paralellize with stadnard approach with errgroups both of the ReadFiles
	// potentially even using the filesystem package for it

	cpuMaxData, err := os.ReadFile(cpuMaxPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read cpu.max: %w", err)
	}

	// Parse cpu.max
	quotaCores, periodMicros, err := parseCPUMax(cpuMaxData)
	if err != nil {
		return nil, err
	}

	info.QuotaCores = quotaCores
	info.PeriodMicros = periodMicros

	// Read cpu.stat for throttling information
	cpuStatPath := "/sys/fs/cgroup/cpu.stat"

	cpuStatData, err := os.ReadFile(cpuStatPath)
	if err != nil {
		// cpu.stat might not exist in all environments
		c.logger.Debugf("Could not read cpu.stat: %v", err)

		return info, nil
	}

	// Parse cpu.stat
	nrPeriods, nrThrottled, throttledUsec, err := parseCPUStats(cpuStatData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse cpu.stat: %w", err)
	}

	info.NrPeriods = nrPeriods
	info.NrThrottled = nrThrottled
	info.ThrottledUsec = throttledUsec

	// Calculate throttle ratio
	if info.NrPeriods > 0 {
		info.ThrottleRatio = float64(info.NrThrottled) / float64(info.NrPeriods)
		// Consider throttled if throttle ratio exceeds threshold
		info.IsThrottled = info.ThrottleRatio > constants.CPUThrottleRatioThreshold
	}

	return info, nil
}
