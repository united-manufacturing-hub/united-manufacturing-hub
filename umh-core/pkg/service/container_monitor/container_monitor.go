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
	"runtime"
	"time"

	"go.uber.org/zap"

	"encoding/hex"

	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cpuhealth"
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

// ContainerMonitorService implements the Service interface.
type ContainerMonitorService struct {
	lastCollectedAt time.Time
	fs              filesystem.Service
	sampler         cpuhealth.Sampler // cgroup cpu.stat usage_usec sampler
	logger          *zap.SugaredLogger
	windowState     *cpuhealth.WindowState // Caller-held CPU-health verdict state
	instanceName    string
	hwid            string
	architecture    models.ContainerArchitecture //nolint:unused // will be used in the future
	dataPath        string                       // Path to check for disk metrics and HWID file
	wasThrottled    bool                         // Previous throttle state for transition logging
}

// NewContainerMonitorService creates a new container monitor service instance.
func NewContainerMonitorService(fs filesystem.Service) *ContainerMonitorService {
	return NewContainerMonitorServiceWithPath(fs, constants.DataMountPath)
}

// NewContainerMonitorServiceWithPath creates a new container monitor service with a custom data path.
func NewContainerMonitorServiceWithPath(fs filesystem.Service, dataPath string) *ContainerMonitorService {
	log := logger.For(logger.ComponentContainerMonitorService)

	return &ContainerMonitorService{
		fs:           fs,
		logger:       log,
		instanceName: constants.CoreInstanceName, // Single container instance name
		dataPath:     dataPath,
		windowState:  &cpuhealth.WindowState{},
		sampler:      cpuhealth.NewCgroupSampler(fs, "/sys/fs/cgroup"),
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

	// Assess CPU health. CPUHealth/OverallHealth are Degraded ONLY when
	// cpuStat.Health is already Degraded (i.e. throttling detected in
	// getCPUMetrics). High usage alone is NOT ill health — a capped
	// container pinned at its quota is busy, not sick — so raw-usage
	// degradation is not applied here. Windowed saturation logic in
	// cpuhealth.WindowState will reintroduce usage-based degradation
	// later, gated on the dead-zone; until then the two fields stay
	// consistent with cpuStat.Health.
	if cpuStat.Health != nil && cpuStat.Health.Category == models.Degraded {
		status.CPUHealth = models.Degraded
		status.OverallHealth = models.Degraded
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

// getCPUMetrics collects CPU metrics using gopsutil.
// By default, this retrieves host-level usage unless gopsutil is configured
// to read from container cgroup data. See notes below for cgroup-limited usage.
func (c *ContainerMonitorService) getCPUMetrics(ctx context.Context) (*models.CPU, error) {
	usageMCores, coreCount, _, sample, err := c.getRawCPUMetrics(ctx)
	if err != nil {
		return nil, err
	}

	// Get cgroup info for throttling and limits
	cgroupInfo, cgroupErr := c.getCgroupCPUInfo(ctx)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Default to Active health. The curated per-cause message is composed
	// below from the verdict + signals (headline + "Technical Details:" +
	// why + what-to-do from the supertable when degraded, "CPU healthy" +
	// optional limited-visibility note when healthy), replacing the generic
	// "CPU degraded" / "CPU utilization normal" strings.
	category := models.Active

	// Compute the throttle verdict through cpuhealth.Decide's Schmitt flip-latch
	// (fires above ThrottleHigh 0.05, clears only below ThrottleRecover 0.03,
	// holds between). Decide mutates c.windowState in place, maintaining the
	// 60s throttle-counter ring. The numeric ThrottleRatio is read
	// unconditionally from signals (independent of latch state); IsThrottled
	// comes from signals.ThrottleFired. Skip entirely on cgroup read failure to
	// preserve wasThrottled state.
	var (
		isThrottled bool
		// Default to healthy so a cgroup-read failure (cgroup v1, non-container,
		// transient read error) — which skips the Decide call below — still
		// emits State="healthy" on the wire. State has no omitempty, so without
		// this default the zero-value "" would be emitted, violating the
		// always-emitted healthy|degraded contract.
		verdict = cpuhealth.Verdict{State: cpuhealth.StateHealthy}
		signals cpuhealth.Signals
	)

	if cgroupErr == nil && cgroupInfo != nil {
		// Calling c.sampler.Sample here twice re-baselines usage_usec, zeroing
		// the StealFraction/HostBusyCores deltas; reuse the Sample from
		// getRawCPUMetrics (the only c.sampler.Sample call per GetStatus)
		// instead. NrPeriods/NrThrottled are overlaid from cgroupInfo (the
		// authoritative throttle counters the sampler does not parse). On a
		// sampler failure getRawCPUMetrics returns a zero-valued Sample
		// (dead-zone: Quota nil, PSI absent, not virtualized); Decide still
		// evaluates the throttle ring from the overlaid NrPeriods/NrThrottled
		// counters, and the dead-zone saturation backstop runs harmlessly
		// (UsageCores=0 yields fraction=0). This invariant is test-pinned by the
		// "sampler failure dead-zone" spec in sampler_full_sample_wire_test.go.
		sample.Timestamp = time.Now()
		sample.NrPeriods = cgroupInfo.NrPeriods
		sample.NrThrottled = cgroupInfo.NrThrottled
		verdict, signals = cpuhealth.Decide(c.windowState, sample, cpuhealth.DefaultThresholds())
		isThrottled = signals.ThrottleFired
		// ThrottleRatio is read unconditionally from signals, decoupling the
		// numeric metric from latch state. Negatives are already clamped to 0
		// inside Decide, so the wire never sees a negative ratio. IsThrottled
		// still comes from signals.ThrottleFired (the Schmitt latch).
		cgroupInfo.ThrottleRatio = signals.ThrottleRatio
		cgroupInfo.IsThrottled = isThrottled

		if isThrottled && !c.wasThrottled {
			c.logger.Warnf("CPU throttling detected: %.1f%% of periods throttled", cgroupInfo.ThrottleRatio*100)
		}

		c.wasThrottled = isThrottled
	}

	// ComposeMessage returns the curated per-cause two-layer message (headline
	// + "Technical Details:" + why + what-to-do from the supertable) when
	// degraded, and "CPU healthy" (plus an optional limited-visibility note
	// when the sample is in the dead-zone) when healthy — replacing the
	// generic "CPU degraded" / "CPU utilization normal" strings.
	message := cpuhealth.ComposeMessage(verdict, signals)

	// CPU health is driven from verdict.State, not throttle alone. High usage
	// alone is not ill health (a capped container pinned at its quota is busy,
	// not sick), and raw-usage degradation is no longer applied in GetStatus
	// either. The verdict flows through cpuhealth.Decide's Schmitt flip-latches
	// (c.windowState); c.sampler remains live for usage reading in
	// getRawCPUMetrics.
	//
	// StateDegraded now covers throttle OR pressure (and any future cause
	// Decide returns a degraded verdict for), so category is driven from
	// verdict.State so every degradation cause flows to Degraded — not just
	// throttle. The curated per-cause message is composed above via
	// ComposeMessage(verdict, signals); isThrottled is retained for the
	// cgroupInfo.IsThrottled wire field and the wasThrottled transition log
	// above.
	if verdict.State == cpuhealth.StateDegraded && cgroupInfo != nil {
		category = models.Degraded
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
		State:          string(verdict.State),
	}

	if verdict.State == cpuhealth.StateDegraded {
		cpuStat.Attribution = string(verdict.Attribution)
		if len(verdict.Causes) > 0 {
			cpuStat.Causes = make([]models.Cause, len(verdict.Causes))
			for i, c := range verdict.Causes {
				cpuStat.Causes[i] = models.Cause{
					Kind:  models.CauseKind(c.Kind),
					Value: c.Value,
				}
			}
		}
	}

	// Add cgroup info if available
	if cgroupErr == nil {
		cpuStat.CgroupCores = cgroupInfo.QuotaCores
		cpuStat.ThrottleRatio = cgroupInfo.ThrottleRatio
		cpuStat.IsThrottled = cgroupInfo.IsThrottled
	}

	// Percentile mCPU fields are observability-only mirrors of the dead-zone
	// usage ring (signals.*UsageFraction * 1000). They are 0 when the ring
	// holds < 2 entries (outside the dead-zone, or first tick) — omitempty
	// then drops them from the wire. They do not change the verdict.
	cpuStat.AvgMCpu = signals.AvgUsageFraction * 1000
	cpuStat.P95MCpu = signals.P95UsageFraction * 1000
	cpuStat.P99MCpu = signals.P99UsageFraction * 1000

	// StealP95 (steal fraction 0-1) and PressureAvg60 (PSI some-avg60 as a
	// fraction 0-1; the sampler divides the raw kernel 0-100 percentage by 100)
	// are observability-only mirrors of the corresponding Signals fields,
	// populated unconditionally like ThrottleRatio. They are 0 (omitempty drops
	// them) when not virtualized / when PSI is unavailable. They do not change
	// the verdict.
	cpuStat.StealP95 = signals.StealP95
	cpuStat.PressureAvg60 = signals.PressureAvg60Out

	return cpuStat, nil
}

func (c *ContainerMonitorService) getRawCPUMetrics(ctx context.Context) (usageMCores float64, coreCount int, usagePercent float64, sample cpuhealth.Sample, err error) {
	// Try to get cgroup info first for accurate container limits
	cgroupInfo, cgroupErr := c.getCgroupCPUInfo(ctx)
	if ctx.Err() != nil {
		return 0, 0, 0, cpuhealth.Sample{}, ctx.Err()
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

	// Read container-relative CPU usage from the cgroup cpu.stat usage_usec
	// counter. This is the ONLY c.sampler.Sample call per GetStatus; the
	// returned Sample (carrying Quota, PressureAvg60, PsiAvailable,
	// StealFraction, Virtualized, HostBusyCores, LogicalCpus in addition to
	// UsageCores) is threaded through to the Decide call site in
	// getCPUMetrics so the delta-based signals survive. On read failure
	// (cgroup v1, non-container, transient) the error is logged at debug and
	// a zero-valued Sample is returned; a host fallback / surfaceable error
	// is not implemented yet.
	sample, err = c.sampler.Sample(ctx)
	if err != nil {
		c.logger.Debugf("cgroup cpu usage unavailable, reporting zero: %v", err)
	}

	if ctx.Err() != nil {
		return 0, 0, 0, cpuhealth.Sample{}, ctx.Err()
	}

	usageCores := sample.UsageCores
	usageMCores = usageCores * 1000

	usagePercent = (usageCores / effectiveCores) * 100.0

	return usageMCores, coreCount, usagePercent, sample, nil
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
