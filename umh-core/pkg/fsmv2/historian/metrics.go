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

package fsmv2timescale

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/historian/timescalemetrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

const (
	metricsNamespace = "umh"
	metricsSubsystem = "historian"

	labelHierarchyPath = "hierarchy_path"
	labelAuthState     = "state"
)

var authStates = []models.TimescaleAuthState{
	models.TimescaleAuthValid,
	models.TimescaleAuthInvalid,
	models.TimescaleAuthUnknown,
}

var (
	historianReachable = newHistorianGauge("reachable",
		"1 when the Historian database answered the last connection check, 0 when nothing answered")

	historianAuth = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "auth",
		Help:      "1 for the current authentication state of the Historian connection (valid, invalid, unknown), 0 for the others",
	}, []string{labelHierarchyPath, labelAuthState})

	historianQueryLatencySeconds = newHistorianGauge("query_latency_seconds",
		"Round-trip time of the last successful connection check query")

	historianOccupiedDiskBytes = newHistorianGauge("occupied_disk_bytes",
		"Disk space the Historian tables occupy")

	historianDataSpanSeconds = newHistorianGauge("data_span_seconds",
		"Time between the oldest and the newest row stored in the Historian")

	historianJobs = newHistorianGauge("jobs",
		"Number of TimescaleDB background jobs on the Historian tables")

	historianFailedJobs = newHistorianGauge("failed_jobs",
		"Number of TimescaleDB background jobs on the Historian tables whose last run failed")

	historianDatabaseOccupiedDiskBytes = newHistorianGauge("database_occupied_disk_bytes",
		"Disk space the whole Historian database occupies")
)

var historianSummaryGauges = []*prometheus.GaugeVec{
	historianOccupiedDiskBytes,
	historianDataSpanSeconds,
	historianJobs,
	historianFailedJobs,
}

var historianGauges = append([]*prometheus.GaugeVec{
	historianReachable,
	historianAuth,
	historianQueryLatencySeconds,
	historianDatabaseOccupiedDiskBytes,
}, historianSummaryGauges...)

func newHistorianGauge(name, help string) *prometheus.GaugeVec {
	return promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      name,
		Help:      help,
	}, []string{labelHierarchyPath})
}

// RecordHistorianStatus sets the umh_historian_* gauges from one observation.
// A nil figure deletes its series, because nil means the database cannot be read now.
func RecordHistorianStatus(hierarchyPath string, status TimescaleStatus) {
	historianReachable.WithLabelValues(hierarchyPath).Set(boolToGaugeValue(status.Reachable))
	recordQueryLatency(hierarchyPath, status)

	for _, state := range authStates {
		historianAuth.WithLabelValues(hierarchyPath, string(state)).Set(boolToGaugeValue(status.Auth == state))
	}

	recordSummary(hierarchyPath, status.Summary)
	recordDatabaseOccupiedDiskBytes(hierarchyPath, status.DatabaseOccupiedDiskBytes)
}

// Poll measures the latency only when the check succeeds, so a failed check
// leaves LatencyMs at 0, which is not a measurement.
func recordQueryLatency(hierarchyPath string, status TimescaleStatus) {
	if status.Auth != models.TimescaleAuthValid {
		historianQueryLatencySeconds.DeleteLabelValues(hierarchyPath)

		return
	}

	historianQueryLatencySeconds.WithLabelValues(hierarchyPath).Set(status.LatencyMs / 1000)
}

func recordSummary(hierarchyPath string, summary *timescalemetrics.Summary) {
	if summary == nil {
		for _, gauge := range historianSummaryGauges {
			gauge.DeleteLabelValues(hierarchyPath)
		}

		return
	}

	historianOccupiedDiskBytes.WithLabelValues(hierarchyPath).Set(float64(summary.HistorianOccupiedDiskBytes))
	historianDataSpanSeconds.WithLabelValues(hierarchyPath).Set(float64(summary.DataSpanSeconds))
	historianJobs.WithLabelValues(hierarchyPath).Set(float64(summary.JobCount))
	historianFailedJobs.WithLabelValues(hierarchyPath).Set(float64(summary.FailedJobCount))
}

func recordDatabaseOccupiedDiskBytes(hierarchyPath string, databaseOccupiedDiskBytes *int64) {
	if databaseOccupiedDiskBytes == nil {
		historianDatabaseOccupiedDiskBytes.DeleteLabelValues(hierarchyPath)

		return
	}

	historianDatabaseOccupiedDiskBytes.WithLabelValues(hierarchyPath).Set(float64(*databaseOccupiedDiskBytes))
}

// CleanupHistorianMetrics deletes every umh_historian_* series.
func CleanupHistorianMetrics() {
	for _, gauge := range historianGauges {
		gauge.Reset()
	}
}

func boolToGaugeValue(value bool) float64 {
	if value {
		return 1
	}

	return 0
}
