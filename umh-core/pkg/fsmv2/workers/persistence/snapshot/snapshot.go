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

package snapshot

import (
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// PersistenceDependencies is the dependencies interface for persistence actions (avoids import cycles).
type PersistenceDependencies interface {
	deps.Dependencies
	MetricsRecorder() *deps.MetricsRecorder
	GetStore() storage.TriangularStoreInterface
	GetScheduler() deps.Scheduler
	SetLastCompactionAt(time.Time)
	SetLastMaintenanceAt(time.Time)
}

// PersistenceDesiredState represents the target configuration for the persistence worker.
type PersistenceDesiredState struct {
	config.BaseDesiredState

	CompactionInterval  time.Duration `json:"compactionInterval"`
	RetentionWindow     time.Duration `json:"retentionWindow"`
	MaintenanceInterval time.Duration `json:"maintenanceInterval"`
}

var _ fsmv2.DesiredState = (*PersistenceDesiredState)(nil)

// PersistenceObservedState represents the current state of the persistence worker.
type PersistenceObservedState struct {
	CollectedAt       time.Time `json:"collected_at"`
	LastCompactionAt  time.Time `json:"last_compaction_at,omitempty"`
	LastMaintenanceAt time.Time `json:"last_maintenance_at,omitempty"`

	State string `json:"state"`

	LastActionResults []deps.ActionResult `json:"last_action_results,omitempty"`

	PersistenceDesiredState `json:",inline"`

	deps.MetricsEmbedder `json:",inline"`

	ConsecutiveActionErrors int `json:"consecutive_action_errors"`

	IsPreferredMaintenanceWindow  bool `json:"is_preferred_maintenance_window"`
	IsAcceptableMaintenanceWindow bool `json:"is_acceptable_maintenance_window"`
}

// IsLastActionHealthy returns true if ConsecutiveActionErrors is zero.
func (o PersistenceObservedState) IsLastActionHealthy() bool {
	return o.ConsecutiveActionErrors == 0
}

// IsHealthy returns true if the worker is operating normally.
func (o PersistenceObservedState) IsHealthy() bool {
	return o.IsLastActionHealthy()
}

func (o PersistenceObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &o.PersistenceDesiredState
}

func (o PersistenceObservedState) GetTimestamp() time.Time {
	return o.CollectedAt
}

func (o PersistenceObservedState) SetState(s string) fsmv2.ObservedState {
	o.State = s

	return o
}

func (o PersistenceObservedState) SetShutdownRequested(v bool) fsmv2.ObservedState {
	o.ShutdownRequested = v

	return o
}
