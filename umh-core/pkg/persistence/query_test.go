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
	"testing"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

func TestNewQuery(t *testing.T) {
	query := persistence.NewQuery()

	if query == nil {
		t.Fatal("NewQuery() returned nil")
	}

	if query.Filters != nil {
		t.Errorf("expected nil Filters, got %v", query.Filters)
	}

	if query.SortBy != nil {
		t.Errorf("expected nil SortBy, got %v", query.SortBy)
	}

	if query.LimitCount != 0 {
		t.Errorf("expected LimitCount 0, got %d", query.LimitCount)
	}

	if query.SkipCount != 0 {
		t.Errorf("expected SkipCount 0, got %d", query.SkipCount)
	}
}

func TestQuery_Filter_SingleCondition(t *testing.T) {
	query := persistence.NewQuery().Filter("status", persistence.Eq, "active")

	if len(query.Filters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(query.Filters))
	}

	filter := query.Filters[0]
	if filter.Field != "status" {
		t.Errorf("expected field 'status', got '%s'", filter.Field)
	}

	if filter.Op != persistence.Eq {
		t.Errorf("expected operator Eq, got %v", filter.Op)
	}

	if filter.Value != "active" {
		t.Errorf("expected value 'active', got '%v'", filter.Value)
	}
}

func TestQuery_Filter_MultipleConditions(t *testing.T) {
	query := persistence.NewQuery().
		Filter("status", persistence.Eq, "active").
		Filter("age", persistence.Gt, 18)

	if len(query.Filters) != 2 {
		t.Fatalf("expected 2 filters, got %d", len(query.Filters))
	}

	if query.Filters[0].Field != "status" {
		t.Errorf("expected first field 'status', got '%s'", query.Filters[0].Field)
	}

	if query.Filters[1].Field != "age" {
		t.Errorf("expected second field 'age', got '%s'", query.Filters[1].Field)
	}

	if query.Filters[1].Op != persistence.Gt {
		t.Errorf("expected second operator Gt, got %v", query.Filters[1].Op)
	}

	if query.Filters[1].Value != 18 {
		t.Errorf("expected second value 18, got %v", query.Filters[1].Value)
	}
}

func TestQuery_Filter_ComparisonOperators(t *testing.T) {
	tests := []struct {
		name     string
		operator persistence.Operator
		expected persistence.Operator
	}{
		{"Equal", persistence.Eq, "$eq"},
		{"NotEqual", persistence.Ne, "$ne"},
		{"GreaterThan", persistence.Gt, "$gt"},
		{"GreaterThanOrEqual", persistence.Gte, "$gte"},
		{"LessThan", persistence.Lt, "$lt"},
		{"LessThanOrEqual", persistence.Lte, "$lte"},
		{"In", persistence.In, "$in"},
		{"NotIn", persistence.Nin, "$nin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.operator != tt.expected {
				t.Errorf("operator %v != expected %v", tt.operator, tt.expected)
			}

			query := persistence.NewQuery().Filter("field", tt.operator, "value")
			if query.Filters[0].Op != tt.operator {
				t.Errorf("filter operator %v != expected %v", query.Filters[0].Op, tt.operator)
			}
		})
	}
}

func TestQuery_Sort_SingleField(t *testing.T) {
	query := persistence.NewQuery().Sort("createdAt", persistence.Desc)

	if len(query.SortBy) != 1 {
		t.Fatalf("expected 1 sort field, got %d", len(query.SortBy))
	}

	sort := query.SortBy[0]
	if sort.Field != "createdAt" {
		t.Errorf("expected field 'createdAt', got '%s'", sort.Field)
	}

	if sort.Order != persistence.Desc {
		t.Errorf("expected order Desc (-1), got %v", sort.Order)
	}
}

func TestQuery_Sort_MultipleFields(t *testing.T) {
	query := persistence.NewQuery().
		Sort("createdAt", persistence.Desc).
		Sort("name", persistence.Asc)

	if len(query.SortBy) != 2 {
		t.Fatalf("expected 2 sort fields, got %d", len(query.SortBy))
	}

	if query.SortBy[0].Field != "createdAt" {
		t.Errorf("expected first field 'createdAt', got '%s'", query.SortBy[0].Field)
	}

	if query.SortBy[0].Order != persistence.Desc {
		t.Errorf("expected first order Desc, got %v", query.SortBy[0].Order)
	}

	if query.SortBy[1].Field != "name" {
		t.Errorf("expected second field 'name', got '%s'", query.SortBy[1].Field)
	}

	if query.SortBy[1].Order != persistence.Asc {
		t.Errorf("expected second order Asc, got %v", query.SortBy[1].Order)
	}
}

func TestQuery_Sort_OrderConstants(t *testing.T) {
	if persistence.Asc != 1 {
		t.Errorf("Asc constant should be 1, got %d", persistence.Asc)
	}

	if persistence.Desc != -1 {
		t.Errorf("Desc constant should be -1, got %d", persistence.Desc)
	}
}

func TestQuery_Limit(t *testing.T) {
	query := persistence.NewQuery().Limit(10)

	if query.LimitCount != 10 {
		t.Errorf("expected limit 10, got %d", query.LimitCount)
	}
}

func TestQuery_Skip(t *testing.T) {
	query := persistence.NewQuery().Skip(20)

	if query.SkipCount != 20 {
		t.Errorf("expected skip 20, got %d", query.SkipCount)
	}
}

func TestQuery_LimitSkip_Together(t *testing.T) {
	query := persistence.NewQuery().Limit(10).Skip(20)

	if query.LimitCount != 10 {
		t.Errorf("expected limit 10, got %d", query.LimitCount)
	}

	if query.SkipCount != 20 {
		t.Errorf("expected skip 20, got %d", query.SkipCount)
	}
}

func TestQuery_Chaining(t *testing.T) {
	query := persistence.NewQuery().
		Filter("status", persistence.Eq, "active").
		Filter("age", persistence.Gt, 18).
		Sort("createdAt", persistence.Desc).
		Limit(10).
		Skip(5)

	if len(query.Filters) != 2 {
		t.Errorf("expected 2 filters, got %d", len(query.Filters))
	}

	if len(query.SortBy) != 1 {
		t.Errorf("expected 1 sort field, got %d", len(query.SortBy))
	}

	if query.LimitCount != 10 {
		t.Errorf("expected limit 10, got %d", query.LimitCount)
	}

	if query.SkipCount != 5 {
		t.Errorf("expected skip 5, got %d", query.SkipCount)
	}
}

func TestQuery_ComplexChaining(t *testing.T) {
	query := persistence.NewQuery().
		Filter("status", persistence.Eq, "active").
		Sort("priority", persistence.Desc).
		Filter("category", persistence.In, []string{"urgent", "high"}).
		Sort("createdAt", persistence.Desc).
		Limit(100).
		Skip(50)

	if len(query.Filters) != 2 {
		t.Errorf("expected 2 filters")
	}

	if len(query.SortBy) != 2 {
		t.Errorf("expected 2 sort fields")
	}

	if query.SortBy[0].Field != "priority" {
		t.Errorf("expected first sort field 'priority', got '%s'", query.SortBy[0].Field)
	}

	if query.SortBy[1].Field != "createdAt" {
		t.Errorf("expected second sort field 'createdAt', got '%s'", query.SortBy[1].Field)
	}
}

func TestQuery_InOperator_WithArray(t *testing.T) {
	values := []string{"active", "pending", "processing"}
	query := persistence.NewQuery().Filter("status", persistence.In, values)

	if len(query.Filters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(query.Filters))
	}

	if query.Filters[0].Op != persistence.In {
		t.Errorf("expected In operator, got %v", query.Filters[0].Op)
	}

	valuesInterface, ok := query.Filters[0].Value.([]string)
	if !ok {
		t.Fatalf("expected []string value, got %T", query.Filters[0].Value)
	}

	if len(valuesInterface) != 3 {
		t.Errorf("expected 3 values, got %d", len(valuesInterface))
	}
}

func TestQuery_EmptyQuery(t *testing.T) {
	query := persistence.NewQuery()

	if query.Filters != nil {
		t.Errorf("empty query should have nil filters")
	}

	if query.SortBy != nil {
		t.Errorf("empty query should have nil sort")
	}

	if query.LimitCount != 0 {
		t.Errorf("empty query should have 0 limit")
	}

	if query.SkipCount != 0 {
		t.Errorf("empty query should have 0 skip")
	}
}

func TestQuery_Limit_NegativeValue(t *testing.T) {
	query := persistence.NewQuery().Limit(-10)

	if query.LimitCount != 0 {
		t.Errorf("negative limit should be treated as 0, got %d", query.LimitCount)
	}
}

func TestQuery_Skip_NegativeValue(t *testing.T) {
	query := persistence.NewQuery().Skip(-20)

	if query.SkipCount != 0 {
		t.Errorf("negative skip should be treated as 0, got %d", query.SkipCount)
	}
}

func TestQuery_Validation_InChain(t *testing.T) {
	query := persistence.NewQuery().
		Filter("status", persistence.Eq, "active").
		Limit(-5).
		Skip(-10).
		Sort("createdAt", persistence.Desc)

	if query.LimitCount != 0 {
		t.Errorf("expected limit 0, got %d", query.LimitCount)
	}

	if query.SkipCount != 0 {
		t.Errorf("expected skip 0, got %d", query.SkipCount)
	}

	if len(query.Filters) != 1 {
		t.Errorf("filter should still work, got %d filters", len(query.Filters))
	}

	if len(query.SortBy) != 1 {
		t.Errorf("sort should still work, got %d sorts", len(query.SortBy))
	}
}

func TestQuery_RealWorldExample(t *testing.T) {
	query := persistence.NewQuery().
		Filter("status", persistence.Eq, "active").
		Filter("age", persistence.Gte, 18).
		Filter("age", persistence.Lte, 65).
		Filter("role", persistence.In, []string{"admin", "moderator", "user"}).
		Sort("priority", persistence.Desc).
		Sort("createdAt", persistence.Asc).
		Limit(25).
		Skip(50)

	if len(query.Filters) != 4 {
		t.Errorf("expected 4 filters, got %d", len(query.Filters))
	}

	if query.Filters[0].Field != "status" || query.Filters[0].Op != persistence.Eq {
		t.Errorf("first filter incorrect")
	}

	if query.Filters[1].Field != "age" || query.Filters[1].Op != persistence.Gte {
		t.Errorf("second filter incorrect")
	}

	if query.Filters[2].Field != "age" || query.Filters[2].Op != persistence.Lte {
		t.Errorf("third filter incorrect")
	}

	if query.Filters[3].Field != "role" || query.Filters[3].Op != persistence.In {
		t.Errorf("fourth filter incorrect")
	}

	if len(query.SortBy) != 2 {
		t.Errorf("expected 2 sort fields, got %d", len(query.SortBy))
	}

	if query.SortBy[0].Field != "priority" || query.SortBy[0].Order != persistence.Desc {
		t.Errorf("first sort incorrect")
	}

	if query.SortBy[1].Field != "createdAt" || query.SortBy[1].Order != persistence.Asc {
		t.Errorf("second sort incorrect")
	}

	if query.LimitCount != 25 {
		t.Errorf("expected limit 25, got %d", query.LimitCount)
	}

	if query.SkipCount != 50 {
		t.Errorf("expected skip 50, got %d", query.SkipCount)
	}
}
