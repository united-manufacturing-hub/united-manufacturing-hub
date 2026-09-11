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
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TimescaleMetrics is the aggregate operational picture of the historian database,
// embedded into TimescaleStatus so its fields flatten to the top JSON level.
// TimescaleTable is one hypertable's storage, chunking and policy settings. The
// aggregates beside it answer whether the historian as a whole compresses and
// expires data; this answers which table does not.
type TimescaleTable struct {
	Name                 string `json:"name"`
	UncompressedBytes    int64  `json:"uncompressed_bytes"`
	CompressedBytes      int64  `json:"compressed_bytes"`
	ChunkIntervalSeconds int64  `json:"chunk_interval_seconds"`
	CompressAfterSeconds int64  `json:"compress_after_seconds"`
	DropAfterSeconds     int64  `json:"drop_after_seconds"`
	Chunks               int    `json:"chunks"`
	CompressedChunks     int    `json:"compressed_chunks"`
}

type TimescaleMetrics struct {
	ServerVersion    string `json:"server_version"`
	TimescaleVersion string `json:"timescale_version"`
	// MetricsError carries why the last collection failed, so a database that
	// answers the connection check but refuses the metric reads explains itself
	// instead of reporting zeros.
	MetricsError string `json:"metrics_error"`
	// LastJobError is the most recent background-job failure message. A bare
	// failure count says nothing an operator can act on; this names the table and
	// the reason.
	LastJobError      string `json:"last_job_error"`
	DatabaseBytes     int64  `json:"database_bytes"`
	UncompressedBytes int64  `json:"uncompressed_bytes"`
	CompressedBytes   int64  `json:"compressed_bytes"`
	// CompressAfterSeconds and DropAfterSeconds are the shortest interval any
	// hypertable uses, so the reported figure is the soonest chunks are compressed
	// or dropped rather than a flattering maximum. Zero means no such policy
	// exists: a zero DropAfterSeconds with RetentionJobs zero is a database that
	// grows forever.
	CompressAfterSeconds int64            `json:"compress_after_seconds"`
	DropAfterSeconds     int64            `json:"drop_after_seconds"`
	Hypertables          int              `json:"hypertables"`
	Chunks               int              `json:"chunks"`
	CompressedChunks     int              `json:"compressed_chunks"`
	Jobs                 int              `json:"jobs"`
	CompressionJobs      int              `json:"compression_jobs"`
	RetentionJobs        int              `json:"retention_jobs"`
	FailedJobs           int              `json:"failed_jobs"`
	Tables               []TimescaleTable `json:"tables"`
	// PoliciesUniform reports whether every hypertable agrees on its intervals.
	// When false the single reported interval describes only the shortest table,
	// and the rest have to be read from the database.
	PoliciesUniform bool `json:"policies_uniform"`
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
       coalesce(min(EXTRACT(EPOCH FROM (config->>'compress_after')::interval))::bigint, 0),
       coalesce(min(EXTRACT(EPOCH FROM (config->>'drop_after')::interval))::bigint, 0),
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

const databaseSizeQuery = `SELECT pg_database_size(current_database())`

// collectMetrics reads the historian database's aggregate operational picture.
//
// The reads run cheapest-first and pg_database_size runs last, because it is the
// only one whose cost grows with the deployment: it stats every file backing the
// database, which tracks chunk count rather than stored bytes. A deployment large
// enough to make it slow therefore loses only DatabaseBytes, and returns every
// metric collected before it.
func collectMetrics(ctx context.Context, pool *pgxpool.Pool) (TimescaleMetrics, error) {
	var metrics TimescaleMetrics

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
		&metrics.CompressAfterSeconds,
		&metrics.DropAfterSeconds,
		&metrics.PoliciesUniform,
	); err != nil {
		return metrics, fmt.Errorf("read policies: %w", err)
	}

	// No failure recorded is the normal case, not an error.
	if err := pool.QueryRow(ctx, lastJobErrorQuery, historianSchema).Scan(&metrics.LastJobError); err != nil &&
		!errors.Is(err, pgx.ErrNoRows) {
		return metrics, fmt.Errorf("read last job error: %w", err)
	}

	tables, err := collectTables(ctx, pool)
	if err != nil {
		return metrics, err
	}

	metrics.Tables = tables

	if err := pool.QueryRow(ctx, databaseSizeQuery).Scan(&metrics.DatabaseBytes); err != nil {
		return metrics, fmt.Errorf("read database size: %w", err)
	}

	return metrics, nil
}

type metricsSchedule struct {
	lastRun     time.Time
	lastMetrics TimescaleMetrics
	interval    time.Duration
	mu          sync.Mutex
}

func newMetricsSchedule(interval time.Duration) *metricsSchedule {
	return &metricsSchedule{interval: interval}
}

func (s *metricsSchedule) remember(metrics TimescaleMetrics) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastMetrics = metrics
}

// last returns the most recent collection. Poll reports it on every tick, including
// the ticks that collect nothing: reporting a zero value on those would blank every
// metric between collections and re-sync the whole block twice per interval.
func (s *metricsSchedule) last() TimescaleMetrics {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.lastMetrics
}

func (s *metricsSchedule) claimNextRun(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.lastRun.IsZero() && now.Sub(s.lastRun) < s.interval {
		return false
	}

	s.lastRun = now

	return true
}

func collectTables(ctx context.Context, pool *pgxpool.Pool) ([]TimescaleTable, error) {
	rows, err := pool.Query(ctx, tablesQuery, historianSchema)
	if err != nil {
		return nil, fmt.Errorf("read tables: %w", err)
	}
	defer rows.Close()

	var tables []TimescaleTable

	for rows.Next() {
		var table TimescaleTable
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

		tables = append(tables, table)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read tables: %w", err)
	}

	return tables, nil
}
