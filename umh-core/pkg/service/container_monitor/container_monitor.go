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
	"runtime"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"go.uber.org/zap"
)

// Service defines the interface for the container monitor service
type Service interface {
	// GetStatus collects and returns the current container metrics
	GetStatus(ctx context.Context) (*ContainerMetrics, error)

	// GetHealth returns the container health status based on current metrics
	GetHealth(ctx context.Context) (*models.Health, *models.CPU, *models.Disk, *models.Memory, error)
}

// ContainerMonitorService implements the Service interface
type ContainerMonitorService struct {
	fs           filesystem.Service
	logger       *zap.Logger
	instanceName string
}

// NewContainerMonitorService creates a new container monitor service instance
func NewContainerMonitorService(fs filesystem.Service) *ContainerMonitorService {
	log := logger.New(logger.ComponentContainerMonService, logger.FormatJSON)

	return &ContainerMonitorService{
		fs:           fs,
		logger:       log,
		instanceName: "Core", // Single container instance name
	}
}

// GetStatus collects and returns the current container metrics
func (c *ContainerMonitorService) GetStatus(ctx context.Context) (*ContainerMetrics, error) {
	metrics := &ContainerMetrics{
		CollectedAt: time.Now(),
	}

	// Get CPU metrics
	cpuMetrics, err := c.getCPUMetrics(ctx)
	if err != nil {
		c.logger.Error("Failed to get CPU metrics", zap.Error(err))
		// Continue with other metrics
	} else {
		metrics.CPU = cpuMetrics
	}

	// Get memory metrics
	memoryMetrics, err := c.getMemoryMetrics(ctx)
	if err != nil {
		c.logger.Error("Failed to get memory metrics", zap.Error(err))
		// Continue with other metrics
	} else {
		metrics.Memory = memoryMetrics
	}

	// Get disk metrics
	diskMetrics, err := c.getDiskMetrics(ctx)
	if err != nil {
		c.logger.Error("Failed to get disk metrics", zap.Error(err))
		// Continue with other metrics
	} else {
		metrics.Disk = diskMetrics
	}

	// Get hardware info
	hwid, err := c.getHWID(ctx)
	if err != nil {
		c.logger.Error("Failed to get hardware ID", zap.Error(err))
		// Use empty string as fallback
		hwid = ""
	}
	metrics.HWID = hwid

	// Set architecture directly from runtime
	metrics.Architecture = models.ContainerArchitecture(runtime.GOARCH)

	// Record metrics to Prometheus
	RecordContainerMetrics(metrics, c.instanceName)

	return metrics, nil
}

// GetHealth returns the health status of the container based on current metrics
func (c *ContainerMonitorService) GetHealth(ctx context.Context) (*models.Health, *models.CPU, *models.Disk, *models.Memory, error) {
	// Get current metrics
	metrics, err := c.GetStatus(ctx)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	// Initialize container health with default "good" values
	containerHealth := &models.Health{
		Message:       "Container is operating normally",
		ObservedState: constants.ContainerStateRunning,
		DesiredState:  constants.ContainerStateRunning,
		Category:      models.Active,
	}

	// Initialize component-specific health
	cpuHealth := &models.Health{
		Message:       "CPU utilization normal",
		ObservedState: constants.ContainerStateNormal,
		DesiredState:  constants.ContainerStateNormal,
		Category:      models.Active,
	}

	diskHealth := &models.Health{
		Message:       "Disk utilization normal",
		ObservedState: constants.ContainerStateNormal,
		DesiredState:  constants.ContainerStateNormal,
		Category:      models.Active,
	}

	memoryHealth := &models.Health{
		Message:       "Memory utilization normal",
		ObservedState: constants.ContainerStateNormal,
		DesiredState:  constants.ContainerStateNormal,
		Category:      models.Active,
	}

	// Check CPU health if metrics available
	if metrics.CPU != nil {
		cpuPercent := metrics.CPU.LoadPercent
		if cpuPercent >= constants.CPUCriticalPercent {
			cpuHealth.Message = "CPU utilization critical"
			cpuHealth.ObservedState = constants.ContainerStateCritical
			cpuHealth.Category = models.Degraded

			// Update overall container health
			containerHealth.Message = "Container CPU utilization critical"
			containerHealth.Category = models.Degraded
		} else if cpuPercent >= constants.CPUCriticalPercent*0.8 {
			cpuHealth.Message = "CPU utilization warning"
			cpuHealth.ObservedState = constants.ContainerStateWarning
		}
	}

	// Check memory health if metrics available
	if metrics.Memory != nil && metrics.Memory.CGroupTotalBytes > 0 {
		memPercent := float64(metrics.Memory.CGroupUsedBytes) / float64(metrics.Memory.CGroupTotalBytes) * 100.0
		if memPercent >= constants.MemoryCriticalPercent {
			memoryHealth.Message = "Memory utilization critical"
			memoryHealth.ObservedState = constants.ContainerStateCritical
			memoryHealth.Category = models.Degraded

			// Update overall container health if not already critical
			if containerHealth.Category != models.Degraded {
				containerHealth.Message = "Container memory utilization critical"
				containerHealth.Category = models.Degraded
			}
		} else if memPercent >= constants.MemoryCriticalPercent*0.8 {
			memoryHealth.Message = "Memory utilization warning"
			memoryHealth.ObservedState = constants.ContainerStateWarning
		}
	}

	// Check disk health if metrics available
	if metrics.Disk != nil && metrics.Disk.DataPartitionTotalBytes > 0 {
		diskPercent := float64(metrics.Disk.DataPartitionUsedBytes) / float64(metrics.Disk.DataPartitionTotalBytes) * 100.0
		if diskPercent >= constants.DiskCriticalPercent {
			diskHealth.Message = "Disk utilization critical"
			diskHealth.ObservedState = constants.ContainerStateCritical
			diskHealth.Category = models.Degraded

			// Update overall container health if not already critical
			if containerHealth.Category != models.Degraded {
				containerHealth.Message = "Container disk utilization critical"
				containerHealth.Category = models.Degraded
			}
		} else if diskPercent >= constants.DiskCriticalPercent*0.8 {
			diskHealth.Message = "Disk utilization warning"
			diskHealth.ObservedState = constants.ContainerStateWarning
		}
	}

	// Create CPU model
	cpu := &models.CPU{
		Health:         cpuHealth,
		TotalUsageMCpu: metrics.CPU.TotalUsageMCpu,
		CoreCount:      metrics.CPU.CoreCount,
	}

	// Create Disk model
	disk := &models.Disk{
		Health:                  diskHealth,
		DataPartitionUsedBytes:  metrics.Disk.DataPartitionUsedBytes,
		DataPartitionTotalBytes: metrics.Disk.DataPartitionTotalBytes,
	}

	// Create Memory model
	memory := &models.Memory{
		Health:           memoryHealth,
		CGroupUsedBytes:  metrics.Memory.CGroupUsedBytes,
		CGroupTotalBytes: metrics.Memory.CGroupTotalBytes,
	}

	return containerHealth, cpu, disk, memory, nil
}

// getCPUMetrics collects CPU metrics
func (c *ContainerMonitorService) getCPUMetrics(ctx context.Context) (*CPUMetrics, error) {
	// TODO: Implement actual CPU metrics collection
	// For now, use placeholder values similar to generator.go
	return &CPUMetrics{
		TotalUsageMCpu: 350.0,
		CoreCount:      runtime.NumCPU(),
		LoadPercent:    50.0, // Placeholder for actual load
	}, nil
}

// getMemoryMetrics collects memory metrics
func (c *ContainerMonitorService) getMemoryMetrics(ctx context.Context) (*MemoryMetrics, error) {
	// TODO: Implement actual memory metrics collection
	// For now, use placeholder values similar to generator.go
	return &MemoryMetrics{
		CGroupUsedBytes:  1073741824, // 1 GB
		CGroupTotalBytes: 4294967296, // 4 GB
	}, nil
}

// getDiskMetrics collects disk metrics for the data partition
func (c *ContainerMonitorService) getDiskMetrics(ctx context.Context) (*DiskMetrics, error) {
	// TODO: Implement actual disk metrics collection for /data
	// For now, use placeholder values similar to generator.go
	return &DiskMetrics{
		DataPartitionUsedBytes:  536870912,   // 512 MB
		DataPartitionTotalBytes: 10737418240, // 10 GB
	}, nil
}

// getHWID gets the hardware ID from system
func (c *ContainerMonitorService) getHWID(ctx context.Context) (string, error) {
	// Try to read from machine-id file
	exists, err := c.fs.FileExists(ctx, constants.HWIDFilePath)
	if err != nil {
		return "", WrapMetricsError(ErrHWIDCollection, "error checking if HWID file exists")
	}

	if exists {
		data, err := c.fs.ReadFile(ctx, constants.HWIDFilePath)
		if err != nil {
			return "", WrapMetricsError(ErrHWIDCollection, "error reading HWID file")
		}
		return string(data), nil
	}

	// Fallback to static ID if file doesn't exist
	return "hwid-12345", nil
}
