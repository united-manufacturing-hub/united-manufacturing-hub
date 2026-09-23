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

	"github.com/jackc/pgx/v5"
)

// Querier is the read surface this package needs. *pgx.Conn and *pgxpool.Pool
// both satisfy it.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Table is one table's storage, chunking and policy settings. A lookup table has
// no chunks, so everything describing chunking is zero for it.
type Table struct {
	Name string `json:"name"`
	// Compressed chunks at their compressed size, plus the chunks still uncompressed.
	DiskBytes int64 `json:"diskBytes"`
	// Compressed chunks only, which is all TimescaleDB records a before size for.
	BytesBeforeCompression int64 `json:"bytesBeforeCompression"`
	BytesAfterCompression  int64 `json:"bytesAfterCompression"`
	ChunkIntervalSeconds   int64 `json:"chunkIntervalSeconds"`
	CompressAfterSeconds   int64 `json:"compressAfterSeconds"`
	RetentionSeconds       int64 `json:"retentionSeconds"`
	// RFC 3339, empty when the table holds no rows.
	EarliestRowTimestamp string `json:"earliestRowTimestamp"`
	LatestRowTimestamp   string `json:"latestRowTimestamp"`
	// Exact for a compressed chunk, which records its own count; Postgres's
	// live-tuple tracking for everything else.
	Rows             int64 `json:"rows"`
	Chunks           int   `json:"chunks"`
	CompressedChunks int   `json:"compressedChunks"`
	IsHypertable     bool  `json:"isHypertable"`
}

type Job struct {
	Kind               string `json:"kind"`
	Table              string `json:"table"`
	Status             string `json:"status"`
	ScheduleSeconds    int64  `json:"scheduleSeconds"`
	LastSuccessSeconds int64  `json:"lastSuccessSeconds"`
	NextRunSeconds     int64  `json:"nextRunSeconds"`
	LastRunFailed      bool   `json:"lastRunFailed"`
}

// Metrics is everything Collect reads, returned to the console on request.
type Metrics struct {
	PostgresVersion  string `json:"postgresVersion"`
	TimescaleVersion string `json:"timescaleVersion"`
	DatabaseBytes    int64  `json:"databaseBytes"`
	// From the earliest row in any table to the latest.
	DataSpanSeconds int64   `json:"dataSpanSeconds"`
	Tables          []Table `json:"tables"`
	JobList         []Job   `json:"jobList"`
}
