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

	"github.com/jackc/pgx/v5/pgxpool"
)

// TimescaleMetrics is the aggregate operational picture of the historian database,
// embedded into TimescaleStatus so its fields flatten to the top JSON level.
type TimescaleMetrics struct {
	ServerVersion     string `json:"server_version"`
	TimescaleVersion  string `json:"timescale_version"`
	MetricsError      string `json:"metrics_error"`
	DatabaseBytes     int64  `json:"database_bytes"`
	UncompressedBytes int64  `json:"uncompressed_bytes"`
	CompressedBytes   int64  `json:"compressed_bytes"`
	Hypertables       int    `json:"hypertables"`
	Chunks            int    `json:"chunks"`
	CompressedChunks  int    `json:"compressed_chunks"`
	Jobs              int    `json:"jobs"`
	FailedJobs        int    `json:"failed_jobs"`
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
