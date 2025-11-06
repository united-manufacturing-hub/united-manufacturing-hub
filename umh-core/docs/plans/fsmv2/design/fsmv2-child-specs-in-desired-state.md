# FSMv2: Child Specs in DesiredState (Kubernetes Pattern)

**Date:** 2025-11-02
**Status:** Analysis
**Proposed By:** User insight during investigation
**Related:**
- `fsmv2-userspec-based-child-updates.md` - Separate methods with version-based updates
- `fsmv2-children-as-desired-state.md` - Analysis showing supervisor instances aren't serializable
- `fsmv2-combined-vs-separate-methods.md` - Analysis of combining methods

## The Proposal

**User's Key Insight:**
> "but shouldnt the desirestate only contain the userspec for the children anyway? then it would be easily serializable. the supervisor would then create new workers, and then call on each tick the user spec into the deriveDesiredState of each children to get the actual desired state."

**Architecture:**
- DesiredState contains **child specifications** (serializable config)
- Supervisor creates child supervisors from specs
- Supervisor manages child lifecycle based on specs
- Each child gets its own UserSpec from parent

**This solves TWO problems simultaneously:**
1. ✅ Children ARE part of desired state (philosophically consistent)
2. ✅ DesiredState is serializable (TriangularStore compatible)

---

## Proposed Data Structures

### DesiredState with Child Specs

```go
type DesiredState struct {
    State         string
    UserSpec      UserSpec
    ChildrenSpecs []ChildSpec  // Serializable specifications
}

type ChildSpec struct {
    Name         string
    WorkerType   string  // "ConnectionWorker", "BenthosWorker", etc.
    UserSpec     UserSpec
    StateMapping map[string]string  // Parent state → child desired state
}
```

### Worker Interface

```go
type Worker interface {
    CollectObservedState(ctx context.Context) (ObservedState, error)

    // Returns complete desired state INCLUDING child specs
    DeriveDesiredState(userSpec UserSpec) DesiredState
}
```

**Single method!** No more `DeclareChildren()` - children are part of DesiredState.

### Supervisor Responsibilities

```go
type Supervisor struct {
    worker         Worker
    factory        WorkerFactory  // NEW: Creates workers from WorkerType string
    children       map[string]*childInstance
    triangularStore TriangularStore
}

type childInstance struct {
    name         string
    supervisor   *Supervisor  // Actual child supervisor
    userSpec     UserSpec     // Child's config
    stateMapping map[string]string
}
```

---

## Comparison to Kubernetes

### Kubernetes Pattern

**Deployment (spec):**
```yaml
apiVersion: apps/v1
kind: Deployment
spec:
  replicas: 3  # How many children
  template:    # SPECIFICATION of each child (not actual Pods)
    metadata:
      labels:
        app: nginx
    spec:
      containers:
        - name: nginx
          image: nginx:1.14.2
```

**Controller:**
```go
func (c *DeploymentController) Reconcile(deployment Deployment) {
    desiredPods := deployment.Spec.Replicas
    currentPods := c.listPods(deployment.Selector)

    // Create pods from template spec
    if len(currentPods) < desiredPods {
        for i := 0; i < desiredPods - len(currentPods); i++ {
            pod := createPodFromTemplate(deployment.Spec.Template)
            c.createPod(pod)
        }
    }

    // Delete excess pods
    if len(currentPods) > desiredPods {
        for i := 0; i < len(currentPods) - desiredPods; i++ {
            c.deletePod(currentPods[i])
        }
    }
}
```

### FSMv2 Equivalent

**Parent Worker:**
```go
func (w *BridgeWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    return DesiredState{
        State: "active",
        ChildrenSpecs: []ChildSpec{  // SPECIFICATION of children
            {
                Name:       "connection",
                WorkerType: "ConnectionWorker",
                UserSpec:   UserSpec{IPAddress: userSpec.IPAddress},
                StateMapping: map[string]string{"active": "up", "idle": "down"},
            },
            {
                Name:       "read_flow",
                WorkerType: "BenthosWorker",
                UserSpec:   UserSpec{Config: "..."},
                StateMapping: map[string]string{"active": "running", "idle": "stopped"},
            },
        },
    }
}
```

**Supervisor:**
```go
func (s *Supervisor) Tick(ctx context.Context) error {
    userSpec := s.triangularStore.GetUserSpec()

    // Get desired state with child specs
    desiredState := s.worker.DeriveDesiredState(userSpec)

    // Reconcile children from specs
    s.reconcileChildren(desiredState.ChildrenSpecs)

    // Tick children
    for _, child := range s.children {
        child.supervisor.Tick(ctx)
    }
}

func (s *Supervisor) reconcileChildren(specs []ChildSpec) {
    // Create children from specs
    for _, spec := range specs {
        if _, exists := s.children[spec.Name]; !exists {
            worker := s.factory.CreateWorker(spec.WorkerType, spec.UserSpec)
            supervisor := NewSupervisor(worker, s.logger)
            s.children[spec.Name] = &childInstance{
                name:         spec.Name,
                supervisor:   supervisor,
                userSpec:     spec.UserSpec,
                stateMapping: spec.StateMapping,
            }
        }
    }

    // Remove children not in specs
    for name := range s.children {
        found := false
        for _, spec := range specs {
            if spec.Name == name {
                found = true
                break
            }
        }
        if !found {
            s.removeChild(name)
        }
    }
}
```

**THIS IS EXACTLY THE KUBERNETES PATTERN!**

---

## Serialization Analysis

### Is ChildSpec Fully Serializable?

```go
type ChildSpec struct {
    Name         string                 // ✅ Serializable
    WorkerType   string                 // ✅ Serializable
    UserSpec     UserSpec               // ❓ Depends on UserSpec contents
    StateMapping map[string]string      // ✅ Serializable
}
```

**Critical Question:** Is UserSpec serializable?

**Typical UserSpec:**
```go
type UserSpec struct {
    Version    int64
    IPAddress  string
    Port       int
    Servers    []string
    Config     map[string]interface{}  // ❓ interface{} might not serialize
    // ...
}
```

**If UserSpec contains:**
- Primitives (string, int, bool): ✅ Serializable
- Maps, slices: ✅ Serializable
- Nested structs: ✅ Serializable
- Pointers to structs: ✅ Serializable (pointer value serialized)
- `interface{}`: ⚠️ Depends on actual type (JSON can handle, gob needs registration)
- Channels, functions, goroutines: ❌ Not serializable

**Recommendation:** Ensure UserSpec only contains serializable types.

### TriangularStore Integration

```go
type TriangularStore struct {
    Identity     Identity
    DesiredState DesiredState  // Now contains ChildrenSpecs!
    ObservedState ObservedState
}

// Persist to disk
data, _ := json.Marshal(store)
os.WriteFile("/data/state.json", data, 0644)

// Restore from disk
data, _ := os.ReadFile("/data/state.json")
json.Unmarshal(data, &store)

// After restore, supervisor must recreate child supervisors from specs!
for _, spec := range store.DesiredState.ChildrenSpecs {
    worker := factory.CreateWorker(spec.WorkerType, spec.UserSpec)
    supervisor := NewSupervisor(worker, logger)
    children[spec.Name] = &childInstance{supervisor: supervisor, ...}
}
```

**Full desired state is now persisted and restorable!**

---

## WorkerFactory Pattern

### Interface

```go
type WorkerFactory interface {
    CreateWorker(workerType string, userSpec UserSpec) (Worker, error)
}
```

### Implementation

```go
type DefaultWorkerFactory struct {
    logger *zap.Logger
}

func (f *DefaultWorkerFactory) CreateWorker(workerType string, userSpec UserSpec) (Worker, error) {
    switch workerType {
    case "ConnectionWorker":
        return newConnectionWorker(userSpec, f.logger), nil
    case "BenthosWorker":
        return newBenthosWorker(userSpec, f.logger), nil
    case "OPCUABrowserWorker":
        return newOPCUABrowserWorker(userSpec, f.logger), nil
    case "OPCUAServerBrowserWorker":
        return newOPCUAServerBrowserWorker(userSpec, f.logger), nil
    default:
        return nil, fmt.Errorf("unknown worker type: %s", workerType)
    }
}
```

### Type Safety Options

**Option 1: Constants (Recommended)**

```go
const (
    WorkerTypeConnection         = "ConnectionWorker"
    WorkerTypeBenthos           = "BenthosWorker"
    WorkerTypeOPCUABrowser      = "OPCUABrowserWorker"
    WorkerTypeOPCUAServerBrowser = "OPCUAServerBrowserWorker"
)

// Usage
ChildSpec{
    Name:       "connection",
    WorkerType: WorkerTypeConnection,  // Type-safe constant
    UserSpec:   userSpec,
}
```

**Option 2: Validation in Supervisor**

```go
func (s *Supervisor) validateChildSpec(spec ChildSpec) error {
    if _, err := s.factory.CreateWorker(spec.WorkerType, spec.UserSpec); err != nil {
        return fmt.Errorf("invalid child spec: %w", err)
    }
    return nil
}
```

**Option 3: Registration System**

```go
var workerRegistry = map[string]func(UserSpec, *zap.Logger) Worker{}

func RegisterWorker(name string, constructor func(UserSpec, *zap.Logger) Worker) {
    workerRegistry[name] = constructor
}

func init() {
    RegisterWorker("ConnectionWorker", func(spec UserSpec, logger *zap.Logger) Worker {
        return newConnectionWorker(spec, logger)
    })
    RegisterWorker("BenthosWorker", func(spec UserSpec, logger *zap.Logger) Worker {
        return newBenthosWorker(spec, logger)
    })
}
```

---

## Code Examples

### Bridge Worker (Simplified)

```go
type BridgeWorker struct {
    // NO cached supervisors needed!
    // NO version tracking needed!
}

func (w *BridgeWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    return DesiredState{
        State:    "active",
        UserSpec: userSpec,
        ChildrenSpecs: []ChildSpec{
            {
                Name:       "connection",
                WorkerType: WorkerTypeConnection,
                UserSpec: UserSpec{
                    IPAddress: userSpec.IPAddress,
                    Port:      userSpec.Port,
                },
                StateMapping: map[string]string{
                    "active": "up",
                    "idle":   "down",
                },
            },
            {
                Name:       "read_flow",
                WorkerType: WorkerTypeBenthos,
                UserSpec: UserSpec{
                    IPAddress: userSpec.IPAddress,
                    Config:    generateBenthosConfig(userSpec),
                },
                StateMapping: map[string]string{
                    "active": "running",
                    "idle":   "stopped",
                },
            },
        },
    }
}

func (w *BridgeWorker) CollectObservedState(ctx context.Context) (ObservedState, error) {
    return ObservedState{State: "active"}, nil
}
```

**Lines of code:** ~15 lines (vs 20 in separate methods)

### OPC UA Browser (Dynamic N Children)

```go
type OPCUABrowserWorker struct {
    // NO child supervisor map needed!
}

func (w *OPCUABrowserWorker) DeriveDesiredState(userSpec UserSpec) DesiredState {
    childSpecs := []ChildSpec{}

    for _, serverAddr := range userSpec.Servers {
        childSpecs = append(childSpecs, ChildSpec{
            Name:       serverAddr,
            WorkerType: WorkerTypeOPCUAServerBrowser,
            UserSpec: UserSpec{
                ServerAddress: serverAddr,
            },
            StateMapping: map[string]string{
                "active": "browsing",
                "idle":   "stopped",
            },
        })
    }

    return DesiredState{
        State:         "active",
        UserSpec:      userSpec,
        ChildrenSpecs: childSpecs,
    }
}
```

**Lines of code:** ~12 lines (vs 25 in separate methods with lazy creation)

### Supervisor Logic

```go
type Supervisor struct {
    worker          Worker
    factory         WorkerFactory
    children        map[string]*childInstance
    triangularStore TriangularStore
    logger          *zap.Logger
}

func (s *Supervisor) Tick(ctx context.Context) error {
    userSpec := s.triangularStore.GetUserSpec()

    // 1. Get desired state with child specs
    desiredState := s.worker.DeriveDesiredState(userSpec)

    // 2. Persist desired state (now includes children!)
    s.triangularStore.SetDesiredState(desiredState)

    // 3. Reconcile children from specs
    s.reconcileChildren(desiredState.ChildrenSpecs)

    // 4. Apply state mapping to children
    for name, child := range s.children {
        if childDesiredState, ok := child.stateMapping[desiredState.State]; ok {
            child.supervisor.SetDesiredState(childDesiredState)
        }
    }

    // 5. Tick each child with its userSpec
    for _, child := range s.children {
        // Child supervisor calls DeriveDesiredState(child.userSpec)
        child.supervisor.Tick(ctx)
    }

    // 6. Collect observed state
    observedState, _ := s.worker.CollectObservedState(ctx)
    s.triangularStore.SetObservedState(observedState)

    return nil
}

func (s *Supervisor) reconcileChildren(specs []ChildSpec) error {
    // Build map of desired children
    desiredChildren := make(map[string]ChildSpec)
    for _, spec := range specs {
        desiredChildren[spec.Name] = spec
    }

    // Add or update children
    for name, spec := range desiredChildren {
        if child, exists := s.children[name]; exists {
            // Child exists - check if UserSpec changed
            if !reflect.DeepEqual(child.userSpec, spec.UserSpec) {
                s.logger.Info("Child UserSpec changed, updating",
                    "child", name,
                    "old", child.userSpec,
                    "new", spec.UserSpec,
                )

                // Recreate child supervisor with new UserSpec
                worker, err := s.factory.CreateWorker(spec.WorkerType, spec.UserSpec)
                if err != nil {
                    s.logger.Error("Failed to create worker", "type", spec.WorkerType, "error", err)
                    continue
                }

                // Shutdown old supervisor
                child.supervisor.Shutdown()

                // Create new supervisor
                child.supervisor = NewSupervisor(worker, s.logger)
                child.userSpec = spec.UserSpec
                child.stateMapping = spec.StateMapping
            }
        } else {
            // Child doesn't exist - create it
            s.logger.Info("Adding child", "name", name, "type", spec.WorkerType)

            worker, err := s.factory.CreateWorker(spec.WorkerType, spec.UserSpec)
            if err != nil {
                s.logger.Error("Failed to create worker", "type", spec.WorkerType, "error", err)
                continue
            }

            supervisor := NewSupervisor(worker, s.logger)

            s.children[name] = &childInstance{
                name:         name,
                supervisor:   supervisor,
                userSpec:     spec.UserSpec,
                stateMapping: spec.StateMapping,
            }
        }
    }

    // Remove children not in desired specs
    for name := range s.children {
        if _, desired := desiredChildren[name]; !desired {
            s.logger.Info("Removing child", "name", name)
            s.removeChild(name)
        }
    }

    return nil
}

func (s *Supervisor) removeChild(name string) {
    if child, exists := s.children[name]; exists {
        child.supervisor.Shutdown()
        delete(s.children, name)
    }
}
```

**Lines of code:** ~90 lines (vs 70 in separate methods)

**Added complexity:**
- WorkerFactory dependency
- DeepEqual check on UserSpec
- Child supervisor recreation on config change

---

## Performance Analysis

### Per-Tick Operations

**Assumptions:**
- 100 bridges
- 1 second tick interval
- 1 config change per hour per bridge

### Cost Breakdown

**DeriveDesiredState (called every tick):**
- Create ChildSpec structs: ~1μs per child
- Bridge has 3 children: ~3μs
- 100 bridges: ~300μs total

**reconcileChildren (called every tick):**
- Build desiredChildren map: ~5μs
- Iterate current children: ~10μs per bridge
- DeepEqual UserSpec check: ~2μs per child (if small struct)
- 100 bridges × 3 children × 2μs = ~600μs
- **Total per tick: ~1ms**

**Child supervisor creation (only on config change):**
- Create worker: ~50μs
- Create supervisor: ~100μs
- Per child: ~150μs
- Bridge with 3 children: ~450μs
- **Only happens on config change (~1/hour per bridge)**

### Comparison to Separate Methods

| Approach | Per Tick Cost | Config Change Cost | Per Hour Cost |
|----------|---------------|-------------------|---------------|
| **Separate methods** | ~0.1ms (version check) | ~300μs (create supervisors) | ~360ms + 30ms = 390ms |
| **Specs in DesiredState** | ~1ms (DeepEqual checks) | ~450μs (create supervisors) | ~3.6s + 45ms = 3.6s |

**Performance Degradation:** ~9x worse (3.6s vs 390ms per hour)

**BUT:** Still negligible for reasonable workloads (<1000 bridges).

**Optimization:** Cache hash of UserSpec to avoid DeepEqual every tick:

```go
type childInstance struct {
    userSpec     UserSpec
    userSpecHash string  // SHA256 of UserSpec
    // ...
}

func (s *Supervisor) reconcileChildren(specs []ChildSpec) {
    for name, spec := range desiredChildren {
        if child, exists := s.children[name]; exists {
            newHash := hashUserSpec(spec.UserSpec)
            if child.userSpecHash != newHash {
                // UserSpec changed
                child.userSpecHash = newHash
                // Recreate supervisor
            }
        }
    }
}
```

With hashing: ~same performance as separate methods.

---

## Advantages

1. **✅ Philosophically Consistent**
   - Children ARE part of desired state
   - Matches Kubernetes, Docker Compose, Terraform patterns exactly
   - DesiredState is complete specification of system

2. **✅ Fully Serializable**
   - TriangularStore can persist full desired state
   - Can restore system from disk
   - Can send desired state over network

3. **✅ Simpler Worker Code**
   - No supervisor instance caching
   - No version tracking
   - Just return specs: 15 lines vs 20 lines

4. **✅ Single Method**
   - No DeclareChildren() method
   - Everything in DeriveDesiredState()
   - Cleaner interface

5. **✅ Declarative**
   - Worker declares "what should exist"
   - Supervisor handles "how to make it exist"
   - Clear separation of concerns

6. **✅ State Restoration**
   - Can restart supervisor and restore children from persisted specs
   - No need to remember which supervisors to create

---

## Disadvantages

1. **❌ WorkerFactory Required**
   - New component to maintain
   - String-based worker type registration
   - Need to register all worker types

2. **❌ Type Safety Concerns**
   - WorkerType is string, not type-safe
   - Typos possible: "ConectionWorker" instead of "ConnectionWorker"
   - Mitigated by constants and validation

3. **❌ More Complex Supervisor**
   - +20 lines vs separate methods (90 vs 70)
   - Factory dependency
   - DeepEqual checks every tick
   - Child supervisor recreation logic

4. **❌ Performance Overhead**
   - DeepEqual on UserSpec every tick for every child
   - 9x more CPU per hour (still negligible: 3.6s vs 390ms)
   - Mitigated by hashing optimization

5. **❌ UserSpec Serialization Assumption**
   - Requires UserSpec to be serializable
   - May limit future UserSpec designs
   - Need to enforce in validation

---

## Comparison Table

| Aspect | Separate Methods | Specs in DesiredState |
|--------|------------------|----------------------|
| **API Complexity** | 2 methods (DeriveDesiredState + DeclareChildren) | 1 method (DeriveDesiredState) |
| **Worker Code** | 20 lines | 15 lines |
| **Supervisor Code** | 70 lines | 90 lines |
| **Total Code** | 90 lines | 105 lines |
| **Worker Complexity** | Medium (version tracking) | Low (just return specs) |
| **Supervisor Complexity** | Medium (version check) | High (factory + DeepEqual + recreation) |
| **Serializable** | Partial (state yes, children no) | ✅ Full (state + child specs) |
| **TriangularStore** | Incomplete (no children) | ✅ Complete (children included) |
| **Matches K8s** | Partial | ✅ Exactly |
| **Type Safety** | ✅ Strong (no strings) | ⚠️ Weak (string worker types) |
| **Dependencies** | None | WorkerFactory |
| **Performance** | 390ms/hour | 3.6s/hour (9x worse) |
| **Optimization** | N/A | Hash-based comparison |
| **State Restoration** | ❌ Manual | ✅ Automatic |
| **Philosophical** | Pragmatic | ✅ Consistent |

---

## Decision Criteria

### Choose Specs in DesiredState If:

- ✅ TriangularStore state persistence is important
- ✅ Philosophical consistency matters (children ARE desired state)
- ✅ Matching industry patterns (K8s) is valuable
- ✅ Automatic state restoration needed
- ✅ Worker code simplicity prioritized over supervisor simplicity
- ✅ Performance overhead negligible (< 1000 workers)

### Choose Separate Methods If:

- ✅ Type safety is critical (no string-based types)
- ✅ Minimal dependencies preferred (no WorkerFactory)
- ✅ Simpler supervisor implementation valued
- ✅ Performance optimization important (9x faster)
- ✅ Pragmatic over philosophical consistency
- ✅ Existing codebase momentum

---

## Recommendation

### **ADOPT Specs in DesiredState**

**Rationale:**

1. **Kubernetes Pattern Match**
   - This is EXACTLY how Kubernetes works
   - Industry-proven pattern for hierarchical systems
   - Future developers will understand immediately

2. **Philosophical Consistency**
   - Resolves the "are children desired state?" question definitively
   - DesiredState is now complete system specification
   - TriangularStore contains full truth

3. **Serialization Solved**
   - Full state persistence and restoration
   - Can snapshot system state
   - Can restore after crashes

4. **Worker Code Simplification**
   - 25% less code (15 vs 20 lines)
   - No caching logic
   - No version tracking
   - Just declare "what should exist"

5. **Performance Acceptable**
   - 3.6s per hour for 100 bridges
   - Negligible for production workloads
   - Can optimize with hashing if needed

6. **Single Method Interface**
   - Cleaner API
   - Everything in DeriveDesiredState()
   - No confusion about when to call what

**Trade-offs Accepted:**

1. **WorkerFactory Dependency**
   - Needed anyway for plugin systems
   - Standard pattern in Go (interface registration)
   - Can be simple (switch statement)

2. **String-Based Worker Types**
   - Mitigated with constants
   - Validation catches errors early
   - Same pattern as Kubernetes (kind: Deployment)

3. **Supervisor Complexity**
   - +20 lines is acceptable
   - Complexity in right place (supervisor, not worker)
   - Workers are written more often than supervisors

4. **DeepEqual Overhead**
   - Optimize with hashing if needed
   - Only matters at 1000+ workers
   - Premature optimization to avoid

---

## Implementation Steps

### Phase 1: Core Changes

1. **Update DesiredState struct:**
   ```go
   type DesiredState struct {
       State         string
       UserSpec      UserSpec
       ChildrenSpecs []ChildSpec
   }
   ```

2. **Add ChildSpec:**
   ```go
   type ChildSpec struct {
       Name         string
       WorkerType   string
       UserSpec     UserSpec
       StateMapping map[string]string
   }
   ```

3. **Create WorkerFactory:**
   ```go
   type WorkerFactory interface {
       CreateWorker(workerType string, userSpec UserSpec) (Worker, error)
   }
   ```

4. **Update Worker interface (remove DeclareChildren):**
   ```go
   type Worker interface {
       CollectObservedState(ctx) (ObservedState, error)
       DeriveDesiredState(userSpec) DesiredState  // Returns children in DesiredState
   }
   ```

### Phase 2: Supervisor Implementation

1. Implement `reconcileChildren(specs []ChildSpec)`
2. Add factory to supervisor constructor
3. Update `Tick()` to use specs from DesiredState

### Phase 3: Worker Migration

1. Update BridgeWorker to return ChildrenSpecs
2. Update OPCUABrowserWorker
3. Update other hierarchical workers

### Phase 4: Optimization

1. Add UserSpec hashing if performance becomes issue
2. Add worker type validation
3. Improve error messages

---

## Conclusion

The "specs in DesiredState" approach solves the fundamental tension between philosophical consistency (children ARE desired state) and practical constraints (serialization).

By separating **specification** (what should exist) from **runtime** (actual supervisors), we get:
- ✅ Full serialization
- ✅ Kubernetes pattern match
- ✅ Simpler worker code
- ✅ Complete TriangularStore
- ✅ Automatic state restoration

The trade-offs (WorkerFactory, string types, DeepEqual overhead) are acceptable for the benefits gained.

**This is the cleanest architectural solution to the hierarchical composition problem.**
