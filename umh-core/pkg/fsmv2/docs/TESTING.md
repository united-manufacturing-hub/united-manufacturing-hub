# FSMv2 Testing Guide

This guide covers testing practices for the FSMv2 framework in umh-core.

## Table of Contents

1. [Test Framework](#test-framework)
2. [Running Tests](#running-tests)
3. [Test Organization](#test-organization)
4. [Writing Tests](#writing-tests)
5. [Mocking Patterns](#mocking-patterns)
6. [Test Helpers](#test-helpers)
7. [Best Practices](#best-practices)
8. [Coverage Expectations](#coverage-expectations)

## Test Framework

FSMv2 uses **Ginkgo v2** as the BDD testing framework with **Gomega** for assertions.

```go
import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)
```

### Why Ginkgo v2?

- **BDD-style specs**: Readable, hierarchical test organization
- **Parallel execution**: Run tests concurrently for speed
- **Rich matchers**: Gomega provides expressive assertions
- **Integration testing**: Built-in support for async testing with `Eventually()`

## Running Tests

### Quick Commands

```bash
# Run all tests (unit + integration)
make test

# Run only unit tests (recommended during development)
make unit-test

# Run unit tests without failing fast (see all failures)
make unit-test-fail-slow

# Run specific package tests
ginkgo ./pkg/fsmv2/supervisor

# Run specific test file
ginkgo --focus-file=action_executor_test.go ./pkg/fsmv2/supervisor

# Run tests matching description
ginkgo --focus="ActionExecutor" ./pkg/fsmv2/supervisor
```

### Ginkgo Flags

```bash
# Common flags
-v                           # Verbose output
-r                           # Recursive (all subdirectories)
--race                       # Enable race detector
--fail-fast                  # Stop on first failure
--label-filter               # Filter by labels
--focus                      # Run specs matching regex
--focus-file                 # Run specific test file
--fail-on-focused            # Fail if focused specs exist (CI uses this)

# Example: Run only non-integration tests
ginkgo -r --label-filter='!integration && !redpanda-extended && !tls' ./...
```

### CI Test Commands

The CI pipeline enforces:

```bash
# Check for focused specs (FIt, FDescribe, FContext)
ginkgo -r --fail-on-focused ./...

# Run with race detection
ginkgo -r -v --race --label-filter='!integration && !redpanda-extended && !tls' ./...
```

**IMPORTANT**: Never commit focused specs (`FIt`, `FDescribe`, `FContext`). CI will fail.

## Test Organization

### File Structure

```
pkg/fsmv2/
├── supervisor/
│   ├── supervisor_suite_test.go       # Ginkgo test suite setup
│   ├── supervisor_test.go             # Unit tests for Supervisor
│   ├── integration_test.go            # Integration tests
│   ├── execution/
│   │   ├── execution_suite_test.go    # Sub-package suite
│   │   ├── action_executor_test.go    # ActionExecutor unit tests
│   │   └── action_test_helpers_test.go # Shared test utilities
│   └── ...
├── workers/
│   └── communicator/
│       ├── worker_test.go             # Worker implementation tests
│       ├── mocks_test.go              # Mock implementations
│       └── state/
│           └── state_*_test.go        # Individual state tests
└── integration/
    ├── integration_suite_test.go      # Integration test suite
    └── integration_test.go            # End-to-end tests
```

### Suite Files (`*_suite_test.go`)

Every package with tests needs a suite file:

```go
package supervisor_test

import (
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

func TestSupervisor(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "Supervisor Suite")
}
```

**Purpose**: Connects Ginkgo to Go's testing framework. Required for `go test` to work.

### Test Classification

Tests are organized into two categories:

1. **Unit Tests**: Test individual components in isolation
   - Fast (milliseconds)
   - No external dependencies
   - Use mocks/fakes
   - No `integration` label

2. **Integration Tests**: Test component interactions
   - Slower (seconds)
   - May use real dependencies (databases, containers)
   - Labeled with `integration`
   - Example:
     ```go
     It("should reconcile child workers", Label("integration"), func() {
         // Test with real database
     })
     ```

## Writing Tests

### Basic Test Structure

```go
var _ = Describe("ActionExecutor", func() {
    var (
        executor *execution.ActionExecutor
        ctx      context.Context
        cancel   context.CancelFunc
    )

    BeforeEach(func() {
        ctx, cancel = context.WithCancel(context.Background())
        executor = execution.NewActionExecutor(10)
        executor.Start(ctx)
    })

    AfterEach(func() {
        cancel()
        executor.Shutdown()
    })

    Describe("EnqueueAction", func() {
        Context("when action succeeds", func() {
            It("should execute action without blocking", func() {
                actionID := "test-action"
                executed := make(chan bool, 1)

                action := &testAction{
                    execute: func(ctx context.Context) error {
                        executed <- true
                        return nil
                    },
                }

                err := executor.EnqueueAction(actionID, action)
                Expect(err).ToNot(HaveOccurred())

                Eventually(executed).Should(Receive())
            })
        })

        Context("when action is already in progress", func() {
            It("should return error", func() {
                // Test implementation
            })
        })
    })
})
```

### BDD Structure

Ginkgo uses a hierarchical BDD structure:

- **`Describe`**: Groups related tests (usually a type or function)
- **`Context`**: Groups tests under specific conditions
- **`It`**: Individual test case (should be atomic)

```go
Describe("State Machine", func() {
    Context("when in Running state", func() {
        It("should transition to Stopping on shutdown", func() { })
        It("should stay in Running when healthy", func() { })
    })

    Context("when in Degraded state", func() {
        It("should transition to Running when recovered", func() { })
        It("should transition to Stopping on shutdown", func() { })
    })
})
```

### Gomega Matchers

Common assertion patterns:

```go
// Equality
Expect(result).To(Equal(expected))
Expect(result).ToNot(Equal(unexpected))

// Nil checks
Expect(err).ToNot(HaveOccurred())
Expect(value).To(BeNil())

// Boolean
Expect(isValid).To(BeTrue())
Expect(isEmpty).To(BeFalse())

// Collections
Expect(list).To(HaveLen(3))
Expect(list).To(ContainElement(item))
Expect(list).To(BeEmpty())

// Strings
Expect(message).To(ContainSubstring("error"))
Expect(path).To(HavePrefix("/data"))

// Numbers
Expect(count).To(BeNumerically(">", 0))
Expect(count).To(BeNumerically("<=", 100))

// Channels
Eventually(ch).Should(Receive())
Eventually(ch).Should(BeClosed())

// Async assertions
Eventually(func() bool {
    return executor.HasActionInProgress(actionID)
}).Should(BeFalse())

Consistently(func() int {
    return len(queue)
}, "100ms", "10ms").Should(Equal(0))
```

### Testing State Transitions

```go
Describe("TryingToStartState", func() {
    var (
        state    *TryingToStartState
        snapshot fsmv2.Snapshot
    )

    BeforeEach(func() {
        state = &TryingToStartState{}
        snapshot = fsmv2.Snapshot{
            Identity: fsmv2.Identity{
                ID:         "test-worker",
                Name:       "Test Worker",
                WorkerType: "container",
            },
            Observed: &mockObservedState{Running: false},
            Desired:  &mockDesiredState{shutdownRequested: false},
        }
    })

    Context("when process is not running", func() {
        It("should emit start action", func() {
            nextState, signal, action := state.Next(snapshot)

            Expect(nextState).To(Equal(state))
            Expect(signal).To(Equal(fsmv2.SignalNone))
            Expect(action).ToNot(BeNil())
            Expect(action.Name()).To(ContainSubstring("start"))
        })
    })

    Context("when process is running", func() {
        BeforeEach(func() {
            snapshot.Observed.(*mockObservedState).Running = true
        })

        It("should transition to Running state", func() {
            nextState, signal, action := state.Next(snapshot)

            Expect(nextState).To(BeAssignableToTypeOf(&RunningState{}))
            Expect(signal).To(Equal(fsmv2.SignalNone))
            Expect(action).To(BeNil())
        })
    })

    Context("when shutdown requested", func() {
        BeforeEach(func() {
            snapshot.Desired.(*mockDesiredState).shutdownRequested = true
        })

        It("should transition to Stopping state", func() {
            nextState, signal, action := state.Next(snapshot)

            Expect(nextState).To(BeAssignableToTypeOf(&StoppingState{}))
            Expect(signal).To(Equal(fsmv2.SignalNone))
            Expect(action).To(BeNil())
        })
    })
})
```

### Testing Actions

All actions MUST have idempotency tests:

```go
Describe("StartProcessAction", func() {
    var (
        action    *StartProcessAction
        processID string
    )

    BeforeEach(func() {
        processID = "test-process"
        action = NewStartProcessAction(processID, "/path/to/binary")
    })

    Describe("Idempotency", func() {
        It("should be safe to call multiple times", func() {
            ctx := context.Background()

            // Execute action 3 times
            execution.VerifyActionIdempotency(action, 3, func() {
                // Verify final state
                Expect(isProcessRunning(processID)).To(BeTrue())
            })
        })
    })

    Describe("Execute", func() {
        Context("when process is not running", func() {
            It("should start the process", func() {
                ctx := context.Background()

                err := action.Execute(ctx)

                Expect(err).ToNot(HaveOccurred())
                Expect(isProcessRunning(processID)).To(BeTrue())
            })
        })

        Context("when process is already running", func() {
            BeforeEach(func() {
                startProcess(processID)
            })

            It("should succeed without restarting", func() {
                pid1 := getProcessPID(processID)

                ctx := context.Background()
                err := action.Execute(ctx)

                Expect(err).ToNot(HaveOccurred())
                pid2 := getProcessPID(processID)
                Expect(pid2).To(Equal(pid1)) // Same PID = not restarted
            })
        })

        Context("when context is cancelled", func() {
            It("should stop execution", func() {
                ctx, cancel := context.WithCancel(context.Background())
                cancel() // Cancel immediately

                err := action.Execute(ctx)

                Expect(err).To(Equal(context.Canceled))
            })
        })
    })
})
```

## Mocking Patterns

### Mock Interfaces

Create mocks in `*_test.go` files (not exported):

```go
type mockWorker struct {
    collectErr   error
    observed     fsmv2.ObservedState
    initialState fsmv2.State
    collectFunc  func(ctx context.Context) (fsmv2.ObservedState, error)
}

func (m *mockWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
    if m.collectFunc != nil {
        return m.collectFunc(ctx)
    }
    if m.collectErr != nil {
        return nil, m.collectErr
    }
    return m.observed, nil
}

func (m *mockWorker) DeriveDesiredState(spec interface{}) (types.DesiredState, error) {
    return types.DesiredState{State: "running"}, nil
}

func (m *mockWorker) GetInitialState() fsmv2.State {
    if m.initialState != nil {
        return m.initialState
    }
    return &mockState{}
}
```

### Mock Store

For testing persistence:

```go
type mockTriangularStore struct {
    SaveIdentityErr error
    LoadIdentityErr error
    SaveDesiredErr  error
    LoadDesiredErr  error

    identity map[string]map[string]persistence.Document
    desired  map[string]map[string]persistence.Document
    observed map[string]map[string]interface{}

    SaveDesiredCalled  int
    SaveObservedCalled int
}

func newMockTriangularStore() *mockTriangularStore {
    return &mockTriangularStore{
        identity: make(map[string]map[string]persistence.Document),
        desired:  make(map[string]map[string]persistence.Document),
        observed: make(map[string]map[string]interface{}),
    }
}

// Implement TriangularStoreInterface methods...
```

### Mock Usage Pattern

```go
Describe("Supervisor", func() {
    var (
        supervisor *Supervisor
        mockStore  *mockTriangularStore
        mockWorker *mockWorker
    )

    BeforeEach(func() {
        mockStore = newMockTriangularStore()
        mockWorker = &mockWorker{
            observed: &mockObservedState{Running: true},
        }

        supervisor = newSupervisorWithWorker(mockWorker, mockStore, defaultConfig)
    })

    It("should save desired state on tick", func() {
        ctx := context.Background()
        err := supervisor.Tick(ctx)

        Expect(err).ToNot(HaveOccurred())
        Expect(mockStore.SaveDesiredCalled).To(BeNumerically(">", 0))
    })
})
```

## Test Helpers

### Idempotency Test Helper

Located in `pkg/fsmv2/supervisor/execution/action_test_helpers_test.go`:

```go
// VerifyActionIdempotency executes an action multiple times and verifies
// that it produces the same final state (idempotent behavior).
func VerifyActionIdempotency(
    action fsmv2.Action,
    iterations int,
    verifyState func(),
) {
    ctx := context.Background()

    for i := 0; i < iterations; i++ {
        err := action.Execute(ctx)
        Expect(err).ToNot(HaveOccurred(),
            fmt.Sprintf("Action execution %d/%d failed", i+1, iterations))
    }

    // Verify final state after all iterations
    verifyState()
}

// VerifyActionIdempotencyWithSetup allows setup/teardown between iterations
func VerifyActionIdempotencyWithSetup(
    setup func(),
    teardown func(),
    action fsmv2.Action,
    iterations int,
    verifyState func(),
) {
    for i := 0; i < iterations; i++ {
        setup()

        ctx := context.Background()
        err := action.Execute(ctx)
        Expect(err).ToNot(HaveOccurred())

        verifyState()
        teardown()
    }
}
```

### Test Fixture Helpers

```go
func mockIdentity() fsmv2.Identity {
    return fsmv2.Identity{
        ID:         "test-worker",
        Name:       "Test Worker",
        WorkerType: "container",
    }
}

func newSupervisorWithWorker(
    worker *mockWorker,
    customStore storage.TriangularStoreInterface,
    cfg supervisor.CollectorHealthConfig,
) *supervisor.Supervisor {
    identity := mockIdentity()

    // Setup store with collections...

    s := supervisor.NewSupervisor(supervisor.Config{
        WorkerType:      "container",
        Logger:          zap.NewNop().Sugar(),
        CollectorHealth: cfg,
        Store:           triangularStore,
    })

    err := s.AddWorker(identity, worker)
    if err != nil {
        panic(err)
    }

    return s
}
```

## Best Practices

### 1. Use Eventually() for Async Operations

```go
// GOOD: Wait for async operation
Eventually(func() bool {
    return executor.HasActionInProgress(actionID)
}).Should(BeFalse())

// BAD: Race condition
time.Sleep(100 * time.Millisecond)
Expect(executor.HasActionInProgress(actionID)).To(BeFalse())
```

### 2. Test Context Cancellation

All long-running operations must respect context cancellation:

```go
Describe("Context Cancellation", func() {
    It("should stop when context is cancelled", func() {
        ctx, cancel := context.WithCancel(context.Background())

        cancelled := make(chan bool, 1)
        action := &testAction{
            execute: func(ctx context.Context) error {
                select {
                case <-ctx.Done():
                    cancelled <- true
                    return ctx.Err()
                case <-time.After(1 * time.Second):
                    return nil
                }
            },
        }

        err := executor.EnqueueAction("test", action)
        Expect(err).ToNot(HaveOccurred())

        cancel()

        Eventually(cancelled).Should(Receive())
    })
})
```

### 3. Test Race Conditions

Enable race detector:

```bash
ginkgo --race ./...
```

Write tests that expose races:

```go
It("should handle concurrent access", func() {
    var wg sync.WaitGroup

    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            err := executor.EnqueueAction(fmt.Sprintf("action-%d", id), action)
            Expect(err).ToNot(HaveOccurred())
        }(i)
    }

    wg.Wait()
})
```

### 4. Clean Up Resources

Use `AfterEach` for cleanup:

```go
var (
    ctx    context.Context
    cancel context.CancelFunc
    server *testServer
)

BeforeEach(func() {
    ctx, cancel = context.WithCancel(context.Background())
    server = startTestServer()
})

AfterEach(func() {
    cancel()
    server.Shutdown()
})
```

### 5. Test Error Paths

Don't just test happy paths:

```go
Describe("Error Handling", func() {
    Context("when store returns error", func() {
        BeforeEach(func() {
            mockStore.SaveDesiredErr = errors.New("database error")
        })

        It("should return error from Tick", func() {
            err := supervisor.Tick(ctx)
            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("database error"))
        })
    })

    Context("when worker collect fails", func() {
        BeforeEach(func() {
            mockWorker.collectErr = errors.New("collection failed")
        })

        It("should handle gracefully", func() {
            // Test error handling
        })
    })
})
```

### 6. Use Descriptive Names

```go
// GOOD: Self-documenting
It("should transition to Degraded when health check fails repeatedly", func() {})

// BAD: Unclear intent
It("should work correctly", func() {})
```

### 7. One Assertion Per It

```go
// GOOD: Focused test
It("should return error when queue is full", func() {
    err := executor.EnqueueAction(id, action)
    Expect(err).To(HaveOccurred())
})

It("should include 'queue full' in error message", func() {
    err := executor.EnqueueAction(id, action)
    Expect(err.Error()).To(ContainSubstring("queue full"))
})

// ACCEPTABLE: Related assertions
It("should execute action and clear in-progress flag", func() {
    err := executor.EnqueueAction(id, action)
    Expect(err).ToNot(HaveOccurred())

    Eventually(func() bool {
        return executor.HasActionInProgress(id)
    }).Should(BeFalse())
})
```

### 8. Never Commit Focused Specs

```go
// BAD: CI will fail
FIt("should do something", func() { })
FDescribe("Component", func() { })
FContext("when X", func() { })

// GOOD: Use during development, remove before commit
It("should do something", func() { })
```

## Coverage Expectations

### Minimum Coverage

- **Actions**: 100% coverage required (idempotency tests mandatory)
- **State transitions**: All paths tested (shutdown, error, success)
- **Core supervisor logic**: >90% coverage
- **Edge cases**: Explicitly tested (nil checks, concurrent access, timeouts)

### Running Coverage

```bash
# Generate coverage report
go test -coverprofile=coverage.out ./pkg/fsmv2/...

# View coverage in browser
go tool cover -html=coverage.out

# Check coverage percentage
go tool cover -func=coverage.out | grep total
```

### What to Test

**Critical paths** (must have 100% coverage):
- Action execution and idempotency
- State machine transitions
- Error handling and recovery
- Context cancellation
- Race conditions

**Nice to have** (>80% coverage):
- Helper functions
- Logging code
- Metrics collection
- Edge case handling

**Can skip**:
- Generated code
- Trivial getters/setters
- Debug-only code paths

## Common Patterns Reference

### Testing Supervisor Tick

```go
Describe("Supervisor.Tick", func() {
    It("should execute one complete tick cycle", func() {
        ctx := context.Background()

        err := supervisor.Tick(ctx)

        Expect(err).ToNot(HaveOccurred())
        Expect(mockStore.SaveDesiredCalled).To(Equal(1))
        Expect(mockStore.SaveObservedCalled).To(Equal(1))
    })
})
```

### Testing Child Reconciliation

```go
Describe("Child Reconciliation", func() {
    It("should create missing children", func() {
        desiredState := types.DesiredState{
            ChildrenSpecs: []types.ChildSpec{
                {Name: "child-1", WorkerType: "container"},
            },
        }

        err := supervisor.reconcileChildren(ctx, worker, desiredState)

        Expect(err).ToNot(HaveOccurred())

        children := supervisor.ListChildren()
        Expect(children).To(HaveLen(1))
        Expect(children[0].Identity.Name).To(Equal("child-1"))
    })
})
```

### Testing Metrics

```go
Describe("Metrics", func() {
    var metricsRegistry *prometheus.Registry

    BeforeEach(func() {
        metricsRegistry = prometheus.NewRegistry()
        // Register metrics
    })

    It("should increment action counter on execution", func() {
        action := &testAction{execute: func(ctx context.Context) error { return nil }}

        executor.EnqueueAction("test", action)
        Eventually(executed).Should(Receive())

        metrics, _ := metricsRegistry.Gather()
        counter := findMetric(metrics, "fsmv2_actions_total")
        Expect(counter.GetCounter().GetValue()).To(Equal(1.0))
    })
})
```

---

## Summary

- Use Ginkgo v2 for BDD-style testing
- Organize tests hierarchically: Describe → Context → It
- Write idempotency tests for all actions
- Use `Eventually()` for async assertions
- Enable race detection with `--race`
- Test error paths, not just happy paths
- Never commit focused specs (CI enforces this)
- Aim for >90% coverage on critical paths

For more examples, browse existing tests in `pkg/fsmv2/supervisor/*_test.go`.
