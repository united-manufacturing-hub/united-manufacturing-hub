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

// errTimescaleMissing reports a Postgres that answers but carries no TimescaleDB
// extension, so none of the catalog reads below would resolve.
var errTimescaleMissing = errors.New("timescaledb extension is not installed")

// Collect reads every figure the historian metrics view shows: the versions and
// database size, per-table storage and policies, the background jobs, and the
// earliest and latest row timestamp each hypertable holds. Its only bound is ctx.
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

// queryAll runs one query and builds a T from each row it returns, using
// toValue. Both failures are wrapped with subject, so an error says which read
// it was. pgx closes the rows itself and reports a failure part way through
// iteration in the error CollectRows returns.
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

// tablesQuery reads every hypertable's storage, chunking and policies in one pass.
// The before-and-after sizes come from the catalog, which holds them per chunk
// already; the size functions in timescaledb_information stat every chunk's files
// to answer the same question and cost two orders of magnitude more at a few
// thousand chunks. A chunk no policy has compressed yet has no catalog row, so its
// size has to be read from the relation itself or the table reads as empty.
const tablesQuery = `SELECT h.table_name,
       coalesce(sum(s.uncompressed_heap_size + s.uncompressed_index_size + s.uncompressed_toast_size), 0)::bigint,
       coalesce(sum(s.compressed_heap_size + s.compressed_index_size + s.compressed_toast_size), 0)::bigint,
       coalesce(sum(CASE WHEN ch.id IS NULL THEN 0
                         WHEN ch.compressed_chunk_id IS NULL
                         THEN coalesce(pg_total_relation_size(to_regclass(format('%I.%I', ch.schema_name, ch.table_name))), 0)
                         ELSE s.compressed_heap_size + s.compressed_index_size + s.compressed_toast_size END), 0)::bigint,
       coalesce(max(EXTRACT(EPOCH FROM d.time_interval))::bigint, 0),
       coalesce(max(EXTRACT(EPOCH FROM (cj.config->>'compress_after')::interval))::bigint, 0),
       coalesce(max(EXTRACT(EPOCH FROM (rj.config->>'drop_after')::interval))::bigint, 0),
       count(ch.id),
       count(ch.compressed_chunk_id)
  FROM _timescaledb_catalog.hypertable h
  LEFT JOIN _timescaledb_catalog.chunk ch ON ch.hypertable_id = h.id AND NOT ch.dropped
  LEFT JOIN _timescaledb_catalog.compression_chunk_size s ON s.chunk_id = ch.id
  LEFT JOIN timescaledb_information.dimensions d
    ON d.hypertable_schema = h.schema_name AND d.hypertable_name = h.table_name AND d.column_name = 'ts'
  LEFT JOIN timescaledb_information.jobs cj
    ON cj.hypertable_schema = h.schema_name AND cj.hypertable_name = h.table_name AND cj.proc_name = 'policy_compression'
  LEFT JOIN timescaledb_information.jobs rj
    ON rj.hypertable_schema = h.schema_name AND rj.hypertable_name = h.table_name AND rj.proc_name = 'policy_retention'
 WHERE h.schema_name = $1
 GROUP BY h.table_name
 ORDER BY h.table_name`

func readHypertables(ctx context.Context, db Querier) ([]Table, error) {
	return queryAll(ctx, db, tablesQuery, "tables", rowToHypertable, historianSchema)
}

// rowToHypertable scans tablesQuery, whose columns are ordered to read as SQL
// rather than to match Table field for field.
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

// A lookup table's rows come from the same live-tuple tracking as a chunk's,
// which matters more here: these are written rarely enough that autovacuum may
// never analyse them, leaving reltuples at zero for a populated table.
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

// jobsListQuery is scoped to the historian schema so it excludes the built-in
// policy_telemetry job, which carries no hypertable and fails on every run of an
// air-gapped deployment. Listing it would report a permanent false failure. Its
// column order matches Job field for field, which is what lets pgx map the row.
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

// readTimeColumnTables names the hypertables whose time dimension is ts, which
// is the column the per-table timestamp reads take a max and min of.
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

// tableRowsQuery counts rows per hypertable without scanning one, because
// Postgres stores no row count: under MVCC a count walks the rows to see which
// are visible, and that is what a historian cannot afford.
//
// A compressed chunk carries the count the compressor recorded, which is exact.
// An uncompressed one contributes n_live_tup, which the cumulative statistics
// system tracks as rows are inserted, so it is right before anything has
// analysed the chunk, give or take the interval at which a backend flushes what
// it has counted. reltuples stands beside it because it survives a
// pg_stat_reset, which sets n_live_tup back to zero; whichever has seen the
// rows reports the larger number.
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

// timestampAt turns a Unix epoch into an RFC 3339 instant, or an empty string
// when there is none. Empty means no row was found, which is not the same as a
// row stamped in 1970.
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

// dataSpanSeconds is the period between the earliest row in any table and the
// latest. It is taken from the rows themselves rather than from chunk boundaries,
// which reach into the future by up to one chunk width and so read long.
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

// The two queries below are per-table selects, joined with UNION ALL into one
// statement. A table name cannot be bound as a parameter, so each is a format
// string taking the table name, the schema, and the table name again -- the
// first as the literal that labels the row, the last two as the identifier.
// Every name is checked against safeTableName before it is interpolated.

const latestRowTimestampQuery = `SELECT '%s', coalesce(extract(epoch FROM max(ts))::bigint, 0) FROM %s.%s`

const earliestRowTimestampQuery = `SELECT '%s', coalesce(extract(epoch FROM min(ts))::bigint, 0) FROM %s.%s`

var tableNamePattern = regexp.MustCompile(`^[a-z0-9_]+$`)

func safeTableName(name string) bool {
	return tableNamePattern.MatchString(name)
}

// collectTimestamps reads one end of the time column of every hypertable that
// has one, as a single statement joining one select per table with UNION ALL,
// and returns what each table reported. A table name cannot be bound as a
// parameter, so safeTableName guards every name that reaches the format string.
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

// unionOverTables asks query of every readable hypertable in one statement,
// filling the table name into each select. A table absent from readable carries
// no ts column to take a max or min of. It returns an empty string when no table
// qualifies, which the caller reads as nothing to ask.
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
