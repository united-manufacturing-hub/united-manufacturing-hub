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
