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

// Historian gauges, exported as umh_fsmv2_worker_<name>; a figure is current only while its _ready flag is 1.
const (
	gaugeHistorianReachable deps.GaugeName = "historian_reachable"

	gaugeHistorianAuthValid   deps.GaugeName = "historian_auth_valid"
	gaugeHistorianAuthInvalid deps.GaugeName = "historian_auth_invalid"
	gaugeHistorianAuthUnknown deps.GaugeName = "historian_auth_unknown"

	// gaugeHistorianQueryLatencySeconds is current while historian_auth_valid is 1.
	gaugeHistorianQueryLatencySeconds deps.GaugeName = "historian_query_latency_seconds"

	// gaugeHistorianSummaryReady applies to occupied_disk_bytes, data_span_seconds, jobs and failed_jobs.
	gaugeHistorianSummaryReady      deps.GaugeName = "historian_summary_ready"
	gaugeHistorianOccupiedDiskBytes deps.GaugeName = "historian_occupied_disk_bytes"
	gaugeHistorianDataSpanSeconds   deps.GaugeName = "historian_data_span_seconds"
	gaugeHistorianJobs              deps.GaugeName = "historian_jobs"
	gaugeHistorianFailedJobs        deps.GaugeName = "historian_failed_jobs"

	// gaugeHistorianDatabaseSizeReady applies to database_occupied_disk_bytes.
	gaugeHistorianDatabaseSizeReady         deps.GaugeName = "historian_database_size_ready"
	gaugeHistorianDatabaseOccupiedDiskBytes deps.GaugeName = "historian_database_occupied_disk_bytes"
)

func recordStatus(recorder *deps.MetricsRecorder, status TimescaleStatus) {
	recorder.SetGaugeFlag(gaugeHistorianReachable, status.Reachable)
	recorder.SetGaugeFlag(gaugeHistorianAuthValid, status.Auth == models.TimescaleAuthValid)
	recorder.SetGaugeFlag(gaugeHistorianAuthInvalid, status.Auth == models.TimescaleAuthInvalid)
	recorder.SetGaugeFlag(gaugeHistorianAuthUnknown, status.Auth == models.TimescaleAuthUnknown)

	// Poll measures the latency only when the check succeeds; LatencyMs is 0 otherwise.
	if status.Auth == models.TimescaleAuthValid {
		recorder.SetGauge(gaugeHistorianQueryLatencySeconds, status.LatencyMs/1000)
	}

	recordSummary(recorder, status.Summary)
	recordDatabaseOccupiedDiskBytes(recorder, status.DatabaseOccupiedDiskBytes)
}

func recordSummary(recorder *deps.MetricsRecorder, summary *timescalemetrics.Summary) {
	recorder.SetGaugeFlag(gaugeHistorianSummaryReady, summary != nil)

	if summary == nil {
		return
	}

	recorder.SetGauge(gaugeHistorianOccupiedDiskBytes, float64(summary.HistorianOccupiedDiskBytes))
	recorder.SetGauge(gaugeHistorianDataSpanSeconds, float64(summary.DataSpanSeconds))
	recorder.SetGauge(gaugeHistorianJobs, float64(summary.JobCount))
	recorder.SetGauge(gaugeHistorianFailedJobs, float64(summary.FailedJobCount))
}

func recordDatabaseOccupiedDiskBytes(recorder *deps.MetricsRecorder, databaseOccupiedDiskBytes *int64) {
	recorder.SetGaugeFlag(gaugeHistorianDatabaseSizeReady, databaseOccupiedDiskBytes != nil)

	if databaseOccupiedDiskBytes == nil {
		return
	}

	recorder.SetGauge(gaugeHistorianDatabaseOccupiedDiskBytes, float64(*databaseOccupiedDiskBytes))
}
