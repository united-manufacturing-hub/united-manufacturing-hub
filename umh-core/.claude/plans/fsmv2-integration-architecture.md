# FSMv2 Integration Architecture for umh-core

## Status: DRAFT - Iteration 2

## Goal

Bring FSMv2 into umh-core incrementally with feature flags, allowing:
- Step-by-step migration of components
- Side-by-side operation of old and new implementations
- Easy rollback if issues arise
- Clear deprecation path for legacy code

---

## Proposed Architecture

### Feature Flag Hierarchy

```yaml
agent:
  enableFSMv2: true              # Master switch - starts ApplicationSupervisor
  fsmv2StorePrefix: "fsmv2_"     # State isolation (NEW)
  useFSMv2Transport: true        # Migrate communicator to FSMv2
  useFSMv2Benthos: false         # Future: migrate benthos FSM
  useFSMv2Redpanda: false        # Future: migrate redpanda FSM
  useFSMv2ProtocolConverter: false  # Future: migrate protocol converter
```

### Config Validation (ADDED - addresses Migration Agent #1)

```go
func (c *AgentConfig) Validate() error {
    // Auto-enable FSMv2 if any component flag is set
    hasComponentFlag := c.UseFSMv2Transport || c.UseFSMv2Benthos ||
                        c.UseFSMv2Redpanda || c.UseFSMv2ProtocolConverter

    if hasComponentFlag && !c.EnableFSMv2 {
        // Option 1: Error
        // return fmt.Errorf("cannot enable UseFSMv2* flags without EnableFSMv2=true")

        // Option 2: Auto-enable (chosen for better UX)
        c.EnableFSMv2 = true
        log.Warn("Auto-enabling FSMv2 supervisor because UseFSMv2* flag is set")
    }

    return nil
}
```

---

### main.go Structure (REVISED)

```go
func main() {
    // ... existing setup ...

    // Validate config (includes FSMv2 flag validation)
    if err := configData.Agent.Validate(); err != nil {
        sentry.ReportIssuef(sentry.IssueTypeFatal, log, "Config validation failed: %w", err)
        return
    }

    // 1. Initialize FSMv2 supervisor (if master switch enabled)
    var fsmv2Supervisor *FSMv2Integration
    if configData.Agent.EnableFSMv2 {
        fsmv2Supervisor = initializeFSMv2Supervisor(ctx, logger, configData)
        if fsmv2Supervisor == nil {
            // Startup failure - don't continue with FSMv2 components
            log.Error("FSMv2 supervisor failed to start, falling back to legacy")
            configData.Agent.EnableFSMv2 = false
            configData.Agent.UseFSMv2Transport = false
        }
    }

    // 2. Component initialization - choose FSMv2 or legacy for each

    // Communicator - ONLY ONE system manages communicator
    if configData.Agent.UseFSMv2Transport && fsmv2Supervisor != nil {
        fsmv2Supervisor.RegisterCommunicator(configData, communicationState)
        // Do NOT start legacy communicator
    } else if configData.Agent.APIURL != "" && configData.Agent.AuthToken != "" {
        enableBackendConnection(ctx, ...) // Legacy
    }

    // 3. Start FSMv2 supervisor (after all workers registered)
    if fsmv2Supervisor != nil {
        fsmv2Supervisor.Start()
    }

    // 4. Continue with legacy control loop (for non-FSMv2 components)
    controlLoop.Execute(ctx)

    // 5. Coordinated shutdown (ADDED - addresses Reliability Agent #2)
    shutdownCoordinator(ctx, fsmv2Supervisor, controlLoop)
}
```

---

### FSMv2Integration Struct (NEW - addresses Go Idioms Agent DI concern)

```go
// FSMv2Integration encapsulates all FSMv2 setup and dependencies
// No global state - all dependencies injected through this struct
type FSMv2Integration struct {
    supervisor   *application.ApplicationSupervisor
    store        storage.TriangularStoreInterface
    adapters     []*CommunicationStateChannelAdapter
    logger       *zap.SugaredLogger
    cancel       context.CancelFunc  // For stopping adapters
    ctx          context.Context
    done         <-chan struct{}
}

func initializeFSMv2Supervisor(
    parentCtx context.Context,
    logger *zap.SugaredLogger,
    config *config.FullConfig,
) *FSMv2Integration {
    // Create cancellable context for this integration
    ctx, cancel := context.WithCancel(parentCtx)

    // Create isolated store with prefix (addresses Migration Agent #1)
    storePrefix := config.Agent.FSMv2StorePrefix
    if storePrefix == "" {
        storePrefix = "fsmv2_"  // Default isolation
    }
    store := createPrefixedStore(storePrefix, logger)

    sup, err := application.NewApplicationSupervisor(application.SupervisorConfig{
        ID:           "fsmv2-main",
        Name:         "FSMv2 Main Supervisor",
        Store:        store,
        Logger:       logger,
        TickInterval: 100 * time.Millisecond,
    })
    if err != nil {
        logger.Errorw("Failed to create FSMv2 supervisor", "error", err)
        cancel()  // Clean up context
        return nil
    }

    return &FSMv2Integration{
        supervisor: sup,
        store:      store,
        logger:     logger,
        cancel:     cancel,
        ctx:        ctx,
    }
}

func (f *FSMv2Integration) Start() {
    f.done = f.supervisor.Start(f.ctx)
}

func (f *FSMv2Integration) Shutdown(timeout time.Duration) {
    // 1. Signal all adapters to stop accepting new messages
    for _, adapter := range f.adapters {
        adapter.PrepareShutdown()
    }

    // 2. Request graceful shutdown with timeout
    shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()

    f.supervisor.Shutdown()

    select {
    case <-f.done:
        f.logger.Info("FSMv2 supervisor shutdown complete")
    case <-shutdownCtx.Done():
        f.logger.Warn("FSMv2 supervisor shutdown timeout, forcing")
        f.cancel()  // Force cancel
    }

    // 3. Stop adapters after supervisor is done
    for _, adapter := range f.adapters {
        adapter.Stop()
    }

    // 4. Clean up store
    if err := f.store.Close(); err != nil {
        f.logger.Warnw("Failed to close FSMv2 store", "error", err)
    }
}
```

---

### Dependency Injection for ChannelProvider (REVISED - addresses Go Idioms Agent #1)

Instead of global `SetChannelProvider()`, inject via `SupervisorConfig.Dependencies`:

```go
func (f *FSMv2Integration) RegisterCommunicator(
    config *config.FullConfig,
    commState *communication_state.CommunicationState,
) error {
    // Create adapter with explicit lifecycle management
    adapter := NewCommunicationStateChannelAdapter(
        commState.InboundChannel,
        commState.OutboundChannel,
        f.logger,
    )

    // Start adapter with integration's context (auto-stops on shutdown)
    adapter.Start(f.ctx)
    f.adapters = append(f.adapters, adapter)

    // Build YAML config that references the adapter
    // The adapter is passed to the worker factory via SupervisorConfig.Dependencies
    yamlConfig := fmt.Sprintf(`
children:
  - name: "communicator"
    workerType: "communicator"
    userSpec:
      config: |
        relayURL: "%s"
        instanceUUID: "%s"
        authToken: "%s"
        timeout: "10s"
        state: "running"
`, config.Agent.APIURL, instanceUUID, config.Agent.AuthToken)

    // Register dependency for worker creation (no globals!)
    f.supervisor.RegisterDependency("channelProvider", adapter)

    return f.supervisor.AddWorkerFromYAML(yamlConfig)
}
```

**Worker Factory Change:**

```go
// In pkg/fsmv2/workers/communicator/worker.go init()
factory.RegisterWorkerType[...](
    func(id fsmv2.Identity, logger *zap.SugaredLogger, stateReader fsmv2.StateReader, deps map[string]any) fsmv2.Worker {
        // Get channel provider from injected dependencies (not global!)
        var channelProvider ChannelProvider
        if cp, ok := deps["channelProvider"].(ChannelProvider); ok {
            channelProvider = cp
        }

        // Create worker with injected provider
        return NewCommunicatorWorkerWithProvider(id, logger, stateReader, channelProvider)
    },
    // ...
)
```

---

### Revised Channel Adapter (addresses Reliability Agent #1, #3)

```go
type CommunicationStateChannelAdapter struct {
    fsmInbound     chan *transport.UMHMessage
    fsmOutbound    chan *transport.UMHMessage
    legacyInbound  chan *models.UMHMessage
    legacyOutbound chan *models.UMHMessage
    logger         *zap.SugaredLogger

    ctx         context.Context
    cancel      context.CancelFunc
    wg          sync.WaitGroup     // Track goroutines
    shuttingDown atomic.Bool       // Signal to stop accepting
}

func (a *CommunicationStateChannelAdapter) Start(parentCtx context.Context) {
    a.ctx, a.cancel = context.WithCancel(parentCtx)

    // Track goroutines for graceful shutdown
    a.wg.Add(2)

    // Goroutine 1: FSMv2 inbound → Legacy inbound
    go func() {
        defer a.wg.Done()
        defer func() {
            if r := recover(); r != nil {
                a.logger.Errorw("adapter_inbound_panic", "panic", r)
            }
        }()

        for {
            select {
            case <-a.ctx.Done():
                return
            case msg := <-a.fsmInbound:
                if msg == nil || a.shuttingDown.Load() {
                    continue
                }
                // ... conversion and send ...
            }
        }
    }()

    // Goroutine 2: Legacy outbound → FSMv2 outbound
    go func() {
        defer a.wg.Done()
        defer func() {
            if r := recover(); r != nil {
                a.logger.Errorw("adapter_outbound_panic", "panic", r)
            }
        }()

        for {
            select {
            case <-a.ctx.Done():
                return
            case msg := <-a.legacyOutbound:
                if msg == nil || a.shuttingDown.Load() {
                    continue
                }
                // ... conversion and send ...
            }
        }
    }()
}

func (a *CommunicationStateChannelAdapter) PrepareShutdown() {
    a.shuttingDown.Store(true)
}

func (a *CommunicationStateChannelAdapter) Stop() {
    a.cancel()
    a.wg.Wait()  // Wait for goroutines to finish
}

// GetChannels returns channels - caller must hold reference for lifetime
// No race condition because channels are created at construction
func (a *CommunicationStateChannelAdapter) GetChannels(_ string) (
    inbound chan<- *transport.UMHMessage,
    outbound <-chan *transport.UMHMessage,
) {
    return a.fsmInbound, a.fsmOutbound
}
```

---

### Coordinated Shutdown (NEW - addresses Reliability Agent #2)

```go
func shutdownCoordinator(
    ctx context.Context,
    fsmv2 *FSMv2Integration,
    controlLoop *control.ControlLoop,
) {
    // Wait for main context cancellation (SIGTERM/SIGINT)
    <-ctx.Done()

    log.Info("Shutdown initiated, coordinating FSMv2 and legacy systems")

    // 1. Stop FSMv2 first (it feeds into legacy Router)
    if fsmv2 != nil {
        fsmv2.Shutdown(30 * time.Second)
    }

    // 2. Stop legacy control loop (Router will drain remaining messages)
    controlLoop.Stop()

    // 3. Wait for legacy to finish (with timeout)
    legacyDone := make(chan struct{})
    go func() {
        controlLoop.Wait()
        close(legacyDone)
    }()

    select {
    case <-legacyDone:
        log.Info("Legacy control loop shutdown complete")
    case <-time.After(30 * time.Second):
        log.Warn("Legacy control loop shutdown timeout")
    }

    log.Info("Coordinated shutdown complete")
}
```

---

### State Isolation Strategy (NEW - addresses Migration Agent #2)

```go
// State prefixing ensures FSMv2 and legacy don't conflict
func createPrefixedStore(prefix string, logger *zap.SugaredLogger) storage.TriangularStoreInterface {
    basicStore := memory.NewInMemoryStore()
    // Or for production:
    // basicStore := persistence.NewFileStore("/data/fsmv2-state")

    return storage.NewPrefixedTriangularStore(basicStore, prefix, logger)
}
```

**Rollback Safety:**
- FSMv2 state is stored with `fsmv2_` prefix
- Legacy state uses unprefixed keys
- On rollback (disable FSMv2 flag), legacy continues with its own state
- FSMv2 state can be cleaned up manually or left for next enablement

---

### Message Ordering Guarantee (CLARIFIED - addresses Migration Agent #3)

**Rule: Only ONE system owns the communicator at a time.**

```go
// CORRECT - mutually exclusive
if configData.Agent.UseFSMv2Transport && fsmv2Supervisor != nil {
    fsmv2Supervisor.RegisterCommunicator(...)
    // Legacy communicator NOT started
} else {
    enableBackendConnection(...)  // Legacy communicator
    // FSMv2 communicator NOT registered
}

// WRONG - would cause ordering issues
// fsmv2Supervisor.RegisterCommunicator(...)
// enableBackendConnection(...)  // DON'T do both!
```

The adapter only bridges FSMv2 ↔ legacy Router (for processing), not two communicators.

---

## Config Structure Changes

### pkg/config/config.go

```go
type AgentConfig struct {
    // ... existing fields ...

    // FSMv2 feature flags
    EnableFSMv2               bool   `yaml:"enableFSMv2,omitempty"`               // Master switch
    FSMv2StorePrefix          string `yaml:"fsmv2StorePrefix,omitempty"`          // State isolation prefix
    UseFSMv2Transport         bool   `yaml:"useFSMv2Transport,omitempty"`         // Communicator
    UseFSMv2Benthos           bool   `yaml:"useFSMv2Benthos,omitempty"`           // Benthos FSM
    UseFSMv2Redpanda          bool   `yaml:"useFSMv2Redpanda,omitempty"`          // Redpanda FSM
    UseFSMv2ProtocolConverter bool   `yaml:"useFSMv2ProtocolConverter,omitempty"` // Protocol converter
}

func (c *AgentConfig) Validate() error {
    hasComponentFlag := c.UseFSMv2Transport || c.UseFSMv2Benthos ||
                        c.UseFSMv2Redpanda || c.UseFSMv2ProtocolConverter

    if hasComponentFlag && !c.EnableFSMv2 {
        c.EnableFSMv2 = true
        // Auto-enabled - could also return error for strict validation
    }

    return nil
}
```

---

## Migration Path

### Phase 1: Communicator (Current)
- [x] Implement FSMv2 communicator worker
- [x] Add UseFSMv2Transport flag
- [ ] Refactor to use FSMv2Integration struct (remove globals)
- [ ] Add config validation
- [ ] Implement coordinated shutdown
- [ ] Test scenarios:
  - [ ] Legacy → FSMv2 (no message loss)
  - [ ] FSMv2 → Legacy rollback
  - [ ] FSMv2 failure → fallback to legacy
  - [ ] Shutdown with in-flight messages

### Phase 2: Benthos FSM
- [ ] Implement FSMv2 benthos worker
- [ ] Add UseFSMv2Benthos flag
- [ ] Migrate S6 service management to FSMv2
- [ ] Test with protocol converters

### Phase 3: Redpanda FSM
- [ ] Implement FSMv2 redpanda worker
- [ ] Add UseFSMv2Redpanda flag
- [ ] Migrate monitoring to FSMv2

### Phase 4: Full Migration

**Deprecation Criteria (must all be true):**
- [ ] FSMv2 enabled by default for 2 release cycles
- [ ] No rollbacks to legacy in production for 30 days
- [ ] All integration tests pass with FSMv2-only
- [ ] Legacy code paths have 0% coverage in production telemetry
- [ ] Remove legacy FSM code
- [ ] Remove feature flags (FSMv2 becomes default)

---

## Metrics Strategy (addresses Migration Agent suggestion)

Separate namespaces during migration:
- Legacy metrics: `umh_communicator_*`
- FSMv2 metrics: `umh_fsmv2_communicator_*`

After full migration, rename FSMv2 metrics to standard names.

---

## Files to Modify

| File | Change |
|------|--------|
| `pkg/config/config.go` | Add EnableFSMv2, FSMv2StorePrefix, and component flags |
| `pkg/config/config.go` | Add Validate() method |
| `cmd/main.go` | Add FSMv2Integration struct and lifecycle |
| `cmd/main.go` | Add shutdownCoordinator |
| `cmd/main.go` | Refactor component initialization |
| `pkg/fsmv2/supervisor/config.go` | Add Dependencies map to SupervisorConfig |
| `pkg/fsmv2/factory/worker_factory.go` | Pass dependencies to factory functions |
| `pkg/fsmv2/workers/communicator/worker.go` | Accept ChannelProvider via factory deps |

---

## Addressed Feedback Summary

| Agent | Issue | Resolution |
|-------|-------|------------|
| Go Idioms | Global ChannelProvider | Injected via SupervisorConfig.Dependencies |
| Go Idioms | Shutdown without context | Added Shutdown(timeout) |
| Reliability | Goroutine leak | WaitGroup + Stop() in adapter |
| Reliability | Shutdown coordination | shutdownCoordinator() function |
| Reliability | Race condition | Channels created at construction, no global lookup |
| Migration | Config validation | Validate() with auto-enable |
| Migration | State persistence | Prefixed store isolation |
| Migration | Message ordering | Mutually exclusive communicator ownership |

---

## Evaluation Criteria

This plan should be evaluated on:

1. **Go Idioms**: Does it follow Go patterns? Is DI clean? Lifecycle management?
2. **Reliability**: Failure modes? Race conditions? Graceful shutdown?
3. **Migration Safety**: Rollback path? Side-by-side operation? Incremental steps?

---

## Ralph Wiggum Implementation Loop

### Overview

The implementation uses a Ralph Wiggum loop - an iterative process where the same prompt is fed repeatedly until all evaluator agents approve. Each iteration builds on the previous work visible in files and git history.

### Implementation Steps (Atomic Units)

Each step is implemented in a separate loop iteration:

| Step | Description | Files | Exit Criteria |
|------|-------------|-------|---------------|
| 1 | Add FSMv2 feature flags to config | `pkg/config/config.go` | Build + Lint + Unit tests |
| 2 | Add config Validate() method | `pkg/config/config.go` | Build + Lint + Unit tests |
| 3 | Add Dependencies map to SupervisorConfig | `pkg/fsmv2/supervisor/config.go` | Build + Lint |
| 4 | Update worker factory to accept deps | `pkg/fsmv2/factory/worker_factory.go` | Build + Lint |
| 5 | Update communicator worker for DI | `pkg/fsmv2/workers/communicator/` | Build + Lint + Unit tests |
| 6 | Create FSMv2Integration struct | `cmd/main.go` | Build + Lint |
| 7 | Refactor channel adapter with WaitGroup | `cmd/main.go` | Build + Lint |
| 8 | Implement shutdownCoordinator | `cmd/main.go` | Build + Lint |
| 9 | Wire everything in main() | `cmd/main.go` | Build + Lint + All tests |
| 10 | Integration testing | All | Integration tests pass |

---

### Evaluator Subagents

Six specialized evaluators run after each implementation step:

#### 1. Build Evaluator

**Purpose**: Verify code compiles without errors

**Prompt**:
```
You are a Go build validator. Your ONLY job is to check if the code compiles.

Run: `go build -o /tmp/umh-core-test ./cmd/main.go`

Provide:
- **Verdict**: PASS or FAIL
- **Errors**: List any compilation errors with file:line
- **Missing Imports**: List any missing imports

Do NOT suggest fixes. Just report what's broken.
```

**Exit Condition**: `go build` returns exit code 0

---

#### 2. Lint Evaluator

**Purpose**: Verify code follows Go style guidelines

**Prompt**:
```
You are a Go lint validator. Your ONLY job is to check code style.

Run: `golangci-lint run ./cmd/... ./pkg/config/... ./pkg/fsmv2/...`

Provide:
- **Verdict**: PASS (0 issues) or FAIL (has issues)
- **Issues**: List each issue with file:line and rule name
- **Severity**: Classify as BLOCKING (must fix) or WARNING (can defer)

BLOCKING rules: nlreturn, wsl_v5, govet, staticcheck
WARNING rules: godot, gofumpt

Do NOT suggest fixes. Just report violations.
```

**Exit Condition**: 0 blocking lint issues

---

#### 3. Unit Test Evaluator

**Purpose**: Verify all unit tests pass

**Prompt**:
```
You are a Go test validator. Your ONLY job is to run and report test results.

Run:
- `go test ./pkg/config/... -v -count=1`
- `go test ./pkg/fsmv2/... -v -count=1`
- `go test ./cmd/... -v -count=1` (if tests exist)

Provide:
- **Verdict**: PASS (all pass) or FAIL (any failures)
- **Failed Tests**: List each failed test with name and error message
- **Coverage**: Report coverage percentage for modified packages

Do NOT suggest fixes. Just report failures.
```

**Exit Condition**: All tests pass

---

#### 4. Code Correctness Evaluator

**Purpose**: Verify implementation matches the architecture plan

**Prompt**:
```
You are a code correctness validator. Compare the implementation against the plan.

Read the plan at: `.claude/plans/fsmv2-integration-architecture.md`
Read the modified files: [list of files for this step]

Check:
1. **Struct fields**: Do structs match the plan's definitions?
2. **Method signatures**: Do methods match the plan's signatures?
3. **Logic flow**: Does the control flow match the plan's sequence?
4. **Error handling**: Are errors handled as specified?
5. **Dependencies**: Are dependencies injected, not global?

Provide:
- **Verdict**: CORRECT, PARTIAL, or INCORRECT
- **Deviations**: List any differences from the plan
- **Missing**: List any planned code not yet implemented
- **Extras**: List any code added that's not in the plan

PARTIAL means deviations exist but are acceptable.
INCORRECT means deviations break the architecture.
```

**Exit Condition**: CORRECT or PARTIAL (with documented reasons)

---

#### 5. Concurrency Safety Evaluator

**Purpose**: Check for race conditions and goroutine leaks

**Prompt**:
```
You are a concurrency safety validator. Analyze the code for threading issues.

Read the modified files: [list of files for this step]

Check:
1. **Race conditions**: Are shared variables protected by mutex or atomic?
2. **Goroutine lifecycle**: Do all goroutines have clear exit conditions?
3. **Channel operations**: Can any channel send/receive block indefinitely?
4. **Deadlocks**: Can lock ordering cause deadlocks?
5. **Resource leaks**: Are goroutines, channels, and contexts properly cleaned up?

For each issue found, provide:
- **Location**: file:line
- **Issue type**: RACE | LEAK | DEADLOCK | BLOCK
- **Severity**: CRITICAL (must fix) or WARNING (monitor)
- **Description**: What could go wrong

Provide:
- **Verdict**: SAFE or UNSAFE
- **Critical Issues**: List all CRITICAL severity issues
- **Warnings**: List all WARNING severity issues
```

**Exit Condition**: No CRITICAL issues

---

#### 6. Integration Test Evaluator

**Purpose**: Verify end-to-end behavior works correctly

**Prompt**:
```
You are an integration test validator. Test the actual runtime behavior.

Run these scenarios:

1. **Legacy mode (default)**:
   - Config: enableFSMv2=false
   - Expectation: Legacy communicator starts, FSMv2 supervisor NOT created
   - Verify: Logs show "Enabling backend connection"

2. **FSMv2 mode**:
   - Config: enableFSMv2=true, useFSMv2Transport=true
   - Expectation: FSMv2 supervisor starts, legacy communicator NOT started
   - Verify: Logs show "FSMv2 supervisor" and state transitions

3. **Auto-enable mode**:
   - Config: enableFSMv2=false, useFSMv2Transport=true
   - Expectation: FSMv2 auto-enabled, warning logged
   - Verify: Logs show "Auto-enabling FSMv2 supervisor"

4. **Graceful shutdown**:
   - Start with FSMv2 enabled, send SIGTERM
   - Expectation: Coordinated shutdown completes
   - Verify: Logs show "Coordinated shutdown complete"

5. **FSMv2 startup failure fallback**:
   - Config: enableFSMv2=true but sabotage supervisor creation
   - Expectation: Falls back to legacy
   - Verify: Logs show "FSMv2 supervisor failed" then legacy starts

Provide:
- **Verdict**: PASS (all scenarios pass) or FAIL
- **Scenario Results**: For each scenario: PASS/FAIL with evidence
- **Logs**: Include relevant log snippets as evidence
```

**Exit Condition**: All 5 scenarios pass

---

### Loop Structure

```
IMPLEMENTATION_LOOP:
    For each step in [1..10]:
        STEP_LOOP:
            1. Read current state of files
            2. Implement/fix code for this step
            3. Run evaluators in PARALLEL:
               - Build Evaluator
               - Lint Evaluator
               - Unit Test Evaluator
               - Code Correctness Evaluator
               - Concurrency Safety Evaluator
            4. Collect all verdicts
            5. If ANY evaluator returns FAIL/UNSAFE/INCORRECT:
               - Analyze feedback
               - Fix issues
               - GOTO STEP_LOOP (re-run same step)
            6. If step 10:
               - Run Integration Test Evaluator
               - If FAIL: fix and GOTO STEP_LOOP
            7. All evaluators pass → commit changes
            8. Move to next step

    All steps complete → OUTPUT: <promise>IMPLEMENTATION COMPLETE</promise>
```

---

### Completion Promise

The loop outputs this promise when ALL conditions are met:

```
<promise>IMPLEMENTATION COMPLETE</promise>
```

**Conditions for promise**:
- [ ] All 10 steps implemented
- [ ] Build passes (exit code 0)
- [ ] Lint passes (0 blocking issues)
- [ ] All unit tests pass
- [ ] Code matches plan (CORRECT or PARTIAL)
- [ ] No critical concurrency issues
- [ ] All 5 integration test scenarios pass

---

### Prompt Template for Main Loop

```markdown
# FSMv2 Integration Implementation - Step {N}

## Context
You are implementing the FSMv2 integration architecture for umh-core.
The approved plan is at: `.claude/plans/fsmv2-integration-architecture.md`

## Current Step
Step {N}: {step_description}
Files to modify: {file_list}

## Previous Work
{summary of what's already implemented in steps 1..N-1}

## Your Task
1. Read the plan section relevant to this step
2. Implement the code changes
3. Ensure code compiles and lint passes
4. Do NOT implement code for future steps

## Evaluators Will Check
After you complete, these evaluators will run:
- Build: `go build`
- Lint: `golangci-lint run`
- Unit Tests: `go test`
- Code Correctness: Compare to plan
- Concurrency Safety: Race/leak analysis
{if step 10: - Integration Tests: 5 scenarios}

## Exit Condition
Output `<promise>STEP {N} COMPLETE</promise>` when:
- Code compiles
- Lint passes
- Tests pass
- Implementation matches plan

If evaluators find issues, they will provide feedback and the loop continues.
```

---

### Parallel Execution Strategy

For efficiency, evaluators run in parallel where possible:

```
┌─────────────────────────────────────────────────────────┐
│                  After Code Change                       │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐               │
│  │  Build   │  │   Lint   │  │  Tests   │   PARALLEL    │
│  │ Evaluator│  │ Evaluator│  │ Evaluator│               │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘               │
│       │             │             │                      │
│       ▼             ▼             ▼                      │
│  ┌──────────────────────────────────────┐               │
│  │         Results Available            │               │
│  └──────────────────┬───────────────────┘               │
│                     │                                    │
│                     ▼                                    │
│  ┌──────────┐  ┌──────────────┐                         │
│  │  Code    │  │ Concurrency  │   PARALLEL (if build    │
│  │Correctness│ │   Safety     │   passed)               │
│  └────┬─────┘  └──────┬───────┘                         │
│       │               │                                  │
│       ▼               ▼                                  │
│  ┌──────────────────────────────────────┐               │
│  │      All Results Collected           │               │
│  └──────────────────────────────────────┘               │
│                                                          │
│  If step 10 and all pass:                               │
│  ┌──────────────────────────────────────┐               │
│  │      Integration Test Evaluator      │   SEQUENTIAL  │
│  └──────────────────────────────────────┘               │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

---

### Failure Recovery Strategy

When an evaluator fails:

| Evaluator | Recovery Action |
|-----------|-----------------|
| Build | Fix compilation errors, missing imports |
| Lint | Fix style violations (prioritize BLOCKING) |
| Unit Tests | Fix failing tests OR fix implementation bug |
| Code Correctness | Align implementation with plan OR update plan with deviation reason |
| Concurrency Safety | Add mutex/atomic, fix goroutine lifecycle |
| Integration Tests | Debug runtime behavior, check logs |

**Maximum iterations per step**: 5 (then escalate to human)

---

### Example Evaluator Invocation

```go
// Spawn all evaluators in parallel
tasks := []Task{
    {agent: "general-purpose", prompt: buildEvaluatorPrompt},
    {agent: "general-purpose", prompt: lintEvaluatorPrompt},
    {agent: "general-purpose", prompt: testEvaluatorPrompt},
    {agent: "general-purpose", prompt: correctnessEvaluatorPrompt},
    {agent: "general-purpose", prompt: concurrencyEvaluatorPrompt},
}

// Wait for all results
results := parallel(tasks)

// Check verdicts
if allPass(results) {
    if step == 10 {
        integrationResult := run(integrationEvaluator)
        if integrationResult.Pass {
            output("<promise>STEP 10 COMPLETE</promise>")
        }
    } else {
        output("<promise>STEP {N} COMPLETE</promise>")
    }
} else {
    // Collect feedback, iterate
}
```
