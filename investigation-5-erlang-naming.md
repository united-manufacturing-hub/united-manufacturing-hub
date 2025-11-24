# Investigation: Erlang/OTP Naming Patterns for FSM v2

## Executive Summary

After investigating Erlang/OTP, Kubernetes, and Akka naming patterns, the recommendation is to use **"application"** as the name for the FSM v2 root orchestrator, placed in `workers/application/` to maintain clean top-level interfaces. This aligns with Erlang/OTP concepts while respecting Go conventions and user preferences for file organization.

## Erlang/OTP Architecture Overview

### Core Concepts

1. **Application**: In Erlang/OTP, an application is a complete unit of functionality that includes:
   - A supervision tree (hierarchy of supervisors and workers)
   - Configuration and metadata
   - Start/stop lifecycle management
   - Resource management

   An application is NOT just FSMs - it's the complete system that coordinates multiple processes.

2. **Supervisor**: A process that monitors and manages child processes (workers or other supervisors). Key responsibilities:
   - Start child processes
   - Restart failed children according to strategy
   - Shutdown children gracefully
   - Build fault-tolerant hierarchies

3. **gen_server/gen_fsm**: Generic behaviors (templates) for common patterns:
   - `gen_server`: Client-server interactions
   - `gen_fsm`: Finite state machines (deprecated for `gen_statem`)
   - Both are "workers" that do actual work

4. **Supervision Tree**: Hierarchical arrangement where:
   - Top-level application supervisor starts everything
   - Supervisors monitor workers
   - Workers perform actual computations
   - Failures cascade up, restarts cascade down

### How These Relate to FSM v2

| Erlang/OTP Concept | FSM v2 Equivalent | Notes |
|-------------------|-------------------|-------|
| Application | Root orchestrator | Manages entire system lifecycle |
| Supervisor | Supervisor type | Manages child workers |
| gen_server/gen_fsm | Worker implementations | Individual FSMs |
| Supervision tree | Worker hierarchy | Parent-child relationships |

## Current FSM v2 Structure

### Top-Level Files (interfaces only)
```
pkg/fsmv2/
├── api.go              # Core interfaces (Worker, State, ObservedState, DesiredState)
├── base_state.go       # Base state implementation
├── dependencies.go     # Dependency injection types
├── state_adapter.go    # State adaptation utilities
└── worker_base.go      # Base worker implementation
```

### Implementation Directories
```
pkg/fsmv2/
├── workers/           # Worker implementations
│   ├── communicator/  # Specific worker type
│   └── example/       # Example implementations
├── supervisor/        # Supervisor implementation
├── root/             # Current "root" implementation
├── config/           # Configuration types
└── factory/          # Factory pattern for dynamic creation
```

### Current Issues
- "root" is too generic - doesn't convey purpose
- Top-level has some implementations mixed with interfaces
- Unclear where the main orchestrator should live

## Naming Analysis

### Option 1: "application" (Recommended)
**Pros:**
- Direct mapping to Erlang/OTP concept
- Clearly indicates "the thing that runs everything"
- Familiar to distributed systems developers
- Distinguishes from individual workers/FSMs

**Cons:**
- Might be confused with "software application" in general
- In Go, less common than "service" or "manager"

**Cultural baggage:** Positive - implies complete, self-contained functionality

### Option 2: "system"
**Pros:**
- Conveys coordination of multiple parts
- Language-agnostic
- Clear hierarchy (system > supervisor > worker)

**Cons:**
- Very generic
- Could mean operating system, distributed system, etc.
- Doesn't convey lifecycle management

**Cultural baggage:** Neutral but vague

### Option 3: "runtime"
**Pros:**
- Implies execution environment
- Common in managed platforms (Java runtime, .NET runtime)

**Cons:**
- Usually means language runtime, not application coordinator
- Doesn't convey supervision/management

**Cultural baggage:** Confusing - typically means VM or interpreter

### Option 4: "engine"
**Pros:**
- Implies processing and execution
- Common in workflow engines, rules engines

**Cons:**
- Doesn't convey hierarchy or supervision
- More about processing than lifecycle management

**Cultural baggage:** Implies data processing, not orchestration

### Option 5: "manager"
**Pros:**
- Common in Kubernetes (controller-manager)
- Clear management role

**Cons:**
- Overused in software (ConnectionManager, ConfigManager, etc.)
- Doesn't distinguish from other managers

**Cultural baggage:** Enterprise Java flashbacks

## Proposed Structure Options

### Option A: workers/application/ (Recommended)
```
pkg/fsmv2/
├── api.go                    # Interfaces only
├── workers/
│   ├── application/          # The root orchestrator
│   │   ├── types.go         # Application-specific types
│   │   ├── worker.go        # Application worker implementation
│   │   ├── setup.go         # Setup and initialization
│   │   └── factory.go       # Registration with factory
│   ├── communicator/
│   └── [other workers]/
```

**Pros:**
- Keeps implementations in workers/ as user prefers
- Top-level remains clean (interfaces only)
- Clear that application is a special type of worker
- Follows existing pattern

**Cons:**
- Application is "special" but lives with regular workers
- Might need clear documentation about its role

### Option B: application/ (top-level)
```
pkg/fsmv2/
├── api.go                    # Interfaces only
├── application/              # The root orchestrator (special status)
│   ├── orchestrator.go      # Main coordination logic
│   ├── lifecycle.go         # System lifecycle management
│   └── bootstrap.go         # System initialization
├── workers/                  # Regular workers
└── supervisor/               # Supervisor implementation
```

**Pros:**
- Clear separation: application is special
- Emphasizes orchestrator role
- Parallel to supervisor/ directory

**Cons:**
- User prefers implementations not at top level
- Breaks pattern of workers in workers/

### Option C: Keep as root/ but improve
```
pkg/fsmv2/
├── api.go
├── root/                     # Keep current name
│   └── application.go        # But use "Application" type internally
```

**Pros:**
- No migration needed
- "root" is technically accurate

**Cons:**
- Doesn't address the core naming issue
- "root" remains generic

## Industry Patterns Comparison

### Kubernetes
- **Controller**: Watches and reconciles resources
- **Manager**: Coordinates multiple controllers
- **Operator**: Domain-specific controller + CRDs

FSM v2 "application" is most like a Kubernetes Manager.

### Akka Actor Systems
- **ActorSystem**: Top-level container (like our application)
- **Guardian actors**: Special supervisors (/user, /system)
- **Root guardian**: Ultimate parent ("/")

FSM v2 "application" combines ActorSystem + root guardian roles.

### Erlang/OTP
- **Application**: Complete functional unit
- **Application supervisor**: Top-level supervisor
- **Workers**: Actual processing units

FSM v2 maps directly to this model.

## Recommendation

### Best Name: "application"
Based on the investigation:
1. **Accurately describes the role**: Coordinates multiple FSMs as a complete system
2. **Has precedent**: Erlang/OTP uses this exact concept
3. **Distinguishes from parts**: Clear that application > supervisor > worker
4. **Implies completeness**: Not just FSMs, but complete functioning system

### Best Structure: workers/application/

Following user preferences:
1. **Keep top-level clean**: Only interfaces in api.go
2. **Implementations in subdirectories**: All implementations under workers/, supervisor/, etc.
3. **Special but not separate**: Application is a special worker, lives with other workers
4. **Clear documentation**: README in workers/application/ explaining its role

### Implementation Plan

1. **Rename types**:
   - `PassthroughWorker` → `ApplicationWorker`
   - `PassthroughObservedState` → `ApplicationObservedState`
   - `PassthroughDesiredState` → `ApplicationDesiredState`

2. **Move and rename**:
   - `root/` → `workers/application/`
   - Update imports throughout codebase

3. **Update documentation**:
   - Explain "application" concept in README
   - Reference Erlang/OTP for those familiar

4. **Clean top-level**:
   - Ensure api.go has only interfaces
   - Move any implementations to appropriate subdirectories

## How to Maintain Clean Top-Level

### Interfaces Only in api.go
```go
// api.go - ONLY interfaces and type definitions
type Worker interface { ... }
type State[T, D any] interface { ... }
type ObservedState interface { ... }
type DesiredState interface { ... }
```

### Implementations in Subdirectories
- `workers/*/` - Worker implementations
- `supervisor/` - Supervisor logic
- `config/` - Configuration structures
- `factory/` - Dynamic instantiation

### Documentation at Each Level
- `api.go` - Interface contracts and invariants
- `workers/application/README.md` - Application orchestrator role
- `PATTERNS.md` - Overall architecture patterns

## References

### Erlang/OTP Documentation
- [OTP Design Principles - Applications](https://www.erlang.org/doc/design_principles/applications.html)
- [Supervision Trees](https://adoptingerlang.org/docs/development/supervision_trees/)
- [Building Applications with OTP](https://learnyousomeerlang.com/building-applications-with-otp)

### Kubernetes Patterns
- [Controller Patterns](https://book.kubebuilder.io/cronjob-tutorial/controller-overview.html)
- [Operator Patterns](https://sdk.operatorframework.io/docs/best-practices/)

### Actor Systems
- [Akka Supervision](https://doc.akka.io/docs/akka/current/typed/guide/tutorial_1.html)
- [Actor Hierarchies](https://doc.akka.io/docs/akka/current/general/supervision.html)

## Conclusion

The name "application" best captures the role of the FSM v2 root orchestrator: it's not just managing FSMs, but orchestrating a complete application with lifecycle, configuration, and fault tolerance. Placing it in `workers/application/` respects the architectural preference for keeping implementations separate from interfaces while acknowledging its special role in the system.

This aligns with established patterns from Erlang/OTP while maintaining Go idioms and the user's specific architectural preferences.