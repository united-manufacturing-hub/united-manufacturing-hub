# GraphQL Code Restructuring Plan

## Problem Statement

The current GraphQL implementation has a **mixed responsibility anti-pattern**:
- `resolver.go` contains `//go:generate` directive (should be generated)
- `resolver.go` contains custom business logic (should be separate)
- Running `make generate-graphql` mangles custom implementation code
- Structure violates gqlgen best practices and is fragile

## Current State Analysis

```
pkg/communicator/graphql/
├── generated.go      # ✅ 146KB pure generated server code (correct)
├── resolver.go       # ❌ Mixed generated + custom (PROBLEM)
├── models.go         # ✅ Generated GraphQL types (correct)  
├── schema.graphqls   # ✅ Schema definition (correct)
├── gqlgen.yml        # ⚠️  Needs configuration updates
├── server.go         # ✅ HTTP server setup (correct)
├── config.go         # ✅ Server configuration (correct)
├── helpers.go        # ✅ Utility functions (correct)
└── *_test.go         # ✅ Test files (correct)
```

**Key Issues:**
1. `resolver.go` line 1: `//go:generate go run github.com/99designs/gqlgen generate`
2. `resolver.go` contains 350+ lines of custom business logic
3. gqlgen regeneration destroys custom implementations
4. Type definitions scattered across files

## Target Architecture

```
pkg/communicator/graphql/
├── generated.go           # ✅ Pure generated server code (untouched)
├── resolver.go            # 🔄 Generated resolver interfaces only
├── resolver_impl.go       # 🆕 Custom resolver implementations  
├── types.go              # 🆕 Custom type definitions + interfaces
├── helpers.go            # ✅ Utility functions (existing, enhanced)
├── generate.go           # 🆕 Generation directive (replaces inline directive)
├── models.go             # ✅ Generated GraphQL types (untouched)
├── schema.graphqls       # ✅ Schema definition (untouched)
├── gqlgen.yml           # 🔄 Updated configuration
├── server.go            # ⚠️  Minor updates for new resolver constructor
├── config.go            # ✅ Server configuration (untouched)
└── *_test.go            # ⚠️  Minor updates for new structure
```

## Implementation Plan

### Phase 1: Setup & Configuration

#### Step 1.1: Update gqlgen Configuration
**File:** `gqlgen.yml`
```yaml
schema:
  - schema.graphqls

exec:
  filename: generated.go

model:
  filename: models.go

resolver:
  filename: resolver.go
  type: Resolver
  # Key change: Don't touch our custom implementations
  layout: follow-schema
  dir: .

# Custom scalar mappings
models:
  Time:
    model: github.com/99designs/gqlgen/graphql.Time
  JSON:
    model: github.com/99designs/gqlgen/graphql.Map

# Don't auto-bind everything - we control the structure
autobind: []

# Skip validation to allow our custom resolver structure
skip_validation: true
```

#### Step 1.2: Create Generation Directive File
**File:** `generate.go`
```go
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

//go:generate go run github.com/99designs/gqlgen generate

package graphql
```

### Phase 2: Extract Type Definitions

#### Step 2.1: Create Custom Types File
**File:** `types.go`
```go
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

package graphql

import (
	tbproto "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/models/topicbrowser/pb"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
)

// TopicBrowserCacheInterface defines the interface for topic browser cache
// This interface abstracts the topic browser cache for testing and dependency injection
type TopicBrowserCacheInterface interface {
	// GetEventMap returns a map of hashed topic IDs to their latest events
	GetEventMap() map[string]*tbproto.EventTableEntry
	
	// GetUnsMap returns the unified namespace topic structure
	GetUnsMap() *tbproto.TopicMap
}

// SnapshotProvider provides access to system snapshots containing topic data
// This interface is for future extensibility of snapshot data sources
type SnapshotProvider interface {
	GetSnapshot() *SystemSnapshot
}

// SystemSnapshot represents the system state snapshot
// Contains information about all managed system components
type SystemSnapshot struct {
	Managers map[string]interface{}
}

// ResolverDependencies holds all external dependencies needed by the GraphQL resolver
// This struct enables dependency injection and makes testing easier
type ResolverDependencies struct {
	// SnapshotManager provides access to system state and configuration
	SnapshotManager *fsm.SnapshotManager
	
	// TopicBrowserCache provides real-time access to topic data and events
	TopicBrowserCache TopicBrowserCacheInterface
}
```

### Phase 3: Create Implementation File

#### Step 3.1: Create Resolver Implementation
**File:** `resolver_impl.go`
```go
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

package graphql

import (
	"context"
)

// NewResolver creates a new GraphQL resolver with the provided dependencies
// This constructor ensures all required dependencies are properly injected
func NewResolver(deps *ResolverDependencies) *Resolver {
	if deps == nil {
		deps = &ResolverDependencies{} // Provide safe defaults
	}
	
	return &Resolver{
		deps: deps,
	}
}

// Topics resolves the GraphQL topics query with filtering and pagination
// This is the main entry point for browsing unified namespace topics
func (r *Resolver) Topics(ctx context.Context, filter *TopicFilter, limit *int) ([]*Topic, error) {
	// Early return if no cache available
	if r.deps == nil || r.deps.TopicBrowserCache == nil {
		return []*Topic{}, nil
	}

	// Get snapshot of cached event data
	eventMap := r.deps.TopicBrowserCache.GetEventMap()
	unsMap := r.deps.TopicBrowserCache.GetUnsMap()

	if unsMap == nil || unsMap.Entries == nil {
		return []*Topic{}, nil
	}

	// Determine effective limit early to optimize processing
	const defaultMaxLimit = 100
	maxLimit := defaultMaxLimit
	if limit != nil && *limit > 0 && *limit < defaultMaxLimit {
		maxLimit = *limit
	}

	var topics []*Topic

	// Convert protobuf data to GraphQL models with early termination for performance
	processedCount := 0
	for _, topicInfo := range unsMap.Entries {
		// Check context cancellation for long-running operations
		if processedCount%50 == 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}

		// Calculate the hashed UNS tree ID for this topic (matches simulator algorithm)
		hashedTreeId := hashUNSTableEntry(topicInfo)

		// Create topic object with metadata
		topic := &Topic{
			Topic:     buildTopicName(topicInfo),
			Metadata:  mapMetadataToGraphQL(topicInfo.Metadata),
			LastEvent: nil, // Will be populated below if event exists
		}

		// Apply filters early to avoid unnecessary event processing
		if filter != nil && !matchesFilter(topic, filter) {
			processedCount++
			continue
		}

		// Get latest event for this topic if available (only after filtering for performance)
		if eventEntry, exists := eventMap[hashedTreeId]; exists {
			topic.LastEvent = mapEventEntryToGraphQL(eventEntry)
		}

		topics = append(topics, topic)

		// Stop processing if we've reached the limit (after filtering)
		if len(topics) >= maxLimit {
			break
		}

		processedCount++
	}

	return topics, nil
}

// Topic resolves the GraphQL topic query for a specific topic path
// This provides detailed information about a single topic
func (r *Resolver) Topic(ctx context.Context, topic string) (*Topic, error) {
	// Reuse the Topics resolver with a text filter for the specific topic
	topics, err := r.Topics(ctx, &TopicFilter{Text: &topic}, nil)
	if err != nil {
		return nil, err
	}

	// Find exact match for the requested topic
	for _, t := range topics {
		if t.Topic == topic {
			return t, nil
		}
	}

	// Return nil if topic not found (GraphQL will return null)
	return nil, nil
}
```

### Phase 4: Update Helper Functions

#### Step 4.1: Enhance helpers.go
**Additions to existing `helpers.go`:**
```go
// Add these functions extracted from resolver.go:

import (
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/cespare/xxhash/v2"
	tbproto "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/models/topicbrowser/pb"
)

// buildTopicName constructs the full topic path from protobuf topic info
// Uses the expert's exact algorithm: Level0 + LocationSublevels + DataContract + VirtualPath + Name
func buildTopicName(topicInfo *tbproto.TopicInfo) string {
	var parts []string
	parts = append(parts, topicInfo.Level0)
	parts = append(parts, topicInfo.LocationSublevels...)
	parts = append(parts, topicInfo.DataContract)

	// VirtualPath may already contain dots; add as-is if present
	if topicInfo.VirtualPath != nil && *topicInfo.VirtualPath != "" {
		parts = append(parts, *topicInfo.VirtualPath)
	}

	parts = append(parts, topicInfo.Name)

	return strings.Join(parts, ".")
}

// hashUNSTableEntry generates an xxHash from the topic information
// This function is copied from the benthos topic browser plugin to ensure
// identical hash generation for compatibility across components
func hashUNSTableEntry(info *tbproto.TopicInfo) string {
	hasher := xxhash.New()

	// Helper function to write each component followed by NUL delimiter to avoid ambiguity
	write := func(s string) {
		_, _ = hasher.Write(append([]byte(s), 0))
	}

	write(info.Level0)

	// Hash all location sublevels
	for _, level := range info.LocationSublevels {
		write(level)
	}

	write(info.DataContract)

	// Hash virtual path if it exists
	if info.VirtualPath != nil {
		write(*info.VirtualPath)
	}

	// Hash the name (new field)
	write(info.Name)

	return hex.EncodeToString(hasher.Sum(nil))
}

// ... (add all other helper functions from current resolver.go)
```

### Phase 5: Update Generated Code Structure

#### Step 5.1: Clean resolver.go
**New `resolver.go` (after extracting custom code):**
```go
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

package graphql

import "context"

// This file contains the generated GraphQL resolver interfaces and delegation
// Custom implementation is in resolver_impl.go to maintain separation of concerns

// Resolver struct contains dependencies and delegates to implementation methods
type Resolver struct {
	deps *ResolverDependencies
}

// Query resolver methods - these delegate to the implementation

// Topics is the resolver for the topics field.
func (r *queryResolver) Topics(ctx context.Context, filter *TopicFilter, limit *int) ([]*Topic, error) {
	return r.Resolver.Topics(ctx, filter, limit)
}

// Topic is the resolver for the topic field.
func (r *queryResolver) Topic(ctx context.Context, topic string) (*Topic, error) {
	return r.Resolver.Topic(ctx, topic)
}

// Generated boilerplate required by gqlgen
func (r *Resolver) Query() QueryResolver { return &queryResolver{r} }

type queryResolver struct{ *Resolver }
```

### Phase 6: Update Integration Points

#### Step 6.1: Update server.go
**Changes to `server.go`:**
```go
// In StartGraphQLServer function, replace resolver creation:

// Old:
// resolver := &Resolver{
//     SnapshotManager:   systemSnapshotManager,
//     TopicBrowserCache: topicBrowserCache,
// }

// New:
resolver := NewResolver(&ResolverDependencies{
    SnapshotManager:   systemSnapshotManager,
    TopicBrowserCache: topicBrowserCache,
})
```

#### Step 6.2: Update Makefile
**Update `generate-graphql` target:**
```makefile
## Generate GraphQL code from schema
generate-graphql:
	@echo "Installing gqlgen..."
	@go install github.com/99designs/gqlgen@latest
	@echo "Generating GraphQL code..."
	@cd pkg/communicator/graphql && go generate ./generate.go
	@echo "GraphQL code generated successfully."
```

## Migration Steps (Execution Order)

### 🚨 Pre-Migration Safety
```bash
# Backup current working implementation
cp pkg/communicator/graphql/resolver.go pkg/communicator/graphql/resolver.go.backup
cp pkg/communicator/graphql/gqlgen.yml pkg/communicator/graphql/gqlgen.yml.backup
```

### Step 1: Create New Files
1. Create `generate.go` with generation directive
2. Create `types.go` with custom type definitions  
3. Create `resolver_impl.go` with implementations
4. Update `gqlgen.yml` configuration

### Step 2: Extract Code from resolver.go
1. Move custom types to `types.go`
2. Move implementation methods to `resolver_impl.go`
3. Move helper functions to `helpers.go` (if not already there)

### Step 3: Clean resolver.go
1. Remove `//go:generate` directive
2. Remove custom implementations
3. Keep only generated interfaces and delegation
4. Update struct to use dependency injection

### Step 4: Update Integration
1. Update `server.go` to use new constructor
2. Update tests to use new structure
3. Update Makefile to use new generation file

### Step 5: Test & Validate
```bash
# Test generation doesn't break custom code
make generate-graphql

# Verify no unintended changes
git diff

# Test GraphQL server functionality
make test-graphql
```

## Rollback Plan

If migration fails:
```bash
# Restore original files
cp pkg/communicator/graphql/resolver.go.backup pkg/communicator/graphql/resolver.go
cp pkg/communicator/graphql/gqlgen.yml.backup pkg/communicator/graphql/gqlgen.yml

# Remove new files
rm pkg/communicator/graphql/generate.go
rm pkg/communicator/graphql/types.go  
rm pkg/communicator/graphql/resolver_impl.go

# Restore working state
make generate-graphql
```

## Benefits After Migration

✅ **Safe Code Generation**: `make generate-graphql` won't destroy custom logic  
✅ **Clear Separation**: Generated vs custom code in separate files  
✅ **Better Testing**: Business logic isolated and easily mockable  
✅ **Maintainable**: Each file has single responsibility  
✅ **Standard**: Follows gqlgen community best practices  
✅ **Dependency Injection**: Resolver dependencies clearly defined  
✅ **Future-Proof**: Schema changes won't break implementations  

## Success Criteria

- [ ] `make generate-graphql` runs without modifying custom code
- [ ] GraphQL server starts and serves requests correctly
- [ ] All existing tests pass
- [ ] New structure follows gqlgen best practices
- [ ] Code is more maintainable and testable
- [ ] Documentation is updated to reflect new structure

---

**Implementation Status:** 📋 Planning Phase  
**Next Action:** Execute Phase 1 (Setup & Configuration)  
**Est. Time:** 2-3 hours total implementation time 