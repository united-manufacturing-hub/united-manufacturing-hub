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

const historianSchema = "umh"

const versionQuery = `SELECT current_setting('server_version'),
       (SELECT extversion FROM pg_extension WHERE extname = 'timescaledb')`

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

// jobsQuery is scoped to the historian schema so it excludes the built-in
// policy_telemetry job, which carries no hypertable and fails on every run of an
// air-gapped deployment. Counting it would report a permanent false failure.
const jobsQuery = `SELECT count(*), count(*) FILTER (WHERE s.last_run_status = 'Failed')
  FROM timescaledb_information.jobs j
  LEFT JOIN timescaledb_information.job_stats s USING (job_id)
 WHERE j.hypertable_schema = $1`

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
