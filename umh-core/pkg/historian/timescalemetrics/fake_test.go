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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

var _ = Describe("FakeDatabase", func() {
	ctx := context.Background()

	contents := FakeContents{
		PostgresVersion:  "17.6",
		TimescaleVersion: "2.24.0",
		Tables: []Table{
			{
				Name:                 "value_line",
				IsHypertable:         true,
				OccupiedDiskBytes:    5_000,
				EarliestRowTimestamp: "2026-01-01T00:00:00Z",
				LatestRowTimestamp:   "2026-01-02T00:00:00Z",
			},
			{Name: "tag", OccupiedDiskBytes: 300, Rows: 12},
			{Name: "customer_export", OccupiedDiskBytes: 1_000_000},
		},
		JobCount:                  2,
		FailedJobCount:            1,
		DatabaseOccupiedDiskBytes: 9_000_000,
	}

	It("answers CollectSummary from its contents", func() {
		summary, err := CollectSummary(ctx, NewFakeDatabase(contents))

		Expect(err).NotTo(HaveOccurred())
		Expect(summary).To(Equal(Summary{
			Versions: []models.Version{
				{Name: "PostgreSQL", Version: "17.6"},
				{Name: "TimescaleDB", Version: "2.24.0"},
			},
			TableNames:                 []string{"value_line", "tag"},
			HistorianOccupiedDiskBytes: 5_300,
			DataSpanSeconds:            24 * 60 * 60,
			JobCount:                   2,
			FailedJobCount:             1,
		}))
	})

	It("answers ReadDatabaseOccupiedDiskBytes from its contents", func() {
		databaseOccupiedDiskBytes, err := ReadDatabaseOccupiedDiskBytes(ctx, NewFakeDatabase(contents))

		Expect(err).NotTo(HaveOccurred())
		Expect(databaseOccupiedDiskBytes).To(Equal(int64(9_000_000)))
	})

	It("lists tables by name, as the real queries order them", func() {
		summary, err := CollectSummary(ctx, NewFakeDatabase(FakeContents{
			TimescaleVersion: "2.24.0",
			Tables: []Table{
				{Name: "value_b", IsHypertable: true},
				{Name: "value_a", IsHypertable: true},
			},
		}))

		Expect(err).NotTo(HaveOccurred())
		Expect(summary.TableNames).To(Equal([]string{"value_a", "value_b"}))
	})

	It("fails to scan NULL into a destination that is not a pointer, as pgx does", func() {
		rows := newFakeRows([]any{nil})
		Expect(rows.Next()).To(BeTrue())

		var name string

		Expect(rows.Scan(&name)).To(MatchError("FakeDatabase column 0: cannot scan NULL into *string"))
	})

	It("fails a query it has no answer for, naming the query", func() {
		var one int

		err := NewFakeDatabase(contents).QueryRow(ctx, "SELECT 2").Scan(&one)
		Expect(err).To(MatchError(ContainSubstring(`"SELECT 2"`)))

		_, err = NewFakeDatabase(contents).Query(ctx, "SELECT 2")
		Expect(err).To(MatchError(ContainSubstring(`"SELECT 2"`)))
	})

	It("fails every query with the error SetErr gave it, until SetErr(nil)", func() {
		fake := NewFakeDatabase(contents)

		fake.SetErr(ErrFakeConnectionRefused)

		_, err := CollectSummary(ctx, fake)
		Expect(err).To(MatchError(ErrFakeConnectionRefused))

		_, err = fake.Query(ctx, tablesQuery, historianSchema)
		Expect(err).To(MatchError(ErrFakeConnectionRefused))

		fake.SetErr(nil)

		_, err = CollectSummary(ctx, fake)
		Expect(err).NotTo(HaveOccurred())
	})
})
