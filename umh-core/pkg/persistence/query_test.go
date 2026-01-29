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

package persistence_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

var _ = Describe("Query", func() {
	Describe("NewQuery", func() {
		It("should create an empty query with nil fields and zero counts", func() {
			query := persistence.NewQuery()

			Expect(query).NotTo(BeNil())
			Expect(query.Filters).To(BeNil())
			Expect(query.SortBy).To(BeNil())
			Expect(query.LimitCount).To(Equal(0))
			Expect(query.SkipCount).To(Equal(0))
		})
	})

	Describe("Filter", func() {
		It("should add a single filter condition", func() {
			query := persistence.NewQuery().Filter("status", persistence.Eq, "active")

			Expect(query.Filters).To(HaveLen(1))
			Expect(query.Filters[0].Field).To(Equal("status"))
			Expect(query.Filters[0].Op).To(Equal(persistence.Eq))
			Expect(query.Filters[0].Value).To(Equal("active"))
		})

		It("should add multiple filter conditions", func() {
			query := persistence.NewQuery().
				Filter("status", persistence.Eq, "active").
				Filter("age", persistence.Gt, 18)

			Expect(query.Filters).To(HaveLen(2))
			Expect(query.Filters[0].Field).To(Equal("status"))
			Expect(query.Filters[1].Field).To(Equal("age"))
			Expect(query.Filters[1].Op).To(Equal(persistence.Gt))
			Expect(query.Filters[1].Value).To(Equal(18))
		})

		DescribeTable("should support all comparison operators",
			func(operator persistence.Operator, expected persistence.Operator) {
				Expect(operator).To(Equal(expected))

				query := persistence.NewQuery().Filter("field", operator, "value")
				Expect(query.Filters[0].Op).To(Equal(operator))
			},
			Entry("Equal", persistence.Eq, persistence.Operator("$eq")),
			Entry("NotEqual", persistence.Ne, persistence.Operator("$ne")),
			Entry("GreaterThan", persistence.Gt, persistence.Operator("$gt")),
			Entry("GreaterThanOrEqual", persistence.Gte, persistence.Operator("$gte")),
			Entry("LessThan", persistence.Lt, persistence.Operator("$lt")),
			Entry("LessThanOrEqual", persistence.Lte, persistence.Operator("$lte")),
			Entry("In", persistence.In, persistence.Operator("$in")),
			Entry("NotIn", persistence.Nin, persistence.Operator("$nin")),
		)

		It("should support In operator with array value", func() {
			values := []string{"active", "pending", "processing"}
			query := persistence.NewQuery().Filter("status", persistence.In, values)

			Expect(query.Filters).To(HaveLen(1))
			Expect(query.Filters[0].Op).To(Equal(persistence.In))

			valuesInterface, ok := query.Filters[0].Value.([]string)
			Expect(ok).To(BeTrue())
			Expect(valuesInterface).To(HaveLen(3))
		})
	})

	Describe("Sort", func() {
		It("should add a single sort field", func() {
			query := persistence.NewQuery().Sort("createdAt", persistence.Desc)

			Expect(query.SortBy).To(HaveLen(1))
			Expect(query.SortBy[0].Field).To(Equal("createdAt"))
			Expect(query.SortBy[0].Order).To(Equal(persistence.Desc))
		})

		It("should add multiple sort fields", func() {
			query := persistence.NewQuery().
				Sort("createdAt", persistence.Desc).
				Sort("name", persistence.Asc)

			Expect(query.SortBy).To(HaveLen(2))
			Expect(query.SortBy[0].Field).To(Equal("createdAt"))
			Expect(query.SortBy[0].Order).To(Equal(persistence.Desc))
			Expect(query.SortBy[1].Field).To(Equal("name"))
			Expect(query.SortBy[1].Order).To(Equal(persistence.Asc))
		})

		It("should have correct order constants", func() {
			Expect(persistence.Asc).To(Equal(persistence.SortOrder(1)))
			Expect(persistence.Desc).To(Equal(persistence.SortOrder(-1)))
		})
	})

	Describe("Limit and Skip", func() {
		It("should set limit correctly", func() {
			query := persistence.NewQuery().Limit(10)
			Expect(query.LimitCount).To(Equal(10))
		})

		It("should set skip correctly", func() {
			query := persistence.NewQuery().Skip(20)
			Expect(query.SkipCount).To(Equal(20))
		})

		It("should support both limit and skip together", func() {
			query := persistence.NewQuery().Limit(10).Skip(20)

			Expect(query.LimitCount).To(Equal(10))
			Expect(query.SkipCount).To(Equal(20))
		})

		It("should treat negative limit as 0", func() {
			query := persistence.NewQuery().Limit(-10)
			Expect(query.LimitCount).To(Equal(0))
		})

		It("should treat negative skip as 0", func() {
			query := persistence.NewQuery().Skip(-20)
			Expect(query.SkipCount).To(Equal(0))
		})
	})

	Describe("Method Chaining", func() {
		It("should support full chaining", func() {
			query := persistence.NewQuery().
				Filter("status", persistence.Eq, "active").
				Filter("age", persistence.Gt, 18).
				Sort("createdAt", persistence.Desc).
				Limit(10).
				Skip(5)

			Expect(query.Filters).To(HaveLen(2))
			Expect(query.SortBy).To(HaveLen(1))
			Expect(query.LimitCount).To(Equal(10))
			Expect(query.SkipCount).To(Equal(5))
		})

		It("should support complex chaining with interleaved operations", func() {
			query := persistence.NewQuery().
				Filter("status", persistence.Eq, "active").
				Sort("priority", persistence.Desc).
				Filter("category", persistence.In, []string{"urgent", "high"}).
				Sort("createdAt", persistence.Desc).
				Limit(100).
				Skip(50)

			Expect(query.Filters).To(HaveLen(2))
			Expect(query.SortBy).To(HaveLen(2))
			Expect(query.SortBy[0].Field).To(Equal("priority"))
			Expect(query.SortBy[1].Field).To(Equal("createdAt"))
		})

		It("should validate constraints in chain", func() {
			query := persistence.NewQuery().
				Filter("status", persistence.Eq, "active").
				Limit(-5).
				Skip(-10).
				Sort("createdAt", persistence.Desc)

			Expect(query.LimitCount).To(Equal(0))
			Expect(query.SkipCount).To(Equal(0))
			Expect(query.Filters).To(HaveLen(1))
			Expect(query.SortBy).To(HaveLen(1))
		})
	})

	Describe("Real World Example", func() {
		It("should handle complex queries correctly", func() {
			query := persistence.NewQuery().
				Filter("status", persistence.Eq, "active").
				Filter("age", persistence.Gte, 18).
				Filter("age", persistence.Lte, 65).
				Filter("role", persistence.In, []string{"admin", "moderator", "user"}).
				Sort("priority", persistence.Desc).
				Sort("createdAt", persistence.Asc).
				Limit(25).
				Skip(50)

			Expect(query.Filters).To(HaveLen(4))
			Expect(query.Filters[0].Field).To(Equal("status"))
			Expect(query.Filters[0].Op).To(Equal(persistence.Eq))
			Expect(query.Filters[1].Field).To(Equal("age"))
			Expect(query.Filters[1].Op).To(Equal(persistence.Gte))
			Expect(query.Filters[2].Field).To(Equal("age"))
			Expect(query.Filters[2].Op).To(Equal(persistence.Lte))
			Expect(query.Filters[3].Field).To(Equal("role"))
			Expect(query.Filters[3].Op).To(Equal(persistence.In))

			Expect(query.SortBy).To(HaveLen(2))
			Expect(query.SortBy[0].Field).To(Equal("priority"))
			Expect(query.SortBy[0].Order).To(Equal(persistence.Desc))
			Expect(query.SortBy[1].Field).To(Equal("createdAt"))
			Expect(query.SortBy[1].Order).To(Equal(persistence.Asc))

			Expect(query.LimitCount).To(Equal(25))
			Expect(query.SkipCount).To(Equal(50))
		})
	})
})
