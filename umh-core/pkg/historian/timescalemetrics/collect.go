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

// Querier is the read surface this package needs. Both *pgx.Conn and
// *pgxpool.Pool satisfy it, so a one-shot caller opens a single connection while
// a long-lived one keeps its pool.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Table is one hypertable's storage, chunking and policy settings. The
// aggregates beside it answer whether the historian as a whole compresses and
// expires data; this answers which table does not.
type Table struct {
	Name string `json:"name"`
	// Bytes is what the table occupies on disk now: the compressed chunks at their
	// compressed size plus the chunks no policy has compressed yet.
	Bytes int64 `json:"bytes"`
	// UncompressedBytes and CompressedBytes cover the compressed chunks only,
	// because that is what TimescaleDB records a before size for. They answer how
	// much compression saved, not how large the table is.
	UncompressedBytes    int64 `json:"uncompressedBytes"`
	CompressedBytes      int64 `json:"compressedBytes"`
	ChunkIntervalSeconds int64 `json:"chunkIntervalSeconds"`
	CompressAfterSeconds int64 `json:"compressAfterSeconds"`
	DropAfterSeconds     int64 `json:"dropAfterSeconds"`
	// NewestTimestamp and OldestTimestamp are the max and min of the table's time
	// column, not when it was written: Postgres records no write time for a table,
	// and a bridge writing history backwards would make the two differ. They are
	// RFC 3339 instants rather than ages, because a reader that receives an age
	// resolves it against its own clock, which puts the reported moment out by
	// however far that clock is wrong. Empty means no row was found, which is not
	// the same as a row stamped in 1970.
	NewestTimestamp string `json:"newestTimestamp"`
	OldestTimestamp string `json:"oldestTimestamp"`
	// Rows is approximate, read from planner statistics rather than by counting:
	// an exact count scans every chunk, which a historian cannot afford.
	Rows             int64 `json:"rows"`
	Chunks           int   `json:"chunks"`
	CompressedChunks int   `json:"compressedChunks"`
	IsHypertable     bool  `json:"isHypertable"`
}

// OtherTables summarises the tables in the historian schema that this product did
// not create.
type OtherTables struct {
	Tables int   `json:"tables"`
	Bytes  int64 `json:"bytes"`
	Rows   int64 `json:"rows"`
}

type Job struct {
	Kind               string `json:"kind"`
	Table              string `json:"table"`
	Status             string `json:"status"`
	ScheduleSeconds    int64  `json:"scheduleSeconds"`
	LastSuccessSeconds int64  `json:"lastSuccessSeconds"`
	NextRunSeconds     int64  `json:"nextRunSeconds"`
	Failures           int    `json:"failures"`
}

// Metrics is the aggregate operational picture of the historian database,
// embedded into TimescaleStatus so its fields flatten to the top JSON level.
type Metrics struct {
	ServerVersion     string `json:"serverVersion"`
	TimescaleVersion  string `json:"timescaleVersion"`
	DatabaseBytes     int64  `json:"databaseBytes"`
	UncompressedBytes int64  `json:"uncompressedBytes"`
	CompressedBytes   int64  `json:"compressedBytes"`
	// DataSpanSeconds is the period the data covers, from the oldest row in any
	// table to the newest. Divided into DatabaseBytes it gives a growth rate.
	DataSpanSeconds  int64   `json:"dataSpanSeconds"`
	Hypertables      int     `json:"hypertables"`
	Chunks           int     `json:"chunks"`
	CompressedChunks int     `json:"compressedChunks"`
	Tables           []Table `json:"tables"`
	// OtherTables aggregates every table in the schema the historian did not
	// create, so the database size stays explainable without listing them.
	OtherTables OtherTables `json:"otherTables"`
	JobList     []Job       `json:"jobList"`
}

const historianSchema = "umh"

// errTimescaleMissing reports a Postgres that answers but carries no TimescaleDB
// extension, so none of the catalog reads below would resolve.
var errTimescaleMissing = errors.New("timescaledb extension is not installed")

const versionQuery = `SELECT current_setting('server_version'),
       (SELECT extversion FROM pg_extension WHERE extname = 'timescaledb')`

const countsQuery = `SELECT count(DISTINCT h.id), count(ch.id), count(ch.compressed_chunk_id)
  FROM _timescaledb_catalog.hypertable h
  LEFT JOIN _timescaledb_catalog.chunk ch
    ON ch.hypertable_id = h.id AND NOT ch.dropped
 WHERE h.schema_name = $1`

const compressionQuery = `SELECT
       coalesce(sum(s.uncompressed_heap_size + s.uncompressed_index_size + s.uncompressed_toast_size), 0),
       coalesce(sum(s.compressed_heap_size + s.compressed_index_size + s.compressed_toast_size), 0)
  FROM _timescaledb_catalog.compression_chunk_size s
  JOIN _timescaledb_catalog.chunk ch ON ch.id = s.chunk_id
  JOIN _timescaledb_catalog.hypertable h ON h.id = ch.hypertable_id
 WHERE h.schema_name = $1`

// jobsQuery is scoped to the historian schema so it excludes the built-in
// policy_telemetry job, which carries no hypertable and fails on every run of an
// air-gapped deployment. Counting it would report a permanent false failure.
const jobsQuery = `SELECT count(*), count(*) FILTER (WHERE s.last_run_status = 'Failed')
  FROM timescaledb_information.jobs j
  LEFT JOIN timescaledb_information.job_stats s USING (job_id)
 WHERE j.hypertable_schema = $1`

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
       coalesce(s.total_failures, 0)
  FROM timescaledb_information.jobs j
  LEFT JOIN timescaledb_information.job_stats s USING (job_id)
 WHERE j.hypertable_schema = $1
 ORDER BY coalesce(s.total_failures, 0) DESC, j.hypertable_name, j.job_id`

const databaseSizeQuery = `SELECT pg_database_size(current_database())`

const regularTablesQuery = `SELECT c.relname, pg_total_relation_size(c.oid)::bigint, greatest(c.reltuples, 0)::bigint
  FROM pg_class c
  JOIN pg_namespace n ON n.oid = c.relnamespace
 WHERE n.nspname = $1 AND c.relkind = 'r'
   AND c.relname <> 'schema_migrations'
   AND NOT EXISTS (SELECT 1 FROM _timescaledb_catalog.hypertable h
                    WHERE h.schema_name = n.nspname AND h.table_name = c.relname)
 ORDER BY c.relname`

const tsHypertablesQuery = `SELECT h.table_name
  FROM _timescaledb_catalog.hypertable h
  JOIN _timescaledb_catalog.dimension d ON d.hypertable_id = h.id
 WHERE h.schema_name = $1 AND d.column_name = 'ts'`

// tableRowsQuery counts rows per hypertable without scanning one. A compressed
// chunk records its own pre-compression count, which is exact; an uncompressed
// chunk contributes the planner's estimate, which autovacuum maintains and which
// is -1 until it first runs. The catalog count is exact for compressed chunks
// because it is what the compressor recorded; an estimate derived from batch
// counts is not, since a batch holds anything up to a thousand rows.
const tableRowsQuery = `SELECT h.table_name,
       coalesce(sum(s.numrows_pre_compression), 0)
     + coalesce(sum(CASE WHEN ch.compressed_chunk_id IS NULL
                         THEN greatest(c.reltuples, 0)::bigint ELSE 0 END), 0)
  FROM _timescaledb_catalog.hypertable h
  JOIN _timescaledb_catalog.chunk ch ON ch.hypertable_id = h.id AND NOT ch.dropped
  LEFT JOIN _timescaledb_catalog.compression_chunk_size s ON s.chunk_id = ch.id
  LEFT JOIN pg_namespace n ON n.nspname = ch.schema_name
  LEFT JOIN pg_class c ON c.relname = ch.table_name AND c.relnamespace = n.oid
 WHERE h.schema_name = $1
 GROUP BY h.table_name`

// The three queries below are per-table selects, joined with UNION ALL into one
// statement. A table name cannot be bound as a parameter, so each is a format
// string taking the table name, the schema, and the table name again -- the
// first as the literal that labels the row, the last two as the identifier.
// Every name is checked against safeTableName before it is interpolated.

const newestTimestampQuery = `SELECT '%s', coalesce(extract(epoch FROM max(ts))::bigint, 0) FROM %s.%s`

const oldestTimestampQuery = `SELECT '%s', coalesce(extract(epoch FROM min(ts))::bigint, 0) FROM %s.%s`

const lookupCountQuery = `SELECT '%s', count(*)::bigint FROM %s.%s`

var tableNamePattern = regexp.MustCompile(`^[a-z0-9_]+$`)

func safeTableName(name string) bool {
	return tableNamePattern.MatchString(name)
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

func assignOldestTimestamps(tables []Table, epochs map[string]int64) {
	for i := range tables {
		tables[i].OldestTimestamp = timestampAt(epochs[tables[i].Name])
	}
}

func assignNewestTimestamps(tables []Table, epochs map[string]int64) {
	for i := range tables {
		tables[i].NewestTimestamp = timestampAt(epochs[tables[i].Name])
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

// collectMetrics reads the historian database's aggregate operational picture.
//
// The reads run cheapest-first and pg_database_size runs last, because it is the
// only one whose cost grows with the deployment: it stats every file backing the
// database, which tracks chunk count rather than stored bytes. A deployment large
// enough to make it slow therefore loses only DatabaseBytes, and returns every
// metric collected before it.
// collectPerTable runs one statement built from a per-table select, joined with
// UNION ALL, and returns the value each table reported. A table name cannot be
// bound as a parameter, so keep decides which tables qualify and safeTableName
// guards every name that reaches the format string.
func collectPerTable(
	ctx context.Context,
	db Querier,
	tables []Table,
	query string,
	what string,
	keep func(Table) bool,
) (map[string]int64, error) {
	statement := perTableStatement(tables, query, keep)
	if statement == "" {
		return map[string]int64{}, nil
	}

	rows, err := db.Query(ctx, statement)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", what, err)
	}
	defer rows.Close()

	values, err := scanNamedValues(rows)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", what, err)
	}

	return values, nil
}

// perTableStatement joins one copy of query per qualifying table with UNION ALL,
// filling in the table name three times: once as the literal that labels the row
// and twice as the identifier. It returns an empty string when no table
// qualifies, which the caller reads as nothing to ask.
func perTableStatement(tables []Table, query string, keep func(Table) bool) string {
	selects := make([]string, 0, len(tables))

	for _, table := range tables {
		if !safeTableName(table.Name) || !keep(table) {
			continue
		}

		selects = append(selects, fmt.Sprintf(query, table.Name, historianSchema, table.Name))
	}

	return strings.Join(selects, " UNION ALL ")
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

// collectNewestTimestamps reads the newest row of each hypertable. Only
// hypertables with a ts column qualify: the others carry no time column to take a
// max of.
func collectNewestTimestamps(ctx context.Context, db Querier, tables []Table) (map[string]int64, error) {
	readable, err := timeColumnTables(ctx, db)
	if err != nil {
		return nil, err
	}

	return collectPerTable(ctx, db, tables, newestTimestampQuery, "newest timestamps",
		func(table Table) bool { return table.IsHypertable && readable[table.Name] })
}

// collectOldestTimestamps reads the oldest row of each hypertable.
func collectOldestTimestamps(ctx context.Context, db Querier, tables []Table) (map[string]int64, error) {
	readable, err := timeColumnTables(ctx, db)
	if err != nil {
		return nil, err
	}

	return collectPerTable(ctx, db, tables, oldestTimestampQuery, "oldest timestamps",
		func(table Table) bool { return table.IsHypertable && readable[table.Name] })
}

// countLookupTables counts the historian's own plain tables exactly. The planner
// estimate they would otherwise carry stays zero until autovacuum first analyses
// them, which on a small, rarely-written lookup table may never happen, leaving a
// populated table reporting no rows at all. They are bounded by how many distinct
// tags exist rather than by ingest rate, so counting them outright is affordable
// where counting a hypertable is not.
func countLookupTables(ctx context.Context, db Querier, tables []Table) (map[string]int64, error) {
	return collectPerTable(ctx, db, tables, lookupCountQuery, "lookup table counts",
		func(table Table) bool { return !table.IsHypertable && historianCreated(table.Name) })
}

func collectMetrics(ctx context.Context, db Querier) (Metrics, error) {
	var metrics Metrics

	var timescaleVersion *string
	if err := db.QueryRow(ctx, versionQuery).Scan(&metrics.ServerVersion, &timescaleVersion); err != nil {
		return metrics, fmt.Errorf("read versions: %w", err)
	}

	if timescaleVersion == nil {
		return metrics, errTimescaleMissing
	}

	metrics.TimescaleVersion = *timescaleVersion

	if err := db.QueryRow(ctx, countsQuery, historianSchema).
		Scan(&metrics.Hypertables, &metrics.Chunks, &metrics.CompressedChunks); err != nil {
		return metrics, fmt.Errorf("read table counts: %w", err)
	}

	if err := db.QueryRow(ctx, compressionQuery, historianSchema).
		Scan(&metrics.UncompressedBytes, &metrics.CompressedBytes); err != nil {
		return metrics, fmt.Errorf("read compression totals: %w", err)
	}

	tables, err := collectTables(ctx, db)
	if err != nil {
		return metrics, err
	}

	metrics.Tables = tables

	jobs, err := collectJobs(ctx, db)
	if err != nil {
		return metrics, err
	}

	metrics.JobList = jobs

	if err := db.QueryRow(ctx, databaseSizeQuery).Scan(&metrics.DatabaseBytes); err != nil {
		return metrics, fmt.Errorf("read database size: %w", err)
	}

	return metrics, nil
}

func collectJobs(ctx context.Context, db Querier) ([]Job, error) {
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

func collectTables(ctx context.Context, db Querier) ([]Table, error) {
	rows, err := db.Query(ctx, tablesQuery, historianSchema)
	if err != nil {
		return nil, fmt.Errorf("read tables: %w", err)
	}
	defer rows.Close()

	var tables []Table

	for rows.Next() {
		var table Table
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

		table.IsHypertable = true
		tables = append(tables, table)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read tables: %w", err)
	}

	regular, err := collectRegularTables(ctx, db)
	if err != nil {
		return nil, err
	}

	return append(tables, regular...), nil
}

func timeColumnTables(ctx context.Context, db Querier) (map[string]bool, error) {
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

func collectRegularTables(ctx context.Context, db Querier) ([]Table, error) {
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

// Collect reads every figure the historian metrics view shows: the catalog
// aggregates, per-table storage and policies, the background jobs, and when each
// hypertable was first and last written. Its only bound is ctx.
func Collect(ctx context.Context, db Querier) (Metrics, error) {
	metrics, err := collectMetrics(ctx, db)
	if err != nil {
		return metrics, err
	}

	newest, err := collectNewestTimestamps(ctx, db, metrics.Tables)
	if err != nil {
		return metrics, err
	}

	oldest, err := collectOldestTimestamps(ctx, db, metrics.Tables)
	if err != nil {
		return metrics, err
	}

	rowCounts, err := tableRows(ctx, db)
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

	assignNewestTimestamps(metrics.Tables, newest)
	assignOldestTimestamps(metrics.Tables, oldest)
	assignRowCounts(metrics.Tables, rowCounts)

	metrics.Tables, metrics.OtherTables = splitForeignTables(metrics.Tables)

	return metrics, nil
}

// historianTables names what benthos-umh's historian output creates: two
// hypertables per data contract, plus the shared lookup tables.
var historianTablePrefixes = []string{"value_", "attribute_"}

var historianTableNames = map[string]bool{"tag": true, "topic": true, "location": true}

func historianCreated(name string) bool {
	if historianTableNames[name] {
		return true
	}

	for _, prefix := range historianTablePrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}

	return false
}

// splitForeignTables separates the historian's own tables from everything else in
// the schema. The rest are counted and summed rather than listed: they are not
// this product's to explain, but they occupy disk the reported database size
// includes, so dropping them silently would leave the total unaccounted for.
func splitForeignTables(tables []Table) ([]Table, OtherTables) {
	kept := make([]Table, 0, len(tables))

	var others OtherTables

	for _, table := range tables {
		if historianCreated(table.Name) {
			kept = append(kept, table)

			continue
		}

		others.Tables++
		others.Bytes += table.Bytes
		others.Rows += table.Rows
	}

	return kept, others
}

func tableRows(ctx context.Context, db Querier) (map[string]int64, error) {
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
