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
	"regexp"
	"strings"
)

// The three queries below are per-table selects, joined with UNION ALL into one
// statement. A table name cannot be bound as a parameter, so each is a format
// string taking the table name, the schema, and the table name again -- the
// first as the literal that labels the row, the last two as the identifier.
// Every name is checked against safeTableName before it is interpolated.

const latestRowTimestampQuery = `SELECT '%s', coalesce(extract(epoch FROM max(ts))::bigint, 0) FROM %s.%s`

const earliestRowTimestampQuery = `SELECT '%s', coalesce(extract(epoch FROM min(ts))::bigint, 0) FROM %s.%s`

const lookupCountQuery = `SELECT '%s', count(*)::bigint FROM %s.%s`

var tableNamePattern = regexp.MustCompile(`^[a-z0-9_]+$`)

func safeTableName(name string) bool {
	return tableNamePattern.MatchString(name)
}

// historianTablePrefixes and historianTableNames name what benthos-umh's
// historian output creates: two hypertables per data contract, plus the shared
// lookup tables.
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

// historianTables drops every table in the schema this product did not create: a
// customer's own table alongside ours says nothing about how the historian is
// doing, and its columns would all read as absent.
func historianTables(tables []Table) []Table {
	kept := make([]Table, 0, len(tables))

	for _, table := range tables {
		if historianCreated(table.Name) {
			kept = append(kept, table)
		}
	}

	return kept
}

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

	pairs, err := queryAll(ctx, db, statement, what, rowToNamedValue)
	if err != nil {
		return nil, err
	}

	return valuesByName(pairs), nil
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

// collectTimestamps reads one end of each hypertable's time column. Only
// hypertables that have one qualify, which is what readable names: the others
// carry no ts to take a max or min of.
func collectTimestamps(
	ctx context.Context,
	db Querier,
	tables []Table,
	query string,
	what string,
	readable map[string]bool,
) (map[string]int64, error) {
	return collectPerTable(ctx, db, tables, query, what,
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
