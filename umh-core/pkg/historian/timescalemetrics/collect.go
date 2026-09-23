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
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const historianSchema = "umh"

const databaseSizeQuery = `SELECT pg_database_size(current_database())`

// errTimescaleMissing reports a Postgres with no TimescaleDB extension, where
// every catalog read below would fail.
var errTimescaleMissing = errors.New("timescaledb extension is not installed")

// Collect reads the whole picture: versions, database size, per-table storage
// and policies, the background jobs, and each hypertable's row timestamps. Its
// only bound is ctx.
//
// Foreign tables are dropped before the per-table reads, so a customer's own
// table is never queried. pg_database_size runs last because it stats every file
// backing the database: when it is slow, only DatabaseBytes is lost.
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

	latest, err := collectTimestamps(ctx, db, metrics.Tables,
		latestRowTimestampQuery, "latest row timestamps", readable)
	if err != nil {
		return metrics, err
	}

	earliest, err := collectTimestamps(ctx, db, metrics.Tables,
		earliestRowTimestampQuery, "earliest row timestamps", readable)
	if err != nil {
		return metrics, err
	}

	rowCounts, err := readTableRows(ctx, db)
	if err != nil {
		return metrics, err
	}

	metrics.DataSpanSeconds = dataSpanSeconds(earliest, latest)

	assignTimestamps(metrics.Tables, earliest, latest)
	assignRowCounts(metrics.Tables, rowCounts)

	if err := db.QueryRow(ctx, databaseSizeQuery).Scan(&metrics.DatabaseBytes); err != nil {
		return metrics, fmt.Errorf("read database size: %w", err)
	}

	return metrics, nil
}

// queryAll runs one query and builds a T from each row with toValue, naming the
// read as subject in either error. CollectRows closes the rows and reports a
// failure part way through iteration.
// https://pkg.go.dev/github.com/jackc/pgx/v5#CollectRows
func queryAll[T any](
	ctx context.Context,
	db Querier,
	query string,
	subject string,
	toValue pgx.RowToFunc[T],
	args ...any,
) ([]T, error) {
	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", subject, err)
	}

	values, err := pgx.CollectRows(rows, toValue)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", subject, err)
	}

	return values, nil
}

const versionQuery = `SELECT current_setting('server_version'),
       (SELECT extversion FROM pg_extension WHERE extname = 'timescaledb')`

func readVersions(ctx context.Context, db Querier, metrics *Metrics) error {
	var timescaleVersion *string
	if err := db.QueryRow(ctx, versionQuery).Scan(&metrics.PostgresVersion, &timescaleVersion); err != nil {
		return fmt.Errorf("read versions: %w", err)
	}

	if timescaleVersion == nil {
		return errTimescaleMissing
	}

	metrics.TimescaleVersion = *timescaleVersion

	return nil
}

// Sizes come from the catalog rather than timescaledb_information's size
// functions, which stat every chunk's files and cost two orders of magnitude more
// at a few thousand chunks. An uncompressed chunk has no catalog row, so its size
// has to come from the relation or the table reads as empty.
const tablesQuery = `WITH sizes AS (
  SELECT h.id, h.schema_name, h.table_name,
         coalesce(sum(s.uncompressed_heap_size + s.uncompressed_index_size + s.uncompressed_toast_size), 0)::bigint AS bytes_before_compression,
         coalesce(sum(s.compressed_heap_size + s.compressed_index_size + s.compressed_toast_size), 0)::bigint AS bytes_after_compression,
         coalesce(sum(CASE WHEN ch.id IS NULL THEN 0
                           WHEN ch.compressed_chunk_id IS NULL
                           THEN coalesce(pg_total_relation_size(to_regclass(format('%I.%I', ch.schema_name, ch.table_name))), 0)
                           ELSE s.compressed_heap_size + s.compressed_index_size + s.compressed_toast_size END), 0)::bigint AS disk_bytes,
         count(ch.id) AS chunks,
         count(ch.compressed_chunk_id) AS compressed_chunks
    FROM _timescaledb_catalog.hypertable h
    LEFT JOIN _timescaledb_catalog.chunk ch ON ch.hypertable_id = h.id AND NOT ch.dropped
    LEFT JOIN _timescaledb_catalog.compression_chunk_size s ON s.chunk_id = ch.id
   WHERE h.schema_name = $1
   GROUP BY h.id
)
SELECT DISTINCT ON (t.table_name) t.table_name,
       t.bytes_before_compression,
       t.bytes_after_compression,
       t.disk_bytes,
       coalesce(EXTRACT(EPOCH FROM d.time_interval)::bigint, 0),
       coalesce(EXTRACT(EPOCH FROM (cj.config->>'compress_after')::interval)::bigint, 0),
       coalesce(EXTRACT(EPOCH FROM (rj.config->>'drop_after')::interval)::bigint, 0),
       t.chunks,
       t.compressed_chunks
  FROM sizes t
  LEFT JOIN timescaledb_information.dimensions d
    ON d.hypertable_schema = t.schema_name AND d.hypertable_name = t.table_name AND d.column_name = 'ts'
  LEFT JOIN timescaledb_information.jobs cj
    ON cj.hypertable_schema = t.schema_name AND cj.hypertable_name = t.table_name AND cj.proc_name = 'policy_compression'
  LEFT JOIN timescaledb_information.jobs rj
    ON rj.hypertable_schema = t.schema_name AND rj.hypertable_name = t.table_name AND rj.proc_name = 'policy_retention'
 ORDER BY t.table_name`

func readHypertables(ctx context.Context, db Querier) ([]Table, error) {
	return queryAll(ctx, db, tablesQuery, "tables", rowToHypertable, historianSchema)
}

// tablesQuery orders its columns to read as SQL, not to match Table.
func rowToHypertable(row pgx.CollectableRow) (Table, error) {
	table := Table{IsHypertable: true}
	err := row.Scan(
		&table.Name,
		&table.BytesBeforeCompression,
		&table.BytesAfterCompression,
		&table.DiskBytes,
		&table.ChunkIntervalSeconds,
		&table.CompressAfterSeconds,
		&table.RetentionSeconds,
		&table.Chunks,
		&table.CompressedChunks,
	)

	return table, err
}

// Lookup tables are written rarely enough that autovacuum may never analyse
// them, which leaves reltuples at zero for a populated table.
const regularTablesQuery = `SELECT c.relname, pg_total_relation_size(c.oid)::bigint,
       greatest(coalesce(st.n_live_tup, 0), greatest(c.reltuples, 0)::bigint)
  FROM pg_class c
  JOIN pg_namespace n ON n.oid = c.relnamespace
  LEFT JOIN pg_stat_all_tables st ON st.relid = c.oid
 WHERE n.nspname = $1 AND c.relkind = 'r'
   AND c.relname <> 'schema_migrations'
   AND NOT EXISTS (SELECT 1 FROM _timescaledb_catalog.hypertable h
                    WHERE h.schema_name = n.nspname AND h.table_name = c.relname)
 ORDER BY c.relname`

func readPlainTables(ctx context.Context, db Querier) ([]Table, error) {
	return queryAll(ctx, db, regularTablesQuery, "regular tables", rowToPlainTable, historianSchema)
}

func rowToPlainTable(row pgx.CollectableRow) (Table, error) {
	var table Table
	err := row.Scan(&table.Name, &table.DiskBytes, &table.Rows)

	return table, err
}

// Scoping to the historian schema excludes the built-in policy_telemetry job,
// which fails on every run of an air-gapped deployment. The column order matches
// Job field for field, which is what lets pgx map the row.
const jobsListQuery = `SELECT
       CASE j.proc_name
         WHEN 'policy_compression' THEN 'compression'
         WHEN 'policy_retention' THEN 'retention'
         ELSE j.proc_name
       END,
       coalesce(j.hypertable_name, ''),
       coalesce(s.last_run_status, ''),
       coalesce(extract(epoch FROM j.schedule_interval)::bigint, 0),
       CASE WHEN s.last_successful_finish IS NULL OR s.last_successful_finish = '-infinity'::timestamptz THEN 0
            ELSE greatest(extract(epoch FROM (now() - s.last_successful_finish))::bigint, 0) END,
       CASE WHEN s.next_start IS NULL OR s.next_start = '-infinity'::timestamptz OR s.next_start = 'infinity'::timestamptz THEN 0
            ELSE greatest(extract(epoch FROM (s.next_start - now()))::bigint, 0) END,
       coalesce(s.last_run_status = 'Failed', false)
  FROM timescaledb_information.jobs j
  LEFT JOIN timescaledb_information.job_stats s USING (job_id)
 WHERE j.hypertable_schema = $1
 ORDER BY coalesce(s.last_run_status = 'Failed', false) DESC, j.hypertable_name, j.job_id`

func readJobs(ctx context.Context, db Querier) ([]Job, error) {
	return queryAll(ctx, db, jobsListQuery, "jobs", pgx.RowToStructByPos[Job], historianSchema)
}

const tsHypertablesQuery = `SELECT h.table_name
  FROM _timescaledb_catalog.hypertable h
  JOIN _timescaledb_catalog.dimension d ON d.hypertable_id = h.id
 WHERE h.schema_name = $1 AND d.column_name = 'ts'`

// readTimeColumnTables names the hypertables whose time dimension is ts.
func readTimeColumnTables(ctx context.Context, db Querier) (map[string]bool, error) {
	names, err := queryAll(ctx, db, tsHypertablesQuery, "time columns", pgx.RowTo[string], historianSchema)
	if err != nil {
		return nil, err
	}

	readable := make(map[string]bool, len(names))
	for _, name := range names {
		readable[name] = true
	}

	return readable, nil
}

// Counting rows outright is what a historian cannot afford: Postgres stores no
// count, so under MVCC a count walks every row. A compressed chunk carries the
// count the compressor recorded; an uncompressed one contributes n_live_tup,
// which tracks inserts and so is right before anything analyses the chunk, a
// flush interval behind. reltuples stands beside it because it survives a
// pg_stat_reset, which zeroes n_live_tup.
// https://www.postgresql.org/docs/current/monitoring-stats.html
const tableRowsQuery = `SELECT h.table_name,
       coalesce(sum(s.numrows_pre_compression), 0)
     + coalesce(sum(CASE WHEN ch.compressed_chunk_id IS NULL
                         THEN greatest(coalesce(st.n_live_tup, 0), greatest(c.reltuples, 0)::bigint)
                         ELSE 0 END), 0)
  FROM _timescaledb_catalog.hypertable h
  JOIN _timescaledb_catalog.chunk ch ON ch.hypertable_id = h.id AND NOT ch.dropped
  LEFT JOIN _timescaledb_catalog.compression_chunk_size s ON s.chunk_id = ch.id
  LEFT JOIN pg_namespace n ON n.nspname = ch.schema_name
  LEFT JOIN pg_class c ON c.relname = ch.table_name AND c.relnamespace = n.oid
  LEFT JOIN pg_stat_all_tables st ON st.relid = c.oid
 WHERE h.schema_name = $1
 GROUP BY h.table_name`

func readTableRows(ctx context.Context, db Querier) (map[string]int64, error) {
	pairs, err := queryAll(ctx, db, tableRowsQuery, "table rows", rowToNamedValue, historianSchema)
	if err != nil {
		return nil, err
	}

	return valuesByName(pairs), nil
}

// namedValue is one row of a query that reports a single figure per table.
type namedValue struct {
	Name  string
	Value int64
}

var rowToNamedValue = pgx.RowToStructByPos[namedValue]

func valuesByName(pairs []namedValue) map[string]int64 {
	values := make(map[string]int64, len(pairs))
	for _, pair := range pairs {
		values[pair.Name] = pair.Value
	}

	return values
}

// timestampAt returns an empty string for no row, which is not 1970.
func timestampAt(epoch int64) string {
	if epoch <= 0 {
		return ""
	}

	return time.Unix(epoch, 0).UTC().Format(time.RFC3339)
}

func assignTimestamps(tables []Table, earliest, latest map[string]int64) {
	for i := range tables {
		tables[i].EarliestRowTimestamp = timestampAt(earliest[tables[i].Name])
		tables[i].LatestRowTimestamp = timestampAt(latest[tables[i].Name])
	}
}

// A table the counts do not mention keeps what it has: hypertable and plain-table
// counts are read separately, and one must not blank the other.
func assignRowCounts(tables []Table, counts map[string]int64) {
	for i := range tables {
		if count, ok := counts[tables[i].Name]; ok {
			tables[i].Rows = count
		}
	}
}

// Taken from the rows rather than the chunk boundaries, which reach up to one
// chunk width into the future and so read long.
func dataSpanSeconds(earliestPerTable, latestPerTable map[string]int64) int64 {
	var earliest, latest int64

	for _, epoch := range earliestPerTable {
		if epoch > 0 && (earliest == 0 || epoch < earliest) {
			earliest = epoch
		}
	}

	for _, epoch := range latestPerTable {
		if epoch > latest {
			latest = epoch
		}
	}

	if earliest == 0 || latest <= earliest {
		return 0
	}

	return latest - earliest
}

// A table name cannot be bound as a parameter, so these two are format strings
// taking the name three times: once as the literal labelling the row, twice as
// the identifier. safeTableName guards every name that reaches them.

const latestRowTimestampQuery = `SELECT '%s', coalesce(extract(epoch FROM max(ts))::bigint, 0) FROM %s.%s`

const earliestRowTimestampQuery = `SELECT '%s', coalesce(extract(epoch FROM min(ts))::bigint, 0) FROM %s.%s`

var tableNamePattern = regexp.MustCompile(`^[a-z0-9_]+$`)

func safeTableName(name string) bool {
	return tableNamePattern.MatchString(name)
}

// collectTimestamps reads one end of the ts column of every readable hypertable
// and returns what each reported.
func collectTimestamps(
	ctx context.Context,
	db Querier,
	tables []Table,
	query string,
	subject string,
	readable map[string]bool,
) (map[string]int64, error) {
	statement := unionOverTables(tables, query, readable)
	if statement == "" {
		return map[string]int64{}, nil
	}

	pairs, err := queryAll(ctx, db, statement, subject, rowToNamedValue)
	if err != nil {
		return nil, err
	}

	return valuesByName(pairs), nil
}

// unionOverTables asks query of every readable hypertable in one statement.
// Empty when no table qualifies, which the caller reads as nothing to ask.
func unionOverTables(tables []Table, query string, readable map[string]bool) string {
	selects := make([]string, 0, len(tables))

	for _, table := range tables {
		if !table.IsHypertable || !readable[table.Name] || !safeTableName(table.Name) {
			continue
		}

		selects = append(selects, fmt.Sprintf(query, table.Name, historianSchema, table.Name))
	}

	return strings.Join(selects, " UNION ALL ")
}
