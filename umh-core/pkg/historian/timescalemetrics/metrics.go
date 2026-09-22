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
	// Rows is exact for a compressed chunk, which records its own pre-compression
	// count, and for the small lookup tables, which are counted outright. The rest
	// is the planner's estimate: counting every chunk is what a historian cannot
	// afford.
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
	Failures           int    `json:"failures"`
}

// Metrics is the aggregate operational picture of the historian database,
// embedded into TimescaleStatus so its fields flatten to the top JSON level.
type Metrics struct {
	ServerVersion    string `json:"serverVersion"`
	TimescaleVersion string `json:"timescaleVersion"`
	DatabaseBytes    int64  `json:"databaseBytes"`
	// DataSpanSeconds is the period the data covers, from the oldest row in any
	// table to the newest.
	DataSpanSeconds int64   `json:"dataSpanSeconds"`
	Tables          []Table `json:"tables"`
	JobList         []Job   `json:"jobList"`
}
