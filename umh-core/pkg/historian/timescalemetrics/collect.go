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
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const historianSchema = "umh"

const databaseOccupiedDiskBytesQuery = `SELECT pg_database_size(current_database())`

// errTimescaleMissing reports a Postgres with no TimescaleDB extension, where
// every catalog read below would fail.
var errTimescaleMissing = errors.New("timescaledb extension is not installed")

// Collect reads per-table storage and policies, the background jobs, and each
// hypertable's row timestamps. Its only bound is ctx.
//
// Tables this product did not create are dropped before the per-table reads, so
// a customer's own table is never queried.
func Collect(ctx context.Context, db Database) (Metrics, error) {
	var metrics Metrics

	if _, _, err := readVersions(ctx, db); err != nil {
		return metrics, err
	}

	tables, err := readHistorianTables(ctx, db)
	if err != nil {
		return metrics, err
	}

	metrics.Tables = tables

	metrics.JobList, err = readJobs(ctx, db)
	if err != nil {
		return metrics, err
	}

	spans, err := readSpans(ctx, db)
	if err != nil {
		return metrics, err
	}

	assignTimestamps(metrics.Tables, spans)

	rowCounts, err := readTableRows(ctx, db)
	if err != nil {
		return metrics, err
	}

	assignRowCounts(metrics.Tables, rowCounts)

	return metrics, nil
}

func readHistorianTables(ctx context.Context, db Database) ([]Table, error) {
	hypertables, err := readHypertables(ctx, db)
	if err != nil {
		return nil, err
	}

	plainTables, err := readPlainTables(ctx, db)
	if err != nil {
		return nil, err
	}

	return filterHistorianTables(append(hypertables, plainTables...)), nil
}

// readDatabaseOccupiedDiskBytes returns zero when the read fails, so a slow
// pg_database_size, which stats every file of the database, costs only this figure.
// https://github.com/postgres/postgres/blob/master/src/backend/utils/adt/dbsize.c
func readDatabaseOccupiedDiskBytes(ctx context.Context, db Database) int64 {
	var databaseOccupiedDiskBytes int64
	if err := db.QueryRow(ctx, databaseOccupiedDiskBytesQuery).Scan(&databaseOccupiedDiskBytes); err != nil {
		return 0
	}

	return databaseOccupiedDiskBytes
}

// queryAll runs one query and builds a T from each row with toValue, naming the
// read as subject in either error. CollectRows closes the rows and reports a
// failure part way through iteration.
// https://pkg.go.dev/github.com/jackc/pgx/v5#CollectRows
func queryAll[T any](
	ctx context.Context,
	db Database,
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

func readVersions(ctx context.Context, db Database) (string, string, error) {
	var postgresVersion string

	var timescaleVersion *string
	if err := db.QueryRow(ctx, versionQuery).Scan(&postgresVersion, &timescaleVersion); err != nil {
		return "", "", fmt.Errorf("read versions: %w", err)
	}

	if timescaleVersion == nil {
		return "", "", errTimescaleMissing
	}

	return postgresVersion, *timescaleVersion, nil
}

// Sizes come from the catalog rather than timescaledb_information's size
// functions, which stat every chunk's files and cost two orders of magnitude more
// at a few thousand chunks. An uncompressed chunk has no catalog row, and a partial
// one (status bit 8) keeps the rows written after compression in its own heap, so
// both are read from the relation.
const chunkOccupiedDiskBytes = `CASE WHEN ch.id IS NULL THEN 0
            WHEN ch.compressed_chunk_id IS NULL OR ch.status & 8 = 8
            THEN coalesce(s.compressed_heap_size + s.compressed_index_size + s.compressed_toast_size, 0)
               + coalesce(pg_total_relation_size(to_regclass(format('%I.%I', ch.schema_name, ch.table_name))), 0)
            ELSE s.compressed_heap_size + s.compressed_index_size + s.compressed_toast_size END`

const tablesQuery = `WITH sizes AS (
  SELECT h.id, h.schema_name, h.table_name,
         coalesce(sum(s.uncompressed_heap_size + s.uncompressed_index_size + s.uncompressed_toast_size), 0)::bigint AS bytes_before_compression,
         coalesce(sum(s.compressed_heap_size + s.compressed_index_size + s.compressed_toast_size), 0)::bigint AS bytes_after_compression,
         coalesce(sum(` + chunkOccupiedDiskBytes + `), 0)::bigint AS disk_bytes,
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

func readHypertables(ctx context.Context, db Database) ([]Table, error) {
	return queryAll(ctx, db, tablesQuery, "tables", rowToHypertable, historianSchema)
}

// tablesQuery orders its columns to read as SQL, not to match Table.
func rowToHypertable(row pgx.CollectableRow) (Table, error) {
	read := Table{IsHypertable: true}
	err := row.Scan(
		&read.Name,
		&read.BytesBeforeCompression,
		&read.BytesAfterCompression,
		&read.OccupiedDiskBytes,
		&read.ChunkIntervalSeconds,
		&read.CompressAfterSeconds,
		&read.RetentionSeconds,
		&read.Chunks,
		&read.CompressedChunks,
	)

	return read, err
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

func readPlainTables(ctx context.Context, db Database) ([]Table, error) {
	return queryAll(ctx, db, regularTablesQuery, "regular tables", rowToPlainTable, historianSchema)
}

func rowToPlainTable(row pgx.CollectableRow) (Table, error) {
	var table Table
	err := row.Scan(&table.Name, &table.OccupiedDiskBytes, &table.Rows)

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

func readJobs(ctx context.Context, db Database) ([]Job, error) {
	return queryAll(ctx, db, jobsListQuery, "jobs", pgx.RowToStructByPos[Job], historianSchema)
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

func readTableRows(ctx context.Context, db Database) (map[string]int64, error) {
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

// timestampAt returns an empty string for a table with no rows, which is not 1970.
func timestampAt(epoch *int64) string {
	if epoch == nil {
		return ""
	}

	return time.Unix(*epoch, 0).UTC().Format(time.RFC3339)
}

func assignTimestamps(tables []Table, spans map[string]rowSpan) {
	for i := range tables {
		span := spans[tables[i].Name]
		tables[i].EarliestRowTimestamp = timestampAt(span.Earliest)
		tables[i].LatestRowTimestamp = timestampAt(span.Latest)
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
func dataSpanSeconds(spans map[string]rowSpan) int64 {
	var earliest, latest *int64

	for _, span := range spans {
		if span.Earliest != nil && (earliest == nil || *span.Earliest < *earliest) {
			earliest = span.Earliest
		}

		if span.Latest != nil && (latest == nil || *span.Latest > *latest) {
			latest = span.Latest
		}
	}

	if earliest == nil || latest == nil || *latest <= *earliest {
		return 0
	}

	return *latest - *earliest
}

const chunksQuery = `SELECT h.table_name, ch.schema_name, ch.table_name
  FROM _timescaledb_catalog.hypertable h
  JOIN _timescaledb_catalog.dimension d ON d.hypertable_id = h.id AND d.column_name = 'ts'
  JOIN _timescaledb_catalog.chunk ch ON ch.hypertable_id = h.id AND NOT ch.dropped
  JOIN _timescaledb_catalog.chunk_constraint cc ON cc.chunk_id = ch.id
  JOIN _timescaledb_catalog.dimension_slice ds ON ds.id = cc.dimension_slice_id AND ds.dimension_id = d.id
 WHERE h.schema_name = $1
 ORDER BY h.table_name, ds.range_start, ch.id`

type chunkRow struct {
	Table       string
	ChunkSchema string
	ChunkName   string
}

func readChunks(ctx context.Context, db Database) (map[string][]pgx.Identifier, error) {
	rows, err := queryAll(ctx, db, chunksQuery, "chunks", pgx.RowToStructByPos[chunkRow], historianSchema)
	if err != nil {
		return nil, err
	}

	chunks := map[string][]pgx.Identifier{}

	for _, row := range rows {
		if !isHistorianTable(row.Table) {
			continue
		}

		chunks[row.Table] = append(chunks[row.Table], pgx.Identifier{row.ChunkSchema, row.ChunkName})
	}

	return chunks, nil
}

func readSpans(ctx context.Context, db Database) (map[string]rowSpan, error) {
	chunks, err := readChunks(ctx, db)
	if err != nil {
		return nil, err
	}

	return readSpansOf(ctx, db, chunks)
}

// A table name cannot be bound as a parameter, so it is formatted in as the
// literal labelling the row. safeTableName guards every name that reaches it.
const (
	earliestRowQuery = `SELECT '%s', extract(epoch FROM min(ts))::bigint FROM %s`
	latestRowQuery   = `SELECT '%s', extract(epoch FROM max(ts))::bigint FROM %s`
)

var tableNamePattern = regexp.MustCompile(`^[a-z0-9_]+$`)

func safeTableName(name string) bool {
	return tableNamePattern.MatchString(name)
}

// rowSpan is one table's first and last row timestamp as epoch seconds.
type rowSpan struct {
	Earliest *int64
	Latest   *int64
}

// A query over a whole hypertable locks every chunk of it, and across all
// hypertables in one statement that exceeds max_locks_per_transaction.
// https://www.postgresql.org/docs/current/runtime-config-locks.html
func readSpansOf(ctx context.Context, db Database, chunks map[string][]pgx.Identifier) (map[string]rowSpan, error) {
	oldestFirst := map[string][]pgx.Identifier{}
	newestFirst := map[string][]pgx.Identifier{}

	for table, tableChunks := range chunks {
		if !safeTableName(table) {
			continue
		}

		oldestFirst[table] = tableChunks
		newestFirst[table] = slices.Clone(tableChunks)
		slices.Reverse(newestFirst[table])
	}

	earliest, err := readFirstAnswer(ctx, db, earliestRowQuery, oldestFirst)
	if err != nil {
		return nil, err
	}

	latest, err := readFirstAnswer(ctx, db, latestRowQuery, newestFirst)
	if err != nil {
		return nil, err
	}

	spans := make(map[string]rowSpan, len(earliest))
	for table, epoch := range earliest {
		spans[table] = rowSpan{Earliest: epoch, Latest: latest[table]}
	}

	return spans, nil
}

type namedEpoch struct {
	Name  string
	Epoch *int64
}

// A chunk whose rows were all deleted answers null.
func readFirstAnswer(
	ctx context.Context,
	db Database,
	query string,
	chunks map[string][]pgx.Identifier,
) (map[string]*int64, error) {
	answers := map[string]*int64{}

	for position := 0; ; position++ {
		selects := []string{}

		for table, tableChunks := range chunks {
			if answers[table] != nil || position >= len(tableChunks) {
				continue
			}

			selects = append(selects, fmt.Sprintf(query, table, tableChunks[position].Sanitize()))
		}

		if len(selects) == 0 {
			return answers, nil
		}

		rows, err := queryAll(ctx, db, strings.Join(selects, " UNION ALL "), "row timestamps", pgx.RowToStructByPos[namedEpoch])
		if err != nil {
			return nil, err
		}

		for _, row := range rows {
			if row.Epoch != nil {
				answers[row.Name] = row.Epoch
			}
		}
	}
}
