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
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// ContainerIdentity represents the immutable identity of a container.
// These fields never change after creation.
type ContainerIdentity struct {
	ID   string // Unique container identifier
	Name string // Human-readable name
}

// ContainerDesiredState represents what we want the container monitoring to be.
// This is derived from user configuration.
type ContainerDesiredState struct {
	MonitoringEnabled    bool // Whether monitoring is enabled
	CollectionIntervalMs int  // How often to collect metrics (milliseconds)
	shutdownRequested    bool // Set by supervisor to initiate shutdown
}

// ShutdownRequested returns whether shutdown has been requested.
// States MUST check this first in their Next() method.
func (d *ContainerDesiredState) ShutdownRequested() bool {
	return d.shutdownRequested
}

// SetShutdownRequested sets the shutdown flag.
// Called by the supervisor to initiate graceful shutdown.
func (d *ContainerDesiredState) SetShutdownRequested(requested bool) {
	d.shutdownRequested = requested
}

// GetMonitoringEnabled returns whether monitoring is enabled.
func (d *ContainerDesiredState) GetMonitoringEnabled() bool {
	return d.MonitoringEnabled
}

// GetCollectionIntervalMs returns the collection interval in milliseconds.
func (d *ContainerDesiredState) GetCollectionIntervalMs() int {
	return d.CollectionIntervalMs
}

// ContainerObservedState represents the actual state of container monitoring.
// It mirrors the desired state fields plus additional observed-only fields.
type ContainerObservedState struct {
	// Mirror of desired state (what the system believes it should be)
	ObservedDesiredState ContainerDesiredState

	// Observed-only fields (system reality)
	CPUUsageMCores   float64 // CPU usage in milli-cores
	CPUCoreCount     int     // Number of CPU cores
	CgroupCores      float64 // Cgroup CPU quota in cores
	ThrottleRatio    float64 // Percentage of periods throttled
	IsThrottled      bool    // Whether CPU is currently throttled
	MemoryUsedBytes  int64   // Memory used in bytes
	MemoryTotalBytes int64   // Total memory in bytes
	DiskUsedBytes    int64   // Disk used in bytes
	DiskTotalBytes   int64   // Total disk in bytes

	// Health assessments
	OverallHealth models.HealthCategory
	CPUHealth     models.HealthCategory
	MemoryHealth  models.HealthCategory
	DiskHealth    models.HealthCategory

	CollectedAt time.Time // When metrics were collected
}

// GetObservedDesiredState returns the mirror of the desired state.
// This allows comparing what's deployed vs what we want to deploy.
func (o *ContainerObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &o.ObservedDesiredState
}

// GetTimestamp returns when this observed state was collected.
func (o *ContainerObservedState) GetTimestamp() time.Time {
	return o.CollectedAt
}

// GetMonitoringEnabled returns the observed monitoring enabled state.
func (o *ContainerObservedState) GetMonitoringEnabled() bool {
	return o.ObservedDesiredState.MonitoringEnabled
}

// GetCollectionIntervalMs returns the observed collection interval.
func (o *ContainerObservedState) GetCollectionIntervalMs() int {
	return o.ObservedDesiredState.CollectionIntervalMs
}

// GetCPUUsageMCores returns CPU usage in milli-cores.
func (o *ContainerObservedState) GetCPUUsageMCores() float64 {
	return o.CPUUsageMCores
}

// GetCPUCoreCount returns the number of CPU cores.
func (o *ContainerObservedState) GetCPUCoreCount() int {
	return o.CPUCoreCount
}

// GetCgroupCores returns the cgroup CPU quota in cores.
func (o *ContainerObservedState) GetCgroupCores() float64 {
	return o.CgroupCores
}

// GetThrottleRatio returns the percentage of periods throttled.
func (o *ContainerObservedState) GetThrottleRatio() float64 {
	return o.ThrottleRatio
}

// GetIsThrottled returns whether CPU is currently throttled.
func (o *ContainerObservedState) GetIsThrottled() bool {
	return o.IsThrottled
}

// GetMemoryUsedBytes returns memory used in bytes.
func (o *ContainerObservedState) GetMemoryUsedBytes() int64 {
	return o.MemoryUsedBytes
}

// GetMemoryTotalBytes returns total memory in bytes.
func (o *ContainerObservedState) GetMemoryTotalBytes() int64 {
	return o.MemoryTotalBytes
}

// GetDiskUsedBytes returns disk used in bytes.
func (o *ContainerObservedState) GetDiskUsedBytes() int64 {
	return o.DiskUsedBytes
}

// GetDiskTotalBytes returns total disk in bytes.
func (o *ContainerObservedState) GetDiskTotalBytes() int64 {
	return o.DiskTotalBytes
}

// GetOverallHealth returns the overall health status.
func (o *ContainerObservedState) GetOverallHealth() string {
	return o.OverallHealth.String()
}

// GetCPUHealth returns the CPU health status.
func (o *ContainerObservedState) GetCPUHealth() string {
	return o.CPUHealth.String()
}

// GetMemoryHealth returns the memory health status.
func (o *ContainerObservedState) GetMemoryHealth() string {
	return o.MemoryHealth.String()
}

// GetDiskHealth returns the disk health status.
func (o *ContainerObservedState) GetDiskHealth() string {
	return o.DiskHealth.String()
}

// GetTimestampUnix returns the collection timestamp as Unix timestamp.
func (o *ContainerObservedState) GetTimestampUnix() int64 {
	return o.CollectedAt.Unix()
}
