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

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"

	"golang.org/x/sys/unix"
)

// ServiceInfo contains both raw metrics and health assessments
type ServiceInfo struct {
	// Raw metrics (keeping same structure for compatibility)
	CPU    *models.CPU    // Keep existing CPU metrics
	Memory *models.Memory // Keep existing Memory metrics
	Disk   *models.Disk   // Keep existing Disk metrics

	// Health assessments using existing models.HealthCategory
	OverallHealth models.HealthCategory
	CPUHealth     models.HealthCategory
	MemoryHealth  models.HealthCategory
	DiskHealth    models.HealthCategory

	// Existing fields
	Hwid         string
	Architecture models.ContainerArchitecture
}

// Service defines the interface for container monitoring
type Service interface {
	// GetStatus returns container metrics with health assessments
	GetStatus(ctx context.Context) (*ServiceInfo, error)
}

// ContainerMonitorService implements the Service interface
type ContainerMonitorService struct {
	fs              filesystem.Service
	logger          *zap.SugaredLogger
	instanceName    string
	lastCollectedAt time.Time
	hwid            string
	architecture    models.ContainerArchitecture
	dataPath        string // Path to check for disk metrics and HWID file
}

// NewContainerMonitorService creates a new container monitor service instance
func NewContainerMonitorService(fs filesystem.Service) *ContainerMonitorService {
	return NewContainerMonitorServiceWithPath(fs, constants.DataMountPath)
}

// NewContainerMonitorServiceWithPath creates a new container monitor service with a custom data path
func NewContainerMonitorServiceWithPath(fs filesystem.Service, dataPath string) *ContainerMonitorService {
	log := logger.For(logger.ComponentContainerMonService)

	return &ContainerMonitorService{
		fs:           fs,
		logger:       log,
		instanceName: "Core", // Single container instance name
		dataPath:     dataPath,
	}
}

// GetFilesystemService returns the filesystem service - used for testing only
func (c *ContainerMonitorService) GetFilesystemService() filesystem.Service {
	return c.fs
}

// SetDataPath changes the data path - used for testing only
func (c *ContainerMonitorService) SetDataPath(path string) {
	c.dataPath = path
}

// GetStatus collects and returns the current container metrics
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
	if cpuStat.CoreCount > 0 {
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

// GetHealth returns the health status of the container based on current metrics
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
		if status.CPUHealth == models.Degraded {
			message = "CPU metrics degraded"
		} else if status.MemoryHealth == models.Degraded {
			message = "Memory metrics degraded"
		} else if status.DiskHealth == models.Degraded {
			message = "Disk metrics degraded"
		} else {
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
	usageMCores, coreCount, err := c.getRawCPUMetrics(ctx)
	if err != nil {
		return nil, err
	}

	usagePercent := usageMCores / float64(coreCount) * 100.0

	// Default to Active health
	category := models.Active
	message := "CPU utilization normal"

	if usagePercent >= constants.CPUHighThresholdPercent {
		category = models.Degraded
		message = "CPU utilization critical"
	} else if usagePercent >= constants.CPUMediumThresholdPercent {
		// Still Active but with a warning message
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

	return cpuStat, nil
}

func (c *ContainerMonitorService) getRawCPUMetrics(ctx context.Context) (usageMCores float64, coreCount int, err error) {
	// Fetching from cgroup is incredibly difficult, so we fallback to host-level usage

	// -- FALLBACK: host-level usage with cpu.Percent() --
	// Gather CPU usage over a short interval (0 => immediate snapshot).
	// Optionally you could do time.Sleep and call cpu.Percent again for a delta.
	usagePercent, err := cpu.PercentWithContext(ctx, 0, false)
	if err != nil {
		return 0, 0, err
	}
	var cpuUsagePercent float64
	if len(usagePercent) > 0 {
		cpuUsagePercent = usagePercent[0]
	}

	// Convert usage percent to mCPU (i.e. 1000 mCPU = 1 core).
	// For example, if usage is 50% on a system with 4 cores,
	// the container is effectively using 2 cores => 2000 mCPU.
	coreCount = runtime.NumCPU()
	usageCores := (cpuUsagePercent / 100.0) * float64(coreCount)
	usageMCores = usageCores * 1000

	return usageMCores, coreCount, nil
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

// getMacOSAdjustedDiskMetrics retrieves adjusted disk metrics using unix.Statfs.
// It uses stat.Frsize when available, since on Docker Desktop for macOS the reported
// Bsize is often 1024× larger than the actual block size.
// We use unix.Statfs directly instead of relying on gopsutil's disk.Usage because:
// 1. It gives us direct access to the Frsize field which is crucial for proper block size calculation
// 2. gopsutil doesn't handle the Docker Desktop for macOS edge case correctly
func (c *ContainerMonitorService) getMacOSAdjustedDiskMetrics() (usedBytes, totalBytes uint64, err error) {
	var stat unix.Statfs_t
	err = unix.Statfs(c.dataPath, &stat)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to stat filesystem at %s: %w", c.dataPath, err)
	}

	// Use Frsize if available; it represents the fundamental block size for macOS.
	bSize := uint64(stat.Bsize)
	if stat.Frsize > 0 {
		bSize = uint64(stat.Frsize)
	}

	// Compute total and used bytes based on the corrected block size.
	totalBytes = stat.Blocks * bSize
	usedBytes = (stat.Blocks - stat.Bfree) * bSize

	return usedBytes, totalBytes, nil
}

// getHWID gets the hardware ID from system
func (c *ContainerMonitorService) getHWID(ctx context.Context) (string, error) {
	// Try to read from the hardware ID file
	hwidPath := fmt.Sprintf("%s/hwid", c.dataPath)
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

// generateNewHWID creates a new hardware ID file with a random hash
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
	hwid := fmt.Sprintf("%x", hash.Sum(nil))

	// Write the hash to the file
	hwidPath := fmt.Sprintf("%s/hwid", c.dataPath)
	err = c.fs.WriteFile(ctx, hwidPath, []byte(hwid), 0644)
	if err != nil {
		return "", WrapMetricsError(ErrHWIDCollection, "error writing HWID file")
	}

	return hwid, nil
}
