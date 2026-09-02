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
)

// cpuGaugeInputs returns the usage and core figures for the CPU gauges,
// and whether both were measured this tick. useFSMv2CPU says which generation
// filled the record, because only one of them ever does: under the flag the
// figures come from the worker's evidence and the flat fields are empty, and
// without it the reverse.
//
// The two generations do not measure the same thing. The worker reports a
// 60-second mean of THIS CONTAINER's usage; the legacy path an instantaneous
// sample of the HOST's usage scaled by the quota. These gauges therefore change
// meaning when the flag flips, and an alert on them has to be re-based.
func cpuGaugeInputs(cpu *models.CPU, useFSMv2CPU bool) (usageMCores, cores float64, ok bool) {
	if useFSMv2CPU {
		// UsageRingActive and a positive LogicalCpus are the evidence's own
		// readability flags; 0 is a legitimate value for both figures, so
		// without them an unmeasured tick would record an idle box.
		if cpu.CPUHealth == nil || !cpu.CPUHealth.UsageRingActive || cpu.CPUHealth.LogicalCpus <= 0 {
			return 0, 0, false
		}

		return cpu.CPUHealth.AvgUsageCores * 1000, cpu.CPUHealth.LogicalCpus, true
	}

	if cpu.TotalUsageMCpu == nil || cpu.CoreCount == nil || *cpu.CoreCount == 0 {
		return 0, 0, false
	}

	return *cpu.TotalUsageMCpu, float64(*cpu.CoreCount), true
}

// RecordContainerStatus updates Prometheus metrics based on the new ContainerStatus type.
func RecordContainerStatus(status *ServiceInfo, instanceName string, useFSMv2CPU bool) {
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

	// CPU metrics. A nil measurement (an unmeasured tick) leaves each gauge
	// untouched: the previous value stays, and no fabricated number is recorded.
	if status.CPU != nil {
		usageMCores, cores, ok := cpuGaugeInputs(status.CPU, useFSMv2CPU)
		if ok {
			containerCPUUsageMCores.WithLabelValues(instanceName).Set(usageMCores)
			containerCPUCoreCount.WithLabelValues(instanceName).Set(cores)
			containerCPULoadPercent.WithLabelValues(instanceName).Set((usageMCores / 1000.0) / cores * 100.0)
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
