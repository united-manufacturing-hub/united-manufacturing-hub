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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/historian/timescalemetrics"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// Historian gauges, exported as umh_fsmv2_worker_<name>. A figure that cannot
// be read keeps its last value, and its _ready flag turns 0.
const (
	GaugeHistorianReachable deps.GaugeName = "historian_reachable"

	GaugeHistorianAuthValid   deps.GaugeName = "historian_auth_valid"
	GaugeHistorianAuthInvalid deps.GaugeName = "historian_auth_invalid"
	GaugeHistorianAuthUnknown deps.GaugeName = "historian_auth_unknown"

	// GaugeHistorianQueryLatencySeconds is current while historian_auth_valid is 1.
	GaugeHistorianQueryLatencySeconds deps.GaugeName = "historian_query_latency_seconds"

	// GaugeHistorianSummaryReady applies to occupied_disk_bytes, data_span_seconds, jobs and failed_jobs.
	GaugeHistorianSummaryReady      deps.GaugeName = "historian_summary_ready"
	GaugeHistorianOccupiedDiskBytes deps.GaugeName = "historian_occupied_disk_bytes"
	GaugeHistorianDataSpanSeconds   deps.GaugeName = "historian_data_span_seconds"
	GaugeHistorianJobs              deps.GaugeName = "historian_jobs"
	GaugeHistorianFailedJobs        deps.GaugeName = "historian_failed_jobs"

	// GaugeHistorianDatabaseSizeReady applies to database_occupied_disk_bytes.
	GaugeHistorianDatabaseSizeReady         deps.GaugeName = "historian_database_size_ready"
	GaugeHistorianDatabaseOccupiedDiskBytes deps.GaugeName = "historian_database_occupied_disk_bytes"
)

// recordStatus sets the Historian gauges from one observation.
func recordStatus(recorder *deps.MetricsRecorder, status TimescaleStatus) {
	recorder.SetGaugeFlag(GaugeHistorianReachable, status.Reachable)
	recorder.SetGaugeFlag(GaugeHistorianAuthValid, status.Auth == models.TimescaleAuthValid)
	recorder.SetGaugeFlag(GaugeHistorianAuthInvalid, status.Auth == models.TimescaleAuthInvalid)
	recorder.SetGaugeFlag(GaugeHistorianAuthUnknown, status.Auth == models.TimescaleAuthUnknown)

	// Poll measures the latency only when the check succeeds; LatencyMs is 0 otherwise.
	if status.Auth == models.TimescaleAuthValid {
		recorder.SetGauge(GaugeHistorianQueryLatencySeconds, status.LatencyMs/1000)
	}

	recordSummary(recorder, status.Summary)
	recordDatabaseOccupiedDiskBytes(recorder, status.DatabaseOccupiedDiskBytes)
}

func recordSummary(recorder *deps.MetricsRecorder, summary *timescalemetrics.Summary) {
	recorder.SetGaugeFlag(GaugeHistorianSummaryReady, summary != nil)

	if summary == nil {
		return
	}

	recorder.SetGauge(GaugeHistorianOccupiedDiskBytes, float64(summary.HistorianOccupiedDiskBytes))
	recorder.SetGauge(GaugeHistorianDataSpanSeconds, float64(summary.DataSpanSeconds))
	recorder.SetGauge(GaugeHistorianJobs, float64(summary.JobCount))
	recorder.SetGauge(GaugeHistorianFailedJobs, float64(summary.FailedJobCount))
}

func recordDatabaseOccupiedDiskBytes(recorder *deps.MetricsRecorder, databaseOccupiedDiskBytes *int64) {
	recorder.SetGaugeFlag(GaugeHistorianDatabaseSizeReady, databaseOccupiedDiskBytes != nil)

	if databaseOccupiedDiskBytes == nil {
		return
	}

	recorder.SetGauge(GaugeHistorianDatabaseOccupiedDiskBytes, float64(*databaseOccupiedDiskBytes))
}
