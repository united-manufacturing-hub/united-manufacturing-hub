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
	"crypto/rand"
	"crypto/sha3"
	"fmt"
	"os"
	"runtime"
	"time"

	"go.uber.org/zap"

	"encoding/hex"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/env"
	fsmv2cpu "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/cpu"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/simple"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
)

// ServiceInfo contains both raw metrics and health assessments.
type ServiceInfo struct {
	// Raw metrics (keeping same structure for compatibility)
	CPU    *models.CPU    // Keep existing CPU metrics
	Memory *models.Memory // Keep existing Memory metrics
	Disk   *models.Disk   // Keep existing Disk metrics

	// Existing fields
	Hwid         string
	Architecture models.ContainerArchitecture

	// Health assessments using existing models.HealthCategory
	OverallHealth models.HealthCategory
	CPUHealth     models.HealthCategory
	MemoryHealth  models.HealthCategory
	DiskHealth    models.HealthCategory
}

// Service defines the interface for container monitoring.
type Service interface {
	// GetStatus returns container metrics with health assessments
	GetStatus(ctx context.Context) (*ServiceInfo, error)
}

// cgroupSnapshot stores cgroup CPU counters at a point in time for sliding window calculation.
type cgroupSnapshot struct {
	timestamp   time.Time
	nrPeriods   int64
	nrThrottled int64
}

// ContainerMonitorService implements the Service interface.
type ContainerMonitorService struct {
	fs                filesystem.Service
	logger            *zap.SugaredLogger
	instanceName      string
	lastCollectedAt   time.Time
	hwid              string
	architecture      models.ContainerArchitecture //nolint:unused // will be used in the future
	dataPath          string                       // Path to check for disk metrics and HWID file
	throttleSnapshots []cgroupSnapshot             // Sliding window of cgroup counter snapshots
	wasThrottled      bool                         // Previous throttle state for transition logging
	useFSMv2CPU       bool                         // USE_FSMV2_CPU read once at construction; gates the fsmv2 CPU worker seam
	cpuWorkerWarned   bool                         // One-time latch: the flag-on-but-no-client warning fires once, never per tick
}

// NewContainerMonitorService creates a new container monitor service instance.
func NewContainerMonitorService(fs filesystem.Service) *ContainerMonitorService {
	return NewContainerMonitorServiceWithPath(fs, constants.DataMountPath)
}

// NewContainerMonitorServiceWithPath creates a new container monitor service with a custom data path.
func NewContainerMonitorServiceWithPath(fs filesystem.Service, dataPath string) *ContainerMonitorService {
	log := logger.For(logger.ComponentContainerMonitorService)

	useFSMv2CPU, _ := env.GetAsBool("USE_FSMV2_CPU", false, false)

	return &ContainerMonitorService{
		fs:           fs,
		logger:       log,
		instanceName: constants.CoreInstanceName, // Single container instance name
		dataPath:     dataPath,
		useFSMv2CPU:  useFSMv2CPU,
	}
}

// GetFilesystemService returns the filesystem service - used for testing only.
func (c *ContainerMonitorService) GetFilesystemService() filesystem.Service {
	return c.fs
}

// SetDataPath changes the data path - used for testing only.
func (c *ContainerMonitorService) SetDataPath(path string) {
	c.dataPath = path
}

// GetStatus collects and returns the current container metrics.
func (c *ContainerMonitorService) GetStatus(ctx context.Context) (*ServiceInfo, error) {
	// Create a new status with default health (Active)
	status := &ServiceInfo{
		CPUHealth:     models.Active,
		MemoryHealth:  models.Active,
		DiskHealth:    models.Active,
		OverallHealth: models.Active,
		Hwid:          c.hwid,
		Architecture:  models.ContainerArchitecture(runtime.GOARCH),
	}

	// Get CPU stats
	cpuStat, err := c.getCPUMetrics(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get CPU metrics: %w", err)
	}

	status.CPU = cpuStat

	// CPU seam: with USE_FSMV2_CPU on at construction and a Fresh, non-empty
	// verdict from the fsmv2 CPU worker, that verdict fills status.CPU.Health.
	// The override is skipped only for a box the legacy path has judged
	// throttle-degraded this tick: that throttle record is what the aggregate
	// check below reads, and the usage-% rule below does not recompute it, so a
	// healthy worker verdict must not erase it (a genuinely throttled box would
	// otherwise fall through to the usage rule and can report Active). A
	// usage-degraded legacy record is safe to override: the usage-% rule
	// recomputes the same verdict from the raw numbers, so no degraded report
	// is masked there. A Fresh observation carrying no recognisable verdict, or
	// one that is not Fresh, does not reach the verdict mapping this rung (the
	// freshness direction is the next rung's own, not pinned here). A healthy
	// verdict does not stop the legacy CPU-usage rule below from re-judging
	// status.CPUHealth (over-degrading is the known, fail-safe interim until
	// that rule is gated). The flag is read once at construction, so a later
	// toggle does not move the seam.
	if c.useFSMv2CPU {
		if workerHealth, ok := c.readWorkerCPUHealth(ctx); ok && !cpuStat.IsThrottled {
			status.CPU.Health = workerHealth
		}
	}

	// Get memory stats
	memStat, err := c.getMemoryMetrics(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get memory metrics: %w", err)
	}

	status.Memory = memStat

	// Get disk stats
	diskStat, err := c.getDiskMetrics(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get disk metrics: %w", err)
	}

	status.Disk = diskStat

	// Get hardware info
	hwid, err := c.getHWID(ctx)
	if err != nil {
		c.logger.Error("Failed to get hardware ID", zap.Error(err))
		// Use empty string as fallback
		hwid = ""
	}

	status.Hwid = hwid

	// Update last collected timestamp
	c.lastCollectedAt = time.Now()

	// Assess CPU health
	// Check if CPU is already marked as degraded (e.g., due to throttling)
	if cpuStat.Health != nil && cpuStat.Health.Category == models.Degraded {
		status.CPUHealth = models.Degraded
		status.OverallHealth = models.Degraded
	} else {
		// Calculate CPU percentage against effective cores (cgroup limit if available)
		//
		// NOTE: CPU percentage is fundamentally misleading for understanding performance:
		// 1. In containers, throttling matters more than usage percentage
		// 2. CPU % doesn't scale linearly due to hyperthreading, turbo boost, etc.
		// 3. Users need to know throttling status, not just usage
		//
		// See ENG-3423 for planned improvements to show mCPU instead of percentage
		// See https://www.brendanlong.com/cpu-utilization-is-a-lie.html for why CPU % is misleading
		//
		// We maintain percentage calculation for API compatibility, but throttling
		// detection (handled elsewhere) is the more important health signal.
		effectiveCores := cpuStat.CgroupCores
		if effectiveCores <= 0 {
			// Fall back to host cores if cgroup info unavailable
			effectiveCores = float64(cpuStat.CoreCount)
		}

		if effectiveCores > 0 {
			cpuPercent := (cpuStat.TotalUsageMCpu / 1000.0) / effectiveCores * 100.0

			if cpuPercent > constants.CPUHighThresholdPercent {
				status.CPUHealth = models.Degraded
				status.OverallHealth = models.Degraded
			}
		}
	}

	// Assess memory health
	if memStat.CGroupTotalBytes > 0 {
		memPercent := float64(memStat.CGroupUsedBytes) / float64(memStat.CGroupTotalBytes) * 100.0

		if memPercent > constants.MemoryHighThresholdPercent {
			status.MemoryHealth = models.Degraded
			status.OverallHealth = models.Degraded
		}
	}

	// Assess disk health
	if diskStat.DataPartitionTotalBytes > 0 {
		diskPercent := float64(diskStat.DataPartitionUsedBytes) / float64(diskStat.DataPartitionTotalBytes) * 100.0

		if diskPercent > constants.DiskHighThresholdPercent {
			status.DiskHealth = models.Degraded
			status.OverallHealth = models.Degraded
		}
	}

	// Record metrics
	RecordContainerStatus(status, c.instanceName)

	return status, nil
}

// GetHealth returns the health status of the container based on current metrics.
func (c *ContainerMonitorService) GetHealth(ctx context.Context) (*models.Health, error) {
	status, err := c.GetStatus(ctx)
	if err != nil {
		return nil, err
	}

	// Create a Health object from the ContainerStatus
	health := &models.Health{
		Category:      status.OverallHealth,
		ObservedState: status.OverallHealth.String(),
		DesiredState:  models.Active.String(),
	}

	// Generate an appropriate message
	if status.OverallHealth == models.Degraded {
		var message string

		switch {
		case status.CPUHealth == models.Degraded:
			message = "CPU metrics degraded"
		case status.MemoryHealth == models.Degraded:
			message = "Memory metrics degraded"
		case status.DiskHealth == models.Degraded:
			message = "Disk metrics degraded"
		default:
			message = "One or more metrics degraded"
		}

		health.Message = message
	} else {
		health.Message = "Container is operating normally"
	}

	return health, nil
}

// cpuWorkerMaxAge is how old the fsmv2 CPU worker's observation may be and still
// count as Fresh for the seam. It is 3x the worker's 1s poll interval
// (pollInterval in pkg/fsmv2/cpu), so one slow or missed poll cannot flip the
// seam to the legacy path.
const cpuWorkerMaxAge = 3 * time.Second

// readWorkerCPUHealth reads the fsmv2 CPU worker's Fresh observation and maps a
// real verdict to a models.Health. The second return is false when the caller
// must keep the legacy getCPUMetrics health: no client is reachable (warned
// once per service lifetime, never per tick), the observation is not Fresh (the
// freshness direction is a later rung's own), or the tick left no determination
// and no framework degraded declaration. The stored observation is
// simple.Status[CPUStatus] — the developer's poll result plus the framework
// Degraded/Reason verdict — and the framework verdict maps first: a Degraded
// observation is the worker declaring it cannot measure (SPEC §2.7), and its
// Reason is the health message. A healthy verdict maps to Active and is still
// the worker's judgement: it does not silence the legacy CPU-usage rule that
// re-judges status.CPUHealth later.
func (c *ContainerMonitorService) readWorkerCPUHealth(ctx context.Context) (*models.Health, bool) {
	client := fsmv2client.GetClient()
	if client == nil {
		if !c.cpuWorkerWarned {
			c.cpuWorkerWarned = true
			c.logger.Warn(c.cpuSeamClientUnavailableMessage())
		}

		return nil, false
	}

	// Read the simple.Status wrapper, never the bare CPUStatus: a Poll error
	// does not surface as a GetFresh error. The simple worker persists
	// Status{Degraded: true, Reason: "poll error: ..."} with a nil error and a
	// zero result verdict, and decoding that flat JSON into CPUStatus alone
	// drops the Degraded/Reason keys, so the worker's "cannot measure"
	// declaration would never reach the verdict mapping.
	workerStatus, freshness, err := fsmv2client.GetFresh[simple.Status[fsmv2cpu.CPUStatus]](ctx, client, fsmv2cpu.Ref, cpuWorkerMaxAge)
	if err != nil || freshness != fsmv2client.Fresh {
		return nil, false
	}

	// The framework verdict wins: Degraded means the worker could not measure
	// (the poll-error case arrives Fresh with a nil error), so the result
	// verdict — empty when the poll failed — is meaningless. Map to Degraded
	// with Status.Reason as the message.
	if workerStatus.Degraded {
		return &models.Health{
			Message:       workerStatus.Reason,
			ObservedState: models.Degraded.String(),
			DesiredState:  models.Active.String(),
			Category:      models.Degraded,
		}, true
	}

	// A Fresh observation whose framework verdict is not degraded carries the
	// developer's judgement in Result. The switch maps only the two spelled-out
	// states, so a rename of either fails the seam tests too.
	switch cpuhealth.State(workerStatus.Result.Verdict) {
	case cpuhealth.StateHealthy:
		return &models.Health{
			Message:       workerStatus.Result.Message,
			ObservedState: models.Active.String(),
			DesiredState:  models.Active.String(),
			Category:      models.Active,
		}, true
	case cpuhealth.StateDegraded:
		return &models.Health{
			Message:       workerStatus.Result.Message,
			ObservedState: models.Degraded.String(),
			DesiredState:  models.Active.String(),
			Category:      models.Degraded,
		}, true
	default:
		// Empty result verdict AND Degraded == false is a genuine "no
		// determination" — a successful poll produced no verdict. Keep the
		// legacy health; do not read it as healthy.
		return nil, false
	}
}

// cpuSeamClientUnavailableMessage names the prerequisite that stopped the fsmv2
// supervisor from publishing the CPU worker client, so the once-per-lifetime
// warning reads as a diagnosis rather than "no client". The CPU monitor child
// only exists inside the fsmv2 supervisor tree (gated by USE_FSMV2_TRANSPORT),
// which is never built without backend credentials (API_URL and AUTH_TOKEN).
func (c *ContainerMonitorService) cpuSeamClientUnavailableMessage() string {
	transportOn, _ := env.GetAsBool("USE_FSMV2_TRANSPORT", false, true)
	if !transportOn {
		return "USE_FSMV2_CPU is enabled but USE_FSMV2_TRANSPORT is off, so the fsmv2 supervisor never runs and no CPU worker client is published; falling back to legacy CPU metrics"
	}

	if os.Getenv("API_URL") == "" || os.Getenv("AUTH_TOKEN") == "" {
		return "USE_FSMV2_CPU is enabled but API_URL or AUTH_TOKEN is unset, so the fsmv2 supervisor never runs and no CPU worker client is published; falling back to legacy CPU metrics"
	}

	return "USE_FSMV2_CPU is enabled but no fsmv2 client is reachable yet (the fsmv2 supervisor may still be starting); falling back to legacy CPU metrics"
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
	if cgroupErr == nil && cgroupInfo != nil {
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
		TotalUsageMCpu: usageMCores,
		CoreCount:      coreCount,
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

	// Get actual CPU usage
	usagePercentages, err := cpu.PercentWithContext(ctx, 0, false)
	if err != nil {
		return 0, 0, 0, err
	}

	if len(usagePercentages) > 0 {
		usagePercent = usagePercentages[0]
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

// getMemoryMetrics collects memory metrics, preferring cgroup values when available.
// Falls back to host-level gopsutil values in non-container environments.
func (c *ContainerMonitorService) getMemoryMetrics(ctx context.Context) (*models.Memory, error) {
	vmStat, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return nil, err
	}

	usedBytes := vmStat.Used
	totalBytes := vmStat.Total

	// Try cgroup values: prefer container-aware limits over host values
	cgroupInfo, cgroupErr := c.getCgroupMemoryInfo(ctx)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	if cgroupErr == nil {
		usedBytes = uint64(cgroupInfo.CurrentBytes)
		if !cgroupInfo.Unlimited && cgroupInfo.LimitBytes > 0 {
			// Only override totalBytes when a cgroup limit is set.
			// When unlimited, keep host total (same approach as CPU with unlimited quota).
			totalBytes = uint64(cgroupInfo.LimitBytes)
		}
	} else {
		c.logger.Debugf("cgroup memory info unavailable, using host values: %v", cgroupErr)
	}

	// Default to Active health
	category := models.Active
	message := "Memory utilization normal"

	memPercent := float64(usedBytes) / float64(totalBytes) * 100.0
	if memPercent >= constants.MemoryHighThresholdPercent {
		category = models.Degraded
		message = "Memory utilization critical"
	} else if memPercent >= constants.MemoryMediumThresholdPercent {
		// Still Active but with a warning message
		message = "Memory utilization warning"
	}

	memStat := &models.Memory{
		Health: &models.Health{
			Message:       message,
			ObservedState: category.String(),
			DesiredState:  models.Active.String(),
			Category:      category,
		},
		CGroupUsedBytes:  int64(usedBytes),
		CGroupTotalBytes: int64(totalBytes),
	}

	return memStat, nil
}

// oneTB represents one terabyte in bytes.
const oneTB uint64 = 1024 * 1024 * 1024 * 1024

// getDiskMetrics collects disk usage metrics using gopsutil for the data path.
// It applies a special handling for Docker Desktop on macOS, where the underlying
// Linux VM (using LinuxKit) may report an unrealistic disk size (e.g. > 10TB) due to
// block size translation issues.
func (c *ContainerMonitorService) getDiskMetrics(ctx context.Context) (*models.Disk, error) {
	// Start with gopsutil as the default approach for consistency.
	usageStat, err := disk.UsageWithContext(ctx, c.dataPath)
	if err != nil {
		return nil, err
	}

	usedBytes := usageStat.Used
	totalBytes := usageStat.Total

	// If the total reported size is greater than 10TB and we are on Docker Desktop on macOS,
	// then it is likely we are observing the known block-size inflation issue.
	if IsDockerDesktopMac() && totalBytes > 10*oneTB {
		// Use the macOS-adjusted approach as a fallback.
		correctedUsed, correctedTotal, err := c.getMacOSAdjustedDiskMetrics()
		if err == nil {
			usedBytes = correctedUsed
			totalBytes = correctedTotal
		} else {
			return nil, fmt.Errorf("failed to get macOS-adjusted disk metrics: %w", err)
		}
	}

	// Determine health status based on disk usage thresholds.
	category := models.Active
	message := "Disk utilization normal"

	diskPercent := float64(usedBytes) / float64(totalBytes) * 100.0
	if diskPercent >= constants.DiskHighThresholdPercent {
		category = models.Degraded
		message = "Disk utilization critical"
	} else if diskPercent >= constants.DiskMediumThresholdPercent {
		// Still Active but with a warning message.
		message = "Disk utilization warning"
	}

	diskStat := &models.Disk{
		Health: &models.Health{
			Message:       message,
			ObservedState: category.String(),
			DesiredState:  models.Active.String(),
			Category:      category,
		},
		DataPartitionUsedBytes:  int64(usedBytes),
		DataPartitionTotalBytes: int64(totalBytes),
	}

	return diskStat, nil
}

// getHWID gets the hardware ID from system.
func (c *ContainerMonitorService) getHWID(ctx context.Context) (string, error) {
	// Try to read from the hardware ID file
	hwidPath := c.dataPath + "/hwid"

	exists, err := c.fs.FileExists(ctx, hwidPath)
	if err != nil {
		return "", WrapMetricsError(ErrHWIDCollection, "error checking if HWID file exists")
	}

	if exists {
		data, err := c.fs.ReadFile(ctx, hwidPath)
		if err != nil {
			return "", WrapMetricsError(ErrHWIDCollection, "error reading HWID file")
		}

		return string(data), nil
	}

	// File doesn't exist, create a new one with a random hash
	hwid, err := c.generateNewHWID(ctx)
	if err != nil {
		c.logger.Error("Failed to generate new HWID", zap.Error(err))
		// Fallback to static ID if generation fails
		return "hwid-12345", nil
	}

	return hwid, nil
}

// generateNewHWID creates a new hardware ID file with a random hash.
func (c *ContainerMonitorService) generateNewHWID(ctx context.Context) (string, error) {
	// Ensure the data directory exists
	err := c.fs.EnsureDirectory(ctx, c.dataPath)
	if err != nil {
		return "", WrapMetricsError(ErrHWIDCollection, "error ensuring data directory exists")
	}

	// Generate 1024 bytes of random data
	buffer := make([]byte, 1024)

	_, err = rand.Read(buffer)
	if err != nil {
		return "", WrapMetricsError(ErrHWIDCollection, "error generating random data")
	}

	// Create a SHA3-256 hash
	hash := sha3.New256()
	_, _ = hash.Write(buffer)
	hwid := hex.EncodeToString(hash.Sum(nil))

	// Write the hash to the file
	hwidPath := c.dataPath + "/hwid"

	err = c.fs.WriteFile(ctx, hwidPath, []byte(hwid), 0644)
	if err != nil {
		return "", WrapMetricsError(ErrHWIDCollection, "error writing HWID file")
	}

	return hwid, nil
}
