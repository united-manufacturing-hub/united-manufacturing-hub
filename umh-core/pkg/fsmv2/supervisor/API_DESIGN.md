# FSMv2 Supervisor API Design

## Overview

This document defines the public API boundary for the FSMv2 Supervisor, separating the public interface from internal implementation details. This separation improves encapsulation, prevents misuse, and clarifies the contract between supervisor and external callers.

## Design Principles

1. **Minimal Public Surface**: Only expose what external callers absolutely need
2. **Clear Intent**: Public methods should reflect high-level operations, not implementation details
3. **Encapsulation**: Hide FSM mechanics, reconciliation logic, and internal state management
4. **Testing Support**: Provide read-only accessors for testing without exposing mutable state
5. **Go Idioms**: Use lowercase for unexported methods, clear naming conventions

## Public API

These methods form the external contract. They are safe to call from outside the supervisor package and will remain stable across refactoring.

### Lifecycle Management

```go
// NewSupervisor creates a new supervisor instance with the given configuration.
// This is the only way to construct a Supervisor.
func NewSupervisor(cfg Config) *Supervisor

// Start begins the supervisor's lifecycle: observation collection, tick loop, metrics.
// Returns a channel that closes when the supervisor stops.
func (s *Supervisor) Start(ctx context.Context) <-chan struct{}

// Shutdown gracefully stops the supervisor and all its workers.
// This method is idempotent - calling it multiple times is safe.
func (s *Supervisor) Shutdown()
```

**Rationale**: These three methods control the supervisor's entire lifecycle. External callers need to create, start, and stop supervisors. The Start channel allows callers to wait for completion.

### Worker Registry Management

```go
// AddWorker registers a new worker with the supervisor.
// Returns error if worker with same ID already exists or if type discovery fails.
func (s *Supervisor) AddWorker(identity fsmv2.Identity, worker fsmv2.Worker) error

// RemoveWorker unregisters a worker from the supervisor.
// Returns error if worker not found.
func (s *Supervisor) RemoveWorker(ctx context.Context, workerID string) error

// GetWorker returns the worker context for the given ID.
// Returns error if worker not found.
func (s *Supervisor) GetWorker(workerID string) (*WorkerContext, error)

// ListWorkers returns all worker IDs currently managed by this supervisor.
func (s *Supervisor) ListWorkers() []string

// GetWorkers returns all worker identities currently managed by this supervisor.
func (s *Supervisor) GetWorkers() []fsmv2.Identity
```

**Rationale**: These methods allow external code to populate and query the worker registry. They are the primary way to interact with the supervisor's worker collection. `GetWorker` exposes `WorkerContext` which is also public (contains worker identity and current state).

### State Inspection

```go
// GetWorkerState returns the current state name and reason for a worker.
// Thread-safe and can be called concurrently with tick operations.
// Returns "Unknown" state with reason "current state is nil" if state is nil.
// Returns error if worker not found.
func (s *Supervisor) GetWorkerState(workerID string) (string, string, error)

// GetCurrentState returns the current state name for the first worker.
// Provided for backwards compatibility.
func (s *Supervisor) GetCurrentState() string
```

**Rationale**: External code needs to inspect worker states for monitoring, debugging, and decision-making. These methods provide read-only access to FSM state without exposing internal state management.

### Configuration Management

```go
// SetGlobalVariables sets the global variables for this supervisor.
// Global variables come from the management system and are fleet-wide settings.
// They are injected into UserSpec.Variables.Global before DeriveDesiredState() is called.
func (s *Supervisor) SetGlobalVariables(vars map[string]any)
```

**Rationale**: External systems (e.g., Management Console) need to provide global configuration that flows down the supervisor hierarchy. This is the entry point for fleet-wide settings.

### Testing Support (Read-Only)

```go
// GetChildren returns a copy of the children map for inspection.
// Thread-safe and primarily used in tests to verify hierarchical composition.
func (s *Supervisor) GetChildren() map[string]*Supervisor

// GetMappedParentState returns the mapped parent state for this supervisor.
// Returns empty string if this supervisor has no parent or no mapping has been applied.
// Primarily used for testing hierarchical state mapping.
func (s *Supervisor) GetMappedParentState() string
```

**Rationale**: Tests need to verify hierarchical composition and state mapping without exposing mutable internal state. These methods return copies or read-only views.

## Internal Implementation

These methods are implementation details and should NOT be called from outside the supervisor package. They will be moved to the `internal/` package or made unexported.

### FSM Execution (Core Engine)

```go
// tickWorker performs one FSM tick for a specific worker.
// INTERNAL: Called by Tick() orchestration, not by external code.
func (s *Supervisor) tickWorker(ctx context.Context, workerID string) error

// Tick performs one supervisor tick cycle, integrating all FSMv2 phases.
// INTERNAL: Called by tickLoop(), not by external code directly.
func (s *Supervisor) Tick(ctx context.Context) error

// TickAll performs one FSM tick for all workers in the registry.
// INTERNAL: Alternative to Tick() for multi-worker scenarios.
func (s *Supervisor) TickAll(ctx context.Context) error

// tickLoop is the main FSM loop goroutine.
// INTERNAL: Started by Start(), runs until context cancellation.
func (s *Supervisor) tickLoop(ctx context.Context)
```

**Rationale**: These methods implement the FSM execution engine. External code should never call them directly - they are invoked by the supervisor's internal tick loop. Making them public would allow callers to bypass lifecycle management and break invariants.

### Signal Processing

```go
// processSignal handles signals from states (removal, restart, etc.).
// INTERNAL: Called by tickWorker() after state transitions.
func (s *Supervisor) processSignal(ctx context.Context, workerID string, signal fsmv2.Signal) error

// RequestShutdown requests a worker to shut down by setting the shutdown flag in desired state.
// INTERNAL: Called by processSignal() or escalation logic.
func (s *Supervisor) RequestShutdown(ctx context.Context, workerID string, reason string) error
```

**Rationale**: Signal processing is an internal FSM concern. External code should not directly send signals - they are emitted by states during transitions. `RequestShutdown` is a special case used by escalation logic, but should not be called externally.

### Data Freshness and Health

```go
// CheckDataFreshness validates observation age against thresholds.
// INTERNAL: Called by tickWorker() before state transitions (Layer 1 defense).
func (s *Supervisor) CheckDataFreshness(snapshot *fsmv2.Snapshot) bool

// RestartCollector restarts the observation collector for a worker.
// INTERNAL: Called by tickWorker() when data timeout detected (Layer 2 defense).
func (s *Supervisor) RestartCollector(ctx context.Context, workerID string) error

// Health config accessors (GetStaleThreshold, GetCollectorTimeout, etc.)
// INTERNAL: Used by tests to verify configuration, not needed by external code.
func (s *Supervisor) GetStaleThreshold() time.Duration
func (s *Supervisor) GetCollectorTimeout() time.Duration
func (s *Supervisor) GetMaxRestartAttempts() int
func (s *Supervisor) GetRestartCount() int
func (s *Supervisor) SetRestartCount(count int)
```

**Rationale**: Data freshness validation and collector health management are internal defense mechanisms. External code should not bypass the FSM's automatic health checks. The health config accessors are only used by tests and should move to internal package.

### Hierarchical Composition (Phase 0)

```go
// reconcileChildren reconciles actual child supervisors to match desired ChildSpec array.
// INTERNAL: Called by Tick() as part of Phase 0 hierarchical composition.
func (s *Supervisor) reconcileChildren(specs []config.ChildSpec) error

// applyStateMapping applies parent state mapping to all children.
// INTERNAL: Called by Tick() after reconciliation.
func (s *Supervisor) applyStateMapping()

// UpdateUserSpec updates the user-provided configuration for this supervisor.
// INTERNAL: Called by parent supervisors during reconciliation to update child config.
func (s *Supervisor) UpdateUserSpec(spec config.UserSpec)
```

**Rationale**: Child reconciliation and state mapping are internal orchestration details of hierarchical composition. Only the supervisor itself should trigger reconciliation - external code should not manipulate the supervisor hierarchy directly. `UpdateUserSpec` is called by parent supervisors, but this is still internal to the supervisor package (parent-child communication).

### Metrics and Observability

```go
// startMetricsReporter starts a goroutine that periodically records hierarchy metrics.
// INTERNAL: Called by Start(), runs in background until Shutdown().
func (s *Supervisor) startMetricsReporter(ctx context.Context)

// recordHierarchyMetrics records current hierarchy depth and size metrics.
// INTERNAL: Called by metrics reporter goroutine.
func (s *Supervisor) recordHierarchyMetrics()

// calculateHierarchyDepth returns the depth of this supervisor in the hierarchy tree.
// INTERNAL: Used by metrics reporting.
func (s *Supervisor) calculateHierarchyDepth() int

// calculateHierarchySize returns the total number of supervisors in this subtree.
// INTERNAL: Used by metrics reporting.
func (s *Supervisor) calculateHierarchySize() int
```

**Rationale**: Metrics collection is an internal background task. External code should not trigger metrics recording - it happens automatically. The hierarchy calculation methods are implementation details of metrics.

### Internal State Accessors

```go
// isStarted returns whether the supervisor has been started.
// INTERNAL: Used internally to prevent duplicate starts.
func (s *Supervisor) isStarted() bool

// getContext returns the current context for the supervisor.
// INTERNAL: Used internally for goroutine coordination.
func (s *Supervisor) getContext() context.Context

// getStartedContext atomically checks if started and returns context.
// INTERNAL: Prevents TOCTOU races in Start()/Shutdown().
func (s *Supervisor) getStartedContext() (context.Context, bool)
```

**Rationale**: These methods expose raw internal state that external code should not access. The supervisor manages its own lifecycle state.

### Helper Functions (Package-Level)

```go
// normalizeType strips pointer indirection to get the base struct type.
// INTERNAL: Used by AddWorker and tickWorker for type validation.
func normalizeType(t reflect.Type) reflect.Type

// getString safely extracts a string value from a Document.
// INTERNAL: Used when loading snapshots from storage.
func getString(doc interface{}, key string, defaultValue string) string

// getRecoveryStatus returns a human-readable recovery status based on attempt count.
// INTERNAL: Used by Tick() for escalation logging.
func (s *Supervisor) getRecoveryStatus() string

// getEscalationSteps returns manual runbook steps for specific child types.
// INTERNAL: Used by Tick() for escalation logging.
func (s *Supervisor) getEscalationSteps(childName string) string
```

**Rationale**: These are utility functions for internal implementation. External code should not depend on type normalization logic, document parsing, or escalation formatting.

## Proposed Package Structure

After this refactoring, the supervisor package will have:

```
pkg/fsmv2/supervisor/
├── supervisor.go              # Public API only
├── types.go                   # Public types (Config, CollectorHealthConfig, WorkerContext)
├── constants.go               # Public constants
├── internal/
│   ├── execution.go          # FSM execution (tickWorker, Tick, TickAll, tickLoop)
│   ├── signals.go            # Signal processing (processSignal, RequestShutdown)
│   ├── health.go             # Data freshness and collector health
│   ├── hierarchy.go          # Child reconciliation and state mapping
│   ├── metrics.go            # Metrics reporting
│   ├── lifecycle.go          # Lifecycle helpers (isStarted, getContext, etc.)
│   └── helpers.go            # Utility functions (normalizeType, getString, etc.)
└── internal_test.go          # Tests for internal implementation
```

## Migration Strategy

1. **Phase 1 (RED)**: Write tests that verify current API boundaries (fail because methods still public)
2. **Phase 2 (GREEN)**: Move internal methods to `internal/` package, update references
3. **Phase 3 (REFACTOR)**: Optimize internal organization without changing public API

## Benefits

1. **Clear Contract**: Public API is explicit and documented
2. **Prevents Misuse**: Internal methods cannot be called from outside package
3. **Easier Refactoring**: Internal implementation can change without breaking external code
4. **Better Testing**: Tests can focus on public behavior vs internal mechanics
5. **Improved Documentation**: Public API is smaller and easier to document
6. **Maintainability**: Clear separation makes codebase easier to navigate

## Backwards Compatibility

This change is **100% backwards compatible** for external callers because:
- All methods in the Public API section remain exported and unchanged
- Only internal methods (not meant to be called externally) are being hidden
- Existing tests that use public API will continue to work
- Phase 0 integration tests verify no behavioral changes

The only "breaking change" would be if external code was calling internal methods like `tickWorker()` or `reconcileChildren()` - which should never happen in correct usage.
