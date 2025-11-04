# Phase 0: Hierarchical Composition Foundation

**Timeline:** Weeks 1-2
**Effort:** ~410 lines of code
**Dependencies:** None (foundational)
**Blocks:** ALL subsequent phases

---

## Overview

Phase 0 establishes the foundational hierarchical composition architecture for FSMv2. This phase implements the core patterns that allow parent FSMs to manage child FSMs declaratively.

**Why This Phase First:**
- Foundation for all hierarchical patterns (ProtocolConverter → Connection + DataFlows)
- No other phase can work without parent-child relationships
- WorkerFactory enables dynamic worker creation from string types
- reconcileChildren() is the core reconciliation loop

**Key Pattern (Kubernetes-inspired):**
```go
// Parent declares children in DeriveDesiredState()
func (w *ProtocolConverterWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    return DesiredState{
        State: "running",
        ChildrenSpecs: []ChildSpec{
            {Name: "connection", WorkerType: "mqtt_connection", ...},
            {Name: "source_flow", WorkerType: "benthos_dataflow", ...},
            {Name: "sink_flow", WorkerType: "benthos_dataflow", ...},
        },
    }
}

// Supervisor reconciles (add/update/remove children)
func (s *Supervisor) reconcileChildren(specs []ChildSpec) {
    // Children not in s.children → create new supervisor
    // Children in both → update UserSpec
    // Children only in s.children → remove supervisor
}
```

---

## Deliverables

### 1. Data Structures (pkg/fsmv2/types/)

**ChildSpec** - Serializable child specification:
```go
type ChildSpec struct {
    Name         string            // Child instance ID
    WorkerType   string            // Worker factory type
    UserSpec     UserSpec          // Child configuration
    StateMapping map[string]string // Parent state → child state
}
```

**DesiredState** - Return type for DeriveDesiredState():
```go
type DesiredState struct {
    State         string      // Operational state
    ChildrenSpecs []ChildSpec // Children to manage
}
```

**Estimated:** 50 lines (structures + documentation)

**Tests:**
- Serialization/deserialization (YAML, JSON)
- StateMapping validation
- ChildSpec immutability checks

---

### 2. WorkerFactory Pattern (pkg/fsmv2/factory/)

**Interface:**
```go
type WorkerFactory interface {
    // NewWorker creates worker from string type
    NewWorker(workerType string) (Worker, error)

    // Register adds worker type to registry
    Register(workerType string, constructor func() Worker)
}
```

**Registry Implementation:**
```go
var globalRegistry = make(map[string]func() Worker)

func Register(workerType string, constructor func() Worker) {
    globalRegistry[workerType] = constructor
}

func NewWorker(workerType string) (Worker, error) {
    constructor, exists := globalRegistry[workerType]
    if !exists {
        return nil, fmt.Errorf("unknown worker type: %s", workerType)
    }
    return constructor(), nil
}
```

**Estimated:** 30 lines (interface + registry)

**Tests:**
- Register and create worker
- Unknown worker type returns error
- Multiple registrations (last wins or error)

---

### 3. Worker.DeriveDesiredState() Signature Update

**BEFORE:**
```go
type Worker interface {
    DeriveDesiredState(userSpec UserSpec) string // Returns state only
}
```

**AFTER:**
```go
type Worker interface {
    DeriveDesiredState(userSpec UserSpec) DesiredState // Returns state + children
}
```

**Migration:** All existing workers return `DesiredState{State: "...", ChildrenSpecs: nil}`

**Estimated:** 5 lines (signature change) + updates to existing workers

**Tests:**
- Workers without children return empty ChildrenSpecs
- Workers with children return populated ChildrenSpecs

---

### 4. Supervisor.reconcileChildren() Implementation

**Core reconciliation logic:**
```go
func (s *Supervisor) reconcileChildren(specs []ChildSpec) {
    // 1. Add new children
    for _, spec := range specs {
        if _, exists := s.children[spec.Name]; !exists {
            childSupervisor := s.createChildSupervisor(spec)
            s.children[spec.Name] = childSupervisor
        }
    }

    // 2. Update existing children
    for _, spec := range specs {
        if child, exists := s.children[spec.Name]; exists {
            child.UpdateUserSpec(spec.UserSpec)
            child.stateMapping = spec.StateMapping
        }
    }

    // 3. Remove children not in specs
    specNames := make(map[string]bool)
    for _, spec := range specs {
        specNames[spec.Name] = true
    }

    for name := range s.children {
        if !specNames[name] {
            s.children[name].Shutdown()
            delete(s.children, name)
        }
    }
}

func (s *Supervisor) createChildSupervisor(spec ChildSpec) *Supervisor {
    // Use WorkerFactory to create child worker
    childWorker, err := factory.NewWorker(spec.WorkerType)
    if err != nil {
        // Handle error (return nil, log, etc.)
    }

    // Create child supervisor with isolated TriangularStore
    childSupervisor := NewSupervisor(childWorker, s.store.ChildStore(spec.Name))
    childSupervisor.stateMapping = spec.StateMapping
    childSupervisor.SetUserSpec(spec.UserSpec)

    return childSupervisor
}
```

**Estimated:** 80 lines (reconciliation + child creation)

**Tests:**
- Add new child (not in s.children)
- Update existing child (UserSpec changes)
- Remove child (not in specs)
- Multiple reconcile calls (idempotent)
- Circular dependency detection (parent cannot be child's child)

---

### 5. StateMapping Application

**In Supervisor.Tick():**
```go
// After reconcileChildren(), apply state mapping
for _, child := range s.children {
    if mappedState, ok := child.stateMapping[desiredState.State]; ok {
        child.SetDesiredState(DesiredState{State: mappedState})
    }
}
```

**Example:**
```go
// Parent: ProtocolConverter in "idle" state
// Child: Connection with StateMapping: {"idle": "stopped", "active": "running"}
// → Child gets DesiredState{State: "stopped"}
```

**Estimated:** 15 lines (tick loop integration)

**Tests:**
- StateMapping applied correctly
- Missing mapping preserves child's current state
- Multiple children with different mappings

---

### 6. Recursive Tick Propagation

**In Supervisor.Tick():**
```go
// After applying state mapping, tick children
for _, child := range s.children {
    child.Tick(ctx)
}
```

**Estimated:** 10 lines (tick loop integration)

**Tests:**
- Children tick after parent
- Children tick only if parent tick succeeds
- Deep hierarchy (3+ levels) ticks correctly

---

### 7. Integration Tests

**Scenarios:**
1. **Parent with no children** - ChildrenSpecs: [] works
2. **Parent adds child** - reconcileChildren() creates supervisor
3. **Parent removes child** - reconcileChildren() shuts down supervisor
4. **Parent updates child config** - reconcileChildren() updates UserSpec
5. **StateMapping propagation** - Parent state changes → child state changes
6. **Recursive tick** - Parent tick → child tick → grandchild tick
7. **Circular dependency detection** - Parent cannot be child of descendant

**Estimated:** 220 lines (7 scenarios × ~30 lines each)

---

## Task Breakdown

### Task 0.1: ChildSpec and DesiredState Structures (2 hours)

**File**: `pkg/fsmv2/types/childspec.go`
**Estimated Time**: 2 hours

#### RED: Write Failing Test

```go
// pkg/fsmv2/types/childspec_test.go
package types_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "gopkg.in/yaml.v3"

    "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/types"
)

var _ = Describe("ChildSpec", func() {
    Describe("Serialization", func() {
        It("should serialize to YAML correctly", func() {
            spec := types.ChildSpec{
                Name:       "connection",
                WorkerType: "mqtt_connection",
                UserSpec: types.UserSpec{
                    Config: "mqtt:\n  url: tcp://localhost:1883",
                },
                StateMapping: map[string]string{
                    "idle":   "stopped",
                    "active": "running",
                },
            }

            data, err := yaml.Marshal(spec)
            Expect(err).ToNot(HaveOccurred())
            Expect(string(data)).To(ContainSubstring("name: connection"))
            Expect(string(data)).To(ContainSubstring("workerType: mqtt_connection"))
            Expect(string(data)).To(ContainSubstring("stateMapping"))
        })

        It("should deserialize from YAML correctly", func() {
            yamlData := `
name: connection
workerType: mqtt_connection
userSpec:
  config: "mqtt:\n  url: tcp://localhost:1883"
stateMapping:
  idle: stopped
  active: running
`
            var spec types.ChildSpec
            err := yaml.Unmarshal([]byte(yamlData), &spec)

            Expect(err).ToNot(HaveOccurred())
            Expect(spec.Name).To(Equal("connection"))
            Expect(spec.WorkerType).To(Equal("mqtt_connection"))
            Expect(spec.StateMapping["idle"]).To(Equal("stopped"))
        })

        It("should handle empty StateMapping", func() {
            spec := types.ChildSpec{
                Name:       "child",
                WorkerType: "simple_worker",
                UserSpec:   types.UserSpec{},
            }

            data, err := yaml.Marshal(spec)
            Expect(err).ToNot(HaveOccurred())

            var deserialized types.ChildSpec
            err = yaml.Unmarshal(data, &deserialized)
            Expect(err).ToNot(HaveOccurred())
            Expect(deserialized.StateMapping).To(BeNil())
        })
    })

    Describe("DesiredState", func() {
        It("should include both State and ChildrenSpecs", func() {
            desired := types.DesiredState{
                State: "running",
                ChildrenSpecs: []types.ChildSpec{
                    {Name: "child1", WorkerType: "type1"},
                    {Name: "child2", WorkerType: "type2"},
                },
            }

            Expect(desired.State).To(Equal("running"))
            Expect(desired.ChildrenSpecs).To(HaveLen(2))
        })

        It("should handle empty ChildrenSpecs", func() {
            desired := types.DesiredState{
                State:         "stopped",
                ChildrenSpecs: nil,
            }

            data, err := yaml.Marshal(desired)
            Expect(err).ToNot(HaveOccurred())
            Expect(string(data)).To(ContainSubstring("state: stopped"))
        })
    })
})
```

**Step 2: Run test to verify it fails**

```bash
cd /Users/jeremytheocharis/umh-git/umh-core-eng-3806/umh-core
ginkgo -v pkg/fsmv2/types
```

Expected: FAIL with "cannot find package types"

#### GREEN: Minimal Implementation

```go
// pkg/fsmv2/types/childspec.go
package types

type ChildSpec struct {
    Name         string            `yaml:"name" json:"name"`
    WorkerType   string            `yaml:"workerType" json:"workerType"`
    UserSpec     UserSpec          `yaml:"userSpec" json:"userSpec"`
    StateMapping map[string]string `yaml:"stateMapping,omitempty" json:"stateMapping,omitempty"`
}

type DesiredState struct {
    State         string      `yaml:"state" json:"state"`
    ChildrenSpecs []ChildSpec `yaml:"childrenSpecs,omitempty" json:"childrenSpecs,omitempty"`
}

type UserSpec struct {
    Config string `yaml:"config" json:"config"`
}
```

**Step 3: Run test to verify it passes**

```bash
ginkgo -v pkg/fsmv2/types
```

Expected: PASS (all tests green)

#### REFACTOR: Add Validation

```go
// pkg/fsmv2/types/childspec.go
import "fmt"

func (c *ChildSpec) Validate() error {
    if c.Name == "" {
        return fmt.Errorf("ChildSpec.Name cannot be empty")
    }
    if c.WorkerType == "" {
        return fmt.Errorf("ChildSpec.WorkerType cannot be empty")
    }
    return nil
}

func (d *DesiredState) Validate() error {
    if d.State == "" {
        return fmt.Errorf("DesiredState.State cannot be empty")
    }
    for i, child := range d.ChildrenSpecs {
        if err := child.Validate(); err != nil {
            return fmt.Errorf("ChildSpec[%d]: %w", i, err)
        }
    }
    return nil
}
```

**Add validation test**:
```go
// Add to childspec_test.go
Describe("Validation", func() {
    It("should reject empty Name", func() {
        invalidSpec := types.ChildSpec{Name: "", WorkerType: "type"}
        err := invalidSpec.Validate()
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("Name cannot be empty"))
    })

    It("should reject empty WorkerType", func() {
        invalidSpec := types.ChildSpec{Name: "child", WorkerType: ""}
        err := invalidSpec.Validate()
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("WorkerType cannot be empty"))
    })

    It("should accept valid ChildSpec", func() {
        validSpec := types.ChildSpec{Name: "child", WorkerType: "type"}
        err := validSpec.Validate()
        Expect(err).ToNot(HaveOccurred())
    })

    It("should validate nested children in DesiredState", func() {
        invalidDesired := types.DesiredState{
            State: "running",
            ChildrenSpecs: []types.ChildSpec{
                {Name: "", WorkerType: "type"},
            },
        }
        err := invalidDesired.Validate()
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("ChildSpec[0]"))
    })
})
```

**Step 4: Run test to verify refactoring passes**

```bash
ginkgo -v pkg/fsmv2/types
```

Expected: PASS (all tests green including validation)

**Step 5: Commit**

```bash
git add pkg/fsmv2/types/childspec.go pkg/fsmv2/types/childspec_test.go
git commit -m "feat(fsmv2): add ChildSpec and DesiredState structures with validation

Implements hierarchical composition data structures:
- ChildSpec with serialization (YAML/JSON)
- DesiredState with state + children
- Validation for required fields
- StateMapping for parent-child state control

Part of Phase 0: Hierarchical Composition Foundation

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 0.2: WorkerFactory Pattern (2 hours)

**File**: `pkg/fsmv2/factory/worker_factory.go`
**Estimated Time**: 2 hours

#### RED: Write Failing Test

```go
// pkg/fsmv2/factory/worker_factory_test.go
package factory_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
    "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/types"
)

type mockWorker struct {
    id string
}

func (m *mockWorker) DeriveDesiredState(userSpec types.UserSpec) types.DesiredState {
    return types.DesiredState{State: "running"}
}

var _ = Describe("WorkerFactory", func() {
    BeforeEach(func() {
        factory.ClearRegistry()
    })

    Describe("Register", func() {
        It("should register a worker constructor", func() {
            factory.Register("mock_worker", func() factory.Worker {
                return &mockWorker{id: "test"}
            })

            worker, err := factory.NewWorker("mock_worker")
            Expect(err).ToNot(HaveOccurred())
            Expect(worker).ToNot(BeNil())
        })

        It("should allow multiple different worker types", func() {
            factory.Register("worker_a", func() factory.Worker {
                return &mockWorker{id: "a"}
            })
            factory.Register("worker_b", func() factory.Worker {
                return &mockWorker{id: "b"}
            })

            workerA, err := factory.NewWorker("worker_a")
            Expect(err).ToNot(HaveOccurred())
            Expect(workerA).ToNot(BeNil())

            workerB, err := factory.NewWorker("worker_b")
            Expect(err).ToNot(HaveOccurred())
            Expect(workerB).ToNot(BeNil())
        })
    })

    Describe("NewWorker", func() {
        It("should return error for unknown worker type", func() {
            worker, err := factory.NewWorker("unknown_type")
            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("unknown worker type"))
            Expect(worker).To(BeNil())
        })

        It("should create new worker instance each time", func() {
            factory.Register("mock_worker", func() factory.Worker {
                return &mockWorker{}
            })

            worker1, err := factory.NewWorker("mock_worker")
            Expect(err).ToNot(HaveOccurred())

            worker2, err := factory.NewWorker("mock_worker")
            Expect(err).ToNot(HaveOccurred())

            // Different instances
            Expect(worker1).ToNot(BeIdenticalTo(worker2))
        })
    })

    Describe("ClearRegistry", func() {
        It("should remove all registered workers", func() {
            factory.Register("worker_a", func() factory.Worker {
                return &mockWorker{}
            })
            factory.Register("worker_b", func() factory.Worker {
                return &mockWorker{}
            })

            factory.ClearRegistry()

            _, err := factory.NewWorker("worker_a")
            Expect(err).To(HaveOccurred())
            _, err = factory.NewWorker("worker_b")
            Expect(err).To(HaveOccurred())
        })
    })
})
```

**Step 2: Run test to verify it fails**

```bash
ginkgo -v pkg/fsmv2/factory
```

Expected: FAIL with "undefined: factory.Register"

#### GREEN: Minimal Implementation

```go
// pkg/fsmv2/factory/worker_factory.go
package factory

import (
    "fmt"

    "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/types"
)

type Worker interface {
    DeriveDesiredState(userSpec types.UserSpec) types.DesiredState
}

type WorkerConstructor func() Worker

var globalRegistry = make(map[string]WorkerConstructor)

func Register(workerType string, constructor WorkerConstructor) {
    globalRegistry[workerType] = constructor
}

func NewWorker(workerType string) (Worker, error) {
    constructor, exists := globalRegistry[workerType]
    if !exists {
        return nil, fmt.Errorf("unknown worker type: %s", workerType)
    }
    return constructor(), nil
}

func ClearRegistry() {
    globalRegistry = make(map[string]WorkerConstructor)
}
```

**Step 3: Run test to verify it passes**

```bash
ginkgo -v pkg/fsmv2/factory
```

Expected: PASS (all tests green)

#### REFACTOR: Add Thread Safety and Validation

```go
// pkg/fsmv2/factory/worker_factory.go
import (
    "sync"
)

var (
    globalRegistry = make(map[string]WorkerConstructor)
    registryMu     sync.RWMutex
)

func Register(workerType string, constructor WorkerConstructor) {
    registryMu.Lock()
    defer registryMu.Unlock()

    if workerType == "" {
        panic("worker type cannot be empty")
    }
    if constructor == nil {
        panic("worker constructor cannot be nil")
    }

    globalRegistry[workerType] = constructor
}

func NewWorker(workerType string) (Worker, error) {
    registryMu.RLock()
    defer registryMu.RUnlock()

    constructor, exists := globalRegistry[workerType]
    if !exists {
        return nil, fmt.Errorf("unknown worker type: %s", workerType)
    }
    return constructor(), nil
}

func ClearRegistry() {
    registryMu.Lock()
    defer registryMu.Unlock()
    globalRegistry = make(map[string]WorkerConstructor)
}

func ListRegisteredTypes() []string {
    registryMu.RLock()
    defer registryMu.RUnlock()

    types := make([]string, 0, len(globalRegistry))
    for workerType := range globalRegistry {
        types = append(types, workerType)
    }
    return types
}
```

**Add refactor tests**:
```go
// Add to worker_factory_test.go
Describe("Thread Safety", func() {
    It("should handle concurrent registrations", func() {
        done := make(chan bool)

        for i := 0; i < 10; i++ {
            go func(id int) {
                factory.Register(fmt.Sprintf("worker_%d", id), func() factory.Worker {
                    return &mockWorker{}
                })
                done <- true
            }(i)
        }

        for i := 0; i < 10; i++ {
            <-done
        }

        types := factory.ListRegisteredTypes()
        Expect(types).To(HaveLen(10))
    })
})

Describe("ListRegisteredTypes", func() {
    It("should return all registered types", func() {
        factory.Register("worker_a", func() factory.Worker { return &mockWorker{} })
        factory.Register("worker_b", func() factory.Worker { return &mockWorker{} })

        types := factory.ListRegisteredTypes()
        Expect(types).To(ContainElement("worker_a"))
        Expect(types).To(ContainElement("worker_b"))
        Expect(types).To(HaveLen(2))
    })
})
```

**Step 4: Run test to verify refactoring passes**

```bash
ginkgo -v pkg/fsmv2/factory
```

Expected: PASS (all tests green)

**Step 5: Commit**

```bash
git add pkg/fsmv2/factory/worker_factory.go pkg/fsmv2/factory/worker_factory_test.go
git commit -m "feat(fsmv2): add WorkerFactory pattern for dynamic worker creation

Implements worker registry with:
- Global registry for worker types
- NewWorker() creates instances from string types
- Thread-safe registration and lookup
- ListRegisteredTypes() for introspection

Part of Phase 0: Hierarchical Composition Foundation

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 0.3: DeriveDesiredState() Signature Update (2 hours)

**Files**:
- Modify: `pkg/fsmv2/factory/worker_factory.go` (Worker interface)
- Modify: All existing worker implementations
**Estimated Time**: 2 hours

#### RED: Update Tests to Expect DesiredState Return Type

```go
// pkg/fsmv2/worker/simple_worker_test.go (example)
package worker_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/types"
    "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/worker"
)

var _ = Describe("SimpleWorker", func() {
    var w *worker.SimpleWorker

    BeforeEach(func() {
        w = &worker.SimpleWorker{}
    })

    Describe("DeriveDesiredState", func() {
        It("should return DesiredState with state and empty children", func() {
            userSpec := types.UserSpec{
                Config: "state: active",
            }

            desired := w.DeriveDesiredState(userSpec)

            Expect(desired.State).To(Equal("active"))
            Expect(desired.ChildrenSpecs).To(BeNil())
        })

        It("should default to stopped when config is empty", func() {
            userSpec := types.UserSpec{}

            desired := w.DeriveDesiredState(userSpec)

            Expect(desired.State).To(Equal("stopped"))
            Expect(desired.ChildrenSpecs).To(BeNil())
        })
    })
})

// pkg/fsmv2/worker/parent_worker_test.go (example with children)
var _ = Describe("ParentWorker", func() {
    var w *worker.ParentWorker

    BeforeEach(func() {
        w = &worker.ParentWorker{}
    })

    Describe("DeriveDesiredState", func() {
        It("should return DesiredState with children", func() {
            userSpec := types.UserSpec{
                Config: "children:\n  - name: child1\n    type: simple_worker",
            }

            desired := w.DeriveDesiredState(userSpec)

            Expect(desired.State).To(Equal("running"))
            Expect(desired.ChildrenSpecs).To(HaveLen(1))
            Expect(desired.ChildrenSpecs[0].Name).To(Equal("child1"))
            Expect(desired.ChildrenSpecs[0].WorkerType).To(Equal("simple_worker"))
        })
    })
})
```

**Step 2: Run test to verify it fails**

```bash
ginkgo -v pkg/fsmv2/worker
```

Expected: FAIL with "cannot use string as DesiredState"

#### GREEN: Update Worker Interface and Implementations

```go
// pkg/fsmv2/factory/worker_factory.go (already updated in Task 0.2)
type Worker interface {
    DeriveDesiredState(userSpec types.UserSpec) types.DesiredState
}

// pkg/fsmv2/worker/simple_worker.go (example)
package worker

import (
    "gopkg.in/yaml.v3"

    "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/types"
)

type SimpleWorker struct{}

func (w *SimpleWorker) DeriveDesiredState(userSpec types.UserSpec) types.DesiredState {
    var config struct {
        State string `yaml:"state"`
    }

    if err := yaml.Unmarshal([]byte(userSpec.Config), &config); err != nil {
        return types.DesiredState{State: "stopped"}
    }

    if config.State == "" {
        return types.DesiredState{State: "stopped"}
    }

    return types.DesiredState{
        State:         config.State,
        ChildrenSpecs: nil,
    }
}

// pkg/fsmv2/worker/parent_worker.go (example with children)
type ParentWorker struct{}

type ParentConfig struct {
    State    string `yaml:"state"`
    Children []struct {
        Name string `yaml:"name"`
        Type string `yaml:"type"`
    } `yaml:"children"`
}

func (w *ParentWorker) DeriveDesiredState(userSpec types.UserSpec) types.DesiredState {
    var config ParentConfig

    if err := yaml.Unmarshal([]byte(userSpec.Config), &config); err != nil {
        return types.DesiredState{State: "stopped"}
    }

    children := make([]types.ChildSpec, 0, len(config.Children))
    for _, child := range config.Children {
        children = append(children, types.ChildSpec{
            Name:       child.Name,
            WorkerType: child.Type,
        })
    }

    return types.DesiredState{
        State:         "running",
        ChildrenSpecs: children,
    }
}
```

**Step 3: Run test to verify it passes**

```bash
ginkgo -v pkg/fsmv2/worker
```

Expected: PASS (all tests green)

#### REFACTOR: Add Migration Helper and Validation

```go
// pkg/fsmv2/types/desired_state.go
func NewDesiredState(state string) DesiredState {
    return DesiredState{
        State:         state,
        ChildrenSpecs: nil,
    }
}

func NewDesiredStateWithChildren(state string, children []ChildSpec) DesiredState {
    return DesiredState{
        State:         state,
        ChildrenSpecs: children,
    }
}

// pkg/fsmv2/worker/simple_worker.go (refactored)
func (w *SimpleWorker) DeriveDesiredState(userSpec types.UserSpec) types.DesiredState {
    var config struct {
        State string `yaml:"state"`
    }

    if err := yaml.Unmarshal([]byte(userSpec.Config), &config); err != nil {
        return types.NewDesiredState("stopped")
    }

    if config.State == "" {
        return types.NewDesiredState("stopped")
    }

    return types.NewDesiredState(config.State)
}
```

**Add documentation**:
```go
// pkg/fsmv2/types/desired_state.go
// DesiredState represents the target state and child specifications
// returned by Worker.DeriveDesiredState().
//
// Workers without children should return DesiredState{State: "...", ChildrenSpecs: nil}
// Workers with children should populate ChildrenSpecs with child declarations
type DesiredState struct {
    State         string      `yaml:"state" json:"state"`
    ChildrenSpecs []ChildSpec `yaml:"childrenSpecs,omitempty" json:"childrenSpecs,omitempty"`
}
```

**Step 4: Run test to verify refactoring passes**

```bash
ginkgo -v pkg/fsmv2/worker pkg/fsmv2/types
```

Expected: PASS (all tests green)

**Step 5: Commit**

```bash
git add pkg/fsmv2/factory/worker_factory.go pkg/fsmv2/worker/*.go pkg/fsmv2/types/desired_state.go
git commit -m "feat(fsmv2): update DeriveDesiredState() to return DesiredState

Migration from string return to DesiredState struct:
- Worker interface now returns DesiredState (state + children)
- All existing workers updated (return empty ChildrenSpecs)
- Added helper constructors (NewDesiredState, NewDesiredStateWithChildren)
- Documented migration pattern for workers

Part of Phase 0: Hierarchical Composition Foundation

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 0.4: reconcileChildren() Implementation (6 hours)

**Files**:
- Modify: `pkg/fsmv2/supervisor/supervisor.go`
- Create: `pkg/fsmv2/supervisor/reconcile_children_test.go`
**Estimated Time**: 6 hours

#### RED: Write Tests for Add/Update/Remove Scenarios

```go
// pkg/fsmv2/supervisor/reconcile_children_test.go
package supervisor_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "context"

    "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
    "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/types"
    "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
)

var _ = Describe("ReconcileChildren", func() {
    var (
        sup     *supervisor.Supervisor
        store   *mockTriangularStore
        worker  *mockWorker
    )

    BeforeEach(func() {
        factory.ClearRegistry()
        factory.Register("mock_worker", func() factory.Worker {
            return &mockWorker{}
        })

        store = newMockTriangularStore()
        worker = &mockWorker{}
        sup = supervisor.NewSupervisor(worker, store)
    })

    Describe("Adding Children", func() {
        It("should create new child supervisor when not in s.children", func() {
            specs := []types.ChildSpec{
                {
                    Name:       "child1",
                    WorkerType: "mock_worker",
                    UserSpec:   types.UserSpec{Config: "state: running"},
                },
            }

            err := sup.ReconcileChildren(context.Background(), specs)
            Expect(err).ToNot(HaveOccurred())

            children := sup.ListChildren()
            Expect(children).To(HaveLen(1))
            Expect(children).To(ContainElement("child1"))
        })

        It("should create multiple children", func() {
            specs := []types.ChildSpec{
                {Name: "child1", WorkerType: "mock_worker"},
                {Name: "child2", WorkerType: "mock_worker"},
                {Name: "child3", WorkerType: "mock_worker"},
            }

            err := sup.ReconcileChildren(context.Background(), specs)
            Expect(err).ToNot(HaveOccurred())

            children := sup.ListChildren()
            Expect(children).To(HaveLen(3))
        })

        It("should return error for unknown worker type", func() {
            specs := []types.ChildSpec{
                {Name: "child1", WorkerType: "unknown_type"},
            }

            err := sup.ReconcileChildren(context.Background(), specs)
            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("unknown worker type"))
        })
    })

    Describe("Updating Children", func() {
        BeforeEach(func() {
            specs := []types.ChildSpec{
                {
                    Name:       "child1",
                    WorkerType: "mock_worker",
                    UserSpec:   types.UserSpec{Config: "state: stopped"},
                },
            }
            sup.ReconcileChildren(context.Background(), specs)
        })

        It("should update existing child UserSpec", func() {
            updatedSpecs := []types.ChildSpec{
                {
                    Name:       "child1",
                    WorkerType: "mock_worker",
                    UserSpec:   types.UserSpec{Config: "state: running"},
                },
            }

            err := sup.ReconcileChildren(context.Background(), updatedSpecs)
            Expect(err).ToNot(HaveOccurred())

            child := sup.GetChild("child1")
            Expect(child).ToNot(BeNil())
            Expect(child.GetUserSpec().Config).To(Equal("state: running"))
        })

        It("should update StateMapping", func() {
            updatedSpecs := []types.ChildSpec{
                {
                    Name:       "child1",
                    WorkerType: "mock_worker",
                    StateMapping: map[string]string{
                        "idle": "stopped",
                    },
                },
            }

            err := sup.ReconcileChildren(context.Background(), updatedSpecs)
            Expect(err).ToNot(HaveOccurred())

            child := sup.GetChild("child1")
            Expect(child.GetStateMapping()).To(HaveKeyWithValue("idle", "stopped"))
        })
    })

    Describe("Removing Children", func() {
        BeforeEach(func() {
            specs := []types.ChildSpec{
                {Name: "child1", WorkerType: "mock_worker"},
                {Name: "child2", WorkerType: "mock_worker"},
                {Name: "child3", WorkerType: "mock_worker"},
            }
            sup.ReconcileChildren(context.Background(), specs)
        })

        It("should remove children not in specs", func() {
            newSpecs := []types.ChildSpec{
                {Name: "child1", WorkerType: "mock_worker"},
            }

            err := sup.ReconcileChildren(context.Background(), newSpecs)
            Expect(err).ToNot(HaveOccurred())

            children := sup.ListChildren()
            Expect(children).To(HaveLen(1))
            Expect(children).To(ContainElement("child1"))
            Expect(children).ToNot(ContainElement("child2"))
            Expect(children).ToNot(ContainElement("child3"))
        })

        It("should shutdown removed children", func() {
            child2 := sup.GetChild("child2")
            Expect(child2.IsShutdown()).To(BeFalse())

            newSpecs := []types.ChildSpec{
                {Name: "child1", WorkerType: "mock_worker"},
            }

            err := sup.ReconcileChildren(context.Background(), newSpecs)
            Expect(err).ToNot(HaveOccurred())

            Eventually(func() bool {
                return child2.IsShutdown()
            }).Should(BeTrue())
        })
    })

    Describe("Idempotency", func() {
        It("should handle multiple reconcile calls with same specs", func() {
            specs := []types.ChildSpec{
                {Name: "child1", WorkerType: "mock_worker"},
            }

            err := sup.ReconcileChildren(context.Background(), specs)
            Expect(err).ToNot(HaveOccurred())

            child1 := sup.GetChild("child1")

            err = sup.ReconcileChildren(context.Background(), specs)
            Expect(err).ToNot(HaveOccurred())

            child1Again := sup.GetChild("child1")
            Expect(child1Again).To(BeIdenticalTo(child1))
        })
    })

    Describe("Circular Dependency Detection", func() {
        It("should detect parent as child of descendant", func() {
            // TODO: Implement circular dependency detection
            Skip("Circular dependency detection not yet implemented")
        })
    })
})
```

**Step 2: Run test to verify it fails**

```bash
ginkgo -v pkg/fsmv2/supervisor
```

Expected: FAIL with "undefined: ReconcileChildren"

#### GREEN: Implement Reconciliation Logic

```go
// pkg/fsmv2/supervisor/supervisor.go
func (s *Supervisor) ReconcileChildren(ctx context.Context, specs []types.ChildSpec) error {
    // 1. Add new children
    for _, spec := range specs {
        if _, exists := s.children[spec.Name]; !exists {
            childSupervisor, err := s.createChildSupervisor(ctx, spec)
            if err != nil {
                return fmt.Errorf("failed to create child %s: %w", spec.Name, err)
            }
            s.children[spec.Name] = childSupervisor
        }
    }

    // 2. Update existing children
    for _, spec := range specs {
        if child, exists := s.children[spec.Name]; exists {
            child.SetUserSpec(spec.UserSpec)
            child.stateMapping = spec.StateMapping
        }
    }

    // 3. Remove children not in specs
    specNames := make(map[string]bool)
    for _, spec := range specs {
        specNames[spec.Name] = true
    }

    for name, child := range s.children {
        if !specNames[name] {
            child.Shutdown()
            delete(s.children, name)
        }
    }

    return nil
}

func (s *Supervisor) createChildSupervisor(ctx context.Context, spec types.ChildSpec) (*Supervisor, error) {
    childWorker, err := factory.NewWorker(spec.WorkerType)
    if err != nil {
        return nil, fmt.Errorf("failed to create worker: %w", err)
    }

    childStore := s.store.ChildStore(spec.Name)
    childSupervisor := NewSupervisor(childWorker, childStore)
    childSupervisor.stateMapping = spec.StateMapping
    childSupervisor.SetUserSpec(spec.UserSpec)

    return childSupervisor, nil
}

func (s *Supervisor) ListChildren() []string {
    names := make([]string, 0, len(s.children))
    for name := range s.children {
        names = append(names, name)
    }
    return names
}

func (s *Supervisor) GetChild(name string) *Supervisor {
    return s.children[name]
}

func (s *Supervisor) SetUserSpec(userSpec types.UserSpec) {
    s.userSpec = userSpec
}

func (s *Supervisor) GetUserSpec() types.UserSpec {
    return s.userSpec
}

func (s *Supervisor) GetStateMapping() map[string]string {
    return s.stateMapping
}

func (s *Supervisor) IsShutdown() bool {
    return s.shutdown
}

func (s *Supervisor) Shutdown() {
    s.shutdown = true
    for _, child := range s.children {
        child.Shutdown()
    }
}
```

**Step 3: Run test to verify it passes**

```bash
ginkgo -v pkg/fsmv2/supervisor
```

Expected: PASS (all tests green)

#### REFACTOR: Extract Helper and Add Error Handling

```go
// pkg/fsmv2/supervisor/supervisor.go
import (
    "go.uber.org/zap"
)

func (s *Supervisor) ReconcileChildren(ctx context.Context, specs []types.ChildSpec) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    // Validate specs first
    for _, spec := range specs {
        if err := spec.Validate(); err != nil {
            return fmt.Errorf("invalid ChildSpec: %w", err)
        }
    }

    // Add new children
    for _, spec := range specs {
        if _, exists := s.children[spec.Name]; !exists {
            if err := s.addChild(ctx, spec); err != nil {
                s.logger.Errorw("failed to add child",
                    "child", spec.Name,
                    "error", err,
                )
                return err
            }
        }
    }

    // Update existing children
    for _, spec := range specs {
        if err := s.updateChild(spec); err != nil {
            s.logger.Errorw("failed to update child",
                "child", spec.Name,
                "error", err,
            )
            return err
        }
    }

    // Remove children not in specs
    if err := s.removeOrphanedChildren(specs); err != nil {
        return err
    }

    return nil
}

func (s *Supervisor) addChild(ctx context.Context, spec types.ChildSpec) error {
    childSupervisor, err := s.createChildSupervisor(ctx, spec)
    if err != nil {
        return fmt.Errorf("failed to create child %s: %w", spec.Name, err)
    }

    s.children[spec.Name] = childSupervisor
    s.logger.Infow("added child",
        "child", spec.Name,
        "workerType", spec.WorkerType,
    )

    return nil
}

func (s *Supervisor) updateChild(spec types.ChildSpec) error {
    child, exists := s.children[spec.Name]
    if !exists {
        return nil
    }

    child.SetUserSpec(spec.UserSpec)
    child.stateMapping = spec.StateMapping

    s.logger.Debugw("updated child",
        "child", spec.Name,
        "userSpec", spec.UserSpec,
    )

    return nil
}

func (s *Supervisor) removeOrphanedChildren(specs []types.ChildSpec) error {
    specNames := make(map[string]bool)
    for _, spec := range specs {
        specNames[spec.Name] = true
    }

    for name, child := range s.children {
        if !specNames[name] {
            s.logger.Infow("removing child",
                "child", name,
            )
            child.Shutdown()
            delete(s.children, name)
        }
    }

    return nil
}

func (s *Supervisor) createChildSupervisor(ctx context.Context, spec types.ChildSpec) (*Supervisor, error) {
    childWorker, err := factory.NewWorker(spec.WorkerType)
    if err != nil {
        return nil, fmt.Errorf("failed to create worker: %w", err)
    }

    childStore := s.store.ChildStore(spec.Name)
    childSupervisor := NewSupervisor(childWorker, childStore)
    childSupervisor.stateMapping = spec.StateMapping
    childSupervisor.SetUserSpec(spec.UserSpec)
    childSupervisor.logger = s.logger.With("child", spec.Name)

    return childSupervisor, nil
}
```

**Add error handling tests**:
```go
// Add to reconcile_children_test.go
Describe("Error Handling", func() {
    It("should return error on invalid ChildSpec", func() {
        invalidSpecs := []types.ChildSpec{
            {Name: "", WorkerType: "mock_worker"},
        }

        err := sup.ReconcileChildren(context.Background(), invalidSpecs)
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("invalid ChildSpec"))
    })

    It("should continue processing other children after update error", func() {
        specs := []types.ChildSpec{
            {Name: "child1", WorkerType: "mock_worker"},
            {Name: "child2", WorkerType: "mock_worker"},
        }
        sup.ReconcileChildren(context.Background(), specs)

        // Now reconcile with one valid, one error
        newSpecs := []types.ChildSpec{
            {Name: "child1", WorkerType: "mock_worker"},
        }

        err := sup.ReconcileChildren(context.Background(), newSpecs)
        Expect(err).ToNot(HaveOccurred())

        children := sup.ListChildren()
        Expect(children).To(HaveLen(1))
    })
})
```

**Step 4: Run test to verify refactoring passes**

```bash
ginkgo -v pkg/fsmv2/supervisor
```

Expected: PASS (all tests green)

**Step 5: Commit**

```bash
git add pkg/fsmv2/supervisor/supervisor.go pkg/fsmv2/supervisor/reconcile_children_test.go
git commit -m "feat(fsmv2): implement reconcileChildren() for child lifecycle management

Implements Kubernetes-style reconciliation:
- Add new children not in s.children
- Update existing children (UserSpec, StateMapping)
- Remove children not in specs
- Extracted helpers (addChild, updateChild, removeOrphanedChildren)
- Error handling with structured logging
- Thread-safe with mutex protection

Part of Phase 0: Hierarchical Composition Foundation

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 0.5: StateMapping Application (2 hours)

**Files**:
- Modify: `pkg/fsmv2/supervisor/supervisor.go` (Tick method)
- Create: `pkg/fsmv2/supervisor/state_mapping_test.go`
**Estimated Time**: 2 hours

#### RED: Write Tests for State Mapping Scenarios

```go
// pkg/fsmv2/supervisor/state_mapping_test.go
package supervisor_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "context"

    "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
    "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/types"
    "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
)

var _ = Describe("StateMapping Application", func() {
    var (
        sup    *supervisor.Supervisor
        store  *mockTriangularStore
        worker *mockWorker
    )

    BeforeEach(func() {
        factory.ClearRegistry()
        factory.Register("mock_worker", func() factory.Worker {
            return &mockWorker{desiredState: "running"}
        })

        store = newMockTriangularStore()
        worker = &mockWorker{desiredState: "idle"}
        sup = supervisor.NewSupervisor(worker, store)
    })

    Describe("Applying StateMapping", func() {
        It("should map parent state to child state", func() {
            specs := []types.ChildSpec{
                {
                    Name:       "child1",
                    WorkerType: "mock_worker",
                    StateMapping: map[string]string{
                        "idle":   "stopped",
                        "active": "running",
                    },
                },
            }

            sup.ReconcileChildren(context.Background(), specs)
            worker.desiredState = "idle"
            sup.Tick(context.Background())

            child := sup.GetChild("child1")
            Expect(child.GetDesiredState()).To(Equal("stopped"))
        })

        It("should handle multiple children with different mappings", func() {
            specs := []types.ChildSpec{
                {
                    Name:       "connection",
                    WorkerType: "mock_worker",
                    StateMapping: map[string]string{
                        "idle": "stopped",
                        "active": "running",
                    },
                },
                {
                    Name:       "dataflow",
                    WorkerType: "mock_worker",
                    StateMapping: map[string]string{
                        "idle": "paused",
                        "active": "processing",
                    },
                },
            }

            sup.ReconcileChildren(context.Background(), specs)
            worker.desiredState = "active"
            sup.Tick(context.Background())

            connection := sup.GetChild("connection")
            Expect(connection.GetDesiredState()).To(Equal("running"))

            dataflow := sup.GetChild("dataflow")
            Expect(dataflow.GetDesiredState()).To(Equal("processing"))
        })

        It("should preserve child state when mapping is missing", func() {
            specs := []types.ChildSpec{
                {
                    Name:       "child1",
                    WorkerType: "mock_worker",
                    StateMapping: map[string]string{
                        "active": "running",
                    },
                },
            }

            sup.ReconcileChildren(context.Background(), specs)
            child := sup.GetChild("child1")
            child.SetDesiredState("custom_state")

            worker.desiredState = "idle"
            sup.Tick(context.Background())

            Expect(child.GetDesiredState()).To(Equal("custom_state"))
        })

        It("should not apply mapping when StateMapping is nil", func() {
            specs := []types.ChildSpec{
                {
                    Name:         "child1",
                    WorkerType:   "mock_worker",
                    StateMapping: nil,
                },
            }

            sup.ReconcileChildren(context.Background(), specs)
            child := sup.GetChild("child1")
            child.SetDesiredState("initial")

            worker.desiredState = "active"
            sup.Tick(context.Background())

            Expect(child.GetDesiredState()).To(Equal("initial"))
        })
    })

    Describe("StateMapping Edge Cases", func() {
        It("should handle empty mapping gracefully", func() {
            specs := []types.ChildSpec{
                {
                    Name:         "child1",
                    WorkerType:   "mock_worker",
                    StateMapping: map[string]string{},
                },
            }

            sup.ReconcileChildren(context.Background(), specs)
            worker.desiredState = "active"

            Expect(func() {
                sup.Tick(context.Background())
            }).ToNot(Panic())
        })

        It("should handle state changes during tick", func() {
            specs := []types.ChildSpec{
                {
                    Name:       "child1",
                    WorkerType: "mock_worker",
                    StateMapping: map[string]string{
                        "idle": "stopped",
                    },
                },
            }

            sup.ReconcileChildren(context.Background(), specs)

            worker.desiredState = "idle"
            sup.Tick(context.Background())

            child := sup.GetChild("child1")
            Expect(child.GetDesiredState()).To(Equal("stopped"))

            worker.desiredState = "active"
            sup.Tick(context.Background())

            Expect(child.GetDesiredState()).To(Equal("stopped"))
        })
    })
})
```

**Step 2: Run test to verify it fails**

```bash
ginkgo -v pkg/fsmv2/supervisor
```

Expected: FAIL with "GetDesiredState not implemented"

#### GREEN: Integrate StateMapping in Tick Loop

```go
// pkg/fsmv2/supervisor/supervisor.go
func (s *Supervisor) Tick(ctx context.Context) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    desiredState := s.worker.DeriveDesiredState(s.userSpec)

    if err := s.ReconcileChildren(ctx, desiredState.ChildrenSpecs); err != nil {
        return fmt.Errorf("failed to reconcile children: %w", err)
    }

    for _, child := range s.children {
        if mappedState, ok := child.stateMapping[desiredState.State]; ok {
            child.SetDesiredState(mappedState)
        }
    }

    s.currentState = desiredState.State
    return nil
}

func (s *Supervisor) SetDesiredState(state string) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.desiredState = state
}

func (s *Supervisor) GetDesiredState() string {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return s.desiredState
}
```

**Step 3: Run test to verify it passes**

```bash
ginkgo -v pkg/fsmv2/supervisor
```

Expected: PASS (all tests green)

#### REFACTOR: Handle Missing Mappings Gracefully

```go
// pkg/fsmv2/supervisor/supervisor.go
func (s *Supervisor) Tick(ctx context.Context) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    desiredState := s.worker.DeriveDesiredState(s.userSpec)

    if err := s.ReconcileChildren(ctx, desiredState.ChildrenSpecs); err != nil {
        s.logger.Errorw("failed to reconcile children", "error", err)
        return fmt.Errorf("failed to reconcile children: %w", err)
    }

    s.applyStateMapping(desiredState.State)

    s.currentState = desiredState.State
    return nil
}

func (s *Supervisor) applyStateMapping(parentState string) {
    for name, child := range s.children {
        if len(child.stateMapping) == 0 {
            s.logger.Debugw("child has no state mapping, skipping",
                "child", name,
            )
            continue
        }

        if mappedState, ok := child.stateMapping[parentState]; ok {
            s.logger.Debugw("applying state mapping",
                "child", name,
                "parentState", parentState,
                "mappedState", mappedState,
            )
            child.SetDesiredState(mappedState)
        } else {
            s.logger.Debugw("no mapping found for parent state, preserving child state",
                "child", name,
                "parentState", parentState,
                "currentChildState", child.GetDesiredState(),
            )
        }
    }
}
```

**Add logging tests**:
```go
// Add to state_mapping_test.go
Describe("StateMapping Logging", func() {
    It("should log when state mapping is applied", func() {
        specs := []types.ChildSpec{
            {
                Name:       "child1",
                WorkerType: "mock_worker",
                StateMapping: map[string]string{
                    "idle": "stopped",
                },
            },
        }

        sup.ReconcileChildren(context.Background(), specs)
        worker.desiredState = "idle"

        logs := captureLogs(func() {
            sup.Tick(context.Background())
        })

        Expect(logs).To(ContainSubstring("applying state mapping"))
        Expect(logs).To(ContainSubstring("parentState=idle"))
        Expect(logs).To(ContainSubstring("mappedState=stopped"))
    })

    It("should log when no mapping found", func() {
        specs := []types.ChildSpec{
            {
                Name:       "child1",
                WorkerType: "mock_worker",
                StateMapping: map[string]string{
                    "active": "running",
                },
            },
        }

        sup.ReconcileChildren(context.Background(), specs)
        worker.desiredState = "idle"

        logs := captureLogs(func() {
            sup.Tick(context.Background())
        })

        Expect(logs).To(ContainSubstring("no mapping found for parent state"))
    })
})
```

**Step 4: Run test to verify refactoring passes**

```bash
ginkgo -v pkg/fsmv2/supervisor
```

Expected: PASS (all tests green)

**Step 5: Commit**

```bash
git add pkg/fsmv2/supervisor/supervisor.go pkg/fsmv2/supervisor/state_mapping_test.go
git commit -m "feat(fsmv2): apply StateMapping for parent-child state control

Implements state mapping in tick loop:
- Maps parent state to child state based on StateMapping
- Preserves child state when mapping is missing
- Handles nil/empty StateMapping gracefully
- Extracted applyStateMapping() helper
- Structured logging for debugging

Part of Phase 0: Hierarchical Composition Foundation

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 0.6: Recursive Tick Propagation (2 hours)

**Files**:
- Modify: `pkg/fsmv2/supervisor/supervisor.go` (Tick method)
- Create: `pkg/fsmv2/supervisor/recursive_tick_test.go`
**Estimated Time**: 2 hours

#### RED: Write Tests for Recursive Tick Scenarios

```go
// pkg/fsmv2/supervisor/recursive_tick_test.go
package supervisor_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "context"
    "sync/atomic"

    "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
    "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/types"
    "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
)

var _ = Describe("Recursive Tick Propagation", func() {
    var (
        rootSup *supervisor.Supervisor
        store   *mockTriangularStore
        worker  *mockWorkerWithTicks
    )

    BeforeEach(func() {
        factory.ClearRegistry()
        factory.Register("mock_worker_ticks", func() factory.Worker {
            return &mockWorkerWithTicks{}
        })

        store = newMockTriangularStore()
        worker = &mockWorkerWithTicks{}
        rootSup = supervisor.NewSupervisor(worker, store)
    })

    Describe("Single Level Tick", func() {
        It("should tick children after parent", func() {
            specs := []types.ChildSpec{
                {Name: "child1", WorkerType: "mock_worker_ticks"},
            }

            rootSup.ReconcileChildren(context.Background(), specs)
            rootSup.Tick(context.Background())

            child := rootSup.GetChild("child1")
            Expect(child.GetTickCount()).To(BeNumerically(">", 0))
        })

        It("should tick all children", func() {
            specs := []types.ChildSpec{
                {Name: "child1", WorkerType: "mock_worker_ticks"},
                {Name: "child2", WorkerType: "mock_worker_ticks"},
                {Name: "child3", WorkerType: "mock_worker_ticks"},
            }

            rootSup.ReconcileChildren(context.Background(), specs)
            rootSup.Tick(context.Background())

            for i := 1; i <= 3; i++ {
                child := rootSup.GetChild(fmt.Sprintf("child%d", i))
                Expect(child.GetTickCount()).To(Equal(1))
            }
        })
    })

    Describe("Deep Hierarchy Tick", func() {
        It("should tick 3-level hierarchy correctly", func() {
            factory.Register("parent_worker", func() factory.Worker {
                return &mockParentWorker{
                    childSpecs: []types.ChildSpec{
                        {Name: "grandchild", WorkerType: "mock_worker_ticks"},
                    },
                }
            })

            specs := []types.ChildSpec{
                {
                    Name:       "child",
                    WorkerType: "parent_worker",
                },
            }

            rootSup.ReconcileChildren(context.Background(), specs)
            rootSup.Tick(context.Background())

            child := rootSup.GetChild("child")
            Expect(child.GetTickCount()).To(Equal(1))

            grandchild := child.GetChild("grandchild")
            Expect(grandchild).ToNot(BeNil())
            Expect(grandchild.GetTickCount()).To(Equal(1))
        })

        It("should tick multiple levels in correct order", func() {
            var tickOrder []string
            tickOrderMutex := sync.Mutex{}

            factory.Register("ordered_worker", func() factory.Worker {
                return &mockOrderedWorker{
                    onTick: func(name string) {
                        tickOrderMutex.Lock()
                        defer tickOrderMutex.Unlock()
                        tickOrder = append(tickOrder, name)
                    },
                }
            })

            specs := []types.ChildSpec{
                {Name: "child1", WorkerType: "ordered_worker"},
                {Name: "child2", WorkerType: "ordered_worker"},
            }

            rootSup.ReconcileChildren(context.Background(), specs)
            rootSup.Tick(context.Background())

            Expect(tickOrder).To(HaveLen(2))
            Expect(tickOrder[0]).To(Equal("child1"))
            Expect(tickOrder[1]).To(Equal("child2"))
        })
    })

    Describe("Tick Error Handling", func() {
        It("should continue ticking other children if one fails", func() {
            factory.Register("failing_worker", func() factory.Worker {
                return &mockFailingWorker{shouldFail: true}
            })
            factory.Register("success_worker", func() factory.Worker {
                return &mockWorkerWithTicks{}
            })

            specs := []types.ChildSpec{
                {Name: "failing_child", WorkerType: "failing_worker"},
                {Name: "success_child", WorkerType: "success_worker"},
            }

            rootSup.ReconcileChildren(context.Background(), specs)
            err := rootSup.Tick(context.Background())

            Expect(err).To(HaveOccurred())
            successChild := rootSup.GetChild("success_child")
            Expect(successChild.GetTickCount()).To(Equal(1))
        })

        It("should stop ticking children if parent tick fails", func() {
            worker.shouldFailTick = true
            specs := []types.ChildSpec{
                {Name: "child1", WorkerType: "mock_worker_ticks"},
            }

            rootSup.ReconcileChildren(context.Background(), specs)
            err := rootSup.Tick(context.Background())

            Expect(err).To(HaveOccurred())
            child := rootSup.GetChild("child1")
            Expect(child.GetTickCount()).To(Equal(0))
        })
    })

    Describe("Context Cancellation", func() {
        It("should respect context cancellation", func() {
            ctx, cancel := context.WithCancel(context.Background())
            cancel()

            specs := []types.ChildSpec{
                {Name: "child1", WorkerType: "mock_worker_ticks"},
            }

            rootSup.ReconcileChildren(context.Background(), specs)
            err := rootSup.Tick(ctx)

            Expect(err).To(HaveOccurred())
            Expect(err).To(MatchError(context.Canceled))
        })
    })
})
```

**Step 2: Run test to verify it fails**

```bash
ginkgo -v pkg/fsmv2/supervisor
```

Expected: FAIL with "children not ticked"

#### GREEN: Add Child Tick Calls in Parent Tick

```go
// pkg/fsmv2/supervisor/supervisor.go
func (s *Supervisor) Tick(ctx context.Context) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    if ctx.Err() != nil {
        return ctx.Err()
    }

    desiredState := s.worker.DeriveDesiredState(s.userSpec)

    if err := s.ReconcileChildren(ctx, desiredState.ChildrenSpecs); err != nil {
        s.logger.Errorw("failed to reconcile children", "error", err)
        return fmt.Errorf("failed to reconcile children: %w", err)
    }

    s.applyStateMapping(desiredState.State)

    for name, child := range s.children {
        if err := child.Tick(ctx); err != nil {
            s.logger.Errorw("child tick failed",
                "child", name,
                "error", err,
            )
            return fmt.Errorf("child %s tick failed: %w", name, err)
        }
    }

    s.currentState = desiredState.State
    s.tickCount++

    return nil
}

func (s *Supervisor) GetTickCount() int {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return s.tickCount
}
```

**Step 3: Run test to verify it passes**

```bash
ginkgo -v pkg/fsmv2/supervisor
```

Expected: PASS (all tests green)

#### REFACTOR: Add Error Handling for Child Tick Failures

```go
// pkg/fsmv2/supervisor/supervisor.go
func (s *Supervisor) Tick(ctx context.Context) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    if ctx.Err() != nil {
        return ctx.Err()
    }

    desiredState := s.worker.DeriveDesiredState(s.userSpec)

    if err := s.ReconcileChildren(ctx, desiredState.ChildrenSpecs); err != nil {
        s.logger.Errorw("failed to reconcile children", "error", err)
        return fmt.Errorf("failed to reconcile children: %w", err)
    }

    s.applyStateMapping(desiredState.State)

    if err := s.tickChildren(ctx); err != nil {
        return err
    }

    s.currentState = desiredState.State
    s.tickCount++

    return nil
}

func (s *Supervisor) tickChildren(ctx context.Context) error {
    var firstError error

    for name, child := range s.children {
        if ctx.Err() != nil {
            return ctx.Err()
        }

        if err := child.Tick(ctx); err != nil {
            s.logger.Errorw("child tick failed",
                "child", name,
                "error", err,
            )
            if firstError == nil {
                firstError = fmt.Errorf("child %s tick failed: %w", name, err)
            }
        }
    }

    return firstError
}
```

**Add error handling tests**:
```go
// Add to recursive_tick_test.go
Describe("Error Recovery", func() {
    It("should log all child tick errors but return first", func() {
        factory.Register("failing_worker_1", func() factory.Worker {
            return &mockFailingWorker{
                failureMessage: "error 1",
                shouldFail:     true,
            }
        })
        factory.Register("failing_worker_2", func() factory.Worker {
            return &mockFailingWorker{
                failureMessage: "error 2",
                shouldFail:     true,
            }
        })

        specs := []types.ChildSpec{
            {Name: "child1", WorkerType: "failing_worker_1"},
            {Name: "child2", WorkerType: "failing_worker_2"},
        }

        rootSup.ReconcileChildren(context.Background(), specs)

        logs := captureLogs(func() {
            err := rootSup.Tick(context.Background())
            Expect(err).To(HaveOccurred())
        })

        Expect(logs).To(ContainSubstring("child tick failed"))
        Expect(logs).To(ContainSubstring("child1"))
    })
})

Describe("Performance", func() {
    It("should tick many children efficiently", func() {
        specs := make([]types.ChildSpec, 100)
        for i := 0; i < 100; i++ {
            specs[i] = types.ChildSpec{
                Name:       fmt.Sprintf("child%d", i),
                WorkerType: "mock_worker_ticks",
            }
        }

        rootSup.ReconcileChildren(context.Background(), specs)

        start := time.Now()
        err := rootSup.Tick(context.Background())
        duration := time.Since(start)

        Expect(err).ToNot(HaveOccurred())
        Expect(duration).To(BeNumerically("<", 100*time.Millisecond))
    })
})
```

**Step 4: Run test to verify refactoring passes**

```bash
ginkgo -v pkg/fsmv2/supervisor
```

Expected: PASS (all tests green)

**Step 5: Commit**

```bash
git add pkg/fsmv2/supervisor/supervisor.go pkg/fsmv2/supervisor/recursive_tick_test.go
git commit -m "feat(fsmv2): add recursive tick propagation to children

Implements recursive tick in supervisor loop:
- Children tick after parent tick completes
- Deep hierarchy (3+ levels) ticks correctly
- Context cancellation respected
- Extracted tickChildren() helper
- Error handling continues ticking but returns first error

Part of Phase 0: Hierarchical Composition Foundation

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

### Task 0.7: Integration Tests (4 hours)

**Files**:
- Create: `pkg/fsmv2/integration/hierarchical_composition_test.go`
**Estimated Time**: 4 hours

#### RED: Write 7 Integration Test Scenarios

```go
// pkg/fsmv2/integration/hierarchical_composition_test.go
package integration_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "context"
    "time"

    "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
    "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/types"
    "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
)

var _ = Describe("Hierarchical Composition Integration", func() {
    BeforeEach(func() {
        factory.ClearRegistry()
        setupTestWorkers()
    })

    Describe("Scenario 1: Parent with No Children", func() {
        It("should work correctly with empty ChildrenSpecs", func() {
            worker := &simpleWorker{state: "running"}
            store := newMemoryStore()
            sup := supervisor.NewSupervisor(worker, store)

            err := sup.Tick(context.Background())
            Expect(err).ToNot(HaveOccurred())

            children := sup.ListChildren()
            Expect(children).To(BeEmpty())
        })
    })

    Describe("Scenario 2: Parent Adds Child", func() {
        It("should create child supervisor on reconciliation", func() {
            worker := &parentWorker{
                childSpecs: []types.ChildSpec{
                    {Name: "child1", WorkerType: "simple_worker"},
                },
            }
            store := newMemoryStore()
            sup := supervisor.NewSupervisor(worker, store)

            err := sup.Tick(context.Background())
            Expect(err).ToNot(HaveOccurred())

            children := sup.ListChildren()
            Expect(children).To(ContainElement("child1"))

            child := sup.GetChild("child1")
            Expect(child).ToNot(BeNil())
            Expect(child.GetTickCount()).To(Equal(1))
        })
    })

    Describe("Scenario 3: Parent Removes Child", func() {
        It("should shutdown child supervisor on reconciliation", func() {
            worker := &parentWorker{
                childSpecs: []types.ChildSpec{
                    {Name: "child1", WorkerType: "simple_worker"},
                    {Name: "child2", WorkerType: "simple_worker"},
                },
            }
            store := newMemoryStore()
            sup := supervisor.NewSupervisor(worker, store)

            sup.Tick(context.Background())
            child1 := sup.GetChild("child1")
            child2 := sup.GetChild("child2")

            Expect(child1.IsShutdown()).To(BeFalse())
            Expect(child2.IsShutdown()).To(BeFalse())

            worker.childSpecs = []types.ChildSpec{
                {Name: "child1", WorkerType: "simple_worker"},
            }

            sup.Tick(context.Background())

            children := sup.ListChildren()
            Expect(children).To(HaveLen(1))
            Expect(children).To(ContainElement("child1"))

            Eventually(func() bool {
                return child2.IsShutdown()
            }, time.Second).Should(BeTrue())
        })
    })

    Describe("Scenario 4: Parent Updates Child Config", func() {
        It("should update child UserSpec on reconciliation", func() {
            worker := &parentWorker{
                childSpecs: []types.ChildSpec{
                    {
                        Name:       "child1",
                        WorkerType: "simple_worker",
                        UserSpec:   types.UserSpec{Config: "state: stopped"},
                    },
                },
            }
            store := newMemoryStore()
            sup := supervisor.NewSupervisor(worker, store)

            sup.Tick(context.Background())
            child := sup.GetChild("child1")
            Expect(child.GetUserSpec().Config).To(Equal("state: stopped"))

            worker.childSpecs[0].UserSpec.Config = "state: running"

            sup.Tick(context.Background())
            Expect(child.GetUserSpec().Config).To(Equal("state: running"))
        })
    })

    Describe("Scenario 5: StateMapping Propagation", func() {
        It("should propagate parent state to child via StateMapping", func() {
            worker := &stateChangeWorker{state: "idle"}
            factory.Register("stateful_worker", func() factory.Worker {
                return &statefulWorker{}
            })

            worker.childSpecs = []types.ChildSpec{
                {
                    Name:       "child1",
                    WorkerType: "stateful_worker",
                    StateMapping: map[string]string{
                        "idle":   "stopped",
                        "active": "running",
                    },
                },
            }

            store := newMemoryStore()
            sup := supervisor.NewSupervisor(worker, store)

            sup.Tick(context.Background())
            child := sup.GetChild("child1")
            Expect(child.GetDesiredState()).To(Equal("stopped"))

            worker.state = "active"
            sup.Tick(context.Background())
            Expect(child.GetDesiredState()).To(Equal("running"))
        })
    })

    Describe("Scenario 6: Recursive Tick", func() {
        It("should tick parent → child → grandchild", func() {
            factory.Register("grandparent_worker", func() factory.Worker {
                return &grandparentWorker{}
            })
            factory.Register("parent_worker_with_child", func() factory.Worker {
                return &parentWorkerWithChild{}
            })
            factory.Register("leaf_worker", func() factory.Worker {
                return &leafWorker{}
            })

            worker := &grandparentWorker{}
            store := newMemoryStore()
            sup := supervisor.NewSupervisor(worker, store)

            sup.Tick(context.Background())

            Expect(sup.GetTickCount()).To(Equal(1))

            child := sup.GetChild("child")
            Expect(child).ToNot(BeNil())
            Expect(child.GetTickCount()).To(Equal(1))

            grandchild := child.GetChild("grandchild")
            Expect(grandchild).ToNot(BeNil())
            Expect(grandchild.GetTickCount()).To(Equal(1))
        })

        It("should handle 5-level deep hierarchy", func() {
            factory.Register("nested_worker", func() factory.Worker {
                return &nestedWorker{depth: 0, maxDepth: 5}
            })

            worker := &nestedWorker{depth: 0, maxDepth: 5}
            store := newMemoryStore()
            sup := supervisor.NewSupervisor(worker, store)

            sup.Tick(context.Background())

            current := sup
            for depth := 0; depth < 5; depth++ {
                Expect(current.GetTickCount()).To(Equal(1))

                children := current.ListChildren()
                if depth < 4 {
                    Expect(children).To(HaveLen(1))
                    current = current.GetChild("child")
                }
            }
        })
    })

    Describe("Scenario 7: Circular Dependency Detection", func() {
        It("should detect parent as child of descendant", func() {
            Skip("Circular dependency detection not yet implemented")

            worker := &circularWorker{}
            store := newMemoryStore()
            sup := supervisor.NewSupervisor(worker, store)

            err := sup.Tick(context.Background())
            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("circular dependency"))
        })

        It("should detect indirect circular dependency", func() {
            Skip("Circular dependency detection not yet implemented")
        })
    })
})

func setupTestWorkers() {
    factory.Register("simple_worker", func() factory.Worker {
        return &simpleWorker{state: "running"}
    })

    factory.Register("parent_worker", func() factory.Worker {
        return &parentWorker{}
    })

    factory.Register("stateful_worker", func() factory.Worker {
        return &statefulWorker{}
    })
}

type simpleWorker struct {
    state string
}

func (w *simpleWorker) DeriveDesiredState(userSpec types.UserSpec) types.DesiredState {
    return types.NewDesiredState(w.state)
}

type parentWorker struct {
    childSpecs []types.ChildSpec
}

func (w *parentWorker) DeriveDesiredState(userSpec types.UserSpec) types.DesiredState {
    return types.DesiredState{
        State:         "running",
        ChildrenSpecs: w.childSpecs,
    }
}

type stateChangeWorker struct {
    state      string
    childSpecs []types.ChildSpec
}

func (w *stateChangeWorker) DeriveDesiredState(userSpec types.UserSpec) types.DesiredState {
    return types.DesiredState{
        State:         w.state,
        ChildrenSpecs: w.childSpecs,
    }
}

type statefulWorker struct {
    desiredState string
}

func (w *statefulWorker) DeriveDesiredState(userSpec types.UserSpec) types.DesiredState {
    return types.NewDesiredState(w.desiredState)
}

type grandparentWorker struct{}

func (w *grandparentWorker) DeriveDesiredState(userSpec types.UserSpec) types.DesiredState {
    return types.DesiredState{
        State: "running",
        ChildrenSpecs: []types.ChildSpec{
            {Name: "child", WorkerType: "parent_worker_with_child"},
        },
    }
}

type parentWorkerWithChild struct{}

func (w *parentWorkerWithChild) DeriveDesiredState(userSpec types.UserSpec) types.DesiredState {
    return types.DesiredState{
        State: "running",
        ChildrenSpecs: []types.ChildSpec{
            {Name: "grandchild", WorkerType: "leaf_worker"},
        },
    }
}

type leafWorker struct{}

func (w *leafWorker) DeriveDesiredState(userSpec types.UserSpec) types.DesiredState {
    return types.NewDesiredState("running")
}

type nestedWorker struct {
    depth    int
    maxDepth int
}

func (w *nestedWorker) DeriveDesiredState(userSpec types.UserSpec) types.DesiredState {
    if w.depth >= w.maxDepth-1 {
        return types.NewDesiredState("running")
    }

    return types.DesiredState{
        State: "running",
        ChildrenSpecs: []types.ChildSpec{
            {Name: "child", WorkerType: "nested_worker"},
        },
    }
}
```

**Step 2: Run test to verify it fails**

```bash
ginkgo -v pkg/fsmv2/integration
```

Expected: FAIL with various implementation issues

#### GREEN: Ensure All Scenarios Pass

At this point, all previous tasks should have been completed, so the integration tests should pass. If not, identify and fix gaps in previous implementations.

**Step 3: Run test to verify it passes**

```bash
ginkgo -v pkg/fsmv2/integration
```

Expected: PASS (all integration tests green)

#### REFACTOR: Extract Test Helpers

```go
// pkg/fsmv2/integration/test_helpers.go
package integration_test

import (
    "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
    "github.com/united-manufacturing-hub/umh-core/pkg/fsmv2/types"
)

func newMemoryStore() supervisor.TriangularStore {
    return &memoryStore{
        data: make(map[string]interface{}),
    }
}

type memoryStore struct {
    data map[string]interface{}
}

func (m *memoryStore) ChildStore(name string) supervisor.TriangularStore {
    return &memoryStore{
        data: make(map[string]interface{}),
    }
}

func setupTestSupervisor(worker factory.Worker) *supervisor.Supervisor {
    store := newMemoryStore()
    return supervisor.NewSupervisor(worker, store)
}

func tickUntilStable(sup *supervisor.Supervisor, maxTicks int) error {
    ctx := context.Background()
    for i := 0; i < maxTicks; i++ {
        if err := sup.Tick(ctx); err != nil {
            return err
        }
    }
    return nil
}

func assertChildExists(sup *supervisor.Supervisor, childName string) {
    children := sup.ListChildren()
    Expect(children).To(ContainElement(childName),
        "Expected child %s to exist", childName)
}

func assertChildDoesNotExist(sup *supervisor.Supervisor, childName string) {
    children := sup.ListChildren()
    Expect(children).ToNot(ContainElement(childName),
        "Expected child %s to not exist", childName)
}

func assertHierarchyDepth(sup *supervisor.Supervisor, expectedDepth int) {
    depth := 0
    current := sup
    for {
        children := current.ListChildren()
        if len(children) == 0 {
            break
        }
        depth++
        current = current.GetChild(children[0])
    }
    Expect(depth).To(Equal(expectedDepth))
}
```

**Refactor integration tests to use helpers**:
```go
// Refactored example
Describe("Scenario 2: Parent Adds Child", func() {
    It("should create child supervisor on reconciliation", func() {
        worker := &parentWorker{
            childSpecs: []types.ChildSpec{
                {Name: "child1", WorkerType: "simple_worker"},
            },
        }
        sup := setupTestSupervisor(worker)

        err := sup.Tick(context.Background())
        Expect(err).ToNot(HaveOccurred())

        assertChildExists(sup, "child1")

        child := sup.GetChild("child1")
        Expect(child.GetTickCount()).To(Equal(1))
    })
})
```

**Step 4: Run test to verify refactoring passes**

```bash
ginkgo -v pkg/fsmv2/integration
```

Expected: PASS (all tests green with cleaner test code)

**Step 5: Commit**

```bash
git add pkg/fsmv2/integration/hierarchical_composition_test.go pkg/fsmv2/integration/test_helpers.go
git commit -m "test(fsmv2): add hierarchical composition integration tests

Implements 7 integration test scenarios:
1. Parent with no children (empty ChildrenSpecs)
2. Parent adds child (reconcileChildren creates supervisor)
3. Parent removes child (reconcileChildren shuts down supervisor)
4. Parent updates child config (reconcileChildren updates UserSpec)
5. StateMapping propagation (parent state → child state)
6. Recursive tick (parent → child → grandchild)
7. Circular dependency detection (marked as TODO)

Extracted test helpers:
- setupTestSupervisor()
- tickUntilStable()
- assertChildExists/assertChildDoesNotExist()
- assertHierarchyDepth()

Part of Phase 0: Hierarchical Composition Foundation

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

**Total Estimated Effort:** 20 hours (2.5 days) → Buffer to 2 weeks

---

## Design Documents

See [design/](design/) folder for detailed rationale:

- **[fsmv2-child-specs-in-desired-state.md](design/fsmv2-child-specs-in-desired-state.md)** - Core ChildSpec pattern
- **[fsmv2-children-as-desired-state.md](design/fsmv2-children-as-desired-state.md)** - Why children in DesiredState
- **[fsmv2-child-removal-auto.md](design/fsmv2-child-removal-auto.md)** - Automatic child removal logic
- **[fsmv2-phase0-worker-child-api.md](design/fsmv2-phase0-worker-child-api.md)** - Worker-child API design

---

## Acceptance Criteria

### Functionality
- [ ] Parent can declare children via ChildSpec in DeriveDesiredState()
- [ ] reconcileChildren() adds new children not in s.children
- [ ] reconcileChildren() updates existing children with new UserSpec
- [ ] reconcileChildren() removes children not in specs
- [ ] StateMapping applied correctly (parent state → child state)
- [ ] Recursive tick propagates to all children
- [ ] Circular dependency detection prevents infinite loops

### Quality
- [ ] Unit test coverage >90% (critical foundation phase)
- [ ] All integration tests pass (7 scenarios)
- [ ] No focused specs (`FIt`, `FDescribe`)
- [ ] Ginkgo tests use BDD style (Describe/Context/It)

### Performance
- [ ] reconcileChildren() completes in <10ms for 10 children
- [ ] Tick propagation to 3-level hierarchy <5ms
- [ ] WorkerFactory lookup O(1) time

### Documentation
- [ ] All public methods have godoc comments
- [ ] ChildSpec fields documented with examples
- [ ] Design docs updated with actual implementation details

---

## Review Checkpoint

**After Phase 0 completion:**
- Code review focuses on ChildSpec API design
- Verify WorkerFactory extensibility
- Confirm reconciliation logic is idempotent
- Check integration test coverage

**Next Phase:** [Phase 0.5: Templating & Variables](phase-0.5-templating-variables.md)
