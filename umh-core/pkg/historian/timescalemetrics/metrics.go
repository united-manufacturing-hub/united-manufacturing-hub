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

// Querier is the read surface this package needs. Both *pgx.Conn and
// *pgxpool.Pool satisfy it, so a one-shot caller opens a single connection while
// a long-lived one keeps its pool.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Table is one table's storage, chunking and policy settings. A plain lookup
// table carries only a name, a size and a row count: the rest describes chunking,
// which it has none of.
type Table struct {
	Name string `json:"name"`
	// DiskBytes is the compressed chunks at their compressed size plus the chunks
	// no policy has compressed yet.
	DiskBytes int64 `json:"diskBytes"`
	// BytesBeforeCompression and BytesAfterCompression cover the compressed chunks
	// only, because a chunk is the only thing TimescaleDB records a before size
	// for. They answer how much compression saved, not how large the table is.
	BytesBeforeCompression int64 `json:"bytesBeforeCompression"`
	BytesAfterCompression  int64 `json:"bytesAfterCompression"`
	ChunkIntervalSeconds   int64 `json:"chunkIntervalSeconds"`
	CompressAfterSeconds   int64 `json:"compressAfterSeconds"`
	RetentionSeconds       int64 `json:"retentionSeconds"`
	// RFC 3339 instants, empty when the table holds no rows. A reader handed an
	// age instead would resolve it against its own clock, which puts the reported
	// moment out by however far that clock is wrong.
	EarliestRowTimestamp string `json:"earliestRowTimestamp"`
	LatestRowTimestamp   string `json:"latestRowTimestamp"`
	// Rows is exact for a compressed chunk, which records its own pre-compression
	// count, and for the lookup tables, which are counted outright. A chunk no
	// policy has compressed yet contributes Postgres's live-tuple tracking, which
	// follows the rows as they are written.
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
	// LastRunFailed reports the outcome of the most recent run, which is what a
	// reader acts on. A lifetime failure count keeps flagging a job that failed
	// once and has succeeded ever since.
	LastRunFailed bool `json:"lastRunFailed"`
}

// Metrics is the aggregate operational picture of the historian database,
// embedded into TimescaleStatus so its fields flatten to the top JSON level.
type Metrics struct {
	PostgresVersion  string `json:"postgresVersion"`
	TimescaleVersion string `json:"timescaleVersion"`
	DatabaseBytes    int64  `json:"databaseBytes"`
	// DataSpanSeconds is the period the data covers, from the earliest row in any
	// table to the latest.
	DataSpanSeconds int64   `json:"dataSpanSeconds"`
	Tables          []Table `json:"tables"`
	JobList         []Job   `json:"jobList"`
}
