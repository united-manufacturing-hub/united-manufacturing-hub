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

package container

import (
	"context"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/container_monitor"
)

// ContainerWorker implements the Worker interface for container monitoring.
// It wraps the existing container_monitor.Service to collect metrics.
type ContainerWorker struct {
	identity       fsmv2.Identity
	monitorService container_monitor.Service
}

// NewContainerWorker creates a new container worker.
func NewContainerWorker(id string, name string, monitorService container_monitor.Service) *ContainerWorker {
	return &ContainerWorker{
		identity: fsmv2.Identity{
			ID:   id,
			Name: name,
		},
		monitorService: monitorService,
	}
}

// CollectObservedState monitors the actual system state.
// Called in a separate goroutine, can block for metric collection.
//
// This replaces the whole `_monitor` logic from FSM v1.
func (w *ContainerWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	// Get current metrics from the container monitor service
	serviceInfo, err := w.monitorService.GetStatus(ctx)
	if err != nil {
		return nil, err
	}

	// Convert to ContainerObservedState
	observed := &ContainerObservedState{
		// Observed-only fields from metrics
		CPUUsageMCores:   serviceInfo.CPU.TotalUsageMCpu,
		CPUCoreCount:     serviceInfo.CPU.CoreCount,
		CgroupCores:      serviceInfo.CPU.CgroupCores,
		ThrottleRatio:    serviceInfo.CPU.ThrottleRatio,
		IsThrottled:      serviceInfo.CPU.IsThrottled,
		MemoryUsedBytes:  serviceInfo.Memory.CGroupUsedBytes,
		MemoryTotalBytes: serviceInfo.Memory.CGroupTotalBytes,
		DiskUsedBytes:    serviceInfo.Disk.DataPartitionUsedBytes,
		DiskTotalBytes:   serviceInfo.Disk.DataPartitionTotalBytes,

		// Health assessments
		OverallHealth: serviceInfo.OverallHealth,
		CPUHealth:     serviceInfo.CPUHealth,
		MemoryHealth:  serviceInfo.MemoryHealth,
		DiskHealth:    serviceInfo.DiskHealth,

		CollectedAt: time.Now(),
	}

	return observed, nil
}

// DeriveDesiredState transforms user configuration into desired state.
// Pure function - no side effects. Called on each tick.
//
// The spec parameter comes from user configuration.
func (w *ContainerWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	return &ContainerDesiredState{
		shutdownRequested: false,
	}, nil
}

// GetInitialState returns the starting state for this worker.
// Called once during worker creation.
func (w *ContainerWorker) GetInitialState() fsmv2.State {
	// Container monitoring starts in stopped state
	return &StoppedState{}
}
