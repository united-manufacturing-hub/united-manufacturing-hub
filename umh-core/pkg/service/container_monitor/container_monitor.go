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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"go.uber.org/zap"
)

// Service defines the interface for the container monitor service
type Service interface {
	// GetStatus collects and returns the current container metrics
	// using the standard models.Container
	GetStatus(ctx context.Context) (*models.Container, error)

	// GetHealth returns the container health status based on current metrics
	GetHealth(ctx context.Context) (*models.Health, error)
}

// ContainerMonitorService implements the Service interface
type ContainerMonitorService struct {
	fs              filesystem.Service
	logger          *zap.Logger
	instanceName    string
	lastCollectedAt time.Time
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
func (c *ContainerMonitorService) GetStatus(ctx context.Context) (*models.Container, error) {
	// Create a new container with default health
	container := &models.Container{
		Health: &models.Health{
			Message:       "Container is operating normally",
			ObservedState: constants.ContainerStateRunning,
			DesiredState:  constants.ContainerStateRunning,
			Category:      models.Active,
		},
	}

	// Get CPU metrics
	cpu, err := c.getCPUMetrics(ctx)
	if err != nil {
		c.logger.Error("Failed to get CPU metrics", zap.Error(err))
		// Continue with other metrics
	} else {
		container.CPU = cpu
	}

	// Get memory metrics
	memory, err := c.getMemoryMetrics(ctx)
	if err != nil {
		c.logger.Error("Failed to get memory metrics", zap.Error(err))
		// Continue with other metrics
	} else {
		container.Memory = memory
	}

	// Get disk metrics
	disk, err := c.getDiskMetrics(ctx)
	if err != nil {
		c.logger.Error("Failed to get disk metrics", zap.Error(err))
		// Continue with other metrics
	} else {
		container.Disk = disk
	}

	// Get hardware info
	hwid, err := c.getHWID(ctx)
	if err != nil {
		c.logger.Error("Failed to get hardware ID", zap.Error(err))
		// Use empty string as fallback
		hwid = ""
	}
	container.Hwid = hwid

	// Set architecture directly from runtime
	container.Architecture = models.ContainerArchitecture(runtime.GOARCH)

	// Update last collected timestamp
	c.lastCollectedAt = time.Now()

	// Record metrics to Prometheus
	RecordContainerMetrics(container, c.instanceName)

	return container, nil
}

// GetHealth returns the health status of the container based on current metrics
func (c *ContainerMonitorService) GetHealth(ctx context.Context) (*models.Health, error) {
	// Get current metrics
	container, err := c.GetStatus(ctx)
	if err != nil {
		return nil, err
	}

	// Container already has a health field that is updated during GetStatus
	// Return it directly
	return container.Health, nil
}

// getCPUMetrics collects CPU metrics
func (c *ContainerMonitorService) getCPUMetrics(ctx context.Context) (*models.CPU, error) {
	// TODO: Implement actual CPU metrics collection
	// For now, use placeholder values similar to generator.go
	cpu := &models.CPU{
		Health: &models.Health{
			Message:       "CPU utilization normal",
			ObservedState: constants.ContainerStateNormal,
			DesiredState:  constants.ContainerStateNormal,
			Category:      models.Active,
		},
		TotalUsageMCpu: 350.0,
		CoreCount:      runtime.NumCPU(),
	}

	// Calculate CPU health based on load
	cpuLoadPercent := 50.0 // Placeholder for actual load calculation

	if cpuLoadPercent >= constants.CPUCriticalPercent {
		cpu.Health.Message = "CPU utilization critical"
		cpu.Health.ObservedState = constants.ContainerStateCritical
		cpu.Health.Category = models.Degraded
	} else if cpuLoadPercent >= constants.CPUCriticalPercent*0.8 {
		cpu.Health.Message = "CPU utilization warning"
		cpu.Health.ObservedState = constants.ContainerStateWarning
	}

	return cpu, nil
}

// getMemoryMetrics collects memory metrics
func (c *ContainerMonitorService) getMemoryMetrics(ctx context.Context) (*models.Memory, error) {
	// TODO: Implement actual memory metrics collection
	// For now, use placeholder values similar to generator.go
	memory := &models.Memory{
		Health: &models.Health{
			Message:       "Memory utilization normal",
			ObservedState: constants.ContainerStateNormal,
			DesiredState:  constants.ContainerStateNormal,
			Category:      models.Active,
		},
		CGroupUsedBytes:  1073741824, // 1 GB
		CGroupTotalBytes: 4294967296, // 4 GB
	}

	// Calculate memory health
	memPercent := float64(memory.CGroupUsedBytes) / float64(memory.CGroupTotalBytes) * 100.0
	if memPercent >= constants.MemoryCriticalPercent {
		memory.Health.Message = "Memory utilization critical"
		memory.Health.ObservedState = constants.ContainerStateCritical
		memory.Health.Category = models.Degraded
	} else if memPercent >= constants.MemoryCriticalPercent*0.8 {
		memory.Health.Message = "Memory utilization warning"
		memory.Health.ObservedState = constants.ContainerStateWarning
	}

	return memory, nil
}

// getDiskMetrics collects disk metrics for the data partition
func (c *ContainerMonitorService) getDiskMetrics(ctx context.Context) (*models.Disk, error) {
	// TODO: Implement actual disk metrics collection for /data
	// For now, use placeholder values similar to generator.go
	disk := &models.Disk{
		Health: &models.Health{
			Message:       "Disk utilization normal",
			ObservedState: constants.ContainerStateNormal,
			DesiredState:  constants.ContainerStateNormal,
			Category:      models.Active,
		},
		DataPartitionUsedBytes:  536870912,   // 512 MB
		DataPartitionTotalBytes: 10737418240, // 10 GB
	}

	// Calculate disk health
	diskPercent := float64(disk.DataPartitionUsedBytes) / float64(disk.DataPartitionTotalBytes) * 100.0
	if diskPercent >= constants.DiskCriticalPercent {
		disk.Health.Message = "Disk utilization critical"
		disk.Health.ObservedState = constants.ContainerStateCritical
		disk.Health.Category = models.Degraded
	} else if diskPercent >= constants.DiskCriticalPercent*0.8 {
		disk.Health.Message = "Disk utilization warning"
		disk.Health.ObservedState = constants.ContainerStateWarning
	}

	return disk, nil
}

// getHWID gets the hardware ID from system
func (c *ContainerMonitorService) getHWID(ctx context.Context) (string, error) {
	// Try to read from the hardware ID file
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
	// Ensure the /data directory exists
	err := c.fs.EnsureDirectory(ctx, constants.DataMountPath)
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
	err = c.fs.WriteFile(ctx, constants.HWIDFilePath, []byte(hwid), 0644)
	if err != nil {
		return "", WrapMetricsError(ErrHWIDCollection, "error writing HWID file")
	}

	return hwid, nil
}
