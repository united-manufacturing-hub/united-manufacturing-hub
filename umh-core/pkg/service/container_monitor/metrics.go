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
)

const (
	// ComponentContainerMonitor is the component label for container monitoring metrics.
	ComponentContainerMonitor = "container_monitor"

	// DefaultInstanceName is the instance name used for the single core container.
	DefaultInstanceName = "Core"
)

var (
	metricsOnce sync.Once

	// Standard namespace and subsystem for all metrics.
	namespace = "umh"
	subsystem = "container"

	// CPU metrics.
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

	// Memory metrics.
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

	// Disk metrics.
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

	// Health status metrics.
	containerHealthStatus = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: subsystem,
		Name:      "health_status",
		Help:      "Health status of container components (0=Neutral, 1=Active, 2=Degraded)",
	}, []string{"instance", "component"})

	// Cgroup CPU throttling metrics (using umh_core namespace to maintain compatibility).
	cgroupCPUThrottledPeriods = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "umh",
			Subsystem: "core",
			Name:      "cgroup_cpu_nr_throttled",
			Help:      "Number of periods that were throttled",
		},
	)

	cgroupCPUTotalPeriods = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "umh",
			Subsystem: "core",
			Name:      "cgroup_cpu_nr_periods",
			Help:      "Total number of periods",
		},
	)

	cgroupCPUThrottledTime = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "umh",
			Subsystem: "core",
			Name:      "cgroup_cpu_throttled_usec",
			Help:      "Time spent throttled in microseconds",
		},
	)
)

// RecordContainerStatus updates Prometheus metrics based on the new ContainerStatus type.
func RecordContainerStatus(status *ServiceInfo, instanceName string) {
	if status == nil {
		return
	}

	// Default instance name if not provided
	if instanceName == "" {
		instanceName = DefaultInstanceName
	}

	// Initialize metrics if needed
	metricsOnce.Do(func() {
		// Register with central metrics
		metrics.InitErrorCounter(ComponentContainerMonitor, instanceName)
	})

	// Record health statuses
	containerHealthStatus.WithLabelValues(instanceName, "overall").Set(float64(status.OverallHealth))
	containerHealthStatus.WithLabelValues(instanceName, "cpu").Set(float64(status.CPUHealth))
	containerHealthStatus.WithLabelValues(instanceName, "memory").Set(float64(status.MemoryHealth))
	containerHealthStatus.WithLabelValues(instanceName, "disk").Set(float64(status.DiskHealth))

	// CPU metrics
	if status.CPU != nil {
		containerCPUUsageMCores.WithLabelValues(instanceName).Set(status.CPU.TotalUsageMCpu)
		containerCPUCoreCount.WithLabelValues(instanceName).Set(float64(status.CPU.CoreCount))

		// CPU load percent is calculated during metrics collection
		if status.CPU.TotalUsageMCpu > 0 && status.CPU.CoreCount > 0 {
			cpuLoadPercent := (status.CPU.TotalUsageMCpu / 1000.0) / float64(status.CPU.CoreCount) * 100.0
			containerCPULoadPercent.WithLabelValues(instanceName).Set(cpuLoadPercent)
		}
	}

	// Memory metrics
	if status.Memory != nil {
		containerMemoryUsedBytes.WithLabelValues(instanceName).Set(float64(status.Memory.CGroupUsedBytes))
		containerMemoryTotalBytes.WithLabelValues(instanceName).Set(float64(status.Memory.CGroupTotalBytes))

		// Calculate percentage
		if status.Memory.CGroupTotalBytes > 0 {
			usagePercent := float64(status.Memory.CGroupUsedBytes) / float64(status.Memory.CGroupTotalBytes) * 100.0
			containerMemoryUsagePercent.WithLabelValues(instanceName).Set(usagePercent)
		}
	}

	// Disk metrics
	if status.Disk != nil {
		containerDiskUsedBytes.WithLabelValues(instanceName).Set(float64(status.Disk.DataPartitionUsedBytes))
		containerDiskTotalBytes.WithLabelValues(instanceName).Set(float64(status.Disk.DataPartitionTotalBytes))

		// Calculate percentage
		if status.Disk.DataPartitionTotalBytes > 0 {
			usagePercent := float64(status.Disk.DataPartitionUsedBytes) / float64(status.Disk.DataPartitionTotalBytes) * 100.0
			containerDiskUsagePercent.WithLabelValues(instanceName).Set(usagePercent)
		}
	}
}

// RecordCgroupMetrics updates the cgroup throttling metrics.
func RecordCgroupMetrics(throttledPeriods, totalPeriods, throttledUsec uint64) {
	cgroupCPUThrottledPeriods.Set(float64(throttledPeriods))
	cgroupCPUTotalPeriods.Set(float64(totalPeriods))
	cgroupCPUThrottledTime.Set(float64(throttledUsec))
}
