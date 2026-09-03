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
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// defaultCPUUsagePercent reads the HOST's CPU usage through gopsutil: the first
// element of the non-per-CPU percentage slice.
//
// This is the legacy CPU path's only usage source and it runs ONLY when
// USE_FSMV2_CPU is off.
func defaultCPUUsagePercent(ctx context.Context) (float64, error) {
	usagePercentages, err := cpu.PercentWithContext(ctx, 0, false)
	if err != nil {
		return 0, err
	}

	if len(usagePercentages) > 0 {
		return usagePercentages[0], nil
	}

	return 0, nil
}

// setCPUUsageProvider overrides the CPU usage source - used for testing only.
func (c *ContainerMonitorService) setCPUUsageProvider(fn func(ctx context.Context) (float64, error)) {
	c.cpuUsageProvider = fn
}

// judgeLegacyCPUUsage degrades the instance above CPUHighThresholdPercent of
// the cores it may use, and only the legacy arm of GetStatus calls it, so it
// never runs under USE_FSMV2_CPU.
//
// getCPUMetrics already degrades on that same threshold, so this changes an
// outcome only where the two compute different core counts: a quota below 0.1
// cores, which getCPUMetrics clamps and this does not. See ENG-5384.
func judgeLegacyCPUUsage(status *ServiceInfo, cpuStat *models.CPU) {
	effectiveCores := cpuStat.CgroupCores
	if effectiveCores <= 0 && cpuStat.CoreCount != nil {
		// Fall back to host cores if cgroup info unavailable
		effectiveCores = float64(*cpuStat.CoreCount)
	}

	// A nil TotalUsageMCpu records a tick nothing measured, so there is no
	// percentage to compute.
	if effectiveCores > 0 && cpuStat.TotalUsageMCpu != nil {
		cpuPercent := (*cpuStat.TotalUsageMCpu / 1000.0) / effectiveCores * 100.0

		if cpuPercent > constants.CPUHighThresholdPercent {
			status.CPUHealth = models.Degraded
			status.OverallHealth = models.Degraded
		}
	}
}

// getCPUMetrics collects CPU metrics using gopsutil.
// By default, this retrieves host-level usage unless gopsutil is configured
// to read from container cgroup data. See notes below for cgroup-limited usage.
func (c *ContainerMonitorService) getCPUMetrics(ctx context.Context) (*models.CPU, error) {
	usageMCores, coreCount, usagePercent, err := c.getRawCPUMetrics(ctx)
	if err != nil {
		return nil, err
	}

	// Get cgroup info for throttling and limits
	cgroupInfo, cgroupErr := c.getCgroupCPUInfo(ctx)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Default to Active health
	category := models.Active
	message := "CPU utilization normal"

	// Compute windowed throttle ratio; skip entirely on cgroup read failure to preserve wasThrottled state
	var (
		windowedRatio float64
		isThrottled   bool
	)
	// Every nil-error return from getCgroupCPUInfo carries a non-nil info, so
	// cgroupErr == nil proves the pointer. getRawCPUMetrics dereferences
	// cgroupInfo on that same rule.
	//
	// The throttled-message branch below keeps an explicit nil term instead,
	// because it gates on isThrottled. That bool implies cgroupErr == nil -- it
	// is assigned only inside this block -- but the implication is transitive
	// and Nilaway cannot follow it, so dropping the term fails the analysis.
	if cgroupErr == nil {
		windowedRatio, isThrottled = c.updateThrottleWindow(cgroupInfo)
		cgroupInfo.ThrottleRatio = windowedRatio
		cgroupInfo.IsThrottled = isThrottled

		if isThrottled && !c.wasThrottled {
			c.logger.Warnf("CPU throttling detected: %.1f%% of periods throttled", cgroupInfo.ThrottleRatio*100)
		}

		c.wasThrottled = isThrottled
	}

	switch {
	case usagePercent >= constants.CPUHighThresholdPercent || isThrottled:
		category = models.Degraded

		if isThrottled && cgroupInfo != nil {
			message = fmt.Sprintf("CPU throttled (%.1f%% periods throttled)", cgroupInfo.ThrottleRatio*100)
		} else {
			message = "CPU utilization critical"
		}
	case usagePercent >= constants.CPUMediumThresholdPercent:
		message = "CPU utilization warning"
	}

	cpuStat := &models.CPU{
		Health: &models.Health{
			Message:       message,
			ObservedState: category.String(),
			DesiredState:  models.Active.String(),
			Category:      category,
		},
		TotalUsageMCpu: &usageMCores,
		CoreCount:      &coreCount,
	}

	// Add cgroup info if available
	if cgroupErr == nil {
		cpuStat.CgroupCores = cgroupInfo.QuotaCores
		cpuStat.ThrottleRatio = cgroupInfo.ThrottleRatio
		cpuStat.IsThrottled = cgroupInfo.IsThrottled
	}

	return cpuStat, nil
}

// updateThrottleWindow appends a cgroup snapshot and computes the throttle ratio
// over a sliding window defined by constants.CPUThrottleWindow.
// Returns (0.0, false) when there is insufficient data, nil input, or counter reset.
func (c *ContainerMonitorService) updateThrottleWindow(cgroupInfo *CPUCgroupInfo) (ratio float64, isThrottled bool) {
	// Guard: nil input or zero periods (cpu.stat unreadable)
	if cgroupInfo == nil || cgroupInfo.NrPeriods <= 0 {
		return 0.0, false
	}

	now := time.Now()

	// Detect counter reset: if new counters are lower than the newest snapshot,
	// the cgroup was recreated (pod rescheduled). Clear buffer and start fresh.
	if len(c.throttleSnapshots) > 0 {
		newest := c.throttleSnapshots[len(c.throttleSnapshots)-1]
		if cgroupInfo.NrPeriods < newest.nrPeriods || cgroupInfo.NrThrottled < newest.nrThrottled {
			c.throttleSnapshots = nil
		}
	}

	// Append current snapshot
	c.throttleSnapshots = append(c.throttleSnapshots, cgroupSnapshot{
		timestamp:   now,
		nrPeriods:   cgroupInfo.NrPeriods,
		nrThrottled: cgroupInfo.NrThrottled,
	})

	// Prune entries older than the window
	cutoff := now.Add(-constants.CPUThrottleWindow)

	pruneIdx := 0
	for pruneIdx < len(c.throttleSnapshots) && c.throttleSnapshots[pruneIdx].timestamp.Before(cutoff) {
		pruneIdx++
	}

	if pruneIdx > 0 {
		c.throttleSnapshots = c.throttleSnapshots[pruneIdx:]
	}

	// Need at least 2 snapshots for a delta
	if len(c.throttleSnapshots) < 2 {
		return 0.0, false
	}

	// Compute delta between newest and oldest snapshot in window
	oldest := c.throttleSnapshots[0]
	current := c.throttleSnapshots[len(c.throttleSnapshots)-1]

	deltaPeriods := current.nrPeriods - oldest.nrPeriods
	deltaThrottled := current.nrThrottled - oldest.nrThrottled

	if deltaPeriods <= 0 {
		return 0.0, false
	}

	ratio = float64(deltaThrottled) / float64(deltaPeriods)
	isThrottled = ratio > constants.CPUThrottleRatioThreshold

	return ratio, isThrottled
}

func (c *ContainerMonitorService) getRawCPUMetrics(ctx context.Context) (usageMCores float64, coreCount int, usagePercent float64, err error) {
	// Try to get cgroup info first for accurate container limits
	cgroupInfo, cgroupErr := c.getCgroupCPUInfo(ctx)
	if ctx.Err() != nil {
		return 0, 0, 0, ctx.Err()
	}

	// Get actual CPU usage through the injectable source (gopsutil by default).
	provider := c.cpuUsageProvider
	if provider == nil {
		provider = defaultCPUUsagePercent
	}
	usagePercent, err = provider(ctx)
	if err != nil {
		return 0, 0, 0, err
	}

	// Determine effective core count (keep as float64 to preserve fractional quotas)
	// Use cgroup limit if available, otherwise fall back to host CPU count
	effectiveCores := float64(runtime.NumCPU())
	if cgroupErr == nil && cgroupInfo.QuotaCores > 0 {
		// Use cgroup limit for more accurate mCPU calculation
		// QuotaCores can be fractional (e.g., 0.5 for 500m, 1.5 for 1500m)
		effectiveCores = cgroupInfo.QuotaCores
		// Use a small minimum to avoid divide-by-zero, but preserve fractional limits
		if effectiveCores < 0.1 {
			effectiveCores = 0.1
		}
	}

	coreCount = runtime.NumCPU() // Always report host cores for compatibility

	// Convert usage percent to mCPU based on effective cores
	// This gives us a more accurate representation when cgroups limit CPU
	usageCores := (usagePercent / 100.0) * effectiveCores
	usageMCores = usageCores * 1000

	return usageMCores, coreCount, usagePercent, nil
}
