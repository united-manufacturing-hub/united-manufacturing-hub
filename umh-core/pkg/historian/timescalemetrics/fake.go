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
	"fmt"
	"net"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// ErrFakePasswordRejected is the error a TimescaleDB returns for a wrong password.
var ErrFakePasswordRejected error = &pgconn.PgError{
	Severity: "FATAL",
	Code:     "28P01",
	Message:  `password authentication failed for user "umh_owner"`,
}

// ErrFakeConnectionRefused is the error a dial to a port nobody listens on returns.
var ErrFakeConnectionRefused error = &net.OpError{
	Op:  "dial",
	Net: "tcp",
	Err: os.NewSyscallError("connect", syscall.ECONNREFUSED),
}

// FakeContents is the database a FakeDatabase reports.
type FakeContents struct {
	PostgresVersion  string
	TimescaleVersion string
	// A table's span comes from its EarliestRowTimestamp and LatestRowTimestamp.
	Tables                    []Table
	JobCount                  int64
	FailedJobCount            int64
	DatabaseOccupiedDiskBytes int64
}

// FakeDatabase answers the connection check and the queries of CollectSummary
// and ReadDatabaseOccupiedDiskBytes from its FakeContents.
//
// It is hand-written rather than built on pgxmock. A pgxmock expectation
// answers a fixed number of calls, in order by default, while the historian
// monitor repeats the same queries for as long as a scenario runs.
type FakeDatabase struct {
	contents FakeContents
	err      error
	mu       sync.Mutex
}

// NewFakeDatabase returns a FakeDatabase that reports contents. It lists the
// tables by name, as the real queries order them.
func NewFakeDatabase(contents FakeContents) *FakeDatabase {
	contents.Tables = slices.Clone(contents.Tables)
	slices.SortFunc(contents.Tables, func(a, b Table) int { return strings.Compare(a.Name, b.Name) })

	return &FakeDatabase{contents: contents}
}

// SetErr makes every query fail with err until it is called again with nil.
func (f *FakeDatabase) SetErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.err = err
}

// Query returns the rows the real query would return for the contents.
func (f *FakeDatabase) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	f.mu.Lock()
	err := f.err
	f.mu.Unlock()

	if err != nil {
		return nil, err
	}

	return f.answer(sql)
}

// QueryRow returns the first row of Query. Like pgx, it reports any error from Scan.
func (f *FakeDatabase) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	rows, err := f.Query(ctx, sql, args...)

	return &fakeRow{rows: rows, err: err}
}

const connectionCheckQuery = "SELECT 1"

func (f *FakeDatabase) answer(sql string) (pgx.Rows, error) {
	c := f.contents

	switch sql {
	case connectionCheckQuery:
		return newFakeRows([]any{int32(1)}), nil
	case versionQuery:
		return newFakeRows([]any{c.PostgresVersion, c.TimescaleVersion}), nil
	case tablesQuery:
		return f.hypertableRows(), nil
	case regularTablesQuery:
		return f.regularTableRows(), nil
	case jobCountsQuery:
		return newFakeRows([]any{c.JobCount, c.FailedJobCount}), nil
	case chunksQuery:
		return f.chunkRows(), nil
	case databaseOccupiedDiskBytesQuery:
		return newFakeRows([]any{c.DatabaseOccupiedDiskBytes}), nil
	}

	if rows, ok, err := f.spanRows(sql); ok {
		return rows, err
	}

	return nil, fmt.Errorf("FakeDatabase has no answer for query %q", sql)
}

func (f *FakeDatabase) hypertableRows() pgx.Rows {
	var values [][]any

	for _, t := range f.contents.Tables {
		if t.IsHypertable {
			values = append(values, []any{
				t.Name, t.BytesBeforeCompression, t.BytesAfterCompression, t.OccupiedDiskBytes,
				t.ChunkIntervalSeconds, t.CompressAfterSeconds, t.RetentionSeconds,
				int64(t.Chunks), int64(t.CompressedChunks),
			})
		}
	}

	return newFakeRows(values...)
}

func (f *FakeDatabase) regularTableRows() pgx.Rows {
	var values [][]any

	for _, t := range f.contents.Tables {
		if !t.IsHypertable {
			values = append(values, []any{t.Name, t.OccupiedDiskBytes, t.Rows})
		}
	}

	return newFakeRows(values...)
}

// Each hypertable has one chunk, so every span query reads it.
func (f *FakeDatabase) chunkRows() pgx.Rows {
	var values [][]any

	for _, t := range f.contents.Tables {
		if t.IsHypertable {
			values = append(values, []any{t.Name, "_timescaledb_internal", "_hyper_" + t.Name + "_chunk"})
		}
	}

	return newFakeRows(values...)
}

// spanRows answers the UNION ALL of earliestRowQuery or latestRowQuery that
// readFirstAnswer builds. The second return is false when sql is neither.
func (f *FakeDatabase) spanRows(sql string) (pgx.Rows, bool, error) {
	var values [][]any

	for _, selected := range strings.Split(sql, " UNION ALL ") {
		table, readsEarliest, ok := parseSpanSelect(selected)
		if !ok {
			return nil, false, nil
		}

		epoch, err := f.spanEpoch(table, readsEarliest)
		if err != nil {
			return nil, true, err
		}

		values = append(values, []any{table, epoch})
	}

	return newFakeRows(values...), true, nil
}

var (
	earliestRowPrefix, earliestRowMiddle = splitAtVerbs(earliestRowQuery)
	latestRowPrefix, latestRowMiddle     = splitAtVerbs(latestRowQuery)
)

// splitAtVerbs returns the fixed text before the first %s and between the first two.
func splitAtVerbs(format string) (string, string) {
	prefix, rest, _ := strings.Cut(format, "%s")
	middle, _, _ := strings.Cut(rest, "%s")

	return prefix, middle
}

func parseSpanSelect(selected string) (table string, readsEarliest bool, ok bool) {
	if rest, found := strings.CutPrefix(selected, earliestRowPrefix); found {
		if table, _, found := strings.Cut(rest, earliestRowMiddle); found {
			return table, true, true
		}
	}

	if rest, found := strings.CutPrefix(selected, latestRowPrefix); found {
		if table, _, found := strings.Cut(rest, latestRowMiddle); found {
			return table, false, true
		}
	}

	return "", false, false
}

// spanEpoch returns nil, as Postgres does, for a table that holds no rows.
func (f *FakeDatabase) spanEpoch(table string, readsEarliest bool) (*int64, error) {
	for _, t := range f.contents.Tables {
		if t.Name != table {
			continue
		}

		timestamp := t.LatestRowTimestamp
		if readsEarliest {
			timestamp = t.EarliestRowTimestamp
		}

		if timestamp == "" {
			return nil, nil
		}

		parsed, err := time.Parse(time.RFC3339, timestamp)
		if err != nil {
			return nil, fmt.Errorf("FakeDatabase table %s: %w", table, err)
		}

		epoch := parsed.Unix()

		return &epoch, nil
	}

	return nil, nil
}

type fakeRows struct {
	values [][]any
	next   int
}

func newFakeRows(values ...[]any) *fakeRows {
	return &fakeRows{values: values}
}

func (r *fakeRows) Close()                                       {}
func (r *fakeRows) Err() error                                   { return nil }
func (r *fakeRows) CommandTag() pgconn.CommandTag                { return pgconn.NewCommandTag("SELECT") }
func (r *fakeRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *fakeRows) Conn() *pgx.Conn                              { return nil }
func (r *fakeRows) TypeMap() *pgtype.Map                         { return nil }

func (r *fakeRows) Next() bool {
	if r.next >= len(r.values) {
		return false
	}

	r.next++

	return true
}

func (r *fakeRows) current() []any {
	return r.values[r.next-1]
}

func (r *fakeRows) Values() ([]any, error) {
	return r.current(), nil
}

func (r *fakeRows) RawValues() [][]byte {
	return make([][]byte, len(r.current()))
}

func (r *fakeRows) Scan(dest ...any) error {
	row := r.current()
	if len(dest) != len(row) {
		return fmt.Errorf("FakeDatabase row has %d columns, Scan was given %d destinations", len(row), len(dest))
	}

	for i, value := range row {
		if err := assign(dest[i], value); err != nil {
			return fmt.Errorf("FakeDatabase column %d: %w", i, err)
		}
	}

	return nil
}

// assign stores value in the pointer dest the way pgx scans a column. NULL
// sets a pointer destination to nil and fails for any other destination. A
// pointer destination gets a pointer to the value, and integers convert between
// sizes.
func assign(dest any, value any) error {
	target := reflect.ValueOf(dest)
	if target.Kind() != reflect.Pointer || target.IsNil() {
		return fmt.Errorf("destination %T is not a non-nil pointer", dest)
	}

	target = target.Elem()

	source := reflect.ValueOf(value)
	if !source.IsValid() || (source.Kind() == reflect.Pointer && source.IsNil()) {
		if target.Kind() != reflect.Pointer {
			return fmt.Errorf("cannot scan NULL into %T", dest)
		}

		target.SetZero()

		return nil
	}

	if source.Kind() == reflect.Pointer {
		source = source.Elem()
	}

	if target.Kind() == reflect.Pointer {
		allocated := reflect.New(target.Type().Elem())
		if err := assign(allocated.Interface(), source.Interface()); err != nil {
			return err
		}

		target.Set(allocated)

		return nil
	}

	switch {
	case source.Type().AssignableTo(target.Type()):
		target.Set(source)
	case source.CanInt() && target.CanInt():
		target.SetInt(source.Int())
	default:
		return fmt.Errorf("cannot scan %T into %T", value, dest)
	}

	return nil
}

type fakeRow struct {
	rows pgx.Rows
	err  error
}

func (r *fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}

	if !r.rows.Next() {
		return pgx.ErrNoRows
	}

	return r.rows.Scan(dest...)
}
