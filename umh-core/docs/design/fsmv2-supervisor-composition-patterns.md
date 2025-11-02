# FSMv2 Supervisor-of-Supervisors Composition Patterns

**Created:** 2025-11-01
**Status:** Design Analysis & Recommendation
**Author:** Claude Code

---

## Executive Summary

**Recommendation: Neither pattern as described. Use Pattern C (Direct Integration).**

After applying the Inversion Exercise, Architecture Simplification, and Preserving Productive Tensions skills, the analysis reveals that both Pattern A (Nested Composition) and Pattern B (Shared Global Supervisor) add unnecessary abstraction layers. The direct path is simpler: Benthos FSM directly manages S6 services without a separate S6 Supervisor.

**Key insight from Inversion Exercise:**
> "What if S6 doesn't need a supervisor at all? What if it's just a process management library that Benthos FSM uses directly?"

---

## Table of Contents

1. [Inversion Exercise: Challenging Core Assumptions](#1-inversion-exercise)
2. [Pattern A: Nested Composition (Benthos Owns S6)](#2-pattern-a-nested-composition)
3. [Pattern B: Shared Global Supervisor (Single S6 Supervisor)](#3-pattern-b-shared-global-supervisor)
4. [Pattern C: Direct Integration (Recommended)](#4-pattern-c-direct-integration)
5. [Architecture Simplification Analysis](#5-architecture-simplification-analysis)
6. [Productive Tensions Analysis](#6-productive-tensions-analysis)
7. [Comparison Matrix](#7-comparison-matrix)
8. [Migration Strategy](#8-migration-strategy)
9. [Code Examples](#9-code-examples)
10. [Recommendation Rationale](#10-recommendation-rationale)

---

## 1. Inversion Exercise: Challenging Core Assumptions

Following the Inversion Exercise skill, I'll flip every assumption and see what still works.

### Assumption 1: "S6 needs a supervisor"

**Inverted:** S6 is just a library, not a supervised entity

**What it reveals:**
- S6 is already a process supervisor (s6-svscan, s6-supervise)
- Creating a "supervisor for the supervisor" = meta-abstraction without value
- Benthos FSM can interact with S6 directly via filesystem operations

**Valid inversion?** YES. S6 doesn't need supervision, it IS the supervisor.

---

### Assumption 2: "Supervisors must compose hierarchically"

**Inverted:** Supervisors are independent, coordination happens via shared state

**What it reveals:**
- FSM v2 uses TriangularStore for coordination, not supervisor nesting
- Benthos FSM knows when S6 is ready by checking filesystem (service directories)
- No need for BenthosWorker to contain an S6Supervisor

**Valid inversion?** YES. Flat is better than nested.

---

### Assumption 3: "Multiple Benthos instances need separate S6 supervisors"

**Inverted:** All Benthos instances share one S6 process supervisor

**What it reveals:**
- S6 is singleton (one s6-svscan per system)
- Multiple Benthos FSMs can create service directories under `/data/services/`
- S6 automatically detects and supervises all services
- No need for per-Benthos S6 supervisor

**Valid inversion?** YES. S6 is naturally multi-tenant.

---

### Assumption 4: "Abstraction reduces complexity"

**Inverted:** Removing abstraction reveals simpler patterns

**What it reveals:**
- Benthos FSM → S6 filesystem operations (2 steps)
- Pattern A: Benthos → S6 Supervisor → S6 filesystem (3 steps)
- Pattern B: Benthos → Global S6 Supervisor → S6 filesystem (3 steps)
- Direct path eliminates entire supervisor layer

**Valid inversion?** YES. See Architecture Simplification below.

---

### Assumption 5: "S6 state needs observation collection"

**Inverted:** S6 state is directly readable from filesystem

**What it reveals:**
```go
// No need for ObservedState polling
func CheckS6ServiceExists(name string) bool {
    return fileExists(fmt.Sprintf("/data/services/%s/run", name))
}

func CheckS6ServiceRunning(name string) bool {
    pid, _ := readPidFile(fmt.Sprintf("/data/services/%s/supervise/pid", name))
    return processExists(pid)
}
```

S6 state is synchronous and cheap to read. No async collection needed.

**Valid inversion?** YES. S6 doesn't fit the Worker pattern.

---

### Inversion Exercise Conclusions

**Questions that revealed truth:**
1. "What if S6 is a library, not a worker?" → It is
2. "What if we don't need a supervisor for it?" → We don't
3. "What if direct filesystem operations are simpler?" → They are

**Architecture insight:**

```
Current thinking (wrong):
┌─────────────────┐
│ Benthos Worker  │
│  ├─ S6 Supervisor
│  └─ Benthos FSM │
└─────────────────┘

Inverted (correct):
┌─────────────────┐
│ Benthos Worker  │
│  ├─ S6 client   │  (direct filesystem ops)
│  └─ FSM logic   │
└─────────────────┘
```

**Proceeding with this insight to evaluate patterns A, B, and propose Pattern C.**

---

## 2. Pattern A: Nested Composition (Benthos Owns S6)

### 2.1 Pattern Description

```
BenthosWorker
  └─ Internal S6 Supervisor
      └─ S6 Worker (for Benthos process)
```

**Characteristics:**
- User requests "Benthos worker" → gets everything needed
- S6 services hidden from user (abstraction)
- Benthos manages its own S6 lifecycle
- One Benthos worker = one internal S6 supervisor

### 2.2 Architecture Diagram

```
┌────────────────────────────────────────────────┐
│ Main Supervisor                                │
│                                                │
│  ┌──────────────────────────────────────────┐ │
│  │ BenthosWorker (implements Worker)        │ │
│  │                                          │ │
│  │  ┌────────────────────────────────────┐ │ │
│  │  │ Internal S6 Supervisor             │ │ │
│  │  │                                    │ │ │
│  │  │  ┌──────────────────────────────┐ │ │ │
│  │  │  │ S6Worker                     │ │ │ │
│  │  │  │ - CollectObservedState()     │ │ │ │
│  │  │  │   → checks /data/services/   │ │ │ │
│  │  │  │ - DeriveDesiredState()       │ │ │ │
│  │  │  │   → from benthos config      │ │ │ │
│  │  │  └──────────────────────────────┘ │ │ │
│  │  │                                    │ │ │
│  │  │  States: S6Starting, S6Running    │ │ │
│  │  └────────────────────────────────────┘ │ │
│  │                                          │ │
│  │  States: BenthosStarting, BenthosRunning│ │
│  └──────────────────────────────────────────┘ │
│                                                │
└────────────────────────────────────────────────┘
```

### 2.3 Implementation Pattern

```go
type BenthosWorker struct {
    s6Supervisor  *supervisor.Supervisor  // Internal
    benthosConfig BenthosConfig
}

func (w *BenthosWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
    // Delegate to internal S6 supervisor
    s6State := w.s6Supervisor.GetCurrentState()

    // Combine with Benthos-specific observations
    return &BenthosObservedState{
        S6State:        s6State,
        ProcessRunning: checkBenthosProcess(),
        ConfigValid:    validateConfig(w.benthosConfig),
    }, nil
}

func (w *BenthosWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
    config := spec.(BenthosConfig)

    // Derive S6 service config
    s6Config := convertToS6Config(config)

    // Update internal S6 supervisor's desired state
    w.s6Supervisor.UpdateDesiredState(s6Config)

    return &BenthosDesiredState{
        S6Config:      s6Config,
        BenthosConfig: config,
    }, nil
}
```

### 2.4 Pros

**Encapsulation:**
- S6 complexity hidden from user
- User thinks in terms of "Benthos worker", not "S6 + Benthos"
- Clean API boundary

**Ownership clarity:**
- Benthos owns its S6 lifecycle
- No shared state between Benthos instances
- Each worker self-contained

**Testability:**
- Can mock internal S6 supervisor
- Test Benthos logic in isolation
- Clear dependency injection points

**Flexibility:**
- Each Benthos can have different S6 config
- No global coupling

### 2.5 Cons

**Abstraction leak:**
- S6 state leaks into `BenthosObservedState`
- User sees S6 errors through Benthos interface
- "Starting Benthos" might mean "waiting for S6"

**Complexity:**
- Supervisor-in-supervisor = meta-pattern
- Tick propagation: Main tick → Benthos tick → S6 tick
- Two FSMs to debug instead of one

**Resource overhead:**
- Each Benthos worker = 2 supervisors (main + internal)
- 2 Collector goroutines per worker
- 2 action executors per worker

**Separation of concerns violation:**
- Benthos knows about S6 internals
- Can't reuse S6 supervisor for other services (Redpanda, etc.)
- Business logic (Benthos) entangled with infrastructure (S6)

**Conversion steps (Architecture Simplification):**
```
BenthosConfig
  → DeriveDesiredState() → BenthosDesiredState
  → Extract S6Config → S6DesiredState
  → S6 Actions → Filesystem
```
4 steps when 2 steps would suffice (see Pattern C).

### 2.6 When This Pattern Makes Sense

- Each Benthos instance needs radically different S6 configuration
- S6 state is complex and needs full FSM lifecycle
- User must never see S6 abstraction

**Reality check:** None of these apply. S6 config is uniform, S6 state is simple (service exists or not), and power users will need to understand S6 anyway for debugging.

---

## 3. Pattern B: Shared Global Supervisor (Single S6 Supervisor)

### 3.1 Pattern Description

```
Global S6 Supervisor
  ├─ S6 Worker (managed by Benthos)
  ├─ S6 Worker (managed by Redpanda)
  └─ S6 Worker (managed by other services)

Benthos Manager
  └─ Adds workers to global S6 Supervisor
```

**Characteristics:**
- One global S6 supervisor for all S6 services
- Benthos/Redpanda/etc register their S6 workers with it
- User sees S6 services (less abstraction)
- Centralized S6 management

### 3.2 Architecture Diagram

```
┌────────────────────────────────────────────────────────┐
│ Global S6 Supervisor                                   │
│                                                        │
│  ┌──────────────────┐  ┌──────────────────┐          │
│  │ S6Worker         │  │ S6Worker         │          │
│  │ (benthos-bridge) │  │ (redpanda)       │  ...     │
│  │                  │  │                  │          │
│  │ - Filesystem ops │  │ - Filesystem ops │          │
│  │ - S6 status      │  │ - S6 status      │          │
│  └──────────────────┘  └──────────────────┘          │
│                                                        │
└────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────┐
│ Benthos FSM                                            │
│                                                        │
│  - Generates benthos config                           │
│  - Adds S6Worker to global supervisor                 │
│  - Observes S6Worker state via supervisor             │
└────────────────────────────────────────────────────────┘
```

### 3.3 Implementation Pattern

```go
// Global S6 supervisor (singleton)
var globalS6Supervisor *supervisor.Supervisor

func init() {
    globalS6Supervisor = supervisor.NewSupervisor(supervisor.Config{
        WorkerType: "s6-service",
        // ...
    })
}

// Benthos FSM registers S6 worker
func (f *BenthosFSM) CreateService(config BenthosConfig) error {
    // Create S6 service directory
    createS6ServiceDir(config.ServiceName, config)

    // Register worker with global supervisor
    s6Worker := &S6Worker{
        serviceName: config.ServiceName,
    }

    globalS6Supervisor.AddWorker(fsmv2.Identity{
        ID:   config.ServiceName,
        Name: fmt.Sprintf("s6-%s", config.ServiceName),
    }, s6Worker)

    return nil
}

// Benthos FSM observes S6 state
func (f *BenthosFSM) GetS6State(serviceName string) (string, error) {
    return globalS6Supervisor.GetWorkerState(serviceName)
}
```

### 3.4 Pros

**Centralized management:**
- One supervisor for all S6 services
- Single source of truth for S6 state
- Easy to query "all S6 services"

**No duplication:**
- One Collector goroutine for all S6 workers
- Shared observation logic
- Shared action execution

**Reusability:**
- Redpanda, Benthos, other services use same S6 supervisor
- S6Worker implementation reused

**Visibility:**
- User can query S6 supervisor directly
- Debugging: "what S6 services exist?"
- Clear separation: Benthos FSM vs S6 Supervisor

### 3.5 Cons

**Coupling:**
- Benthos FSM depends on global S6 supervisor
- S6 supervisor must exist before Benthos can start
- Initialization order matters

**Lifecycle coordination:**
- Who owns S6 worker lifecycle? Benthos or S6 supervisor?
- What if Benthos needs to remove S6 worker?
- What if S6 supervisor restarts? Does Benthos re-register?

**Global state:**
- Singleton supervisor = global mutable state
- Testing requires global setup/teardown
- Hard to isolate tests

**Abstraction mismatch:**
- S6 is not a "worker" in the Worker interface sense
- S6 doesn't have async state collection (filesystem is synchronous)
- Forcing S6 into Worker pattern adds unnecessary complexity

**Conversion steps:**
```
BenthosConfig
  → CreateService() → S6 service directory
  → AddWorker() → S6Worker
  → Supervisor manages worker → S6 state
```
4 steps when 2 steps would suffice (see Pattern C).

### 3.6 When This Pattern Makes Sense

- S6 state is complex and needs full observation collection
- Multiple components need to coordinate S6 lifecycle
- User must be able to query "all S6 services" independent of parent service

**Reality check:** S6 state is simple (service exists or not), lifecycle is owned by parent service (Benthos), and querying S6 directly can be done via filesystem without supervisor.

---

## 4. Pattern C: Direct Integration (Recommended)

### 4.1 Pattern Description

**No S6 Supervisor. Benthos FSM directly manages S6 services via filesystem operations.**

```
Benthos Worker
  ├─ Benthos FSM (states, actions)
  ├─ S6 Client (filesystem operations)
  └─ CollectObservedState() checks both Benthos AND S6 state
```

**Characteristics:**
- S6 is a library, not a worker
- Filesystem operations are synchronous and cheap
- No supervisor-in-supervisor nesting
- No global S6 coordinator
- Direct path: Config → Filesystem → Observation

### 4.2 Architecture Diagram

```
┌────────────────────────────────────────────────┐
│ Main Supervisor                                │
│                                                │
│  ┌──────────────────────────────────────────┐ │
│  │ BenthosWorker (implements Worker)        │ │
│  │                                          │ │
│  │  ┌────────────────────────────────────┐ │ │
│  │  │ S6Client (filesystem helper)       │ │ │
│  │  │                                    │ │ │
│  │  │  func CreateService(name, config) │ │ │
│  │  │  func ServiceExists(name) bool    │ │ │
│  │  │  func ServiceRunning(name) bool   │ │ │
│  │  │  func RemoveService(name)         │ │ │
│  │  └────────────────────────────────────┘ │ │
│  │                                          │ │
│  │  CollectObservedState():                 │ │
│  │    - Check S6 service exists             │ │
│  │    - Check process running               │ │
│  │    - Check config valid                  │ │
│  │                                          │ │
│  │  States:                                 │ │
│  │    - TryingToStartState                  │ │
│  │      → CreateServiceAction               │ │
│  │    - RunningState                        │ │
│  │    - StoppingState                       │ │
│  │      → RemoveServiceAction               │ │
│  └──────────────────────────────────────────┘ │
│                                                │
└────────────────────────────────────────────────┘
```

### 4.3 Implementation Pattern

```go
// S6 client (not a worker, just a library)
type S6Client struct {
    servicesDir string  // /data/services/
}

func (c *S6Client) CreateService(name string, config S6ServiceConfig) error {
    serviceDir := filepath.Join(c.servicesDir, name)

    // Create service directory structure
    if err := os.MkdirAll(serviceDir, 0755); err != nil {
        return err
    }

    // Write run script
    runScript := config.GenerateRunScript()
    if err := os.WriteFile(filepath.Join(serviceDir, "run"), []byte(runScript), 0755); err != nil {
        return err
    }

    // S6 automatically detects and starts the service
    return nil
}

func (c *S6Client) ServiceExists(name string) bool {
    serviceDir := filepath.Join(c.servicesDir, name)
    _, err := os.Stat(filepath.Join(serviceDir, "run"))
    return err == nil
}

func (c *S6Client) ServiceRunning(name string) bool {
    pidFile := filepath.Join(c.servicesDir, name, "supervise", "pid")
    pidBytes, err := os.ReadFile(pidFile)
    if err != nil {
        return false
    }

    pid, _ := strconv.Atoi(string(pidBytes))
    return processExists(pid)
}

func (c *S6Client) RemoveService(name string) error {
    serviceDir := filepath.Join(c.servicesDir, name)

    // Create down file to stop service
    downFile := filepath.Join(serviceDir, "down")
    os.WriteFile(downFile, []byte{}, 0644)

    // Wait for S6 to stop the service
    time.Sleep(2 * time.Second)

    // Remove service directory
    return os.RemoveAll(serviceDir)
}

// Benthos Worker uses S6Client directly
type BenthosWorker struct {
    s6Client      *S6Client
    benthosConfig BenthosConfig
}

func (w *BenthosWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
    serviceName := w.benthosConfig.ServiceName

    return &BenthosObservedState{
        S6ServiceExists:  w.s6Client.ServiceExists(serviceName),
        S6ServiceRunning: w.s6Client.ServiceRunning(serviceName),
        ProcessRunning:   w.s6Client.ServiceRunning(serviceName),  // Same check
        ConfigValid:      validateConfig(w.benthosConfig),
    }, nil
}

func (w *BenthosWorker) GetInitialState() fsmv2.State {
    return &TryingToStartState{}
}

// Action to create S6 service
type CreateServiceAction struct {
    s6Client    *S6Client
    serviceName string
    config      S6ServiceConfig
}

func (a *CreateServiceAction) Execute(ctx context.Context) error {
    return a.s6Client.CreateService(a.serviceName, a.config)
}

func (a *CreateServiceAction) Name() string {
    return fmt.Sprintf("CreateS6Service(%s)", a.serviceName)
}
```

### 4.4 Pros

**Simplicity:**
- No supervisor nesting
- No global coordinator
- Direct filesystem operations (2 steps)
- Easy to understand

**Performance:**
- No extra goroutines
- No observation polling (filesystem reads are synchronous)
- No supervisor overhead

**Flexibility:**
- Each Benthos worker has its own S6Client
- No shared state
- No coupling between Benthos instances

**Testability:**
- Mock S6Client interface
- No global supervisor to setup/teardown
- Test in isolation

**YAGNI compliance:**
- Only adds what's needed
- No speculative abstraction
- Can add S6 Supervisor later if needed (but won't be)

**Conversion steps:**
```
BenthosConfig
  → CreateServiceAction → S6Client.CreateService() → Filesystem
```
2 steps. Direct path.

### 4.5 Cons

**Abstraction leak (acceptable):**
- Benthos worker knows about S6
- S6 state visible in BenthosObservedState
- **Mitigation:** This is reality. S6 is infrastructure. Hiding it doesn't eliminate complexity, it obscures it.

**No centralized S6 view:**
- Can't query "all S6 services" without filesystem scan
- **Mitigation:** Not needed. Each parent service (Benthos, Redpanda) knows its own S6 services.

**Duplication:**
- Each worker implements S6Client usage
- **Mitigation:** S6Client is a library. Reuse is via composition, not inheritance.

### 4.6 Why This Pattern Wins

Following Architecture Simplification skill:

| Pattern | Conversion Steps | Abstractions | Complexity |
|---------|------------------|--------------|------------|
| Pattern A | 4 steps | 2 supervisors | High |
| Pattern B | 4 steps | 1 global supervisor | Medium |
| **Pattern C** | **2 steps** | **0 supervisors** | **Low** |

**Direct path analysis:**

```
User config → Benthos config → S6 service directory → Process
```

Pattern A adds: Internal S6 Supervisor (unnecessary)
Pattern B adds: Global S6 Supervisor (unnecessary)
Pattern C: Direct path, no unnecessary layers

**YAGNI test:**
- Do we need S6 state observation collection? **No** (filesystem is synchronous)
- Do we need S6 supervisor lifecycle? **No** (S6 manages itself)
- Do we need S6 worker abstraction? **No** (S6 is infrastructure)

---

## 5. Architecture Simplification Analysis

Following the Architecture Simplification skill:

### 5.1 Conversion Counting

**Pattern A: Nested Composition**

```
Step 1: BenthosConfig → BenthosDesiredState (conversion)
Step 2: BenthosDesiredState → Extract S6Config (conversion)
Step 3: S6Config → S6DesiredState (conversion)
Step 4: S6DesiredState → S6 Actions → Filesystem (side effect)
```

**Total: 4 steps, 3 intermediate types**

**Pattern B: Shared Global Supervisor**

```
Step 1: BenthosConfig → S6ServiceConfig (conversion)
Step 2: S6ServiceConfig → S6Worker (wrapper)
Step 3: S6Worker → AddWorker(globalS6Supervisor) (registration)
Step 4: Supervisor manages worker → Filesystem (side effect)
```

**Total: 4 steps, 2 intermediate types, 1 global singleton**

**Pattern C: Direct Integration**

```
Step 1: BenthosConfig → S6ServiceConfig (conversion)
Step 2: S6ServiceConfig → CreateService() → Filesystem (side effect)
```

**Total: 2 steps, 0 intermediate supervisors**

### 5.2 Abstraction Justification

**From Architecture Simplification skill:**

> "Add abstraction only when concrete requirements demand it."

**Questions:**

1. **Does S6 state require async observation collection?**
   - No. Filesystem reads are synchronous and cheap (< 1ms).
   - Pattern A & B add Collector goroutines for no benefit.

2. **Does S6 need retry/backoff logic?**
   - No. Creating a service directory either succeeds or fails immediately.
   - Pattern A & B add ActionExecutor complexity for no benefit.

3. **Do multiple Benthos instances need to coordinate S6 lifecycle?**
   - No. Each Benthos manages its own S6 services independently.
   - Pattern B adds global singleton for no benefit.

4. **Do we need to abstract S6 from Benthos?**
   - No. S6 is infrastructure. Benthos must understand it for debugging.
   - Pattern A hides S6 but leaks it through error messages anyway.

### 5.3 Rationalizations Debunked

| Excuse | Reality |
|--------|---------|
| "Future extensibility" (other process supervisors) | YAGNI - S6 is the only supervisor we use |
| "Decouples Benthos from S6" | Coupling causing pain NOW? No. |
| "Clean abstraction boundary" | Abstraction leak via error messages |
| "Reusable S6 Supervisor" | 3 services (Benthos, Redpanda, GraphQL) ≠ 50 services. Concrete code wins. |
| "Separation of concerns" | S6 IS the concern. Can't separate infrastructure from itself. |
| "Testability" | Mock S6Client (interface) is easier than mock Supervisor (goroutines) |

---

## 6. Productive Tensions Analysis

Following the Preserving Productive Tensions skill:

### 6.1 Is This a Productive Tension?

**Question:** Do Patterns A and B optimize for different valid priorities?

**Analysis:**

- **Pattern A optimizes for:** Encapsulation (hide S6 from user)
- **Pattern B optimizes for:** Centralization (single S6 management point)
- **Pattern C optimizes for:** Simplicity (no unnecessary abstractions)

**Trade-off reality check:**

- Encapsulation (Pattern A) vs Simplicity (Pattern C): **False trade-off**. Hiding S6 doesn't eliminate complexity, it obscures it. Users still see S6 errors.

- Centralization (Pattern B) vs Independence (Pattern C): **False trade-off**. Benthos doesn't need to coordinate with other Benthos instances. Each is independent.

**Verdict:** Not a productive tension. Patterns A and B don't offer meaningful trade-offs. They add complexity without corresponding benefit.

### 6.2 Should We Preserve the Tension?

**From the skill:**

> "Preserve tensions that reveal context-dependence. Force resolution only when necessary."

**Resolution criteria:**

1. **Implementation cost:** Pattern C is significantly simpler (2 steps vs 4)
2. **Fundamental conflict:** Patterns don't conflict, they layer on top of each other unnecessarily
3. **Clear technical superiority:** Pattern C is objectively simpler for this context
4. **One-way door:** Not a one-way door. Can add supervisors later if concrete need emerges.
5. **Simplicity requires choice:** Yes. YAGNI demands choosing Pattern C.

**Decision:** Force resolution to Pattern C. The tension is not productive—it's speculative complexity.

---

## 7. Comparison Matrix

| Aspect | Pattern A (Nested) | Pattern B (Global) | **Pattern C (Direct)** |
|--------|-------------------|--------------------|-----------------------|
| **Abstraction layers** | 2 supervisors | 1 global supervisor | **0 supervisors** |
| **Conversion steps** | 4 steps | 4 steps | **2 steps** |
| **Goroutines per Benthos** | 2 (main + internal) | 1 (global shared) | **0 (no supervisor)** |
| **Coupling** | Benthos → Internal S6 | Benthos → Global S6 | **Benthos → S6Client (library)** |
| **Testability** | Mock internal supervisor | Mock global singleton | **Mock S6Client interface** |
| **Code complexity** | High (meta-pattern) | Medium (global state) | **Low (direct operations)** |
| **Performance** | 2 tick loops | 1 tick loop | **No tick loop** |
| **S6 visibility** | Hidden (leaks via errors) | Visible (separate entity) | **Visible (part of Benthos)** |
| **Reusability** | Per-Benthos S6 supervisor | Shared for all services | **S6Client reused via composition** |
| **YAGNI compliance** | Poor (speculative) | Poor (speculative) | **Excellent (minimal)** |
| **Migration effort** | High (2 supervisor layers) | Medium (global coordination) | **Low (refactor existing)** |
| **Debugging** | 2 FSMs to trace | 1 FSM + global state | **1 FSM + filesystem** |
| **When pattern makes sense** | S6 extremely complex | Many services coordinate | **Our actual use case** |

**Winner:** Pattern C (Direct Integration)

---

## 8. Migration Strategy

### 8.1 Current State (FSMv1)

```go
// Benthos FSM directly creates S6 services (similar to Pattern C)
func (f *BenthosFSM) createS6Service() error {
    // Write service directory
    os.MkdirAll(serviceDir, 0755)
    os.WriteFile(runScript, ...)

    // S6 automatically starts service
}
```

**Current approach is already close to Pattern C!**

### 8.2 Migration to FSMv2 Pattern C

**Step 1: Extract S6Client library**

```go
// pkg/s6/client.go (new file)
package s6

type Client struct {
    servicesDir string
}

func NewClient(servicesDir string) *Client {
    return &Client{servicesDir: servicesDir}
}

func (c *Client) CreateService(name string, config ServiceConfig) error {
    // Extract from BenthosFSM
}

func (c *Client) ServiceExists(name string) bool {
    // Extract from BenthosFSM
}

func (c *Client) ServiceRunning(name string) bool {
    // Extract from BenthosFSM
}

func (c *Client) RemoveService(name string) error {
    // Extract from BenthosFSM
}
```

**Step 2: Update BenthosWorker to use S6Client**

```go
type BenthosWorker struct {
    s6Client      *s6.Client
    benthosConfig BenthosConfig
}

func (w *BenthosWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
    return &BenthosObservedState{
        S6ServiceExists:  w.s6Client.ServiceExists(w.benthosConfig.ServiceName),
        S6ServiceRunning: w.s6Client.ServiceRunning(w.benthosConfig.ServiceName),
        // ... other observations
    }, nil
}
```

**Step 3: Update actions to use S6Client**

```go
type CreateServiceAction struct {
    s6Client    *s6.Client
    serviceName string
    config      s6.ServiceConfig
}

func (a *CreateServiceAction) Execute(ctx context.Context) error {
    return a.s6Client.CreateService(a.serviceName, a.config)
}
```

**Step 4: Repeat for Redpanda, GraphQL (reuse S6Client)**

```go
type RedpandaWorker struct {
    s6Client      *s6.Client  // Reuse same library
    redpandaConfig RedpandaConfig
}
```

### 8.3 No Migration Needed for Patterns A or B

**Patterns A and B require building NEW supervisor infrastructure:**
- Pattern A: Implement internal S6 Supervisor
- Pattern B: Implement global S6 Supervisor singleton

**Pattern C leverages existing code:**
- Extract S6 operations to library (already exists, just needs refactoring)
- No new supervisors
- No new goroutines
- No new tick loops

---

## 9. Code Examples

### 9.1 S6Client Library (Pattern C)

```go
// pkg/s6/client.go
package s6

import (
    "fmt"
    "os"
    "path/filepath"
    "strconv"
    "time"
)

type ServiceConfig struct {
    Name       string
    Command    string
    Args       []string
    WorkingDir string
    Env        map[string]string
}

type Client struct {
    servicesDir string
}

func NewClient(servicesDir string) *Client {
    return &Client{servicesDir: servicesDir}
}

func (c *Client) CreateService(config ServiceConfig) error {
    serviceDir := filepath.Join(c.servicesDir, config.Name)

    // Create service directory
    if err := os.MkdirAll(serviceDir, 0755); err != nil {
        return fmt.Errorf("failed to create service dir: %w", err)
    }

    // Generate run script
    runScript := fmt.Sprintf("#!/bin/sh\ncd %s\nexec %s %s\n",
        config.WorkingDir,
        config.Command,
        joinArgs(config.Args))

    // Write run script
    runPath := filepath.Join(serviceDir, "run")
    if err := os.WriteFile(runPath, []byte(runScript), 0755); err != nil {
        return fmt.Errorf("failed to write run script: %w", err)
    }

    // S6 automatically detects and starts the service
    return nil
}

func (c *Client) ServiceExists(name string) bool {
    runPath := filepath.Join(c.servicesDir, name, "run")
    _, err := os.Stat(runPath)
    return err == nil
}

func (c *Client) ServiceRunning(name string) bool {
    pidPath := filepath.Join(c.servicesDir, name, "supervise", "pid")
    pidBytes, err := os.ReadFile(pidPath)
    if err != nil {
        return false
    }

    pid, err := strconv.Atoi(string(pidBytes))
    if err != nil {
        return false
    }

    // Check if process exists
    proc, err := os.FindProcess(pid)
    if err != nil {
        return false
    }

    // Try to signal process (signal 0 = check existence)
    return proc.Signal(syscall.Signal(0)) == nil
}

func (c *Client) RemoveService(name string) error {
    serviceDir := filepath.Join(c.servicesDir, name)

    // Create down file to stop service
    downPath := filepath.Join(serviceDir, "down")
    if err := os.WriteFile(downPath, []byte{}, 0644); err != nil {
        return fmt.Errorf("failed to create down file: %w", err)
    }

    // Wait for S6 to stop the service
    for i := 0; i < 10; i++ {
        if !c.ServiceRunning(name) {
            break
        }
        time.Sleep(200 * time.Millisecond)
    }

    // Remove service directory
    if err := os.RemoveAll(serviceDir); err != nil {
        return fmt.Errorf("failed to remove service dir: %w", err)
    }

    return nil
}

func joinArgs(args []string) string {
    // Implementation omitted for brevity
    return strings.Join(args, " ")
}
```

### 9.2 BenthosWorker Using S6Client

```go
// pkg/fsmv2/benthos/worker.go
package benthos

import (
    "context"
    "time"

    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/s6"
)

type Worker struct {
    s6Client      *s6.Client
    benthosConfig Config
}

type Config struct {
    ServiceName string
    ConfigPath  string
    // ... other benthos config
}

type ObservedState struct {
    S6ServiceExists  bool
    S6ServiceRunning bool
    ConfigValid      bool
    ProcessHealthy   bool
    Timestamp        time.Time
}

func (o *ObservedState) GetTimestamp() time.Time {
    return o.Timestamp
}

func NewWorker(s6Client *s6.Client, config Config) *Worker {
    return &Worker{
        s6Client:      s6Client,
        benthosConfig: config,
    }
}

func (w *Worker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
    serviceName := w.benthosConfig.ServiceName

    observed := &ObservedState{
        S6ServiceExists:  w.s6Client.ServiceExists(serviceName),
        S6ServiceRunning: w.s6Client.ServiceRunning(serviceName),
        ConfigValid:      validateConfig(w.benthosConfig.ConfigPath),
        Timestamp:        time.Now(),
    }

    // Check process health (if running)
    if observed.S6ServiceRunning {
        observed.ProcessHealthy = checkBenthosHealth(serviceName)
    }

    return observed, nil
}

func (w *Worker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
    config := spec.(Config)
    return &DesiredState{
        ServiceName: config.ServiceName,
        ConfigPath:  config.ConfigPath,
        Running:     true,  // Always want it running (unless shutdown requested)
    }, nil
}

func (w *Worker) GetInitialState() fsmv2.State {
    return &TryingToStartState{}
}
```

### 9.3 Actions Using S6Client

```go
// pkg/fsmv2/benthos/action_create_service.go
package benthos

import (
    "context"
    "fmt"

    "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/s6"
)

type CreateServiceAction struct {
    s6Client    *s6.Client
    serviceName string
    configPath  string
}

func (a *CreateServiceAction) Execute(ctx context.Context) error {
    // Idempotent: check if service already exists
    if a.s6Client.ServiceExists(a.serviceName) {
        return nil  // Already created
    }

    config := s6.ServiceConfig{
        Name:       a.serviceName,
        Command:    "/usr/local/bin/benthos",
        Args:       []string{"-c", a.configPath},
        WorkingDir: "/data",
        Env:        map[string]string{
            "LOG_LEVEL": "info",
        },
    }

    return a.s6Client.CreateService(config)
}

func (a *CreateServiceAction) Name() string {
    return fmt.Sprintf("CreateS6Service(%s)", a.serviceName)
}
```

---

## 10. Recommendation Rationale

### 10.1 Why Pattern C Wins

Following the skills applied:

**From Inversion Exercise:**
- Flipping "S6 needs supervisor" revealed it doesn't
- S6 IS a supervisor, doesn't need supervision

**From Architecture Simplification:**
- Pattern C has 2 steps vs 4 steps
- Pattern C has 0 unnecessary abstractions
- Direct path: Config → S6Client → Filesystem

**From Preserving Productive Tensions:**
- Patterns A and B don't represent productive tensions
- They represent speculative complexity (YAGNI violation)
- No valid trade-offs to preserve

**From Brainstorming:**
- Pattern A hides complexity without eliminating it
- Pattern B adds global state without benefit
- Pattern C exposes reality (S6 is infrastructure) and handles it directly

### 10.2 What We Learned

**Core insight:**
> "Not everything needs to be a Worker. Some things are just libraries."

**S6 is infrastructure, not a business entity:**
- S6 doesn't have async state (filesystem is synchronous)
- S6 doesn't need observation polling (reads are instant)
- S6 doesn't need retry logic (operations succeed or fail immediately)
- S6 doesn't need its own lifecycle (it's managed by systemd/docker)

**Worker pattern fits when:**
- State collection is async and expensive
- Observations can become stale
- Need retry/backoff for operations
- Entity has complex lifecycle

**Library pattern fits when:**
- Operations are synchronous and cheap
- State is always fresh (or doesn't exist)
- Operations are one-shot (succeed or fail)
- Entity is infrastructure

**S6 is a library, not a worker.**

### 10.3 Risks and Mitigations

**Risk 1: S6 complexity grows, direct operations become unwieldy**

**Mitigation:**
- S6 operations are inherently simple (create dir, write file, check pid)
- If complexity grows, extract helper methods in S6Client
- Can always add supervisor later if concrete need emerges (hasn't in 5 years)

**Risk 2: Multiple services need to coordinate S6 lifecycle**

**Mitigation:**
- Each service owns its S6 services (Benthos owns benthos-bridge-*, Redpanda owns redpanda)
- No coordination needed
- If coordination needed, add coordinator (not supervisor)

**Risk 3: Abstraction leak (Benthos knows about S6)**

**Mitigation:**
- This is reality, not a leak
- Users MUST understand S6 for debugging
- Hiding S6 doesn't eliminate complexity, it obscures it

### 10.4 Final Recommendation

**Use Pattern C: Direct Integration via S6Client library.**

**Migration:**
1. Extract S6 operations to `pkg/s6/client.go`
2. Update BenthosWorker to use S6Client
3. Update actions to use S6Client
4. Reuse S6Client for Redpanda, GraphQL
5. No supervisor infrastructure needed

**Benefits:**
- Simplest pattern (2 steps)
- Reuses existing code structure
- No new goroutines or supervisors
- Easy to test (mock S6Client interface)
- YAGNI compliant

**Trade-offs accepted:**
- Benthos knows about S6 (acceptable, it's infrastructure)
- No centralized S6 view (not needed)
- Duplication of S6Client usage (mitigated by library composition)

**This is the direct path. No unnecessary abstractions. No speculative complexity.**

---

## Appendix: ASCII Diagrams

### Pattern A: Nested Composition

```
┌─────────────────────────────────────────────┐
│ Main Supervisor (Tick Interval: 1s)        │
│ ┌─────────────────────────────────────────┐ │
│ │ BenthosWorker                           │ │
│ │ ┌─────────────────────────────────────┐ │ │
│ │ │ Internal S6 Supervisor (Tick: 1s)   │ │ │
│ │ │ ┌─────────────────────────────────┐ │ │ │
│ │ │ │ S6Worker                        │ │ │ │
│ │ │ │  ObservedState:                 │ │ │ │
│ │ │ │   - ServiceExists               │ │ │ │
│ │ │ │   - ServiceRunning              │ │ │ │
│ │ │ └─────────────────────────────────┘ │ │ │
│ │ │ States: S6Starting, S6Running      │ │ │
│ │ └─────────────────────────────────────┘ │ │
│ │ ObservedState:                          │ │
│ │  - S6State (from internal supervisor)   │ │
│ │  - ProcessRunning                       │ │
│ │ States: BenthosStarting, BenthosRunning │ │
│ └─────────────────────────────────────────┘ │
└─────────────────────────────────────────────┘

Tick propagation: Main → Benthos → S6
Goroutines: 2 (Main Collector + S6 Collector)
```

### Pattern B: Shared Global Supervisor

```
┌──────────────────────────────────────────────┐
│ Global S6 Supervisor (Singleton)             │
│ ┌──────────────┐ ┌──────────────┐          │
│ │ S6Worker     │ │ S6Worker     │  ...     │
│ │ (benthos-1)  │ │ (redpanda)   │          │
│ └──────────────┘ └──────────────┘          │
└──────────────────────────────────────────────┘
          ↑                    ↑
          │ AddWorker()        │ AddWorker()
          │                    │
┌─────────┴─────┐    ┌─────────┴─────┐
│ Benthos FSM   │    │ Redpanda FSM  │
│ - Creates S6  │    │ - Creates S6  │
│   service dir │    │   service dir │
│ - Registers   │    │ - Registers   │
│   with global │    │   with global │
└───────────────┘    └───────────────┘

Coupling: Benthos → Global S6 Supervisor
Goroutines: 1 (Global S6 Collector, shared)
```

### Pattern C: Direct Integration (Recommended)

```
┌─────────────────────────────────────────────┐
│ Main Supervisor                             │
│ ┌─────────────────────────────────────────┐ │
│ │ BenthosWorker                           │ │
│ │ ┌─────────────────────────────────────┐ │ │
│ │ │ S6Client (library, not goroutine)   │ │ │
│ │ │  CreateService(name, config)        │ │ │
│ │ │  ServiceExists(name) -> bool        │ │ │
│ │ │  ServiceRunning(name) -> bool       │ │ │
│ │ │  RemoveService(name)                │ │ │
│ │ └─────────────────────────────────────┘ │ │
│ │ CollectObservedState():                 │ │
│ │  observed.S6ServiceExists =             │ │
│ │    s6Client.ServiceExists(serviceName)  │ │
│ │  observed.ProcessRunning =              │ │
│ │    s6Client.ServiceRunning(serviceName) │ │
│ │ States: TryingToStart, Running          │ │
│ │ Actions: CreateServiceAction            │ │
│ └─────────────────────────────────────────┘ │
└─────────────────────────────────────────────┘
                    ↓
            /data/services/
            ├─ benthos-bridge-1/
            │   ├─ run
            │   └─ supervise/pid
            └─ redpanda/
                ├─ run
                └─ supervise/pid

Direct path: Config → S6Client → Filesystem
Goroutines: 0 (no supervisor, synchronous ops)
```

---

## Conclusion

**Recommendation: Pattern C (Direct Integration)**

**Why:**
- Simplest (2 steps vs 4)
- No unnecessary abstractions
- YAGNI compliant
- Mirrors existing FSMv1 approach
- Easy to test and debug

**Migration:** Extract S6 operations to library, use in workers, no supervisor infrastructure needed.

**This is the direct path.**
