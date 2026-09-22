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
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// errTimescaleMissing reports a Postgres that answers but carries no TimescaleDB
// extension, so none of the catalog reads below would resolve.
var errTimescaleMissing = errors.New("timescaledb extension is not installed")

// Collect reads every figure the historian metrics view shows: the versions and
// database size, per-table storage and policies, the background jobs, and the
// oldest and newest timestamp each hypertable holds. Its only bound is ctx.
//
// The tables this product did not create are dropped before the per-table reads,
// so a customer's own table in the schema is never queried.
//
// pg_database_size runs last because it is the only read whose cost grows with
// the deployment: it stats every file backing the database, which tracks chunk
// count rather than stored bytes. A deployment large enough to make it slow
// therefore loses only DatabaseBytes, and still returns every other metric
// alongside the error.
func Collect(ctx context.Context, db Querier) (Metrics, error) {
	var metrics Metrics

	if err := readVersions(ctx, db, &metrics); err != nil {
		return metrics, err
	}

	hypertables, err := readHypertables(ctx, db)
	if err != nil {
		return metrics, err
	}

	plainTables, err := readPlainTables(ctx, db)
	if err != nil {
		return metrics, err
	}

	metrics.Tables = historianTables(append(hypertables, plainTables...))

	metrics.JobList, err = readJobs(ctx, db)
	if err != nil {
		return metrics, err
	}

	readable, err := readTimeColumnTables(ctx, db)
	if err != nil {
		return metrics, err
	}

	newest, err := collectTimestamps(ctx, db, metrics.Tables,
		newestTimestampQuery, "newest timestamps", readable)
	if err != nil {
		return metrics, err
	}

	oldest, err := collectTimestamps(ctx, db, metrics.Tables,
		oldestTimestampQuery, "oldest timestamps", readable)
	if err != nil {
		return metrics, err
	}

	rowCounts, err := readTableRows(ctx, db)
	if err != nil {
		return metrics, err
	}

	lookupCounts, err := countLookupTables(ctx, db, metrics.Tables)
	if err != nil {
		return metrics, err
	}

	for name, count := range lookupCounts {
		rowCounts[name] = count
	}

	metrics.DataSpanSeconds = dataSpanSeconds(oldest, newest)

	assignTimestamps(metrics.Tables, oldest, newest)
	assignRowCounts(metrics.Tables, rowCounts)

	if err := db.QueryRow(ctx, databaseSizeQuery).Scan(&metrics.DatabaseBytes); err != nil {
		return metrics, fmt.Errorf("read database size: %w", err)
	}

	return metrics, nil
}

func readVersions(ctx context.Context, db Querier, metrics *Metrics) error {
	var timescaleVersion *string
	if err := db.QueryRow(ctx, versionQuery).Scan(&metrics.ServerVersion, &timescaleVersion); err != nil {
		return fmt.Errorf("read versions: %w", err)
	}

	if timescaleVersion == nil {
		return errTimescaleMissing
	}

	metrics.TimescaleVersion = *timescaleVersion

	return nil
}

func readHypertables(ctx context.Context, db Querier) ([]Table, error) {
	rows, err := db.Query(ctx, tablesQuery, historianSchema)
	if err != nil {
		return nil, fmt.Errorf("read tables: %w", err)
	}
	defer rows.Close()

	var tables []Table

	for rows.Next() {
		table := Table{IsHypertable: true}
		if err := rows.Scan(
			&table.Name,
			&table.UncompressedBytes,
			&table.CompressedBytes,
			&table.Bytes,
			&table.ChunkIntervalSeconds,
			&table.CompressAfterSeconds,
			&table.DropAfterSeconds,
			&table.Chunks,
			&table.CompressedChunks,
		); err != nil {
			return nil, fmt.Errorf("scan table: %w", err)
		}

		tables = append(tables, table)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read tables: %w", err)
	}

	return tables, nil
}

func readPlainTables(ctx context.Context, db Querier) ([]Table, error) {
	rows, err := db.Query(ctx, regularTablesQuery, historianSchema)
	if err != nil {
		return nil, fmt.Errorf("read regular tables: %w", err)
	}
	defer rows.Close()

	var tables []Table

	for rows.Next() {
		var table Table
		if err := rows.Scan(&table.Name, &table.Bytes, &table.Rows); err != nil {
			return nil, fmt.Errorf("scan regular table: %w", err)
		}

		tables = append(tables, table)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read regular tables: %w", err)
	}

	return tables, nil
}

func readJobs(ctx context.Context, db Querier) ([]Job, error) {
	rows, err := db.Query(ctx, jobsListQuery, historianSchema)
	if err != nil {
		return nil, fmt.Errorf("read jobs: %w", err)
	}
	defer rows.Close()

	var jobs []Job

	for rows.Next() {
		var job Job
		if err := rows.Scan(
			&job.Kind,
			&job.Table,
			&job.Status,
			&job.ScheduleSeconds,
			&job.LastSuccessSeconds,
			&job.NextRunSeconds,
			&job.Failures,
		); err != nil {
			return nil, fmt.Errorf("scan job: %w", err)
		}

		jobs = append(jobs, job)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read jobs: %w", err)
	}

	return jobs, nil
}

// readTimeColumnTables names the hypertables whose time dimension is ts, which
// is the column the per-table timestamp reads take a max and min of.
func readTimeColumnTables(ctx context.Context, db Querier) (map[string]bool, error) {
	rows, err := db.Query(ctx, tsHypertablesQuery, historianSchema)
	if err != nil {
		return nil, fmt.Errorf("read time columns: %w", err)
	}
	defer rows.Close()

	readable := map[string]bool{}

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan time column: %w", err)
		}

		readable[name] = true
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read time columns: %w", err)
	}

	return readable, nil
}

func readTableRows(ctx context.Context, db Querier) (map[string]int64, error) {
	rows, err := db.Query(ctx, tableRowsQuery, historianSchema)
	if err != nil {
		return nil, fmt.Errorf("read table rows: %w", err)
	}
	defer rows.Close()

	counts, err := scanNamedValues(rows)
	if err != nil {
		return nil, fmt.Errorf("read table rows: %w", err)
	}

	return counts, nil
}

// scanNamedValues reads rows of (name, value) into a map.
func scanNamedValues(rows pgx.Rows) (map[string]int64, error) {
	values := map[string]int64{}

	for rows.Next() {
		var name string

		var value int64
		if err := rows.Scan(&name, &value); err != nil {
			return nil, err
		}

		values[name] = value
	}

	return values, rows.Err()
}

// timestampAt turns a Unix epoch into an RFC 3339 instant, or an empty string
// when there is none. Empty means no row was found, which is not the same as a
// row stamped in 1970.
func timestampAt(epoch int64) string {
	if epoch <= 0 {
		return ""
	}

	return time.Unix(epoch, 0).UTC().Format(time.RFC3339)
}

func assignTimestamps(tables []Table, oldest, newest map[string]int64) {
	for i := range tables {
		tables[i].OldestTimestamp = timestampAt(oldest[tables[i].Name])
		tables[i].NewestTimestamp = timestampAt(newest[tables[i].Name])
	}
}

// assignRowCounts fills in the row count of every table the counts cover. A table
// the map does not mention keeps the count it already has: the hypertable and
// plain-table counts are read separately, and one must not blank the other.
func assignRowCounts(tables []Table, counts map[string]int64) {
	for i := range tables {
		if count, ok := counts[tables[i].Name]; ok {
			tables[i].Rows = count
		}
	}
}

// dataSpanSeconds is the period between the oldest row in any table and the
// newest. It is taken from the rows themselves rather than from chunk boundaries,
// which reach into the future by up to one chunk width and so read long.
func dataSpanSeconds(oldestPerTable, newestPerTable map[string]int64) int64 {
	var oldest, newest int64

	for _, epoch := range oldestPerTable {
		if epoch > 0 && (oldest == 0 || epoch < oldest) {
			oldest = epoch
		}
	}

	for _, epoch := range newestPerTable {
		if epoch > newest {
			newest = epoch
		}
	}

	if oldest == 0 || newest <= oldest {
		return 0
	}

	return newest - oldest
}
