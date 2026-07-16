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
	// wasThrottled is the previous throttle state for onset-transition
	// logging. Updated only on successful sampler ticks: a failure tick
	// carries no throttle reading, and resetting the flag there would re-fire
	// the onset Warn on the next successful still-throttled tick.
	wasThrottled bool
	// inOutage tracks whether the sampler is currently failing, so the
	// outage Warn fires once on the transition INTO the outage (with the
	// sampler error as cause) and recovery is logged once at Info, instead
	// of warning on every failure tick.
	inOutage bool
	// lastVerdict, lastSignals, lastVerdictBasis, lastCgroupCores,
	// lastTotalUsageMCpu, and hasLastVerdict hold the most recent successful
	// sampler tick's verdict state so a sampler-failure tick can re-emit it
	// on the wire instead of flapping to healthy.
	//
	// Hold semantics during a sustained sampler outage:
	//
	//   - The hold is indefinite (no max-hold window). This matches the
	//     hostBusyRing and stealRing hold-on-missing discipline: the ring
	//     prune runs inside cpuhealth.Decide, which the sampler-failure branch
	//     skips, so the rings do not age out during a sustained outage either.
	//   - A held stale-degraded verdict blocks bridge creation via
	//     ProtocolConverterService.IsResourceLimited for the duration of the
	//     outage. This is the conservative choice: block rather than admit a
	//     new bridge to a possibly-degraded host. The sampler is down, so the
	//     current host state is unknowable.
	//   - Recovery: the first successful sampler tick recomputes the
	//     verdict, basis, signals, and CgroupCores fresh from the new
	//     sample.
	lastVerdict        cpuhealth.Verdict
	lastSignals        cpuhealth.Signals
	lastVerdictBasis   *models.VerdictBasis
	lastCgroupCores    float64
	lastTotalUsageMCpu float64
	hasLastVerdict     bool
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

// NewContainerMonitorServiceWithSampler creates a new container monitor
// service with a custom data path and an injected CPU sampler, for tests that
// script the sampler's Sample output directly. The sampler is fixed at
// construction; there is no setter, so it cannot change mid-lifecycle.
func NewContainerMonitorServiceWithSampler(fs filesystem.Service, dataPath string, sampler cpuhealth.Sampler) *ContainerMonitorService {
	svc := NewContainerMonitorServiceWithPath(fs, dataPath)
	svc.sampler = sampler

	return svc
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

	// Assess CPU health: CPUHealth/OverallHealth mirror cpuStat.Health,
	// which getCPUMetrics drives from the cpuhealth verdict. High usage
	// alone is not ill health (a capped container pinned at its quota is
	// busy, not sick), so no raw-usage degradation is applied here.
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
	usageMCores, coreCount, sample, samplerOK, samplerErr, err := c.getRawCPUMetrics(ctx)
	if err != nil {
		return nil, err
	}

	// Default to Active health.
	category := models.Active

	// Run the sample through cpuhealth.Decide, gated on sampler success: a
	// sampler failure means cpu.stat was unreadable, and running Decide on
	// the zero-valued sample would clear all saturation sub-latches,
	// flapping a prior degraded verdict to healthy for the failure tick.
	// Instead, hold the last verdict and signals; the windowState latches
	// also hold, since Decide is their only mutator.
	var (
		// Default to healthy so a sampler failure on the first tick (no prior
		// verdict to hold) still emits State="healthy" on the wire. State has
		// no omitempty, so without this default the zero-value "" would be
		// emitted, violating the always-emitted healthy|degraded contract.
		verdict = cpuhealth.Verdict{State: cpuhealth.StateHealthy}
		signals cpuhealth.Signals
	)

	if samplerOK {
		if c.inOutage {
			c.inOutage = false
			c.logger.Infof("cgroup cpu usage readable again, verdict recomputed from fresh samples")
		}

		sample.Timestamp = time.Now()
		verdict, signals = cpuhealth.Decide(c.windowState, sample, cpuhealth.DefaultThresholds())

		// Throttle onset logging and its transition flag live on the
		// successful path only: a failure tick has no throttle reading, and
		// resetting wasThrottled there would re-fire the onset Warn on the
		// next successful still-throttled tick.
		if signals.ThrottleFired && !c.wasThrottled {
			c.logger.Warnf("CPU throttling detected: %.1f%% of periods throttled", signals.ThrottleRatio*100)
		}

		c.wasThrottled = signals.ThrottleFired

		c.lastVerdict = verdict
		c.lastSignals = signals
		c.lastTotalUsageMCpu = usageMCores
		c.hasLastVerdict = true
	} else {
		// Warn once on the transition into the outage, with the sampler
		// error as the cause; every further failure tick is silent (the
		// recovery is logged at Info above).
		if !c.inOutage {
			c.inOutage = true
			c.logger.Warnf("cgroup cpu usage unavailable, holding last verdict: %v", samplerErr)
		}

		if c.hasLastVerdict {
			verdict = c.lastVerdict
			signals = c.lastSignals
			// Hold TotalUsageMCpu like CgroupCores: the zero sample would
			// emit 0 next to the held nonzero AvgMCpu/P95MCpu/P99MCpu, a
			// self-contradictory wire (zero instantaneous usage under held
			// nonzero averages).
			usageMCores = c.lastTotalUsageMCpu
		}
	}

	message := cpuhealth.ComposeMessage(verdict, signals)

	// Category follows verdict.State so every degradation cause flows to
	// Degraded, not just throttle. High usage alone is not ill health: a
	// capped container pinned at its quota is busy, not sick.
	if verdict.State == cpuhealth.StateDegraded {
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

	// CgroupCores carries the cgroup CPU quota (cores) on the wire and keeps
	// its omitempty-on-0 semantics: a nil Quota (cpu.max unreadable) or a
	// zero Quota (cpu.max = "max", uncapped) yields CgroupCores=0, which
	// omitempty drops from the wire.
	if sample.Quota != nil && *sample.Quota > 0 {
		cpuStat.CgroupCores = *sample.Quota
	}

	// Percentile mCPU fields are observability-only mirrors of the usage
	// ring (signals.*UsageCores * 1000). They are *float64: non-nil (even
	// when pointing at 0) once signals.UsageRingActive is true, nil on the
	// first ticks before the ring holds 2 entries. The flag distinguishes a
	// real 0 from an absent signal, which a value-based 0/omitempty
	// discipline cannot. They do not change the verdict.
	if signals.UsageRingActive {
		cpuStat.AvgMCpu = ptr(signals.AvgUsageCores * 1000)
		cpuStat.P95MCpu = ptr(signals.P95UsageCores * 1000)
		cpuStat.P99MCpu = ptr(signals.P99UsageCores * 1000)
	}

	// Emit the verdict basis whenever Decide ran: the decision variables the
	// verdict acted on (headroom plus the three starvation causes),
	// structured so the MC renders the headline, the host/container split,
	// and the budget dashboard from the verdict's own inputs instead of
	// parsing the message text. Nil only when no verdict exists (a sampler
	// failure on the first tick); the legacy display path covers that case.
	// On a sampler failure after a prior success, the held basis is
	// re-emitted so the wire stays consistent with the held verdict. The
	// sample.LogicalCpus > 0 guard is defensive: the samplerOK gate already
	// keeps a zero sample out of Decide, but if a future code path let one
	// through, VerdictBasis would emit Capacity=0 + Ceiling="host" while
	// CgroupCores>0 is on the wire, a self-contradictory state.
	if samplerOK && sample.LogicalCpus > 0 {
		th := cpuhealth.DefaultThresholds()
		// The headroom's Used and Ceiling follow the mode: limit set →
		// ceiling="limit", used=the container's own 60s-avg usage; no limit →
		// ceiling="host", used=the host-busy 60s mean; no limit and no host
		// stats with the fallback latch fired → used=the container's own
		// 60s-avg usage measured against the HighUsageFraction budget, with
		// reserve=(1−HighUsageFraction)×capacity so the documented invariant
		// Cores = Capacity − Used − Reserve holds in this branch too.
		// HostBusy.Mean is the host observation in BOTH modes (context for
		// the display split and the host-full stacking check); Available
		// mirrors the sampler's /proc/stat readability flag.
		ceiling := "host"

		used := signals.HostBusyCores60sMean
		cores := signals.HeadroomCores
		reserve := signals.ReserveCores

		if signals.LimitApplies {
			ceiling = "limit"
			used = signals.AvgUsageCores
		} else if signals.NoHostStatsSaturationFired {
			used = signals.AvgUsageCores
			cores = th.HighUsageFraction*signals.CapacityCores - signals.AvgUsageCores
			reserve = (1 - th.HighUsageFraction) * signals.CapacityCores
		}

		cpuStat.VerdictBasis = &models.VerdictBasis{
			Headroom: models.VerdictBasisHeadroom{
				Ceiling:                    ceiling,
				Capacity:                   signals.CapacityCores,
				Used:                       used,
				Reserve:                    reserve,
				Cores:                      cores,
				Fired:                      signals.SaturationFired,
				LimitSaturationFired:       signals.LimitSaturationFired,
				HostFullFired:              signals.HostFullFired,
				NoHostStatsSaturationFired: signals.NoHostStatsSaturationFired,
				NoLimitHostFired:           signals.NoLimitHostFired,
			},
			HostBusy: models.VerdictBasisHostBusy{
				Mean:      signals.HostBusyCores60sMean,
				Available: sample.HostBusyCoresAvailable,
			},
			Throttle: models.VerdictBasisCause{
				Value:     signals.ThrottleRatio,
				Threshold: th.ThrottleHigh,
				Fired:     signals.ThrottleFired,
				Applies:   signals.LimitApplies,
			},
			Pressure: models.VerdictBasisCause{
				Value:     signals.PressureAvg60Out,
				Threshold: th.PressureHigh,
				Fired:     signals.PressureFired,
				Applies:   signals.PsiApplies,
			},
			Steal: models.VerdictBasisCause{
				Value:     signals.StealP95,
				Threshold: th.StealHigh,
				Fired:     signals.StealFired,
				Applies:   signals.StealApplies,
			},
			LimitedVisibility: signals.LimitedVisibility,
		}
		c.lastVerdictBasis = cpuStat.VerdictBasis
		// Hold CgroupCores from the successful tick so a sampler-failure tick
		// can re-emit it alongside the held verdict basis. Without this the
		// failure tick drops CgroupCores (the zero Sample has Quota==nil, so
		// the omitempty guard above leaves it at 0) while the held basis still
		// carries the prior ceiling, a self-contradictory machine-readable wire.
		c.lastCgroupCores = cpuStat.CgroupCores
	} else if c.hasLastVerdict {
		cpuStat.VerdictBasis = c.lastVerdictBasis
		// Re-emit the held CgroupCores so the failure-tick wire stays
		// consistent with the re-emitted verdict basis (both held from the
		// last successful tick). The two are consistent by construction: a
		// limit-mode tick stored CgroupCores>0 with ceiling="limit", and a
		// host-mode tick stored CgroupCores=0 (omitted) with ceiling="host".
		cpuStat.CgroupCores = c.lastCgroupCores
	}

	return cpuStat, nil
}

func (c *ContainerMonitorService) getRawCPUMetrics(ctx context.Context) (usageMCores float64, coreCount int, sample cpuhealth.Sample, samplerOK bool, samplerErr error, err error) {
	coreCount = runtime.NumCPU() // Always report host cores for compatibility

	// The ONLY c.sampler.Sample call per GetStatus: the returned Sample is
	// passed on to the Decide call in getCPUMetrics so the delta-based
	// signals survive. On read failure (cgroup v1, non-container, transient)
	// a zero-valued Sample is returned with samplerOK=false and the error in
	// samplerErr, so the caller holds the prior verdict instead of running
	// Decide on a zero sample and can log the cause on the outage
	// transition.
	sample, sErr := c.sampler.Sample(ctx)
	if sErr != nil {
		if ctx.Err() != nil {
			return 0, 0, cpuhealth.Sample{}, false, nil, ctx.Err()
		}

		samplerErr = sErr
	} else {
		samplerOK = true
	}

	if ctx.Err() != nil {
		return 0, 0, cpuhealth.Sample{}, false, nil, ctx.Err()
	}

	usageMCores = sample.UsageCores * 1000

	return usageMCores, coreCount, sample, samplerOK, samplerErr, nil
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

// ptr returns a pointer to v. It is the helper for the *float64 wire fields on
// models.CPU: a non-nil pointer (even to 0) is emitted by encoding/json's
// omitempty, while a nil pointer is omitted, giving the fetchability-based
// emission discipline (emit when fetchable, even 0; omit when un-fetchable).
func ptr(v float64) *float64 {
	return &v
}
