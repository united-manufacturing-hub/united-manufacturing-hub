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

// Table is one hypertable's storage, chunking and policy settings. The
// aggregates beside it answer whether the historian as a whole compresses and
// expires data; this answers which table does not.
type Table struct {
	Name                 string `json:"name"`
	UncompressedBytes    int64  `json:"uncompressedBytes"`
	CompressedBytes      int64  `json:"compressedBytes"`
	ChunkIntervalSeconds int64  `json:"chunkIntervalSeconds"`
	CompressAfterSeconds int64  `json:"compressAfterSeconds"`
	DropAfterSeconds     int64  `json:"dropAfterSeconds"`
	// LastWriteAt and FirstWriteAt are RFC 3339 instants, not ages. A reader that
	// receives an age has to resolve it against its own clock, which puts the
	// reported moment out by however far that clock is wrong. Empty means no row
	// was found, which is not the same as a row written this second.
	LastWriteAt  string `json:"lastWriteAt"`
	FirstWriteAt string `json:"firstWriteAt"`
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
	ServerVersion    string `json:"serverVersion"`
	TimescaleVersion string `json:"timescaleVersion"`
	// MetricsError carries why the last collection failed, so a database that
	// answers the connection check but refuses the metric reads explains itself
	// instead of reporting zeros.
	MetricsError string `json:"metricsError"`
	// LastJobError is the most recent background-job failure message. A bare
	// failure count says nothing an operator can act on; this names the table and
	// the reason.
	LastJobError      string `json:"lastJobError"`
	DatabaseBytes     int64  `json:"databaseBytes"`
	UncompressedBytes int64  `json:"uncompressedBytes"`
	CompressedBytes   int64  `json:"compressedBytes"`
	// DataSpanSeconds is how much history the hypertables cover, from the oldest
	// chunk's start to the newest chunk's end. Divided into DatabaseBytes it gives a
	// growth rate. It reads slightly long, because the newest chunk extends into the
	// future by up to its own width.
	DataSpanSeconds  int64   `json:"dataSpanSeconds"`
	Hypertables      int     `json:"hypertables"`
	Chunks           int     `json:"chunks"`
	CompressedChunks int     `json:"compressedChunks"`
	Jobs             int     `json:"jobs"`
	CompressionJobs  int     `json:"compressionJobs"`
	RetentionJobs    int     `json:"retentionJobs"`
	FailedJobs       int     `json:"failedJobs"`
	Tables           []Table `json:"tables"`
	// OtherTables aggregates every table in the schema the historian did not
	// create, so the database size stays explainable without listing them.
	OtherTables OtherTables `json:"otherTables"`
	JobList     []Job       `json:"jobList"`
	// PoliciesUniform reports whether every hypertable agrees on its intervals.
	// When false the single reported interval describes only the shortest table,
	// and the rest have to be read from the database.
	PoliciesUniform bool `json:"policiesUniform"`
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

// policyQuery reports the compression and retention policies as aggregates. The
// intervals are the shortest any hypertable uses, so the figure is the soonest a
// chunk is compressed or dropped rather than a flattering maximum; PoliciesUniform
// says whether one figure describes every table.
const policyQuery = `SELECT
       count(*) FILTER (WHERE proc_name = 'policy_compression'),
       count(*) FILTER (WHERE proc_name = 'policy_retention'),
       count(DISTINCT config->>'compress_after') <= 1 AND count(DISTINCT config->>'drop_after') <= 1
  FROM timescaledb_information.jobs
 WHERE hypertable_schema = $1`

// lastJobErrorQuery names the most recent background-job failure. job_errors has no
// schema column, so scoping to the historian's own jobs means joining back to jobs
// on job_id -- otherwise the built-in policy_telemetry job, which fails on every run
// of an air-gapped deployment and carries an empty message, is what surfaces.
const lastJobErrorQuery = `SELECT e.err_message
  FROM timescaledb_information.job_errors e
  JOIN timescaledb_information.jobs j USING (job_id)
 WHERE j.hypertable_schema = $1 AND coalesce(e.err_message, '') <> ''
 ORDER BY e.start_time DESC
 LIMIT 1`

// tablesQuery reads every hypertable's storage, chunking and policies in one pass.
// It reads _timescaledb_catalog rather than hypertable_detailed_size, which stats
// the files behind every chunk and measured 2105-2980ms cold at 7112 chunks, past
// the observation deadline; this returns in 17ms at the same scale.
const tablesQuery = `SELECT h.table_name,
       coalesce(sum(s.uncompressed_heap_size + s.uncompressed_index_size + s.uncompressed_toast_size), 0)::bigint,
       coalesce(sum(s.compressed_heap_size + s.compressed_index_size + s.compressed_toast_size), 0)::bigint,
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

// dataSpanQuery reads the chunk time range from _timescaledb_catalog rather than
// timescaledb_information.chunks: the same answer, 7ms against 57ms at 7112 chunks.
// The dimension_slice bounds are microseconds since the epoch.
const dataSpanQuery = `SELECT coalesce((max(ds.range_end) - min(ds.range_start)) / 1000000, 0)
  FROM _timescaledb_catalog.dimension_slice ds
  JOIN _timescaledb_catalog.dimension d ON d.id = ds.dimension_id
  JOIN _timescaledb_catalog.hypertable h ON h.id = d.hypertable_id
 WHERE h.schema_name = $1`

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
// is -1 until it first runs. approximate_row_count is not used: it assumes every
// compression batch is full, and reported 189011 rows for a 200-row table whose
// chunks each held seven.
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

const lastWriteQuery = `SELECT '%s', coalesce(extract(epoch FROM max(ts))::bigint, 0) FROM %s.%s`

const firstWriteQuery = `SELECT '%s', coalesce(extract(epoch FROM min(ts))::bigint, 0) FROM %s.%s`

const lookupCountQuery = `SELECT '%s', count(*)::bigint FROM %s.%s`

var tableNamePattern = regexp.MustCompile(`^[a-z0-9_]+$`)

func safeTableName(name string) bool {
	return tableNamePattern.MatchString(name)
}

func withFreshness(tables []Table, writes map[string]int64, now time.Time) []Table {
	applied := make([]Table, 0, len(tables))

	for _, table := range tables {
		written, ok := writes[table.Name]
		if ok && written > 0 {
			table.LastWriteAt = time.Unix(written, 0).UTC().Format(time.RFC3339)
		}

		applied = append(applied, table)
	}

	return applied
}

func collectFreshness(ctx context.Context, pool Querier, tables []Table) (map[string]int64, error) {
	readable, err := timeColumnTables(ctx, pool)
	if err != nil {
		return nil, err
	}

	selects := make([]string, 0, len(tables))

	for _, table := range tables {
		if !table.IsHypertable || !readable[table.Name] || !safeTableName(table.Name) {
			continue
		}

		selects = append(selects, fmt.Sprintf(
			lastWriteQuery, table.Name, historianSchema, table.Name))
	}

	if len(selects) == 0 {
		return map[string]int64{}, nil
	}

	rows, err := pool.Query(ctx, strings.Join(selects, " UNION ALL "))
	if err != nil {
		return nil, fmt.Errorf("read freshness: %w", err)
	}
	defer rows.Close()

	writes := make(map[string]int64, len(selects))

	for rows.Next() {
		var name string

		var written int64
		if err := rows.Scan(&name, &written); err != nil {
			return nil, fmt.Errorf("scan freshness: %w", err)
		}

		writes[name] = written
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read freshness: %w", err)
	}

	return writes, nil
}

// collectMetrics reads the historian database's aggregate operational picture.
//
// The reads run cheapest-first and pg_database_size runs last, because it is the
// only one whose cost grows with the deployment: it stats every file backing the
// database, which tracks chunk count rather than stored bytes. A deployment large
// enough to make it slow therefore loses only DatabaseBytes, and returns every
// metric collected before it.
func collectMetrics(ctx context.Context, pool Querier) (Metrics, error) {
	var metrics Metrics

	var timescaleVersion *string
	if err := pool.QueryRow(ctx, versionQuery).Scan(&metrics.ServerVersion, &timescaleVersion); err != nil {
		return metrics, fmt.Errorf("read versions: %w", err)
	}

	if timescaleVersion == nil {
		return metrics, errTimescaleMissing
	}

	metrics.TimescaleVersion = *timescaleVersion

	if err := pool.QueryRow(ctx, countsQuery, historianSchema).
		Scan(&metrics.Hypertables, &metrics.Chunks, &metrics.CompressedChunks); err != nil {
		return metrics, fmt.Errorf("read table counts: %w", err)
	}

	if err := pool.QueryRow(ctx, compressionQuery, historianSchema).
		Scan(&metrics.UncompressedBytes, &metrics.CompressedBytes); err != nil {
		return metrics, fmt.Errorf("read compression totals: %w", err)
	}

	if err := pool.QueryRow(ctx, jobsQuery, historianSchema).
		Scan(&metrics.Jobs, &metrics.FailedJobs); err != nil {
		return metrics, fmt.Errorf("read job status: %w", err)
	}

	if err := pool.QueryRow(ctx, policyQuery, historianSchema).Scan(
		&metrics.CompressionJobs,
		&metrics.RetentionJobs,
		&metrics.PoliciesUniform,
	); err != nil {
		return metrics, fmt.Errorf("read policies: %w", err)
	}

	// No failure recorded is the normal case, not an error.
	if err := pool.QueryRow(ctx, lastJobErrorQuery, historianSchema).Scan(&metrics.LastJobError); err != nil &&
		!errors.Is(err, pgx.ErrNoRows) {
		return metrics, fmt.Errorf("read last job error: %w", err)
	}

	if err := pool.QueryRow(ctx, dataSpanQuery, historianSchema).Scan(&metrics.DataSpanSeconds); err != nil {
		return metrics, fmt.Errorf("read data span: %w", err)
	}

	tables, err := collectTables(ctx, pool)
	if err != nil {
		return metrics, err
	}

	metrics.Tables = tables

	jobs, err := collectJobs(ctx, pool)
	if err != nil {
		return metrics, err
	}

	metrics.JobList = jobs

	if err := pool.QueryRow(ctx, databaseSizeQuery).Scan(&metrics.DatabaseBytes); err != nil {
		return metrics, fmt.Errorf("read database size: %w", err)
	}

	return metrics, nil
}

func collectJobs(ctx context.Context, pool Querier) ([]Job, error) {
	rows, err := pool.Query(ctx, jobsListQuery, historianSchema)
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

func collectTables(ctx context.Context, pool Querier) ([]Table, error) {
	rows, err := pool.Query(ctx, tablesQuery, historianSchema)
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

	regular, err := collectRegularTables(ctx, pool)
	if err != nil {
		return nil, err
	}

	return append(tables, regular...), nil
}

func timeColumnTables(ctx context.Context, pool Querier) (map[string]bool, error) {
	rows, err := pool.Query(ctx, tsHypertablesQuery, historianSchema)
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

func collectRegularTables(ctx context.Context, pool Querier) ([]Table, error) {
	rows, err := pool.Query(ctx, regularTablesQuery, historianSchema)
	if err != nil {
		return nil, fmt.Errorf("read regular tables: %w", err)
	}
	defer rows.Close()

	var tables []Table

	for rows.Next() {
		var table Table
		if err := rows.Scan(&table.Name, &table.UncompressedBytes, &table.Rows); err != nil {
			return nil, fmt.Errorf("scan regular table: %w", err)
		}

		tables = append(tables, table)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read regular tables: %w", err)
	}

	return tables, nil
}

// Querier is the read surface this package needs. Both *pgx.Conn and
// *pgxpool.Pool satisfy it, so a one-shot caller opens a single connection while
// a long-lived one keeps its pool.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Collect reads every figure the historian metrics view shows: the catalog
// aggregates, per-table storage and policies, the background jobs, and how long
// ago each hypertable last received a write. It runs on demand rather than on the
// monitor's tick, so it carries no time budget of its own beyond ctx.
func Collect(ctx context.Context, pool Querier) (Metrics, error) {
	metrics, err := collectMetrics(ctx, pool)
	if err != nil {
		return metrics, err
	}

	writes, err := collectFreshness(ctx, pool, metrics.Tables)
	if err != nil {
		return metrics, fmt.Errorf("read freshness: %w", err)
	}

	now := time.Now()
	metrics.Tables = withFreshness(metrics.Tables, writes, now)

	stats, err := tableStats(ctx, pool, metrics.Tables)
	if err != nil {
		return metrics, err
	}

	counts, err := tableRows(ctx, pool)
	if err != nil {
		return metrics, err
	}

	lookupCounts, err := countLookupTables(ctx, pool, metrics.Tables)
	if err != nil {
		return metrics, err
	}

	for name, count := range lookupCounts {
		counts[name] = count
	}

	metrics.Tables, metrics.OtherTables = splitForeignTables(
		withRows(withTableStats(metrics.Tables, stats, now), counts))

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
		others.Bytes += table.UncompressedBytes + table.CompressedBytes
		others.Rows += table.Rows
	}

	return kept, others
}

// tableStatsQuery reads each table's oldest row and approximate row count. The
// oldest row is read without the freshness window: the whole point is how far
// back the table reaches, which is usually further than any window.
func tableStats(ctx context.Context, pool Querier, tables []Table) (map[string]tableStat, error) {
	readable, err := timeColumnTables(ctx, pool)
	if err != nil {
		return nil, err
	}

	selects := make([]string, 0, len(tables))

	for _, table := range tables {
		if !table.IsHypertable || !readable[table.Name] || !safeTableName(table.Name) {
			continue
		}

		selects = append(selects, fmt.Sprintf(
			firstWriteQuery, table.Name, historianSchema, table.Name))
	}

	if len(selects) == 0 {
		return map[string]tableStat{}, nil
	}

	rows, err := pool.Query(ctx, strings.Join(selects, " UNION ALL "))
	if err != nil {
		return nil, fmt.Errorf("read table stats: %w", err)
	}
	defer rows.Close()

	stats := make(map[string]tableStat, len(selects))

	for rows.Next() {
		var name string

		var stat tableStat
		if err := rows.Scan(&name, &stat.firstWrite); err != nil {
			return nil, fmt.Errorf("scan table stats: %w", err)
		}

		stats[name] = stat
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read table stats: %w", err)
	}

	return stats, nil
}

type tableStat struct {
	firstWrite int64
}

func withTableStats(tables []Table, stats map[string]tableStat, now time.Time) []Table {
	for i, table := range tables {
		stat, ok := stats[table.Name]
		if !ok {
			continue
		}

		if stat.firstWrite > 0 {
			tables[i].FirstWriteAt = time.Unix(stat.firstWrite, 0).UTC().Format(time.RFC3339)
		}
	}

	return tables
}

func tableRows(ctx context.Context, pool Querier) (map[string]int64, error) {
	rows, err := pool.Query(ctx, tableRowsQuery, historianSchema)
	if err != nil {
		return nil, fmt.Errorf("read table rows: %w", err)
	}
	defer rows.Close()

	counts := map[string]int64{}

	for rows.Next() {
		var name string

		var count int64
		if err := rows.Scan(&name, &count); err != nil {
			return nil, fmt.Errorf("scan table rows: %w", err)
		}

		counts[name] = count
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read table rows: %w", err)
	}

	return counts, nil
}

func withRows(tables []Table, counts map[string]int64) []Table {
	for i, table := range tables {
		count, ok := counts[table.Name]
		if !ok {
			continue
		}

		tables[i].Rows = count
	}

	return tables
}

// countLookupTables reads the exact row count of the historian's own plain tables.
// The planner estimate they would otherwise carry is zero until autovacuum first
// analyses them, which on a small, rarely-written lookup table may never happen:
// tag and topic both held ten rows and reported reltuples of zero. These tables
// are bounded by how many distinct tags exist rather than by ingest rate, so
// counting them outright is affordable where counting a hypertable is not.
func countLookupTables(ctx context.Context, pool Querier, tables []Table) (map[string]int64, error) {
	selects := make([]string, 0, len(tables))

	for _, table := range tables {
		if table.IsHypertable || !historianCreated(table.Name) || !safeTableName(table.Name) {
			continue
		}

		selects = append(selects, fmt.Sprintf(
			lookupCountQuery, table.Name, historianSchema, table.Name))
	}

	if len(selects) == 0 {
		return map[string]int64{}, nil
	}

	rows, err := pool.Query(ctx, strings.Join(selects, " UNION ALL "))
	if err != nil {
		return nil, fmt.Errorf("count lookup tables: %w", err)
	}
	defer rows.Close()

	counts := map[string]int64{}

	for rows.Next() {
		var name string

		var count int64
		if err := rows.Scan(&name, &count); err != nil {
			return nil, fmt.Errorf("scan lookup table count: %w", err)
		}

		counts[name] = count
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("count lookup tables: %w", err)
	}

	return counts, nil
}
