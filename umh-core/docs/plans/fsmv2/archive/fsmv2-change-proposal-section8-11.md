# FSMv2 Master Plan Change Proposal - Sections 8-11

**Date:** 2025-11-03
**Status:** Draft for Review
**Related:**
- `fsmv2-master-plan-gap-analysis.md` - Gap analysis identifying missing components
- `2025-11-02-fsmv2-supervision-and-async-actions.md` - Current master plan
- `fsmv2-derive-desired-state-complete-definition.md` - Complete API specification
- `fsmv2-child-specs-in-desired-state.md` - Hierarchical composition architecture
- `fsmv2-idiomatic-templating-and-variables.md` - Templating and variables system

---

## Section 8: Updated Testing Strategy

### 8.1: Unit Testing

For each new component, specify test coverage targets and key test scenarios.

#### Hierarchical Composition

**ChildSpec Serialization:**
- JSON roundtrip (marshal → unmarshal → deep equal)
- YAML roundtrip (for config.yaml compatibility)
- Deep equality with nested structures (UserSpec with Variables)
- Empty/nil ChildrenSpecs handling

**WorkerFactory:**
- Registration of 10+ worker types (ConnectionWorker, BenthosWorker, etc.)
- CreateWorker() with valid types returns correct worker instance
- CreateWorker() with unknown type returns clear error
- Factory injection through supervisor hierarchy

**DesiredState with ChildrenSpecs:**
- Empty ChildrenSpecs (leaf worker, no children)
- Single child (Bridge → Connection)
- Multiple children (Bridge → Connection + Benthos + Benthos)
- Deep nesting (3+ levels)

**StateMapping Application:**
- Simple mapping: `{"active": "up", "idle": "down"}`
- Unmapped parent state (child retains previous state)
- Child override of mapped state (safety check)
- Invalid mapping (parent state not in child state space)

**reconcileChildren() Operations:**
- Add new child (not in current children, exists in ChildrenSpecs)
- Update existing child (UserSpec changed)
- Remove child (exists in current children, not in ChildrenSpecs)
- No-op (ChildrenSpecs unchanged from previous tick)
- Multiple operations in single tick (add 2, remove 1, update 1)

**Test Coverage Targets:**
- New components: >80% line coverage
- Modified components: No coverage regression vs baseline
- Critical paths: 100% coverage
  - reconcileChildren() (all 3 operations: add/update/remove)
  - Supervisor.Tick() (full flow with children)
  - WorkerFactory.CreateWorker() (all registered types)

**Example Test:**
```go
Describe("reconcileChildren", func() {
    Context("when adding new child", func() {
        It("creates child supervisor with WorkerFactory", func() {
            supervisor := newTestSupervisor()
            childSpec := ChildSpec{
                Name:       "connection-1",
                WorkerType: "ConnectionWorker",
                UserSpec:   UserSpec{Name: "PLC-Connection"},
            }

            supervisor.reconcileChildren([]ChildSpec{childSpec})

            child := supervisor.children["connection-1"]
            Expect(child).ToNot(BeNil())
            Expect(child.supervisor).ToNot(BeNil())
            Expect(child.supervisor.worker).To(BeAssignableToTypeOf(&ConnectionWorker{}))
        })
    })

    Context("when removing child", func() {
        It("stops and cleans up child supervisor", func() {
            supervisor := newTestSupervisor()
            supervisor.children["connection-1"] = &childInstance{...}

            supervisor.reconcileChildren([]ChildSpec{}) // Empty specs

            Expect(supervisor.children).To(BeEmpty())
            Expect(supervisor.children["connection-1"]).To(BeNil())
        })
    })

    Context("when updating child", func() {
        It("updates UserSpec without recreating supervisor", func() {
            supervisor := newTestSupervisor()
            oldChild := &childInstance{...}
            supervisor.children["connection-1"] = oldChild

            updatedSpec := ChildSpec{
                Name:     "connection-1",
                UserSpec: UserSpec{IP: "192.168.1.101"}, // Changed IP
            }

            supervisor.reconcileChildren([]ChildSpec{updatedSpec})

            Expect(supervisor.children["connection-1"]).To(Equal(oldChild)) // Same instance
            Expect(supervisor.children["connection-1"].spec.UserSpec.IP).To(Equal("192.168.1.101"))
        })
    })
})
```

#### Templating & Variables

**VariableBundle Serialization:**
- JSON roundtrip with all namespaces (User/Global/Internal)
- Empty namespaces (nil vs empty map)
- Nested structures in User namespace (location map)

**Flatten() Method:**
- Empty bundle returns empty map
- User variables become top-level (`IP: "192.168.1.100"` → `{IP: "192.168.1.100"}`)
- Global namespace nested (`Global: {kafka_brokers: "..."}` → `{global: {kafka_brokers: "..."}}`)
- Internal namespace nested similarly
- Partial namespaces (User only, Global only, etc.)

**RenderTemplate() with Strings:**
- Simple substitution: `"{{ .IP }}"` → `"192.168.1.100"`
- Multiple variables: `"{{ .IP }}:{{ .PORT }}"` → `"192.168.1.100:502"`
- Nested access: `"{{ .global.kafka_brokers }}"` → `"localhost:9092"`
- Missing key with strict mode: returns error
- Leftover {{ markers: returns error

**RenderTemplate() with Structs:**
- Entire struct with template fields
- Nested struct fields
- Array of structs with templates
- Partial struct (some fields templated, some static)

**Location Computation:**
- Empty location → all "unknown"
- Partial location → gaps filled with "unknown"
- Full ISA-95 hierarchy → clean path
- Legacy single-level → graceful degradation
- Parent + child merge → correct extension

**Variable Injection in Supervisor:**
- User variables from parent's ChildSpec
- Global variables injected by supervisor
- Internal variables (id, created_at) injected
- All three combined in Flatten()

**Test Coverage Targets:**
- VariableBundle: >90% (simple data structure)
- Flatten(): 100% (critical for templates)
- RenderTemplate(): >95% (many edge cases)
- Location computation: >90% (ISA-95 + legacy)

**Example Test:**
```go
Describe("RenderTemplate", func() {
    Context("with missing variable in strict mode", func() {
        It("returns error", func() {
            template := struct {
                Address string `json:"address"`
            }{
                Address: "{{ .IP }}:{{ .PORT }}",
            }
            scope := map[string]any{
                "IP": "192.168.1.100",
                // PORT missing
            }

            _, err := RenderTemplate(template, scope)

            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("no entry for key \"PORT\""))
        })
    })

    Context("with leftover template markers", func() {
        It("returns error", func() {
            template := "{{ .IP "  // Malformed

            _, err := RenderTemplate(template, scope)

            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("unclosed action"))
        })
    })
})
```

### 8.2: Integration Testing

Integration tests verify cross-component interactions and end-to-end flows.

#### Parent-Child Relationships

**Simple Parent-Child (Bridge → Connection):**
- Bridge creates Connection via ChildSpec
- StateMapping applied (Bridge "active" → Connection "up")
- Variables passed to Connection (IP, PORT)
- Connection tick executes with parent state

**Parent with Multiple Children (Bridge → Connection + 2 Benthos):**
- Bridge creates 3 children in single ChildrenSpecs
- StateMapping applied to all 3 children
- Each child receives different UserSpec
- All 3 children tick after parent derives state

**Dynamic Children (OPC UA Browser → N Servers):**
- Browser derives N ChildSpecs based on discovered servers
- reconcileChildren() adds all N supervisors
- Subsequent tick with N-1 servers removes 1 child
- Subsequent tick with N+2 servers adds 3 children

**Multi-Level Hierarchy (Grandparent → Parent → Child):**
- Grandparent creates Parent as child
- Parent creates Child as grandchild
- Variables flow through 3 levels
- StateMapping propagates correctly
- Tick cascades from grandparent to child

**Example Test:**
```go
Describe("Parent with Multiple Children", func() {
    It("creates and manages 3 children correctly", func() {
        // 1. Setup Bridge worker that returns 3 ChildSpecs
        bridge := newBridgeWorker()
        supervisor := NewSupervisor(bridge, factory, logger)

        // 2. First tick derives ChildrenSpecs
        supervisor.Tick(ctx)

        // 3. Verify 3 children created
        Expect(supervisor.children).To(HaveLen(3))
        Expect(supervisor.children["connection-1"]).ToNot(BeNil())
        Expect(supervisor.children["dfc_read"]).ToNot(BeNil())
        Expect(supervisor.children["dfc_write"]).ToNot(BeNil())

        // 4. Second tick with different specs (remove dfc_write)
        bridge.SetChildSpecs([]ChildSpec{
            {Name: "connection-1", ...},
            {Name: "dfc_read", ...},
        })
        supervisor.Tick(ctx)

        // 5. Verify dfc_write removed
        Expect(supervisor.children).To(HaveLen(2))
        Expect(supervisor.children["dfc_write"]).To(BeNil())
    })
})
```

#### Variable Propagation

**Parent Variables → Child UserSpec:**
- Parent constructs ChildSpec with Variables in UserSpec
- Child receives Variables in DeriveDesiredState()
- Child flattens and accesses variables

**Location Merging Across Hierarchy:**
- Grandparent: `{enterprise: "ACME", site: "Factory"}`
- Parent: `{line: "Line-A"}`
- Child: `{cell: "Cell-5"}`
- Result at child: `{enterprise: "ACME", site: "Factory", line: "Line-A", cell: "Cell-5"}`

**Global Variables Accessible at All Levels:**
- Global set at root supervisor
- Grandparent receives Global in Variables
- Parent receives Global (passed through)
- Child receives Global (passed through)
- All levels can access `{{ .global.kafka_brokers }}`

**Internal Variables Per Supervisor:**
- Grandparent has `internal.id = "grandparent-123"`
- Parent has `internal.id = "parent-456"`
- Child has `internal.id = "child-789"`
- Each supervisor injects own Internal before ticking

**Example Test:**
```go
Describe("Variable Propagation Through Hierarchy", func() {
    It("merges location across 3 levels", func() {
        // 1. Setup grandparent with location
        grandparent := newGrandparentWorker()
        grandparent.userSpec.Variables.User["location"] = map[string]string{
            "enterprise": "ACME",
            "site":       "Factory",
        }

        // 2. Grandparent creates parent with extended location
        grandparent.DeriveDesiredState() // Returns ChildSpec with location + line

        // 3. Parent creates child with further extended location
        parent := supervisor.children["parent-1"].supervisor.worker
        parent.DeriveDesiredState() // Returns ChildSpec with location + cell

        // 4. Verify child receives full location
        child := supervisor.children["parent-1"].supervisor.children["child-1"].supervisor.worker
        location := child.userSpec.Variables.User["location"].(map[string]string)

        Expect(location).To(Equal(map[string]string{
            "enterprise": "ACME",
            "site":       "Factory",
            "line":       "Line-A",
            "cell":       "Cell-5",
        }))
    })
})
```

#### State Control

**StateMapping Applied Correctly:**
- Parent state changes to "active"
- Supervisor applies StateMapping before child tick
- Child supervisor receives SetDesiredState("up")
- Child DeriveDesiredState() sees desired "up"

**Child Can Override for Safety:**
- Parent maps "active" → "up"
- Child DeriveDesiredState() checks safety condition
- Safety condition fails, child returns state "down"
- Child state remains "down" despite parent mapping

**Parent State Changes Propagate:**
- Parent state "idle" → children mapped to "down"
- Parent state "active" → children mapped to "up"
- Multiple transitions verified

**Example Test:**
```go
Describe("StateMapping Application", func() {
    It("applies mapping before child tick", func() {
        parent := newParentWorker()
        parent.SetDesiredState(DesiredState{
            State: "active",
            ChildrenSpecs: []ChildSpec{
                {
                    Name:         "child-1",
                    StateMapping: map[string]string{"active": "up", "idle": "down"},
                },
            },
        })

        supervisor := NewSupervisor(parent, factory, logger)
        supervisor.Tick(ctx)

        // Verify child received "up" state
        child := supervisor.children["child-1"].supervisor
        Expect(child.desiredState.State).To(Equal("up"))
    })
})
```

### 8.3: System Testing

System tests verify complete use cases end-to-end with realistic scenarios.

#### ProtocolConverter Hierarchy

**Setup:**
- Bridge (parent) with Connection (child 1) + DFC Read (child 2) + DFC Write (child 3)
- Templates for Benthos configs (DFC Read, DFC Write)
- Variables: IP, PORT, location

**Test Flow:**
1. Bridge DeriveDesiredState() returns 3 ChildSpecs
2. reconcileChildren() creates 3 supervisors
3. StateMapping applied: Bridge "active" → all children "up"
4. Variables passed: IP, PORT, location_path
5. DFC workers render Benthos configs from templates
6. All 3 children tick successfully
7. Bridge state changes to "idle"
8. StateMapping applied: Bridge "idle" → all children "down"
9. Children transition to "down"

**Example Test:**
```go
Describe("ProtocolConverter Use Case", func() {
    It("manages Bridge → Connection + 2 DFCs end-to-end", func() {
        // Setup
        bridge := newBridgeWorker()
        bridge.userSpec.Variables.User["IP"] = "192.168.1.100"
        bridge.userSpec.Variables.User["PORT"] = 502

        supervisor := NewSupervisor(bridge, factory, logger)

        // First tick: Create children
        supervisor.Tick(ctx)

        Expect(supervisor.children).To(HaveLen(3))
        Expect(supervisor.children["connection-1"]).ToNot(BeNil())
        Expect(supervisor.children["dfc_read"]).ToNot(BeNil())
        Expect(supervisor.children["dfc_write"]).ToNot(BeNil())

        // Verify children received variables
        dfcRead := supervisor.children["dfc_read"].supervisor.worker.(*BenthosWorker)
        config := dfcRead.renderedConfig
        Expect(config).To(ContainSubstring("192.168.1.100:502"))

        // Change bridge state
        bridge.SetDesiredState(DesiredState{State: "idle"})
        supervisor.Tick(ctx)

        // Verify children transitioned to "down"
        for _, child := range supervisor.children {
            Expect(child.supervisor.desiredState.State).To(Equal("down"))
        }
    })
})
```

#### OPC UA Discovery

**Setup:**
- OPC UA Browser (parent) discovers 5 servers dynamically
- Each server becomes a child supervisor
- Servers come and go over time

**Test Flow:**
1. Browser discovers 5 servers
2. Browser DeriveDesiredState() returns 5 ChildSpecs
3. reconcileChildren() creates 5 supervisors
4. Next tick: Browser discovers 3 servers (2 disappeared)
5. reconcileChildren() removes 2 supervisors
6. Next tick: Browser discovers 7 servers (4 new)
7. reconcileChildren() adds 4 supervisors

**Example Test:**
```go
Describe("OPC UA Dynamic Discovery", func() {
    It("adds and removes server children dynamically", func() {
        browser := newOPCUABrowserWorker()
        browser.SetDiscoveredServers([]string{"server1", "server2", "server3"})

        supervisor := NewSupervisor(browser, factory, logger)

        // First tick: 3 servers
        supervisor.Tick(ctx)
        Expect(supervisor.children).To(HaveLen(3))

        // Second tick: 2 servers (1 removed)
        browser.SetDiscoveredServers([]string{"server1", "server2"})
        supervisor.Tick(ctx)
        Expect(supervisor.children).To(HaveLen(2))

        // Third tick: 5 servers (3 added)
        browser.SetDiscoveredServers([]string{"server1", "server2", "server4", "server5", "server6"})
        supervisor.Tick(ctx)
        Expect(supervisor.children).To(HaveLen(5))
    })
})
```

#### Multi-Customer Setup

**Setup:**
- 3-level location hierarchy: Enterprise → Site → Area
- Each level creates children with extended location
- Variables flow through all levels

**Test Flow:**
1. Enterprise supervisor creates 2 sites
2. Site supervisors create 3 areas each
3. Area supervisors create workers
4. Verify location path at leaf: "ACME.Factory-1.Assembly.Line-A"
5. Verify all levels receive Global variables
6. Verify each level has unique Internal variables

#### Infrastructure + Hierarchical

**Circuit Breaker with Child Supervisors:**
- Parent supervisor has InfrastructureHealthChecker
- CheckChildConsistency() fails (Redpanda active, Benthos no connections)
- Circuit opens, tick skipped
- Child restart triggered
- Circuit closes, tick resumes

**Child Restart Recreates from WorkerFactory:**
- Child supervisor fails health check
- restartChild() called
- Old supervisor stopped
- WorkerFactory creates new worker instance
- New supervisor created with fresh worker

**Observations Continue During Circuit Open:**
- Circuit opens due to infrastructure failure
- Collectors keep running (independent goroutines)
- Observations written to TriangularStore every 5s
- Circuit closes, tick reads fresh observations

**Example Test:**
```go
Describe("Circuit Breaker with Children", func() {
    It("stops tick when child consistency fails", func() {
        supervisor := newSupervisorWithHealthChecker()
        supervisor.infraHealthChecker.SetConsistencyCheckFunc(func() error {
            return fmt.Errorf("redpanda active but benthos has 0 connections")
        })

        // Tick with failing health check
        err := supervisor.Tick(ctx)

        Expect(err).ToNot(HaveOccurred()) // Returns nil (circuit open)
        Expect(supervisor.circuitOpen).To(BeTrue())

        // Verify children not ticked
        for _, child := range supervisor.children {
            Expect(child.tickCount).To(Equal(0))
        }
    })
})
```

#### Actions + Hierarchical

**Parent Action Blocks Children:**
- Parent has action in progress
- HasActionInProgress(parent.id) returns true
- Tick skips parent DeriveDesiredState
- Children not ticked (parent didn't derive ChildrenSpecs)

**Child Action Blocks Only Self:**
- Child has action in progress
- HasActionInProgress(child.id) returns true
- Parent ticks normally
- Siblings tick normally
- Only blocked child skipped

**Actions During Child Restart:**
- Child supervisor being restarted
- In-progress actions for that child continue
- Actions timeout if child not ready
- Actions retry with backoff
- Actions succeed once child recovers

**Example Test:**
```go
Describe("Actions with Hierarchical Workers", func() {
    Context("when parent has action in progress", func() {
        It("skips entire tick (parent + children)", func() {
            supervisor := newSupervisorWithActionExecutor()
            supervisor.actionExecutor.EnqueueAction("parent-1", someAction, registry)

            supervisor.Tick(ctx)

            // Verify parent not ticked
            Expect(supervisor.deriveCallCount).To(Equal(0))

            // Verify children not ticked
            for _, child := range supervisor.children {
                Expect(child.tickCount).To(Equal(0))
            }
        })
    })

    Context("when child has action in progress", func() {
        It("skips only that child", func() {
            supervisor := newSupervisorWithActionExecutor()
            supervisor.actionExecutor.EnqueueAction("child-1", someAction, registry)

            supervisor.Tick(ctx)

            // Verify parent ticked
            Expect(supervisor.deriveCallCount).To(Equal(1))

            // Verify child-1 skipped
            Expect(supervisor.children["child-1"].tickCount).To(Equal(0))

            // Verify child-2 ticked
            Expect(supervisor.children["child-2"].tickCount).To(Equal(1))
        })
    })
})
```

### 8.4: Performance Testing

Performance tests verify system meets latency and throughput targets.

#### Benchmarks

**Template Rendering (<1ms per worker):**
```go
func BenchmarkTemplateRendering(b *testing.B) {
    template := BenthosConfig{
        Input: InputConfig{
            Address: "{{ .IP }}:{{ .PORT }}",
        },
        Output: OutputConfig{
            Topic: "umh.v1.{{ .location_path }}.{{ .name }}",
        },
    }
    scope := map[string]any{
        "IP":            "192.168.1.100",
        "PORT":          502,
        "location_path": "ACME.Factory.Line-A",
        "name":          "PLC-Bridge",
    }

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, err := RenderTemplate(template, scope)
        if err != nil {
            b.Fatal(err)
        }
    }
}

// Target: <1ms per operation (p95)
```

**Variable Flattening (<100μs):**
```go
func BenchmarkVariableFlattening(b *testing.B) {
    bundle := VariableBundle{
        User: map[string]any{
            "IP":   "192.168.1.100",
            "PORT": 502,
            "location": map[string]string{
                "enterprise": "ACME",
                "site":       "Factory",
                "line":       "Line-A",
            },
        },
        Global: map[string]any{
            "kafka_brokers": "localhost:9092",
        },
        Internal: map[string]any{
            "id":         "worker-123",
            "created_at": time.Now(),
        },
    }

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _ = bundle.Flatten()
    }
}

// Target: <100μs per operation
```

**reconcileChildren() (<10ms for 100 children):**
```go
func BenchmarkReconcileChildren(b *testing.B) {
    supervisor := newTestSupervisor()

    // 100 children
    specs := make([]ChildSpec, 100)
    for i := 0; i < 100; i++ {
        specs[i] = ChildSpec{
            Name:       fmt.Sprintf("child-%d", i),
            WorkerType: "TestWorker",
            UserSpec:   UserSpec{Name: fmt.Sprintf("test-%d", i)},
        }
    }

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        supervisor.reconcileChildren(specs)
    }
}

// Target: <10ms for 100 children
```

**Tick Loop Overhead (<10% vs FSMv1):**
```go
func BenchmarkTickLoop(b *testing.B) {
    supervisor := newTestSupervisor()

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        supervisor.Tick(context.Background())
    }
}

// Target: <10% slower than FSMv1 baseline
```

#### Load Testing

**100 Bridges with 3 Children Each (400 Workers):**
- Setup: 100 bridge supervisors, each with 3 children
- Measure: Tick latency at p50, p95, p99
- Target:
  - p50 < 50ms
  - p95 < 100ms
  - p99 < 200ms

**Memory Overhead Per Supervisor:**
- Measure: Memory usage with 1 supervisor vs 100 supervisors
- Calculate: Per-supervisor overhead
- Target: <1MB per supervisor

**GC Pressure from Reconciliation:**
- Measure: GC pauses during reconcileChildren() with 100 children
- Measure: Allocations per tick
- Target: <10% increase in GC time vs FSMv1

**Example Load Test:**
```go
var _ = Describe("Load Test: 100 Bridges", func() {
    It("maintains acceptable latency with 400 workers", func() {
        // Setup 100 bridges
        supervisors := make([]*Supervisor, 100)
        for i := 0; i < 100; i++ {
            bridge := newBridgeWorker()
            supervisors[i] = NewSupervisor(bridge, factory, logger)
        }

        // Measure tick latency
        latencies := make([]time.Duration, 1000)
        for i := 0; i < 1000; i++ {
            start := time.Now()
            for _, sup := range supervisors {
                sup.Tick(context.Background())
            }
            latencies[i] = time.Since(start)
        }

        // Calculate percentiles
        sort.Slice(latencies, func(i, j int) bool {
            return latencies[i] < latencies[j]
        })

        p50 := latencies[500]
        p95 := latencies[950]
        p99 := latencies[990]

        Expect(p50).To(BeNumerically("<", 50*time.Millisecond))
        Expect(p95).To(BeNumerically("<", 100*time.Millisecond))
        Expect(p99).To(BeNumerically("<", 200*time.Millisecond))
    })
})
```

---

## Section 9: Migration Strategy

### 9.1: Backward Compatibility

#### Approach

**No Breaking Changes Initially:**
- New structures added alongside old (DesiredState with ChildrenSpecs field)
- Worker interface extended with default implementations
- Supervisors support both old and new worker styles
- Existing workers continue to work without changes

**Deprecation Path:**

**Phase 0: Add New Structures (Week 1-2)**
- Add ChildrenSpecs field to DesiredState (default: empty slice)
- Add WorkerFactory interface and DefaultWorkerFactory
- Mark old patterns as deprecated in documentation
- All new code uses new patterns
- Existing code continues to work

**Phase 1: Migrate Example Workers (Week 3-4)**
- Migrate BridgeWorker to new API (returns ChildrenSpecs)
- Migrate ConnectionWorker (leaf worker, returns empty ChildrenSpecs)
- Document migration process in detail
- Provide before/after examples

**Phase 2: Migrate Remaining Workers (Week 5-8)**
- Migrate all 10+ worker types to new API
- Each worker migration: 1-2 hours
- Update unit tests for each worker
- Update integration tests

**Phase 3: Remove Deprecated Code (Week 9-10)**
- Remove old DeclareChildren() method (if existed)
- Remove fallback code for old patterns
- Clean up documentation
- Final validation

#### Breaking Change Mitigation

**Worker Migration Template:**

```go
// OLD (deprecated):
func (w *BridgeWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    return DesiredState{
        State: "active",
        // No children
    }
}

// NEW (after migration):
func (w *BridgeWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    // Merge location
    parentLocation := userSpec.Variables.User["location"].(map[string]string)
    bridgeLocation := map[string]string{"bridge": userSpec.Name}
    location := mergeLocations(parentLocation, bridgeLocation)
    locationPath := computeLocationPath(location)

    return DesiredState{
        State: "active",
        ChildrenSpecs: []ChildSpec{
            {
                Name:       "connection-1",
                WorkerType: "ConnectionWorker",
                UserSpec: UserSpec{
                    Name: "Connection",
                    Variables: VariableBundle{
                        User: map[string]any{
                            "parent_state":  "active",
                            "IP":            userSpec.Variables.User["IP"],
                            "PORT":          userSpec.Variables.User["PORT"],
                            "location":      location,
                            "location_path": locationPath,
                        },
                    },
                },
                StateMapping: map[string]string{
                    "active": "up",
                    "idle":   "down",
                },
            },
            // ... more children
        },
    }
}
```

**Key Compatibility Points:**

1. **DesiredState Structure:**
   - BEFORE: `{State: string}`
   - AFTER: `{State: string, ChildrenSpecs: []ChildSpec}`
   - Compatibility: Empty ChildrenSpecs = leaf worker (no children)

2. **Worker Interface:**
   - BEFORE: `DeriveDesiredState(userSpec UserSpec) DesiredState`
   - AFTER: Same signature, but DesiredState now has ChildrenSpecs field
   - Compatibility: Existing workers return empty ChildrenSpecs implicitly

3. **Supervisor Constructor:**
   - BEFORE: `NewSupervisor(worker Worker, logger *zap.Logger) *Supervisor`
   - AFTER: `NewSupervisor(worker Worker, factory WorkerFactory, logger *zap.Logger) *Supervisor`
   - Compatibility: Create DefaultWorkerFactory singleton at startup

4. **TriangularStore:**
   - BEFORE: DesiredState without ChildrenSpecs
   - AFTER: DesiredState with ChildrenSpecs
   - Compatibility: Version migration in store loader (add empty slice for old records)

### 9.2: Worker Migration

#### For Each Worker Type

**1. Simple Worker (No Children)**

**Change Required:** None (already compatible)

**Example:** ConnectionWorker

```go
// BEFORE:
func (w *ConnectionWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    state := "down"
    if w.canConnect(userSpec.Variables.User["IP"].(string)) {
        state = "up"
    }
    return DesiredState{State: state}
}

// AFTER (no change needed):
func (w *ConnectionWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    state := "down"
    if w.canConnect(userSpec.Variables.User["IP"].(string)) {
        state = "up"
    }
    return DesiredState{
        State:         state,
        ChildrenSpecs: []ChildSpec{}, // Empty = no children
    }
}
```

**Effort:** None (implicit compatibility)

**2. Parent Worker (With Children)**

**Change Required:** DeriveDesiredState returns ChildrenSpecs instead of calling DeclareChildren

**Example:** BridgeWorker

```go
// BEFORE (hypothetical - FSMv1 pattern):
func (w *BridgeWorker) DeclareChildren() []Child {
    // Old pattern (if it existed)
    return []Child{
        {Name: "connection-1", Type: "Connection"},
        {Name: "dfc_read", Type: "DataFlowComponent"},
    }
}

func (w *BridgeWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    return DesiredState{State: "active"}
}

// AFTER:
func (w *BridgeWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    // 1. Merge location
    parentLocation := userSpec.Variables.User["location"].(map[string]string)
    bridgeLocation := map[string]string{"bridge": userSpec.Name}
    location := mergeLocations(parentLocation, bridgeLocation)
    locationPath := computeLocationPath(location)

    // 2. Build ChildrenSpecs
    children := []ChildSpec{
        {
            Name:       "connection-1",
            WorkerType: "ConnectionWorker",
            UserSpec: UserSpec{
                Name: "Connection",
                Variables: VariableBundle{
                    User: map[string]any{
                        "parent_state":  "active",
                        "IP":            userSpec.Variables.User["IP"],
                        "PORT":          userSpec.Variables.User["PORT"],
                        "location":      location,
                        "location_path": locationPath,
                    },
                },
            },
            StateMapping: map[string]string{
                "active": "up",
                "idle":   "down",
            },
        },
        {
            Name:       "dfc_read",
            WorkerType: "BenthosWorker",
            UserSpec: UserSpec{
                Name:         "DFC-Read",
                TemplateName: "benthos-modbus-read",
                Variables: VariableBundle{
                    User: map[string]any{
                        "parent_state":  "active",
                        "location_path": locationPath,
                        "name":          userSpec.Name,
                    },
                },
            },
            StateMapping: map[string]string{
                "active": "running",
                "idle":   "stopped",
            },
        },
    }

    return DesiredState{
        State:         "active",
        ChildrenSpecs: children,
    }
}
```

**Effort:** 1-2 hours per worker

**Migration Checklist:**
- [ ] Remove DeclareChildren() method (if exists)
- [ ] Add location merging logic
- [ ] Build ChildrenSpecs with correct WorkerType
- [ ] Pass variables via UserSpec.Variables.User
- [ ] Define StateMapping for each child
- [ ] Update unit tests to verify ChildrenSpecs
- [ ] Update integration tests to verify children created

**3. Complex Worker (Dynamic Children)**

**Change Required:** Template rendering + dynamic ChildrenSpecs generation

**Example:** OPCUABrowserWorker

```go
// AFTER:
func (w *OPCUABrowserWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    // 1. Discover servers (dynamic)
    servers, _ := w.discoverServers(userSpec.Variables.User["gateway_url"].(string))

    // 2. Build ChildSpec for each server
    children := make([]ChildSpec, len(servers))
    for i, server := range servers {
        // Merge location
        parentLocation := userSpec.Variables.User["location"].(map[string]string)
        serverLocation := map[string]string{"server": server.Name}
        location := mergeLocations(parentLocation, serverLocation)
        locationPath := computeLocationPath(location)

        children[i] = ChildSpec{
            Name:       fmt.Sprintf("server-%s", server.ID),
            WorkerType: "OPCUAServerWorker",
            UserSpec: UserSpec{
                Name: server.Name,
                Variables: VariableBundle{
                    User: map[string]any{
                        "parent_state":  "active",
                        "endpoint_url":  server.EndpointURL,
                        "location":      location,
                        "location_path": locationPath,
                    },
                },
            },
            StateMapping: map[string]string{
                "active": "browsing",
                "idle":   "stopped",
            },
        }
    }

    return DesiredState{
        State:         "active",
        ChildrenSpecs: children,
    }
}
```

**Effort:** 3-4 hours per worker

**Migration Checklist:**
- [ ] Implement discovery logic (if not already present)
- [ ] Build ChildrenSpecs dynamically based on discovery
- [ ] Handle N → N-1 → N+2 scenarios (children come and go)
- [ ] Pass dynamic variables to each child
- [ ] Define StateMapping for dynamic children
- [ ] Update unit tests to verify dynamic behavior
- [ ] Update integration tests to verify add/remove

### 9.3: Breaking Changes

This section lists all breaking changes, their impact, and migration steps.

#### 1. Worker Interface Changes

**BEFORE:**
```go
type Worker interface {
    DeriveDesiredState(userSpec UserSpec) DesiredState
    CollectObservedState(ctx context.Context) (ObservedState, error)
}

type DesiredState struct {
    State string
}
```

**AFTER:**
```go
type Worker interface {
    DeriveDesiredState(userSpec UserSpec) DesiredState
    CollectObservedState(ctx context.Context) (ObservedState, error)
}

type DesiredState struct {
    State         string
    ChildrenSpecs []ChildSpec  // NEW FIELD
}
```

**Impact:**
- **Compile-time:** All workers must return DesiredState with ChildrenSpecs field
- **Runtime:** No impact (empty slice = no children)
- **Tests:** Tests must verify ChildrenSpecs (can be empty)

**Migration:**
```go
// For leaf workers (no children):
return DesiredState{
    State:         "active",
    ChildrenSpecs: []ChildSpec{}, // Add empty slice
}

// For parent workers:
return DesiredState{
    State:         "active",
    ChildrenSpecs: buildChildren(...), // Build children
}
```

#### 2. Supervisor Constructor Changes

**BEFORE:**
```go
func NewSupervisor(worker Worker, logger *zap.Logger) *Supervisor {
    return &Supervisor{
        worker: worker,
        logger: logger,
    }
}
```

**AFTER:**
```go
func NewSupervisor(worker Worker, factory WorkerFactory, logger *zap.Logger) *Supervisor {
    return &Supervisor{
        worker:  worker,
        factory: factory,  // NEW PARAMETER
        logger:  logger,
    }
}
```

**Impact:**
- **Compile-time:** All supervisor creation sites must provide factory
- **Runtime:** No impact (factory only used when children exist)
- **Tests:** Tests must create factory (can be mock)

**Migration:**
```go
// Create DefaultWorkerFactory singleton
var globalFactory = NewDefaultWorkerFactory(logger)

// Update all NewSupervisor calls
// BEFORE:
supervisor := NewSupervisor(worker, logger)

// AFTER:
supervisor := NewSupervisor(worker, globalFactory, logger)
```

#### 3. TriangularStore Serialization

**BEFORE:**
```go
type DesiredState struct {
    State string `json:"state"`
}

// Serialized:
{"state": "active"}
```

**AFTER:**
```go
type DesiredState struct {
    State         string      `json:"state"`
    ChildrenSpecs []ChildSpec `json:"children_specs"`
}

// Serialized:
{
    "state": "active",
    "children_specs": [
        {
            "name": "connection-1",
            "worker_type": "ConnectionWorker",
            "user_spec": {...},
            "state_mapping": {"active": "up"}
        }
    ]
}
```

**Impact:**
- **Serialization format:** JSON schema changes
- **Version migration:** Old records need empty array for children_specs
- **Tests:** Serialization tests must verify new format

**Migration:**
```go
// Version migration in TriangularStore loader
func (ts *TriangularStore) LoadDesiredState(ctx context.Context, id string) (DesiredState, error) {
    data, err := ts.db.Get(ctx, id)
    if err != nil {
        return DesiredState{}, err
    }

    var state DesiredState
    if err := json.Unmarshal(data, &state); err != nil {
        return DesiredState{}, err
    }

    // Version migration: Add empty slice if missing
    if state.ChildrenSpecs == nil {
        state.ChildrenSpecs = []ChildSpec{}
    }

    return state, nil
}
```

### 9.4: Effort Estimates

**Total Migration Effort:** 2-3 weeks

**Breakdown by Phase:**

**Phase 0: Add New Structures (2-3 days)**
- Add ChildrenSpecs field to DesiredState: 1 hour
- Create WorkerFactory interface: 2 hours
- Create DefaultWorkerFactory: 3 hours
- Update TriangularStore serialization: 4 hours
- Update Supervisor to support factory: 4 hours
- Documentation updates: 2 hours
- **Total:** 16 hours (~2 days)

**Phase 1: Migrate Example Workers (3-4 days)**
- Migrate BridgeWorker: 2 hours
- Migrate ConnectionWorker: 1 hour
- Update unit tests (both): 3 hours
- Update integration tests: 4 hours
- Documentation (migration guide): 4 hours
- **Total:** 14 hours (~2 days)

**Phase 2: Migrate Remaining Workers (1 week)**
- Migrate 10 workers @ 2 hours each: 20 hours
- Update unit tests: 10 hours
- Update integration tests: 10 hours
- **Total:** 40 hours (~1 week)

**Phase 3: Remove Deprecated Code (2-3 days)**
- Remove old patterns: 4 hours
- Clean up documentation: 2 hours
- Final validation: 4 hours
- Code review: 4 hours
- **Total:** 14 hours (~2 days)

**Contingency:** +20% for unexpected issues

**Total:** 16 + 14 + 40 + 14 = 84 hours (~2 weeks) + 20% contingency = **~3 weeks**

---

## Section 10: Risk Analysis Updates

| Risk | Probability | Impact | Detection | Mitigation | Contingency |
|------|-------------|--------|-----------|------------|-------------|
| **Template rendering too slow** | Medium | High | Benchmark fails >1ms (p95) | Cache rendered templates (invalidate on UserSpec change); Use pre-compiled templates; Limit template complexity via validation | If caching insufficient, limit templating to specific fields; If still slow, move to static config generation |
| **Variable propagation bugs** | High | Medium | Integration test failures; User reports missing variables | Extensive validation in unit tests; Integration tests verify end-to-end flow; Add logging for variable injection | Rollback to simple config passing; Add detailed error messages for debugging |
| **WorkerFactory registration errors** | Low | Medium | Startup checks fail; Worker creation returns error | Validation at factory registration time; Unit tests for all worker types; Startup verification checks | Runtime error handling with clear messages; Fallback to manual worker creation |
| **Circular dependencies (parent ↔ child)** | Low | High | Stack overflow; Deadlock during tick | Static analysis of ChildSpec declarations; Explicit parent-child DAG validation; Unit tests for hierarchy depth | Runtime detection with max depth limit; Error on circular reference; Clear error messages |
| **Memory overhead per supervisor** | Medium | Medium | Benchmarks show >1MB per supervisor | Object pooling for ChildSpec structs; Reuse supervisor instances; Minimize per-supervisor state | Limit hierarchy depth to 3 levels; Alert on memory growth; Document memory limits |
| **DeepEqual performance on UserSpec** | Medium | Low | Profiling shows hot spots | Hash-based comparison instead of DeepEqual; Cache comparison results; Only compare on UserSpec change | Simple field-by-field comparison; Skip comparison if hash unchanged; Accept stale detection |
| **Child supervisor leak on restart** | Medium | High | Memory profiling shows growing supervisor count | Explicit cleanup in reconcileChildren(); Finalizers for supervisors; Integration tests verify cleanup | Add metrics for active supervisor count; Alert on growth; Manual cleanup endpoint |
| **StateMapping conflicts** | Low | Low | Child receives invalid state; Logs show mapping errors | Schema validation at ChildSpec creation; Unit tests for all mappings; Child can override for safety | Child ignores invalid states; Log warnings for unmapped states; Fallback to previous state |
| **Template validation failures** | Medium | Medium | RenderTemplate returns errors; Invalid configs generated | Strict mode template rendering; Validate all templates at startup; Unit tests for all templates | Reject invalid templates at creation; Provide clear error messages; Suggest fixes |
| **Location computation errors** | Low | Medium | Invalid location paths; Metrics/UNS broken | Extensive unit tests for location merging; Validate ISA-95 compliance; Gap filling with "unknown" | Manual location override; Log invalid locations; Alert on "unknown" segments |
| **Supervisor tick loop deadlock** | Low | High | Tick never returns; System hangs | Context with timeout for all ticks; Watchdog for tick duration; Integration tests with timeouts | Force tick abort after timeout; Restart supervisor; Log deadlock stack traces |
| **TriangularStore version migration fails** | Low | High | Old data cannot be loaded; System fails to start | Version migration tests; Backward compatibility validation; Dry-run migration on test data | Rollback to previous version; Manual data migration; Keep old schema alongside new |

### Detailed Risk Analysis

#### Template Rendering Too Slow

**Detection:**
- Benchmark test: `BenchmarkTemplateRendering` fails if p95 > 1ms
- Load test: 400 workers with templates show high latency
- Profiling: Template rendering appears in hot spots

**Mitigation:**
```go
// 1. Cache rendered templates
type Worker struct {
    lastUserSpecHash uint64
    cachedConfig     Config
}

func (w *Worker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    hash := hashUserSpec(userSpec)
    if hash != w.lastUserSpecHash {
        template := w.templateStore.Get(userSpec.TemplateName)
        w.cachedConfig, _ = RenderTemplate(template, userSpec.Variables.Flatten())
        w.lastUserSpecHash = hash
    }
    return DesiredState{Config: w.cachedConfig}
}

// 2. Pre-compile templates
type TemplateStore struct {
    compiled map[string]*template.Template
}

func (ts *TemplateStore) Compile(name string, tmpl string) error {
    t, err := template.New(name).Parse(tmpl)
    if err != nil {
        return err
    }
    ts.compiled[name] = t
    return nil
}

// 3. Validate template complexity
func (ts *TemplateStore) ValidateComplexity(tmpl string) error {
    // Reject templates with too many {{ }} markers
    count := strings.Count(tmpl, "{{")
    if count > 50 {
        return fmt.Errorf("template too complex: %d variables (max 50)", count)
    }
    return nil
}
```

**Contingency:**
- If caching insufficient: Limit templating to leaf workers only (e.g., Benthos configs)
- If still too slow: Move to static config generation at deployment time
- If unacceptable: Disable templating, use static configs

**Metrics:**
```
fsmv2_template_render_duration_seconds{worker_type="BenthosWorker"} histogram
fsmv2_template_cache_hits_total{worker_type="BenthosWorker"} counter
fsmv2_template_cache_misses_total{worker_type="BenthosWorker"} counter
```

**Alert:**
```yaml
- alert: TemplateRenderingSlow
  expr: histogram_quantile(0.95, fsmv2_template_render_duration_seconds) > 0.001
  for: 5m
  annotations:
    summary: "Template rendering p95 >1ms"
```

#### Variable Propagation Bugs

**Detection:**
- Integration test: `VariablePropagationThroughHierarchy` fails
- User report: "{{ .IP }} not substituted in config"
- Logs: "missing key 'IP' in template"

**Mitigation:**
```go
// 1. Extensive validation
func validateVariableBundle(vb VariableBundle) error {
    required := []string{"IP", "PORT", "location_path"}
    for _, key := range required {
        if _, exists := vb.User[key]; !exists {
            return fmt.Errorf("missing required variable: %s", key)
        }
    }
    return nil
}

// 2. Detailed logging
func (s *Supervisor) passVariablesToChild(childSpec ChildSpec) {
    s.logger.Debugw("Passing variables to child",
        "child", childSpec.Name,
        "variables", childSpec.UserSpec.Variables.User,
    )
}

// 3. End-to-end tests
It("propagates variables through 3 levels", func() {
    // Verify grandparent → parent → child
    // Verify all variables present at each level
})
```

**Contingency:**
- Rollback to simple config passing (no templates)
- Add detailed error messages showing which variable is missing
- Provide manual override for variables

**Metrics:**
```
fsmv2_variable_propagation_errors_total{child_name="connection-1"} counter
```

#### WorkerFactory Registration Errors

**Detection:**
- Startup check fails: `factory.CreateWorker("UnknownWorker")` returns error
- Worker creation fails: "unknown worker type: FooWorker"

**Mitigation:**
```go
// 1. Validation at registration
func (f *DefaultWorkerFactory) Register(workerType string, creator WorkerCreator) error {
    if workerType == "" {
        return fmt.Errorf("worker type cannot be empty")
    }
    if _, exists := f.creators[workerType]; exists {
        return fmt.Errorf("worker type already registered: %s", workerType)
    }
    f.creators[workerType] = creator
    return nil
}

// 2. Startup verification
func verifyFactoryRegistration(factory WorkerFactory) error {
    requiredTypes := []string{"BridgeWorker", "ConnectionWorker", "BenthosWorker"}
    for _, workerType := range requiredTypes {
        if _, err := factory.CreateWorker(workerType, UserSpec{}); err != nil {
            return fmt.Errorf("factory missing required type: %s", workerType)
        }
    }
    return nil
}
```

**Contingency:**
- Runtime error handling with clear message: "Worker type 'FooWorker' not registered. Available: [BridgeWorker, ConnectionWorker, ...]"
- Fallback to manual worker creation if factory fails
- Log all registered types at startup

#### Circular Dependencies (Parent ↔ Child)

**Detection:**
- Stack overflow during tick
- Deadlock: parent waits for child, child waits for parent
- Integration test hangs

**Mitigation:**
```go
// 1. Static analysis
func validateNoCircularDependencies(children []ChildSpec) error {
    visited := make(map[string]bool)
    stack := make(map[string]bool)

    var visit func(name string) error
    visit = func(name string) error {
        if stack[name] {
            return fmt.Errorf("circular dependency detected: %s", name)
        }
        if visited[name] {
            return nil
        }

        visited[name] = true
        stack[name] = true

        // Check children of this child
        for _, child := range getChildrenOf(name) {
            if err := visit(child); err != nil {
                return err
            }
        }

        stack[name] = false
        return nil
    }

    for _, child := range children {
        if err := visit(child.Name); err != nil {
            return err
        }
    }

    return nil
}

// 2. Runtime detection
func (s *Supervisor) Tick(ctx context.Context) error {
    if s.tickDepth > maxTickDepth {
        return fmt.Errorf("max tick depth exceeded: possible circular dependency")
    }
    s.tickDepth++
    defer func() { s.tickDepth-- }()

    // ... rest of tick
}
```

**Contingency:**
- Runtime max depth limit (default: 10)
- Error message shows dependency chain
- Manual intervention to break cycle

#### Child Supervisor Leak on Restart

**Detection:**
- Memory profiling shows growing supervisor count
- Metrics: `fsmv2_active_supervisors_total` keeps increasing
- Integration test: Create/restart 1000 children, verify cleanup

**Mitigation:**
```go
// 1. Explicit cleanup
func (s *Supervisor) reconcileChildren(specs []ChildSpec) error {
    desiredChildren := make(map[string]ChildSpec)
    for _, spec := range specs {
        desiredChildren[spec.Name] = spec
    }

    // Remove children not in desired
    for name, child := range s.children {
        if _, desired := desiredChildren[name]; !desired {
            // Explicit cleanup
            child.supervisor.Stop(context.Background())
            delete(s.children, name)
        }
    }

    // Add/update children
    // ...
}

// 2. Finalizer pattern
type Supervisor struct {
    cleanup func()
}

func NewSupervisor(...) *Supervisor {
    s := &Supervisor{...}
    s.cleanup = func() {
        // Cleanup resources
        for _, child := range s.children {
            child.supervisor.cleanup()
        }
    }
    runtime.SetFinalizer(s, func(s *Supervisor) {
        s.cleanup()
    })
    return s
}
```

**Contingency:**
- Add metric: `fsmv2_active_supervisors_total`
- Alert if supervisor count grows beyond expected
- Manual cleanup endpoint: `/debug/supervisors/cleanup`

**Metrics:**
```
fsmv2_active_supervisors_total{worker_type="BridgeWorker"} gauge
fsmv2_supervisor_cleanup_total{worker_type="BridgeWorker"} counter
fsmv2_supervisor_cleanup_failures_total{worker_type="BridgeWorker"} counter
```

---

## Section 11: Acceptance Criteria Updates

### 11.1: Phase-Level Acceptance

#### Phase 0 Complete: Hierarchical Composition Foundation

**Core Functionality:**
- [ ] ChildSpec serializes to/from JSON correctly
- [ ] ChildSpec serializes to/from YAML correctly
- [ ] ChildSpec DeepEqual works with nested UserSpec
- [ ] WorkerFactory registers 5+ worker types (BridgeWorker, ConnectionWorker, BenthosWorker, etc.)
- [ ] WorkerFactory.CreateWorker() returns correct worker for valid types
- [ ] WorkerFactory.CreateWorker() returns error for unknown types
- [ ] DesiredState supports empty ChildrenSpecs (leaf worker)
- [ ] DesiredState supports single child
- [ ] DesiredState supports multiple children
- [ ] StateMapping applies correctly (parent "active" → child "up")
- [ ] StateMapping handles unmapped states gracefully
- [ ] reconcileChildren() adds new child supervisor
- [ ] reconcileChildren() removes deleted child supervisor
- [ ] reconcileChildren() updates existing child supervisor
- [ ] reconcileChildren() handles multiple operations in single tick

**Integration:**
- [ ] BridgeWorker migrated to new API (returns ChildrenSpecs)
- [ ] BridgeWorker creates Connection child correctly
- [ ] Connection child receives variables from Bridge parent
- [ ] StateMapping applied: Bridge "active" → Connection "up"
- [ ] Integration test passes: Bridge → Connection end-to-end

**Code Quality:**
- [ ] All unit tests pass (0 failures)
- [ ] Code coverage >80% for new components
- [ ] golangci-lint passes (0 errors)
- [ ] No focused tests (ginkgo -r --fail-on-focused passes)

#### Phase 0.5 Complete: Templating & Variables

**Core Functionality:**
- [ ] VariableBundle serializes to/from JSON correctly
- [ ] VariableBundle handles empty namespaces (nil vs empty map)
- [ ] Flatten() returns empty map for empty bundle
- [ ] Flatten() promotes User variables to top-level
- [ ] Flatten() nests Global under "global" key
- [ ] Flatten() nests Internal under "internal" key
- [ ] RenderTemplate() substitutes simple variables: `"{{ .IP }}"` → `"192.168.1.100"`
- [ ] RenderTemplate() substitutes multiple variables: `"{{ .IP }}:{{ .PORT }}"` → `"192.168.1.100:502"`
- [ ] RenderTemplate() handles nested access: `"{{ .global.kafka_brokers }}"`
- [ ] RenderTemplate() returns error for missing key in strict mode
- [ ] RenderTemplate() returns error for leftover {{ markers
- [ ] RenderTemplate() works with structs (entire struct templated)
- [ ] Location computation returns all "unknown" for empty location
- [ ] Location computation fills gaps with "unknown"
- [ ] Location computation produces clean path for full ISA-95
- [ ] Location merging extends parent with child levels
- [ ] Variable injection works in supervisor tick (User/Global/Internal)

**Integration:**
- [ ] BenthosWorker renders config from template correctly
- [ ] Template rendering completes in <1ms (p95)
- [ ] Variables flow: Parent constructs ChildSpec → Child receives in DeriveDesiredState
- [ ] Location merges across 3 levels: Grandparent → Parent → Child
- [ ] Integration test passes: Template rendering end-to-end

**Code Quality:**
- [ ] All unit tests pass (0 failures)
- [ ] Code coverage >90% for Flatten() (critical path)
- [ ] Code coverage >95% for RenderTemplate()
- [ ] golangci-lint passes (0 errors)
- [ ] Benchmark: Template rendering <1ms (p95)

#### Phase 1 Complete: Infrastructure Supervision

**Core Functionality:**
- [ ] ExponentialBackoff doubles delay on each failure
- [ ] ExponentialBackoff caps at max delay
- [ ] ExponentialBackoff resets after success
- [ ] InfrastructureHealthChecker tracks attempts with window management
- [ ] InfrastructureHealthChecker.ShouldGiveUp() returns true after max attempts
- [ ] CheckChildConsistency() detects Redpanda/Benthos inconsistency
- [ ] CheckChildConsistency() returns nil for consistent children
- [ ] Circuit breaker opens when CheckChildConsistency() fails
- [ ] Circuit breaker pauses tick for ALL children
- [ ] Child restart uses WorkerFactory to recreate worker
- [ ] Child restart applies exponential backoff
- [ ] Circuit breaker closes when child healthy

**Integration:**
- [ ] Circuit breaker affects all children (none tick during circuit open)
- [ ] Child restart recreates supervisor with fresh worker instance
- [ ] Observations continue during circuit open (collectors keep running)
- [ ] Integration test passes: Circuit breaker → child restart → recovery

**Code Quality:**
- [ ] All unit tests pass (0 failures)
- [ ] Code coverage >80% for Infrastructure components
- [ ] golangci-lint passes (0 errors)
- [ ] Integration test verifies end-to-end recovery

#### Phase 2 Complete: Async Action Executor

**Core Functionality:**
- [ ] ActionExecutor creates worker pool with configurable size
- [ ] ActionExecutor.EnqueueAction() adds action to queue
- [ ] ActionExecutor.HasActionInProgress() returns true for queued action
- [ ] ActionExecutor.HasActionInProgress() returns false after action completes
- [ ] Action timeout triggers after 30s (configurable)
- [ ] Failed actions retry with exponential backoff
- [ ] Supervisor.Tick() skips worker when HasActionInProgress() true
- [ ] Supervisor.Tick() ticks worker when no action in progress

**Integration:**
- [ ] Action executes asynchronously (tick returns immediately)
- [ ] Parent action blocks children (parent not ticked → children not derived)
- [ ] Child action blocks only self (siblings tick normally)
- [ ] Integration test passes: Async action → completion → tick resumes

**Code Quality:**
- [ ] All unit tests pass (0 failures)
- [ ] Code coverage >80% for ActionExecutor
- [ ] golangci-lint passes (0 errors)
- [ ] Integration test verifies non-blocking behavior

#### Phase 3 Complete: Integration & Edge Cases

**Core Functionality:**
- [ ] Infrastructure check runs before action check (layered precedence)
- [ ] Circuit breaker pauses ALL workers (infrastructure scope)
- [ ] Action check pauses SINGLE worker (action scope)
- [ ] Both conditions can be true simultaneously (handled correctly)
- [ ] Actions timeout during child restart (~60s recovery)
- [ ] Observations continue during circuit open (fresh data when recovered)

**Edge Cases:**
- [ ] Dynamic children work: N → N-1 → N+2 (add/remove correctly)
- [ ] Multi-level hierarchy (3+ levels) ticks correctly
- [ ] StateMapping conflicts handled gracefully (child can override)
- [ ] Template rendering failures produce clear errors
- [ ] Variable propagation failures produce clear errors
- [ ] Circular dependencies detected with clear error message
- [ ] Child supervisor cleanup verified (no leaks)

**Integration:**
- [ ] ProtocolConverter use case works end-to-end (Bridge → Connection + 2 DFCs)
- [ ] OPC UA discovery works (dynamic children N → N-1 → N+2)
- [ ] Multi-customer setup works (3-level location hierarchy)
- [ ] Integration test passes: Infrastructure + Actions combined

**Code Quality:**
- [ ] All unit tests pass (0 failures)
- [ ] All integration tests pass (0 failures)
- [ ] All edge case tests pass (0 failures)
- [ ] golangci-lint passes (0 errors)

#### Phase 4 Complete: Monitoring & Observability

**Metrics:**
- [ ] Infrastructure recovery metrics implemented (circuit breaker state, restart count)
- [ ] Action execution metrics implemented (queue depth, duration, failures)
- [ ] Hierarchical composition metrics implemented (children count, reconciliation duration)
- [ ] Template rendering metrics implemented (duration, cache hits/misses)

**Dashboards:**
- [ ] Grafana dashboard shows circuit breaker state
- [ ] Grafana dashboard shows child restart history
- [ ] Grafana dashboard shows action queue depth
- [ ] Grafana dashboard shows parent-child relationships (tree view)
- [ ] Grafana dashboard shows template rendering performance

**Alerting:**
- [ ] Alert: Circuit breaker open for >5 minutes
- [ ] Alert: Child restart failures exceed threshold
- [ ] Alert: Action queue depth exceeds threshold
- [ ] Alert: Template rendering p95 >1ms
- [ ] Alert: Supervisor memory growth (leak detection)

**Documentation:**
- [ ] Runbook: Manual intervention for stuck circuit breaker
- [ ] Runbook: Debugging child restart failures
- [ ] Runbook: Handling action queue overflow
- [ ] User guide: Creating hierarchical workers
- [ ] Developer guide: Migrating workers to new API

### 11.2: Overall System Acceptance

#### Functionality

**Hierarchical Composition:**
- [ ] ProtocolConverter use case works end-to-end
- [ ] 3-level hierarchy validated (grandparent → parent → child)
- [ ] Dynamic children work (OPC UA browser with N servers)
- [ ] Templates render correctly in all scenarios
- [ ] Variables propagate through hierarchy
- [ ] StateMapping controls child states

**Infrastructure Supervision:**
- [ ] Circuit breaker detects child inconsistencies
- [ ] Circuit breaker pauses tick for all children
- [ ] Child restart uses exponential backoff
- [ ] Child restart recreates from WorkerFactory
- [ ] Observations continue during circuit open

**Async Actions:**
- [ ] Actions execute asynchronously (non-blocking tick)
- [ ] Parent action blocks children
- [ ] Child action blocks only self
- [ ] Actions timeout and retry during child restart

**Integration:**
- [ ] Infrastructure check runs before action check
- [ ] Both conditions can be true simultaneously
- [ ] Edge cases handled gracefully
- [ ] Clear error messages for all failure modes

#### Performance

**Benchmarks:**
- [ ] Template rendering <1ms per worker (p95)
- [ ] Variable flattening <100μs
- [ ] reconcileChildren() <10ms for 100 children
- [ ] Tick loop overhead <10% vs FSMv1 baseline

**Load Testing:**
- [ ] 100 bridges with 3 children each = 400 workers
- [ ] Tick latency p50 <50ms
- [ ] Tick latency p95 <100ms
- [ ] Tick latency p99 <200ms
- [ ] Memory per supervisor <1MB
- [ ] GC pressure <10% increase vs FSMv1

**No Performance Regression:**
- [ ] FSMv1 benchmarks still pass
- [ ] Existing workers perform equivalently
- [ ] Overall system latency unchanged

#### Quality

**Code Coverage:**
- [ ] New components >80% line coverage
- [ ] reconcileChildren() 100% coverage (critical path)
- [ ] Flatten() 100% coverage (critical path)
- [ ] WorkerFactory 100% coverage (critical path)
- [ ] No coverage regression on existing components

**Testing:**
- [ ] All unit tests pass (0 failures)
- [ ] All integration tests pass (0 failures)
- [ ] All system tests pass (0 failures)
- [ ] All edge case tests pass (0 failures)
- [ ] Focused tests removed (CI check passes)

**Code Quality:**
- [ ] golangci-lint passes (0 errors)
- [ ] go vet passes (0 warnings)
- [ ] nilaway passes (no nil panics detected)
- [ ] No TODO comments in production code
- [ ] All design documents implemented as specified

**Documentation:**
- [ ] User guides complete (creating hierarchical workers)
- [ ] Developer guides complete (migrating workers)
- [ ] API documentation complete (all public methods)
- [ ] Runbooks complete (operational procedures)
- [ ] Architecture diagrams complete (parent-child relationships)

#### Operational

**Metrics:**
- [ ] Prometheus metrics exported for all components
- [ ] Metrics scraped by Prometheus successfully
- [ ] Grafana dashboards rendering correctly
- [ ] Metrics retention configured (30 days)

**Alerting:**
- [ ] All alerts configured in Alertmanager
- [ ] Alert notifications tested (Slack/email)
- [ ] Alert thresholds validated with load tests
- [ ] Runbooks linked from alert annotations

**Monitoring:**
- [ ] Circuit breaker state visible in dashboard
- [ ] Child restart history visible in dashboard
- [ ] Action queue depth visible in dashboard
- [ ] Parent-child relationships visible in dashboard
- [ ] Template rendering performance visible in dashboard

**Operational Readiness:**
- [ ] Runbook tested for common issues (circuit stuck, child restart fails)
- [ ] Manual intervention procedures validated
- [ ] Debugging guides validated with real scenarios
- [ ] On-call engineers trained on new system

### 11.3: Quality Gates

#### Before Phase 0 → Phase 0.5

- [ ] All Phase 0 unit tests pass
- [ ] Code coverage >80% for hierarchical composition
- [ ] Code review approved (2+ reviewers)
- [ ] BridgeWorker migration validated with integration test
- [ ] golangci-lint passes (0 errors)
- [ ] No focused tests

#### Before Phase 0.5 → Phase 1

- [ ] All Phase 0.5 unit tests pass
- [ ] Code coverage >90% for Flatten() and RenderTemplate()
- [ ] Performance benchmarks meet targets (<1ms template rendering)
- [ ] Integration tests with templates pass
- [ ] Code review approved (2+ reviewers)
- [ ] golangci-lint passes (0 errors)

#### Before Phase 1 → Phase 2

- [ ] All Phase 1 unit tests pass
- [ ] Infrastructure Supervision validated with integration tests
- [ ] Child restart tested extensively (no memory leaks)
- [ ] Circuit breaker behavior verified
- [ ] Code review approved (2+ reviewers)
- [ ] golangci-lint passes (0 errors)

#### Before Phase 2 → Phase 3

- [ ] All Phase 2 unit tests pass
- [ ] Async actions integrate cleanly with infrastructure layer
- [ ] No blocking issues found in integration tests
- [ ] Action timeout behavior validated
- [ ] Code review approved (2+ reviewers)
- [ ] golangci-lint passes (0 errors)

#### Before Phase 3 → Phase 4

- [ ] All Phase 3 unit tests pass
- [ ] All edge cases tested (10+ scenarios)
- [ ] Performance targets met (tick latency, template rendering)
- [ ] No critical bugs in integration tests
- [ ] Code review approved (2+ reviewers)
- [ ] golangci-lint passes (0 errors)

#### Before Final Release

- [ ] All acceptance criteria met (functionality, performance, quality, operational)
- [ ] Production readiness review passed
- [ ] Documentation review complete (user guides, developer guides, runbooks)
- [ ] Migration tested on real workload (sandbox environment)
- [ ] Performance validated at scale (400+ workers)
- [ ] Security review passed (no vulnerabilities introduced)
- [ ] Stakeholder approval obtained

**Production Readiness Checklist:**
- [ ] All tests pass (unit, integration, system, edge cases)
- [ ] Performance benchmarks meet targets
- [ ] No known critical bugs
- [ ] Documentation complete and reviewed
- [ ] Monitoring and alerting configured
- [ ] Runbooks tested with real scenarios
- [ ] On-call engineers trained
- [ ] Rollback plan documented and tested
- [ ] Migration guide validated
- [ ] Stakeholders informed of release

---

## Summary

This change proposal completes the FSMv2 master plan with comprehensive testing strategy, migration strategy, risk analysis, and acceptance criteria. The additions cover:

**Section 8: Testing Strategy**
- Detailed unit test coverage for hierarchical composition and templating
- Integration test scenarios for parent-child relationships and variable propagation
- System test use cases (ProtocolConverter, OPC UA discovery, multi-customer)
- Performance benchmarks and load testing targets

**Section 9: Migration Strategy**
- Backward compatibility approach with deprecation path
- Worker migration templates for simple, parent, and complex workers
- Breaking changes documentation with migration steps
- Effort estimates: 2-3 weeks total

**Section 10: Risk Analysis**
- 12 identified risks with probability, impact, detection, mitigation, and contingency
- Detailed analysis of top 6 risks (template rendering, variable propagation, memory leaks)
- Metrics and alerts for risk detection
- Concrete mitigation code examples

**Section 11: Acceptance Criteria**
- Phase-level acceptance criteria for all 5 phases
- Overall system acceptance (functionality, performance, quality, operational)
- Quality gates between phases with clear thresholds
- Production readiness checklist

**Key Highlights:**

- **Testing:** >80% coverage for new components, 100% for critical paths
- **Migration:** Backward compatible approach, 3-week effort estimate
- **Risks:** Template rendering (Medium/High) mitigated with caching
- **Acceptance:** 150+ criteria across functionality, performance, quality, operational

This proposal provides a complete roadmap for implementing FSMv2 hierarchical composition with infrastructure supervision and async actions.
