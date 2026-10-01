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

package timescalemetrics

import (
	"context"
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// Summary is the part of a historian's state reported without being asked.
// Everything else is read on request, through Collect.
//
// A historian with no policies has no jobs to fail, so FailedJobCount at zero is not
// proof that data is compressed or expired.
type Summary struct {
	Versions                   []models.Version `json:"versions"`
	TableNames                 []string         `json:"tableNames"`
	HistorianOccupiedDiskBytes int64            `json:"historianOccupiedDiskBytes"`
	DataSpanSeconds            int64            `json:"dataSpanSeconds"`
	JobCount                   int              `json:"jobCount"`
	FailedJobCount             int              `json:"failedJobCount"`
}

const jobCountsQuery = `SELECT count(*), count(*) FILTER (WHERE s.last_run_status = 'Failed')
  FROM timescaledb_information.jobs j
  LEFT JOIN timescaledb_information.job_stats s USING (job_id)
 WHERE j.hypertable_schema = $1`

// CollectSummary reads what the status message carries.
func CollectSummary(ctx context.Context, db Database) (Summary, error) {
	var summary Summary

	postgresVersion, timescaleVersion, err := readVersions(ctx, db)
	if err != nil {
		return summary, err
	}

	tables, err := readHistorianTables(ctx, db)
	if err != nil {
		return summary, err
	}

	var jobCount, failedJobCount int
	if err := db.QueryRow(ctx, jobCountsQuery, historianSchema).Scan(&jobCount, &failedJobCount); err != nil {
		return summary, fmt.Errorf("read job counts: %w", err)
	}

	spans, err := readSpans(ctx, db)
	if err != nil {
		return summary, err
	}

	summary = Summary{
		Versions: []models.Version{
			{Name: "PostgreSQL", Version: postgresVersion},
			{Name: "TimescaleDB", Version: timescaleVersion},
		},
		TableNames:                 tableNames(tables),
		HistorianOccupiedDiskBytes: occupiedDiskBytes(tables),
		DataSpanSeconds:            dataSpanSeconds(spans),
		JobCount:                   jobCount,
		FailedJobCount:             failedJobCount,
	}

	return summary, nil
}

func tableNames(tables []Table) []string {
	names := make([]string, 0, len(tables))
	for _, table := range tables {
		names = append(names, table.Name)
	}

	return names
}

func occupiedDiskBytes(tables []Table) int64 {
	var total int64
	for _, table := range tables {
		total += table.OccupiedDiskBytes
	}

	return total
}
