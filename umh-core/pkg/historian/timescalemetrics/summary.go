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
)

// Summary is the part of a historian's state reported without being asked.
// Everything else is read on request, through Collect.
//
// A historian with no policies has no jobs to fail, so FailedJobs at zero is not
// proof that data is compressed or expired.
type Summary struct {
	TableNames []string `json:"tableNames"`
	FailedJobs int      `json:"failedJobs"`
}

// Both figures in one round trip, and neither subquery's cost grows with how
// much data the historian holds.
const summaryQuery = `SELECT
       (SELECT count(*) FILTER (WHERE s.last_run_status = 'Failed')
          FROM timescaledb_information.jobs j
          LEFT JOIN timescaledb_information.job_stats s USING (job_id)
         WHERE j.hypertable_schema = $1),
       (SELECT coalesce(array_agg(c.relname ORDER BY c.relname), '{}')
          FROM pg_class c
          JOIN pg_namespace n ON n.oid = c.relnamespace
         WHERE n.nspname = $1 AND c.relkind = 'r' AND c.relname <> 'schema_migrations')`

// CollectSummary reads what the status message carries.
func CollectSummary(ctx context.Context, db Querier) (Summary, error) {
	var summary Summary

	var names []string
	if err := db.QueryRow(ctx, summaryQuery, historianSchema).
		Scan(&summary.FailedJobs, &names); err != nil {
		return summary, fmt.Errorf("read historian summary: %w", err)
	}

	// The schema holds whatever the customer put there. historianCreated is the one
	// definition of which tables are ours, so the two read paths cannot drift.
	summary.TableNames = []string{}

	for _, name := range names {
		if historianCreated(name) {
			summary.TableNames = append(summary.TableNames, name)
		}
	}

	return summary, nil
}
