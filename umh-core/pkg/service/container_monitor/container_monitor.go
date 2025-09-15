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

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
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
	fs              filesystem.Service
	logger          *zap.SugaredLogger
	instanceName    string
	lastCollectedAt time.Time
	hwid            string
	architecture    models.ContainerArchitecture //nolint:unused // will be used in the future
	dataPath        string                       // Path to check for disk metrics and HWID file
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

	// Assess CPU health
	// Check if CPU is already marked as degraded (e.g., due to throttling)
	if cpuStat.Health != nil && cpuStat.Health.Category == models.Degraded {
		status.CPUHealth = models.Degraded
		status.OverallHealth = models.Degraded
	} else if cpuStat.CoreCount > 0 {
		// Also check raw usage percentage
		cpuPercent := (cpuStat.TotalUsageMCpu / 1000.0) / float64(cpuStat.CoreCount) * 100.0

		if cpuPercent > constants.CPUHighThresholdPercent {
			status.CPUHealth = models.Degraded
			status.OverallHealth = models.Degraded
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
	
	// Default to Active health
	category := models.Active
	message := "CPU utilization normal"
	
	// Check for throttling - only if cgroupInfo is not nil
	isThrottled := cgroupErr == nil && cgroupInfo != nil && cgroupInfo.IsThrottled
	highThrottling := cgroupErr == nil && cgroupInfo != nil && cgroupInfo.ThrottleRatio > 0.20

	if usagePercent >= constants.CPUHighThresholdPercent || highThrottling {
		category = models.Degraded
		
		if highThrottling && cgroupInfo != nil {
			message = fmt.Sprintf("CPU throttled (%.1f%% periods throttled)", cgroupInfo.ThrottleRatio*100)
		} else {
			message = "CPU utilization critical"
		}
	} else if usagePercent >= constants.CPUMediumThresholdPercent || isThrottled {
		// Still could be Active or Degraded depending on severity
		if isThrottled && cgroupInfo != nil {
			category = models.Degraded
			message = fmt.Sprintf("CPU throttled (%.1f%% periods throttled)", cgroupInfo.ThrottleRatio*100)
		} else {
			message = "CPU utilization warning"
		}
	}
	
	// Log throttling warnings
	if isThrottled && cgroupInfo != nil {
		c.logger.Warnf("CPU throttling detected: %.1f%% of periods throttled", cgroupInfo.ThrottleRatio*100)
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

func (c *ContainerMonitorService) getRawCPUMetrics(ctx context.Context) (usageMCores float64, coreCount int, usagePercent float64, err error) {
	// Try to get cgroup info first for accurate container limits
	cgroupInfo, cgroupErr := c.getCgroupCPUInfo(ctx)
	
	// Get actual CPU usage
	usagePercentages, err := cpu.PercentWithContext(ctx, 0, false)
	if err != nil {
		return 0, 0, 0, err
	}

	if len(usagePercentages) > 0 {
		usagePercent = usagePercentages[0]
	}

	// Determine effective core count
	// Use cgroup limit if available, otherwise fall back to host CPU count
	effectiveCores := runtime.NumCPU()
	if cgroupErr == nil && cgroupInfo.QuotaCores > 0 {
		// Use cgroup limit for more accurate mCPU calculation
		effectiveCores = int(cgroupInfo.QuotaCores)
		if effectiveCores < 1 {
			effectiveCores = 1
		}
	}
	
	coreCount = runtime.NumCPU() // Always report host cores for compatibility
	
	// Convert usage percent to mCPU based on effective cores
	// This gives us a more accurate representation when cgroups limit CPU
	usageCores := (usagePercent / 100.0) * float64(effectiveCores)
	usageMCores = usageCores * 1000

	return usageMCores, coreCount, usagePercent, nil
}

// getMemoryMetrics collects memory metrics using gopsutil.
// By default, this returns host-level usage, not cgroup-limited usage.
func (c *ContainerMonitorService) getMemoryMetrics(ctx context.Context) (*models.Memory, error) {
	vmStat, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return nil, err
	}

	usedBytes := vmStat.Used
	totalBytes := vmStat.Total

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
