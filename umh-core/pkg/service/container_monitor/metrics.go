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
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/metrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

const (
	// ComponentContainerMonitor is the component label for container monitoring metrics
	ComponentContainerMonitor = "container_monitor"

	// DefaultInstanceName is the instance name used for the single core container
	DefaultInstanceName = "Core"
)

var (
	metricsOnce sync.Once
	metricsInit bool

	// Standard namespace and subsystem for all metrics
	namespace = "umh"
	subsystem = "container"

	// CPU metrics
	containerCPUUsageMCores = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "cpu_usage_mcores",
		Help:      "Current CPU usage in millicores (1000m = 1 core)",
	}, []string{"instance"})

	containerCPUCoreCount = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "cpu_core_count",
		Help:      "Number of CPU cores available",
	}, []string{"instance"})

	containerCPULoadPercent = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "cpu_load_percent",
		Help:      "Current CPU load as percentage (0-100)",
	}, []string{"instance"})

	// Memory metrics
	containerMemoryUsedBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "memory_used_bytes",
		Help:      "Current memory usage in bytes",
	}, []string{"instance"})

	containerMemoryTotalBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "memory_total_bytes",
		Help:      "Total memory available in bytes",
	}, []string{"instance"})

	containerMemoryUsagePercent = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "memory_usage_percent",
		Help:      "Memory usage as percentage of total (0-100)",
	}, []string{"instance"})

	// Disk metrics
	containerDiskUsedBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "disk_used_bytes",
		Help:      "Current disk usage in bytes for data partition",
	}, []string{"instance"})

	containerDiskTotalBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "disk_total_bytes",
		Help:      "Total disk space in bytes for data partition",
	}, []string{"instance"})

	containerDiskUsagePercent = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "disk_usage_percent",
		Help:      "Disk usage as percentage of total (0-100)",
	}, []string{"instance"})
)

// RecordContainerMetrics updates all Prometheus metrics based on the current metrics values
// using standard models.Container type
// instanceName is used as a label for all metrics to distinguish between different containers
func RecordContainerMetrics(container *models.Container, instanceName string) {
	if container == nil {
		return
	}

	// Default instance name if not provided
	if instanceName == "" {
		instanceName = DefaultInstanceName
	}

	// Initialize metrics only once
	metricsOnce.Do(func() {
		metricsInit = true
		// Register with central metrics
		metrics.InitErrorCounter(ComponentContainerMonitor, instanceName)
	})

	// CPU metrics
	if container.CPU != nil {
		containerCPUUsageMCores.WithLabelValues(instanceName).Set(container.CPU.TotalUsageMCpu)
		containerCPUCoreCount.WithLabelValues(instanceName).Set(float64(container.CPU.CoreCount))

		// CPU load percent is calculated during metrics collection
		// but we need to derive it here for Prometheus
		if container.CPU.TotalUsageMCpu > 0 && container.CPU.CoreCount > 0 {
			cpuLoadPercent := (container.CPU.TotalUsageMCpu / 1000.0) / float64(container.CPU.CoreCount) * 100.0
			containerCPULoadPercent.WithLabelValues(instanceName).Set(cpuLoadPercent)
		}
	}

	// Memory metrics
	if container.Memory != nil {
		containerMemoryUsedBytes.WithLabelValues(instanceName).Set(float64(container.Memory.CGroupUsedBytes))
		containerMemoryTotalBytes.WithLabelValues(instanceName).Set(float64(container.Memory.CGroupTotalBytes))

		// Calculate percentage
		if container.Memory.CGroupTotalBytes > 0 {
			usagePercent := float64(container.Memory.CGroupUsedBytes) / float64(container.Memory.CGroupTotalBytes) * 100.0
			containerMemoryUsagePercent.WithLabelValues(instanceName).Set(usagePercent)
		}
	}

	// Disk metrics
	if container.Disk != nil {
		containerDiskUsedBytes.WithLabelValues(instanceName).Set(float64(container.Disk.DataPartitionUsedBytes))
		containerDiskTotalBytes.WithLabelValues(instanceName).Set(float64(container.Disk.DataPartitionTotalBytes))

		// Calculate percentage
		if container.Disk.DataPartitionTotalBytes > 0 {
			usagePercent := float64(container.Disk.DataPartitionUsedBytes) / float64(container.Disk.DataPartitionTotalBytes) * 100.0
			containerDiskUsagePercent.WithLabelValues(instanceName).Set(usagePercent)
		}
	}
}
