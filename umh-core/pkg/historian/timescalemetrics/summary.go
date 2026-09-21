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

	"github.com/jackc/pgx/v5"
)

// Summary is the part of a historian's state reported without being asked: which
// tables it holds, and whether its background jobs are failing. Everything else --
// sizes, policies, per-table detail, the job list -- is read on request.
//
// FailedJobs counts jobs that exist and are not working. A historian with no
// policies at all has no jobs to fail, so a zero here is not on its own proof
// that data is being compressed or expired; the policy counts answer that and are
// read on request.
type Summary struct {
	TableNames []string `json:"tableNames"`
	FailedJobs int      `json:"failedJobs"`
}

// summaryTableNamesQuery lists the historian's hypertables and plain tables in
// one pass, without the sizes and policies the per-table view reads.
const summaryTableNamesQuery = `SELECT c.relname
  FROM pg_class c
  JOIN pg_namespace n ON n.oid = c.relnamespace
 WHERE n.nspname = $1 AND c.relkind = 'r' AND c.relname <> 'schema_migrations'
 ORDER BY c.relname`

// CollectSummary reads what the status message carries. Two statements, neither
// of whose cost grows with how much data the historian holds.
func CollectSummary(ctx context.Context, db Querier) (Summary, error) {
	var summary Summary

	var jobs int
	if err := db.QueryRow(ctx, jobsQuery, historianSchema).
		Scan(&jobs, &summary.FailedJobs); err != nil {
		return summary, fmt.Errorf("read job status: %w", err)
	}

	names, err := summaryTableNames(ctx, db)
	if err != nil {
		return summary, err
	}

	summary.TableNames = names

	return summary, nil
}

func summaryTableNames(ctx context.Context, db Querier) ([]string, error) {
	rows, err := db.Query(ctx, summaryTableNamesQuery, historianSchema)
	if err != nil {
		return nil, fmt.Errorf("read table names: %w", err)
	}

	all, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("read table names: %w", err)
	}

	// The schema holds whatever the customer put there. historianCreated is the
	// one definition of which tables are ours, shared with the split the on-demand
	// read performs, so the answer cannot drift between the two paths.
	names := []string{}

	for _, name := range all {
		if historianCreated(name) {
			names = append(names, name)
		}
	}

	return names, nil
}
