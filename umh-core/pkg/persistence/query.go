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

package persistence

const (
	DefaultMaxFindLimit = 1000
)

// Operator represents MongoDB-style query operators for filtering documents.
//
// DESIGN DECISION: Use MongoDB-style operators ($eq, $gt, $in)
// WHY: Familiar to developers, well-documented syntax, widely used in NoSQL databases.
// Operators like "$eq" are more explicit than "=" and avoid confusion with assignment.
//
// TRADE-OFF: String-based operators instead of type-safe enums mean typos aren't caught
// at compile time. However, the benefit is flexibility - backends can add custom operators
// without changing this package.
//
// INSPIRED BY: MongoDB query language, Linear's filtering system
//
// Example usage:
//
//	query := basic.NewQuery().
//	    Filter("status", basic.Eq, "active").      // Equality: status == "active"
//	    Filter("age", basic.Gt, 18).               // Greater than: age > 18
//	    Filter("role", basic.In, []string{"admin", "moderator"}) // In array
type Operator string

const (
	Eq  Operator = "$eq"  // Equal: field == value
	Ne  Operator = "$ne"  // Not equal: field != value
	Gt  Operator = "$gt"  // Greater than: field > value
	Gte Operator = "$gte" // Greater than or equal: field >= value
	Lt  Operator = "$lt"  // Less than: field < value
	Lte Operator = "$lte" // Less than or equal: field <= value
	In  Operator = "$in"  // In array: field IN (value1, value2, ...)
	Nin Operator = "$nin" // Not in array: field NOT IN (value1, value2, ...)
)

// FilterCondition represents a single filter criterion for querying documents.
//
// DESIGN DECISION: Store field, operator, and value as separate fields
// WHY: Explicit structure makes it easy for backends to translate to native queries.
// SQLite can generate "WHERE field > ?", MongoDB can generate "{field: {$gt: value}}".
//
// TRADE-OFF: More verbose than map[string]interface{} but type-safe and self-documenting.
//
// INSPIRED BY: MongoDB filter syntax, SQL WHERE clause structure
//
// Example:
//
//	condition := FilterCondition{
//	    Field: "age",
//	    Op:    basic.Gt,
//	    Value: 18,
//	}
//	// Translates to SQL: WHERE age > 18
//	// Translates to MongoDB: {age: {$gt: 18}}
type FilterCondition struct {
	Field string
	Op    Operator
	Value interface{}
}

// SortOrder represents sort direction (ascending or descending).
//
// DESIGN DECISION: Use numeric constants (1 for asc, -1 for desc)
// WHY: Matches MongoDB convention, semantically clear, and avoids bool ambiguity.
//
// TRADE-OFF: Could use bool (true=asc) but "1" and "-1" are more universally understood
// across databases. MongoDB, Elasticsearch, and many ORMs use this convention.
//
// INSPIRED BY: MongoDB sort order syntax.
type SortOrder int

const (
	Asc  SortOrder = 1  // Ascending order (A-Z, 0-9, oldest-newest)
	Desc SortOrder = -1 // Descending order (Z-A, 9-0, newest-oldest)
)

// SortField represents a field to sort by and its direction.
//
// DESIGN DECISION: Separate field name from order direction
// WHY: Allows sorting by multiple fields with different directions.
// Example: Sort by priority DESC, then createdAt ASC for stable ordering.
//
// TRADE-OFF: More verbose than string notation ("field:desc") but type-safe.
//
// INSPIRED BY: SQL ORDER BY clause, MongoDB sort syntax
//
// Example:
//
//	sortFields := []SortField{
//	    {Field: "priority", Order: basic.Desc},  // Higher priority first
//	    {Field: "createdAt", Order: basic.Asc},  // Then oldest first
//	}
type SortField struct {
	Field string
	Order SortOrder
}

// Query represents filtering, sorting, and pagination criteria for finding documents.
//
// DESIGN DECISION: Builder pattern with method chaining
// WHY: Readable API that mirrors MongoDB/SQL query construction. Allows building
// queries incrementally without verbose constructor calls.
//
// TRADE-OFF: Mutates the query instance instead of creating new immutable copies.
// This is simpler and more performant for common cases, though less functional.
//
// INSPIRED BY: MongoDB query builder, GORM query builder, Squirrel SQL builder
//
// Usage:
//
//	// Builder pattern (recommended)
//	query := basic.NewQuery().
//	    Filter("status", basic.Eq, "active").
//	    Filter("age", basic.Gt, 18).
//	    Sort("createdAt", basic.Desc).
//	    Limit(10).
//	    Skip(20)
//
//	// Direct construction (alternative)
//	query := basic.Query{
//	    Filters: []basic.FilterCondition{
//	        {Field: "status", Op: basic.Eq, Value: "active"},
//	    },
//	    SortBy:     []basic.SortField{{Field: "createdAt", Order: basic.Desc}},
//	    LimitCount: 10,
//	    SkipCount:  20,
//	}
//
// Pagination:
//
// DESIGN DECISION: Use Limit/Skip for pagination
// WHY: Simple API for small result sets (10-1000 documents), matches SQL LIMIT/OFFSET.
// Most FSM queries return small datasets (active assets, recent data points).
//
// TRADE-OFF: Skip(10000) is inefficient - database must scan and discard 10000 rows.
// For large datasets, cursor-based pagination would be better (future enhancement).
//
// FUTURE: Add cursor-based pagination for large result sets
// Example: NextCursor string, PreviousCursor string (like Linear's pagination)
//
// INSPIRED BY: SQL LIMIT/OFFSET, MongoDB limit/skip, Linear's sync pagination.
type Query struct {
	Filters      []FilterCondition
	SortBy       []SortField
	LimitCount   int
	SkipCount    int
	MaxFindLimit int
}

// NewQuery creates an empty query builder.
//
// DESIGN DECISION: Return pointer for method chaining
// WHY: Allows methods to mutate the query and return the same instance for chaining.
//
// TRADE-OFF: Returning pointer means the query can be accidentally modified elsewhere.
// Alternative would be immutable queries (each method returns new copy), but that's
// more complex and less performant.
//
// Example:
//
//	query := basic.NewQuery()  // Empty query
//	docs, _ := store.Find(ctx, "assets", *query)  // Returns all documents
//
//	query2 := basic.NewQuery().Filter("status", basic.Eq, "active")
//	docs2, _ := store.Find(ctx, "assets", *query2)  // Returns filtered documents
func NewQuery() *Query {
	return &Query{}
}

// Filter adds a filter condition to the query.
//
// DESIGN DECISION: Multiple Filter() calls create AND conditions
// WHY: Most common case is combining multiple filters with AND logic.
// Example: status=active AND age>18 AND role=admin
//
// TRADE-OFF: OR logic requires a different approach (future: FilterGroup with OR support).
// For now, backends can handle this by accepting multiple queries or custom operators.
//
// FUTURE: Add FilterGroup for OR/AND nesting
// Example: (status=active OR status=pending) AND age>18
//
// INSPIRED BY: MongoDB's implicit AND (multiple conditions in same document),
// GORM's Where() chaining
//
// Example:
//
//	// Simple equality
//	query.Filter("status", basic.Eq, "active")
//
//	// Comparison
//	query.Filter("age", basic.Gt, 18)
//	query.Filter("priority", basic.Lte, 5)
//
//	// Array membership
//	query.Filter("role", basic.In, []string{"admin", "moderator"})
//
//	// Chaining (implicit AND)
//	query.Filter("status", basic.Eq, "active").
//	     Filter("age", basic.Gt, 18).
//	     Filter("verified", basic.Eq, true)
//	// Equivalent to: status=active AND age>18 AND verified=true
func (q *Query) Filter(field string, op Operator, value interface{}) *Query {
	q.Filters = append(q.Filters, FilterCondition{
		Field: field,
		Op:    op,
		Value: value,
	})

	return q
}

// Sort adds a sort field to the query.
//
// DESIGN DECISION: Multiple Sort() calls define sort precedence
// WHY: First Sort() is primary, second is tiebreaker, etc. Matches SQL ORDER BY behavior.
//
// TRADE-OFF: Order of Sort() calls matters. Clear in code but could be confusing if
// sorts are added conditionally across different code paths.
//
// INSPIRED BY: SQL ORDER BY (ORDER BY field1 DESC, field2 ASC),
// MongoDB sort (sort: {field1: -1, field2: 1})
//
// Example:
//
//	// Single field sort
//	query.Sort("createdAt", basic.Desc)  // Newest first
//
//	// Multi-field sort (with precedence)
//	query.Sort("priority", basic.Desc).  // First: highest priority
//	     Sort("createdAt", basic.Asc)    // Then: oldest first (stable sort)
//	// Equivalent to SQL: ORDER BY priority DESC, createdAt ASC
func (q *Query) Sort(field string, order SortOrder) *Query {
	q.SortBy = append(q.SortBy, SortField{
		Field: field,
		Order: order,
	})

	return q
}

// Limit sets the maximum number of documents to return.
//
// DESIGN DECISION: Silently ignore negative values
// WHY: Negative limits are programmer errors, not user errors. Panic would crash the
// application, but silently treating as 0 (no limit) is safer and follows principle
// of least surprise.
//
// TRADE-OFF: Masks bugs where negative values are accidentally passed. Alternative
// would be to panic, but that's harsh for what's likely a logic error.
//
// INSPIRED BY: SQL LIMIT (negative values are errors), MongoDB limit() (ignores negatives)
//
// Example:
//
//	query.Limit(10)   // Return at most 10 documents
//	query.Limit(0)    // No limit (return all matching)
//	query.Limit(-5)   // Treated as 0 (no limit)
func (q *Query) Limit(count int) *Query {
	if count < 0 {
		count = 0
	}

	q.LimitCount = count

	return q
}

// Skip sets the number of documents to skip before returning results.
//
// DESIGN DECISION: Silently ignore negative values
// WHY: Same reasoning as Limit - treat programmer errors gracefully without panicking.
//
// TRADE-OFF: Skip(10000) is inefficient but allowed. Caller's responsibility to use
// reasonable skip values or switch to cursor pagination for large offsets.
//
// PERFORMANCE NOTE: Large skip values are slow in most databases
// WHY: Database must scan and discard N rows before returning results.
// For Skip(10000), database reads 10000 rows, throws them away, then returns next batch.
// Consider cursor-based pagination for large datasets.
//
// INSPIRED BY: SQL OFFSET, MongoDB skip()
//
// Example:
//
//	query.Skip(20)     // Skip first 20 documents
//	query.Skip(0)      // No skip (default)
//	query.Skip(-10)    // Treated as 0 (no skip)
//
//	// Pagination example
//	page := 3
//	pageSize := 10
//	query.Limit(pageSize).Skip((page - 1) * pageSize)  // Skip 20, return 10
func (q *Query) Skip(count int) *Query {
	if count < 0 {
		count = 0
	}

	q.SkipCount = count

	return q
}

func (q *Query) WithMaxFindLimit(limit int) *Query {
	if limit < 0 {
		limit = 0
	}

	q.MaxFindLimit = limit

	return q
}
