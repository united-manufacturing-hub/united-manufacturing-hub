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

// Package snapshot holds the persistence worker's Config and Status value
// types plus the PersistenceDependencies interface consumed by actions. It
// exists as a separate leaf package so the state sub-package can depend on
// these types without introducing an import cycle with the worker package.
//
// Post-PR2-C9 the persistence worker uses fsmv2.Observation[PersistenceStatus]
// and *fsmv2.WrappedDesiredState[PersistenceConfig]; the underlying value
// types are defined here and re-exported from the worker package as type
// aliases for caller convenience.
package snapshot

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// PersistenceConfig holds the user-provided configuration for the persistence worker.
// Embeds BaseUserSpec to expose GetState() for WorkerBase.DeriveDesiredState, allowing
// WorkerBase.DeriveDesiredState to extract the desired state from the "state"
// YAML field.
type PersistenceConfig struct {
	config.BaseUserSpec `yaml:",inline"`
	CompactionInterval  time.Duration `json:"compactionInterval"  yaml:"compactionInterval"`
	RetentionWindow     time.Duration `json:"retentionWindow"     yaml:"retentionWindow"`
	MaintenanceInterval time.Duration `json:"maintenanceInterval" yaml:"maintenanceInterval"`
}

// PersistenceStatus holds the runtime observation data for the persistence worker.
// Framework fields (CollectedAt, State, LastActionResults, MetricsEmbedder,
// ShutdownRequested, children counts) are carried by fsmv2.Observation[PersistenceStatus]
// and are not duplicated here.
type PersistenceStatus struct {
	// LastCompactionAt records the most recent successful compaction timestamp.
	LastCompactionAt time.Time `json:"last_compaction_at"`
	// LastMaintenanceAt records the most recent successful maintenance timestamp.
	LastMaintenanceAt time.Time `json:"last_maintenance_at"`
	// ConsecutiveActionErrors counts how many action attempts have failed in a row.
	ConsecutiveActionErrors int `json:"consecutive_action_errors"`
	// IsPreferredMaintenanceWindow reports whether the current time is a
	// preferred maintenance window (weekend + low-traffic hour).
	IsPreferredMaintenanceWindow bool `json:"is_preferred_maintenance_window"`
	// IsAcceptableMaintenanceWindow reports whether the current time is an
	// acceptable maintenance window (low-traffic hour).
	IsAcceptableMaintenanceWindow bool `json:"is_acceptable_maintenance_window"`
}

// IsLastActionHealthy returns true if ConsecutiveActionErrors is zero.
func (s PersistenceStatus) IsLastActionHealthy() bool {
	return s.ConsecutiveActionErrors == 0
}

// IsHealthy returns true if the worker is operating normally.
func (s PersistenceStatus) IsHealthy() bool {
	return s.IsLastActionHealthy()
}

// PersistenceDependencies is the dependencies interface for persistence actions (avoids import cycles).
type PersistenceDependencies interface {
	deps.Dependencies
	MetricsRecorder() *deps.MetricsRecorder
	GetStore() storage.TriangularStoreInterface
	GetScheduler() deps.Scheduler
	SetLastCompactionAt(time.Time)
	SetLastMaintenanceAt(time.Time)
}
