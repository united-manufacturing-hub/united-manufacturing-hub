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
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// ContainerMetrics contains all the metrics collected by the container monitor service.
// The structure aligns with models.Container from generator.go.
type ContainerMetrics struct {
	CPU          *CPUMetrics                  `json:"cpu"`
	Memory       *MemoryMetrics               `json:"memory"`
	Disk         *DiskMetrics                 `json:"disk"`
	HWID         string                       `json:"hwid"`
	Architecture models.ContainerArchitecture `json:"architecture"`
	CollectedAt  time.Time                    `json:"collectedAt"` // Timestamp when metrics were collected
}

// CPUMetrics contains CPU-related metrics.
// Aligns with models.CPU from generator.go.
type CPUMetrics struct {
	TotalUsageMCpu float64 `json:"totalUsageMCpu"` // Total usage in milli-cores (1000m = 1 core)
	CoreCount      int     `json:"coreCount"`      // Number of CPU cores
	LoadPercent    float64 `json:"loadPercent"`    // Current CPU load as percentage (0-100)
}

// MemoryMetrics contains memory-related metrics.
// Aligns with models.Memory from generator.go.
type MemoryMetrics struct {
	CGroupUsedBytes  int64 `json:"cGroupUsedBytes"`  // Used bytes of the cgroup's memory
	CGroupTotalBytes int64 `json:"cGroupTotalBytes"` // Total bytes of the cgroup's memory
}

// DiskMetrics contains disk-related metrics.
// Aligns with models.Disk from generator.go.
type DiskMetrics struct {
	DataPartitionUsedBytes  int64 `json:"dataPartitionUsedBytes"`  // Used bytes of the disk's data partition
	DataPartitionTotalBytes int64 `json:"dataPartitionTotalBytes"` // Total bytes of the disk's data partition
}

// HealthStatus contains the health assessment of the container.
// This is used internally to calculate the final Health model.
type HealthStatus struct {
	CPUHealth       models.Health
	MemoryHealth    models.Health
	DiskHealth      models.Health
	ContainerHealth models.Health
}
