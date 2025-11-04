# FSMv2 Child Removal Auto Plan

## Overview

Implement automatic child removal when workers no longer declare children in `DeriveDesiredState()`. This follows declarative system patterns (Kubernetes, Terraform, React) where components are automatically cleaned up when removed from the desired state.

## Core Principle

**Workers declare desired state, supervisor reconciles reality to match.**

If a child is no longer in the desired state declaration, it should be automatically removed with clear logging to prevent developer surprise.

## Architecture

### Worker API

Workers declare children via `DeriveDesiredState()`:

```go
func (w *ProtocolConverterWorker) DeriveDesiredState() *ChildDeclaration {
    children := []ChildSpec{}

    // Only declare connection child when needed
    if w.needsConnection() {
        children = append(children, ChildSpec{
            Name: "connection",
            Supervisor: w.connectionSupervisor,
            // ... config
        })
    }

    // Dynamically add DFC children based on config
    for _, dfcConfig := range w.config.DataFlowComponents {
        children = append(children, ChildSpec{
            Name: fmt.Sprintf("dfc_%s", dfcConfig.Name),
            Supervisor: w.dfcSupervisor,
            // ... config
        })
    }

    return &ChildDeclaration{Children: children}
}
```

**No explicit removal code needed.** When a DFC is removed from config, it simply won't appear in the next `DeriveDesiredState()` call.

### Supervisor Implementation

The supervisor maintains a child registry and performs diff-based reconciliation:

```go
type childRegistry struct {
    current map[string]*childInstance // Name -> instance
    desired map[string]ChildSpec      // Name -> spec
}

func (s *Supervisor) reconcileChildren(desired *ChildDeclaration) {
    s.registry.desired = indexByName(desired.Children)

    // Add new children
    for name, spec := range s.registry.desired {
        if _, exists := s.registry.current[name]; !exists {
            s.addChild(name, spec)
        }
    }

    // Remove children not in desired state
    for name, instance := range s.registry.current {
        if _, desired := s.registry.desired[name]; !desired {
            s.removeChild(name, instance)
        }
    }

    // Update existing children
    for name, spec := range s.registry.desired {
        if instance, exists := s.registry.current[name]; exists {
            s.updateChild(name, instance, spec)
        }
    }
}
```

## Removal Process

### 1. Detection

On each tick, supervisor calls `worker.DeriveDesiredState()` and diffs against current children:

```go
func (s *Supervisor) Tick() error {
    // Get desired state from worker
    desired := s.worker.DeriveDesiredState()

    // Detect removals
    toRemove := []string{}
    for name := range s.registry.current {
        if _, stillDesired := desired.Children[name]; !stillDesired {
            toRemove = append(toRemove, name)
        }
    }

    // Log removals BEFORE executing
    if len(toRemove) > 0 {
        s.logger.Info("Auto-removing children no longer in desired state",
            "worker", s.workerName,
            "children", toRemove,
            "reason", "not_in_desired_state")
    }

    // Execute removal with finalizers
    for _, name := range toRemove {
        s.removeChild(name)
    }

    // Continue with normal tick...
}
```

### 2. Finalizers (Graceful Shutdown)

Before removing a child, supervisor runs finalizers for graceful cleanup:

```go
func (s *Supervisor) removeChild(name string) error {
    instance := s.registry.current[name]

    // Run finalizers (e.g., set desired state to "stopped")
    if err := s.runFinalizers(instance); err != nil {
        s.logger.Error("Finalizer failed, forcing removal",
            "child", name,
            "error", err)
        // Continue with removal despite error
    }

    // Remove from registry
    delete(s.registry.current, name)

    // Clean up TriangularStore entries
    s.store.DeleteIdentity(instance.identity)

    s.logger.Info("Child removed",
        "child", name,
        "final_state", instance.lastObservedState)

    return nil
}
```

### 3. Logging Strategy

**Verbose logging prevents surprise:**

```
INFO Auto-removing children no longer in desired state
    worker=protocol_converter
    children=[dfc_deprecated_sensor, dfc_old_machine]
    reason=not_in_desired_state

INFO Running finalizer for child
    child=dfc_deprecated_sensor
    finalizer=graceful_stop
    setting_desired_state=stopped

INFO Child stopped gracefully
    child=dfc_deprecated_sensor
    shutdown_duration=245ms

INFO Child removed
    child=dfc_deprecated_sensor
    final_state=stopped
```

## Patterns from Research

### Kubernetes Garbage Collection
- **ownerReferences** create parent-child relationships
- **Automatic deletion** when parent removed or child not desired
- **Finalizers** run before deletion (graceful cleanup)
- **Cascading deletion** with configurable policies

### Terraform Plan/Apply
- **Plan phase** shows what will be removed (dry-run)
- **Explicit confirmation** before destructive changes
- **State tracking** in terraform.tfstate

### React Component Lifecycle
- **componentWillUnmount()** called before removal
- **Automatic cleanup** when component removed from render tree
- **No manual removal** required - declarative

### Docker Compose
- **`--remove-orphans`** flag auto-removes unlisted services
- **Verbose output** shows removed containers
- **Explicit opt-in** via flag

## FSMv2 Implementation Strategy

### Phase 1: Basic Auto-Removal (Milestone 1)

```go
// supervisor/child_lifecycle.go

func (s *Supervisor) reconcileChildren(desired *ChildDeclaration) {
    // Simple diff and remove
    for name := range s.registry.current {
        if !desired.HasChild(name) {
            s.logger.Warn("Auto-removing child",
                "child", name,
                "worker", s.workerName)
            delete(s.registry.current, name)
        }
    }
}
```

**Deliverable:** Children automatically removed with warning logs.

### Phase 2: Finalizers (Milestone 2)

```go
// supervisor/finalizers.go

type Finalizer interface {
    Run(ctx context.Context, child *childInstance) error
}

type GracefulStopFinalizer struct{}

func (f *GracefulStopFinalizer) Run(ctx context.Context, child *childInstance) error {
    // Set desired state to "stopped"
    child.supervisor.SetDesiredState("stopped")

    // Wait for child to reach stopped state (with timeout)
    return child.supervisor.WaitForState(ctx, "stopped", 10*time.Second)
}

// Worker can declare finalizers
func (w *ProtocolConverterWorker) DeriveDesiredState() *ChildDeclaration {
    return &ChildDeclaration{
        Children: []ChildSpec{
            {
                Name: "connection",
                Supervisor: w.connectionSupervisor,
                Finalizers: []Finalizer{
                    &GracefulStopFinalizer{},
                },
            },
        },
    }
}
```

**Deliverable:** Graceful shutdown before removal.

### Phase 3: TriangularStore Cleanup (Milestone 3)

```go
// supervisor/storage_cleanup.go

func (s *Supervisor) removeChild(name string) error {
    instance := s.registry.current[name]

    // Run finalizers first
    s.runFinalizers(instance)

    // Clean up storage
    if err := s.store.DeleteIdentity(instance.identity); err != nil {
        s.logger.Error("Failed to clean up storage",
            "child", name,
            "error", err)
        // Continue with removal despite storage error
    }

    // Remove from registry
    delete(s.registry.current, name)

    return nil
}
```

**Deliverable:** No orphaned state in TriangularStore.

### Phase 4: Metrics and Observability (Milestone 4)

```go
// supervisor/metrics.go

var (
    childrenRemoved = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "fsmv2_children_removed_total",
            Help: "Total number of children auto-removed",
        },
        []string{"worker", "child_type"},
    )

    childRemovalDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "fsmv2_child_removal_duration_seconds",
            Help: "Time taken to remove a child including finalizers",
        },
        []string{"worker", "child_type"},
    )
)

func (s *Supervisor) removeChild(name string) error {
    start := time.Now()
    defer func() {
        childRemovalDuration.WithLabelValues(
            s.workerName,
            instance.spec.Type,
        ).Observe(time.Since(start).Seconds())

        childrenRemoved.WithLabelValues(
            s.workerName,
            instance.spec.Type,
        ).Inc()
    }()

    // ... removal logic
}
```

**Deliverable:** Prometheus metrics for removal operations.

## Edge Cases

### 1. Rapid Config Changes

**Scenario:** Config changes rapidly, child added then removed within seconds.

**Solution:** Grace period before removal (configurable per child type):

```go
type ChildSpec struct {
    Name string
    Supervisor *Supervisor
    RemovalGracePeriod time.Duration // Default: 0 (immediate)
}

type childInstance struct {
    spec ChildSpec
    removedAt *time.Time // Set when first detected as not desired
}

func (s *Supervisor) reconcileChildren(desired *ChildDeclaration) {
    for name, instance := range s.registry.current {
        if !desired.HasChild(name) {
            if instance.removedAt == nil {
                // First time not desired, start grace period
                now := time.Now()
                instance.removedAt = &now
                s.logger.Info("Child scheduled for removal",
                    "child", name,
                    "grace_period", instance.spec.RemovalGracePeriod)
            } else if time.Since(*instance.removedAt) > instance.spec.RemovalGracePeriod {
                // Grace period elapsed, remove now
                s.removeChild(name)
            }
        } else {
            // Child is desired again, cancel removal
            if instance.removedAt != nil {
                s.logger.Info("Child removal cancelled",
                    "child", name,
                    "reason", "reappeared_in_desired_state")
                instance.removedAt = nil
            }
        }
    }
}
```

### 2. Finalizer Timeout

**Scenario:** Child won't stop gracefully, finalizer blocks.

**Solution:** Context timeout with forced removal:

```go
func (s *Supervisor) runFinalizers(instance *childInstance) error {
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    for _, finalizer := range instance.spec.Finalizers {
        if err := finalizer.Run(ctx, instance); err != nil {
            if errors.Is(err, context.DeadlineExceeded) {
                s.logger.Error("Finalizer timeout, forcing removal",
                    "child", instance.spec.Name,
                    "finalizer", fmt.Sprintf("%T", finalizer))
                return err // Continue with forced removal
            }
            return err
        }
    }

    return nil
}
```

### 3. Parent Shutdown During Child Removal

**Scenario:** Parent receives shutdown signal while removing children.

**Solution:** Wait for removal to complete (don't orphan):

```go
func (s *Supervisor) Shutdown(ctx context.Context) error {
    s.logger.Info("Shutdown initiated, removing all children")

    // Remove all children before shutdown
    for name := range s.registry.current {
        select {
        case <-ctx.Done():
            s.logger.Error("Shutdown timeout, forcing exit",
                "remaining_children", len(s.registry.current))
            return ctx.Err()
        default:
            if err := s.removeChild(name); err != nil {
                s.logger.Error("Failed to remove child during shutdown",
                    "child", name,
                    "error", err)
                // Continue removing other children
            }
        }
    }

    return nil
}
```

## Testing Strategy

### Unit Tests

```go
func TestAutoRemoval(t *testing.T) {
    supervisor := NewSupervisor(mockWorker)

    // Tick 1: Child declared
    mockWorker.SetDesiredChildren([]ChildSpec{
        {Name: "child1"},
    })
    supervisor.Tick()
    assert.Equal(t, 1, len(supervisor.Children()))

    // Tick 2: Child no longer declared
    mockWorker.SetDesiredChildren([]ChildSpec{})
    supervisor.Tick()
    assert.Equal(t, 0, len(supervisor.Children()))
}

func TestGracePeriod(t *testing.T) {
    spec := ChildSpec{
        Name: "child1",
        RemovalGracePeriod: 5 * time.Second,
    }

    // Tick 1: Child removed from desired state
    supervisor.Tick() // child1 scheduled for removal
    assert.Equal(t, 1, len(supervisor.Children()))

    // Tick 2: Within grace period
    time.Sleep(2 * time.Second)
    supervisor.Tick()
    assert.Equal(t, 1, len(supervisor.Children())) // Still exists

    // Tick 3: After grace period
    time.Sleep(4 * time.Second)
    supervisor.Tick()
    assert.Equal(t, 0, len(supervisor.Children())) // Removed
}

func TestRemovalCancellation(t *testing.T) {
    // Child scheduled for removal
    supervisor.Tick()

    // Child reappears before grace period
    mockWorker.SetDesiredChildren([]ChildSpec{{Name: "child1"}})
    supervisor.Tick()

    assert.Equal(t, 1, len(supervisor.Children()))
    assert.Nil(t, supervisor.Children()["child1"].removedAt)
}
```

### Integration Tests

```go
func TestProtocolConverterDFCRemoval(t *testing.T) {
    // Setup ProtocolConverter with 3 DFCs
    config := &Config{
        DataFlowComponents: []DFCConfig{
            {Name: "sensor1"},
            {Name: "sensor2"},
            {Name: "sensor3"},
        },
    }
    pc := NewProtocolConverterWorker(config)
    supervisor := NewSupervisor(pc)

    supervisor.Tick()
    assert.Equal(t, 4, len(supervisor.Children())) // connection + 3 DFCs

    // Update config to remove sensor2
    config.DataFlowComponents = []DFCConfig{
        {Name: "sensor1"},
        {Name: "sensor3"},
    }
    pc.UpdateConfig(config)

    supervisor.Tick()
    assert.Equal(t, 3, len(supervisor.Children())) // connection + 2 DFCs
    assert.Nil(t, supervisor.Children()["dfc_sensor2"]) // Removed
}
```

## Migration from FSMv1

FSMv1 requires manual child removal:

```go
// FSMv1 (manual removal)
func (s *ProtocolConverterService) RemoveDFC(name string) error {
    manager, exists := s.dfcManagers[name]
    if !exists {
        return fmt.Errorf("DFC not found: %s", name)
    }

    // Manual shutdown
    if err := manager.Stop(); err != nil {
        return err
    }

    delete(s.dfcManagers, name)
    return nil
}
```

FSMv2 auto-removal:

```go
// FSMv2 (automatic removal)
func (w *ProtocolConverterWorker) DeriveDesiredState() *ChildDeclaration {
    children := []ChildSpec{}

    // Simply don't include removed DFC
    for _, dfcConfig := range w.config.DataFlowComponents {
        children = append(children, ChildSpec{
            Name: fmt.Sprintf("dfc_%s", dfcConfig.Name),
            // ...
        })
    }

    return &ChildDeclaration{Children: children}
}
// No manual removal code needed!
```

## Success Metrics

1. **Developer Experience:**
   - Zero manual removal code in workers
   - Clear logs prevent surprise
   - Grace periods catch config errors

2. **Reliability:**
   - No orphaned children in TriangularStore
   - Graceful shutdown via finalizers
   - Metrics track removal operations

3. **Performance:**
   - Removal completes within one tick cycle (< 100ms)
   - Finalizers timeout after 30s max
   - No memory leaks from orphaned state

## Rollout Plan

1. **Week 1:** Implement basic auto-removal (Phase 1)
2. **Week 2:** Add finalizers and graceful shutdown (Phase 2)
3. **Week 3:** TriangularStore cleanup (Phase 3)
4. **Week 4:** Metrics and observability (Phase 4)
5. **Week 5:** Integration testing with real workers
6. **Week 6:** Documentation and migration guide

## Documentation

Create user guide at `docs/fsmv2/child-lifecycle.md`:

```markdown
# Child Lifecycle in FSMv2

## Automatic Removal

Children are automatically removed when no longer declared in `DeriveDesiredState()`:

// Child exists
func (w *Worker) DeriveDesiredState() *ChildDeclaration {
    return &ChildDeclaration{
        Children: []ChildSpec{{Name: "child1"}},
    }
}

// Child automatically removed
func (w *Worker) DeriveDesiredState() *ChildDeclaration {
    return &ChildDeclaration{
        Children: []ChildSpec{}, // child1 removed
    }
}

## Graceful Shutdown

Add finalizers for graceful cleanup:

Children: []ChildSpec{
    {
        Name: "connection",
        Finalizers: []Finalizer{
            &GracefulStopFinalizer{},
        },
    },
}

## Grace Periods

Prevent accidental removal from rapid config changes:

Children: []ChildSpec{
    {
        Name: "sensor",
        RemovalGracePeriod: 30 * time.Second,
    },
}
```

## Conclusion

Auto-removal follows declarative system best practices:
- ✅ Workers declare intent, supervisor handles execution
- ✅ No manual cleanup code required
- ✅ Verbose logging prevents surprise
- ✅ Finalizers enable graceful shutdown
- ✅ Grace periods catch config errors
- ✅ Metrics provide observability

This aligns with FSMv2 goal: **Workers remain simple, complexity extracted to supervisor.**
