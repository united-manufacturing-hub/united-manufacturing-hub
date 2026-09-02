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
	"sync"
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
	useFSMv2CPU       bool                         // when true the fsmv2 CPU worker's verdict replaces the legacy CPU health; read once at construction
	cpuWorkerWarnOnce sync.Once
	cpuUsageProvider  func(ctx context.Context) (float64, error) // CPU usage source, overridable for tests; defaults to the gopsutil provider
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
		fs:               fs,
		logger:           log,
		instanceName:     constants.CoreInstanceName, // Single container instance name
		dataPath:         dataPath,
		useFSMv2CPU:      useFSMv2CPU,
		cpuUsageProvider: defaultCPUUsagePercent,
	}
}

// defaultCPUUsagePercent reads the host CPU usage through gopsutil, matching
// the legacy source: the first element of the non-per-CPU percentage slice.
//
// The legacy judgement this feeds is not dead code behind USE_FSMV2_CPU.
// ProtocolConverterService.IsResourceLimited blocks bridge creation on the CPU
// health category and message produced from this reading, and sizes the bridge
// ceiling from CPU.CgroupCores. The legacy path stays until admission reads the
// fsmv2 evidence instead, which is ENG-5265.
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

	fsmv2Judged := c.applyFSMv2CPUVerdict(ctx, cpuStat)

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

	// Assess CPU health. Whoever judged this tick has written its verdict onto
	// cpuStat.Health; the legacy usage rule runs only when nobody did.
	if cpuStat.Health != nil && cpuStat.Health.Category == models.Degraded {
		status.CPUHealth = models.Degraded
		status.OverallHealth = models.Degraded
	} else if !fsmv2Judged {
		judgeLegacyCPUUsage(status, cpuStat)
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

// judgeLegacyCPUUsage is the CPU rule that ran before the fsmv2 CPU worker
// existed: degrade the instance above CPUHighThresholdPercent of the cores it
// may use. GetStatus calls it only when the worker did not judge this tick,
// which is every tick when USE_FSMV2_CPU is off.
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

// applyFSMv2CPUVerdict lets the fsmv2 CPU worker judge this tick's CPU health,
// and reports whether it did. When it reports true the worker is the only judge:
// GetStatus runs none of its legacy CPU rules, including throttling, which the
// worker assesses itself.
//
// It reports false when USE_FSMV2_CPU was off at construction, or when the fsmv2
// runtime is not answering at all - no client published, the worker ref never
// registered, or a fresh observation that produced no verdict. Those are the
// only cases where the legacy rules still decide.
//
// The worker's own windows need up to a minute to fill, so a box that is busy or
// throttled inside that minute reports Active where the legacy rules degraded
// it. That gap is accepted: the flag means the worker judges, and a worker that
// has not measured yet has nothing to say.
func (c *ContainerMonitorService) applyFSMv2CPUVerdict(ctx context.Context, cpuStat *models.CPU) bool {
	if !c.useFSMv2CPU {
		return false
	}

	workerHealth, workerCPUHealth, measured := c.readWorkerCPUHealth(ctx)
	if workerHealth == nil {
		return false
	}

	cpuStat.Health = workerHealth
	cpuStat.CPUHealth = workerCPUHealth

	// The legacy figures come from a tick the worker did not measure, so they
	// would sit beside a verdict drawn from different numbers.
	degradedWithoutMeasurement := workerHealth.Category == models.Degraded && !measured
	if degradedWithoutMeasurement {
		cpuStat.TotalUsageMCpu = nil
		cpuStat.CoreCount = nil
	}

	return true
}

// readWorkerCPUHealth reads the fsmv2 CPU worker's observation and maps it to a
// models.Health. A nil health means the caller must keep the legacy
// getCPUMetrics health: no client is reachable, the ref was never registered, or
// a fresh tick produced no verdict.
//
// Freshness is judged before the verdict. A stale, never-observed or unreadable
// observation fails closed to Degraded, because an old or absent measurement is
// not a healthy one, and the protocol-converter resource-limit check
// (IsResourceLimited) uses the message as its bridge-block reason.
//
// cpuHealth is the verdict beside the Details it judged. It is non-nil only on
// the two arms that measured, so an unmeasured tick omits the wire key instead
// of shipping an empty one.
//
// measured reports whether the health rests on a real measurement. The caller
// reads it to decide whether the legacy numeric fields describe the same
// measurement the verdict judged and may be kept, or an absent one it must
// omit.
func (c *ContainerMonitorService) readWorkerCPUHealth(ctx context.Context) (health *models.Health, cpuHealth *models.CPUHealth, measured bool) {
	client := fsmv2client.GetClient()
	if client == nil {
		c.cpuWorkerWarnOnce.Do(func() {
			c.logger.Warn(c.cpuSeamClientUnavailableMessage())
		})

		return nil, nil, false
	}

	// Read the simple.Status wrapper, never the bare CPUStatus: a Poll error
	// does not surface as a GetFresh error. The simple worker persists
	// Status{Degraded: true, Reason: "poll error: ..."} with a nil error and a
	// zero result verdict, and decoding that flat JSON into CPUStatus alone
	// drops the Degraded/Reason keys, so the worker's "cannot measure"
	// declaration would never reach the verdict mapping. GetFresh maps the
	// stored observation to a Freshness reason; a read error returns Unknown
	// alongside the verbatim error, so check err before reading Freshness or
	// the status, which are only meaningful when err is nil.
	workerStatus, freshness, err := fsmv2client.GetFresh[simple.Status[fsmv2cpu.CPUStatus]](ctx, client, fsmv2cpu.Ref, cpuWorkerMaxAge)
	if err != nil {
		// Unknown: the read failure prevented the observation from being
		// classified, so it cannot be called healthy. Fail closed with the
		// verbatim store error as the message.
		return &models.Health{
			Message:       fmt.Sprintf("CPU worker observation could not be read: %v", err),
			ObservedState: models.Degraded.String(),
			DesiredState:  models.Active.String(),
			Category:      models.Degraded,
		}, nil, false
	}

	// A non-Fresh observation without a read error is Stale, NeverObserved, or
	// Unregistered. Stale and NeverObserved are absences of a usable measurement
	// and fail closed; Unregistered (the ref was never Upserted) is the
	// absence-of-worker row and falls back to legacy, not a degrade.
	if freshness != fsmv2client.Fresh {
		switch freshness {
		case fsmv2client.Stale:
			return &models.Health{
				Message:       fmt.Sprintf("CPU worker observation is stale (older than %s); cannot trust the verdict it carries", cpuWorkerMaxAge),
				ObservedState: models.Degraded.String(),
				DesiredState:  models.Active.String(),
				Category:      models.Degraded,
			}, nil, false
		case fsmv2client.NeverObserved:
			return &models.Health{
				Message:       "CPU worker has never observed; no measurement to judge",
				ObservedState: models.Degraded.String(),
				DesiredState:  models.Active.String(),
				Category:      models.Degraded,
			}, nil, false
		default:
			return nil, nil, false
		}
	}

	// The framework Degraded flag carries two states, and the result verdict
	// says which. healthFromStatus degrades the wrapper on every good degraded
	// poll, so a degraded verdict state means the worker measured fine and
	// judged the box degraded: that case falls through to the verdict switch
	// below, which reports it measured and ships the evidence. The flag
	// behind any other verdict state is a "could not measure" (a failed poll
	// leaves the verdict empty): map to Degraded with Status.Reason as the
	// message.
	if workerStatus.Degraded && workerStatus.Result.Verdict.State != cpuhealth.StateDegraded {
		return &models.Health{
			Message:       workerStatus.Reason,
			ObservedState: models.Degraded.String(),
			DesiredState:  models.Active.String(),
			Category:      models.Degraded,
		}, nil, false
	}

	// A Fresh observation carries the developer's judgement in Result. The
	// switch maps only the two spelled-out states, so a rename of either
	// fails the seam tests too.
	switch workerStatus.Result.Verdict.State {
	case cpuhealth.StateHealthy:
		return &models.Health{
				Message:       workerStatus.Result.Message,
				ObservedState: models.Active.String(),
				DesiredState:  models.Active.String(),
				Category:      models.Active,
			}, &models.CPUHealth{
				Verdict: workerStatus.Result.Verdict,
				Details: workerStatus.Result.Details,
			}, true
	case cpuhealth.StateDegraded:
		return &models.Health{
				Message:       workerStatus.Result.Message,
				ObservedState: models.Degraded.String(),
				DesiredState:  models.Active.String(),
				Category:      models.Degraded,
			}, &models.CPUHealth{
				Verdict: workerStatus.Result.Verdict,
				Details: workerStatus.Result.Details,
			}, true
	default:
		// Empty result verdict AND Degraded == false is a genuine "no
		// determination" — a successful poll produced no verdict. Keep the
		// legacy health; do not read it as healthy.
		return nil, nil, false
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
