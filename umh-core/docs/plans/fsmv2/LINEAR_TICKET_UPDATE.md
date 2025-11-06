# ENG-3806: Updated Linear Ticket Description

## Ticket Title
**FSMv2: Hierarchical Composition, Templating, Supervision & Async Actions**

---

## Description

### Objective

Build **complete FSMv2 framework** (Phases 0-4, except SQLite) and **validate with Communicator Worker** implementation.

**Primary Goal:** Implement production-ready FSMv2 framework with:
- **Hierarchical composition** - Parent FSMs manage children declaratively (e.g., ProtocolConverter → Connection + DataFlows)
- **Template-driven configuration** - Reusable configs with Go template engine
- **Robust infrastructure supervision** - Circuit breaker pattern for graceful failure recovery
- **Non-blocking async actions** - Global worker pool prevents tick loop blockage
- **Comprehensive monitoring** - Prometheus metrics and structured logging

**Validation:** Refactor Communicator to use FSMv2 framework, proving the pattern with real-world production component.

**Approach:** Build framework FIRST (using mock workers for testing), then implement Communicator Worker to validate framework completeness.

---

### Background

[ENG-3764](https://linear.app/united-manufacturing-hub/issue/ENG-3764/add-watchdog-restart-capability-for-graceful-goroutine-recovery) (watchdog restart capability) was **CANCELLED**. Root cause analysis revealed architectural issues:
- **32+ goroutines** with **42 watchdog instances**
- **2 mutexes** with deadlock risks
- **HTTP client singleton** that cannot be reset
- **Untestable code** with package-level globals

**Decision:** Build FSMv2 framework that eliminates the need for watchdog restarts entirely. Start with Communicator as the first FSMv2 component, proving the pattern before migrating other FSMs.

---

### Architecture

FSMv2 introduces these core abstractions:

1. **Supervisor** - Manages worker lifecycle, children, health checks, actions
2. **Worker** - Business logic in `DeriveDesiredState()` and `NextAction()`
3. **ChildSpec** - Declarative child specification (like Kubernetes PodSpec)
4. **VariableBundle** - Three-tier namespace (User/Global/Internal) for configuration
5. **ActionExecutor** - Global worker pool for async operations
6. **Circuit Breaker** - Infrastructure health monitoring with exponential backoff

**Design Patterns:**
- Kubernetes-style reconciliation (declarative children specs)
- Circuit breaker (infrastructure health monitoring)
- Worker pool (non-blocking action execution)
- Template engine (configuration reuse)
- Observer pattern (state machine + observations)

**Framework vs Communicator Separation:**

| Component | Location | Purpose |
|-----------|----------|---------|
| **Framework** (Generic, Reusable) | `pkg/fsmv2/` | Core FSMv2 abstractions for ANY Worker |
| - Supervisor | `supervisor/supervisor.go` | Manages worker lifecycle, children, health, actions |
| - Worker Interface | `types/worker.go` | Contract: DeriveDesiredState(), NextAction() |
| - ChildSpec | `types/childspec.go` | Declarative child specification |
| - Templates | `template/render.go` | Go template engine with variable injection |
| - Circuit Breaker | `health/infrastructure_checker.go` | Infrastructure health monitoring |
| - Async Actions | `executor/action_executor.go` | Global worker pool |
| - Monitoring | `metrics/prometheus.go` | Metrics and structured logging |
| **Communicator** (Specific Worker) | `pkg/fsmv2/communicator/` | Validates framework with real implementation |
| - Worker | `communicator/worker.go` | Implements Worker interface |
| - States | `communicator/states/` | Stopped, Authenticating, Syncing, Degraded |
| - Actions | `communicator/actions/` | AuthenticateAction, SyncAction |

**Implementation Sequence:**
1. **Phases 0-3:** Build framework (test with mock workers)
2. **After Phase 3:** Implement Communicator Worker (validates framework)
3. **Phase 4:** Add monitoring (parallel with Communicator)

---

### Timeline

**Total Duration:** 12 weeks (10 weeks development + 2 weeks buffer)

**Phase Dependencies:**
- Phase 0 blocks ALL other phases (foundation)
- Phase 0.5 blocks Benthos worker implementation
- Phases 0, 0.5, 1, 2 block Phase 3 (integration)
- Phase 3 blocks Phase 4 (monitoring)

---

### Phase Breakdown

#### **Phase 0: Hierarchical Composition** (Weeks 1-2, 410 LOC)

**Deliverables:**
- Parent FSMs declare children via `ChildSpec` in `DeriveDesiredState()`
- Supervisor reconciles children (add/update/remove) based on specs
- `WorkerFactory` pattern for dynamic worker creation
- `StateMapping` allows parent state to control child states
- Recursive tick propagation (parent → children → grandchildren)

**Files Created:**
- `pkg/fsmv2/types/childspec.go` - ChildSpec and DesiredState structures
- `pkg/fsmv2/factory/worker_factory.go` - WorkerFactory implementation
- `pkg/fsmv2/supervisor/supervisor.go` - reconcileChildren(), applyStateMapping(), tickChildren()
- `pkg/fsmv2/integration/hierarchical_composition_test.go` - 7 integration scenarios

**Acceptance Criteria:**
- [ ] Parent can declare children via ChildSpec
- [ ] reconcileChildren() adds/updates/removes children
- [ ] StateMapping applied correctly
- [ ] Recursive tick propagates to all children
- [ ] Unit test coverage >90%

**Detailed Plan:** [phase-0-hierarchical-composition.md](./phase-0-hierarchical-composition.md)

---

#### **Phase 0.5: Templating & Variables** (Weeks 3-4, 325 LOC)

**Deliverables:**
- Three-tier variable namespace system (User/Global/Internal)
- Template rendering with Go `text/template` engine
- Variable flattening (User variables promoted to top-level for templates)
- Location computation (ISA-95 hierarchy: enterprise → site → area → line → cell)
- Variable propagation from parent to children

**Files Created:**
- `pkg/fsmv2/types/variables.go` - VariableBundle structure
- `pkg/fsmv2/template/render.go` - RenderTemplate() implementation
- `pkg/fsmv2/location/compute.go` - Location merging and path computation
- `pkg/fsmv2/supervisor/supervisor.go` - Variable injection in Tick()
- `pkg/fsmv2/integration/templating_variables_test.go` - 7 integration scenarios

**Acceptance Criteria:**
- [ ] VariableBundle has User/Global/Internal namespaces
- [ ] Flatten() promotes User variables to top-level
- [ ] RenderTemplate() renders templates with strict mode
- [ ] Location merging combines parent + child
- [ ] Variables propagate through hierarchy
- [ ] Unit test coverage >85%

**Detailed Plan:** [phase-0.5-templating-variables.md](./phase-0.5-templating-variables.md)

---

#### **Phase 1: Infrastructure Supervision** (Weeks 5-6, 320 LOC)

**Deliverables:**
- Circuit breaker pattern for infrastructure health monitoring
- Child consistency checks (children exist and are healthy)
- Exponential backoff for child restart retries
- Circuit opens on infrastructure failure, closes on recovery

**Files Created:**
- `pkg/fsmv2/backoff/exponential_backoff.go` - ExponentialBackoff utility
- `pkg/fsmv2/health/infrastructure_checker.go` - InfrastructureHealthChecker
- `pkg/fsmv2/supervisor/supervisor.go` - Circuit breaker integration
- `pkg/fsmv2/supervisor/circuit_breaker_test.go` - Circuit breaker tests

**Acceptance Criteria:**
- [ ] Circuit opens on infrastructure failure
- [ ] Failed children restart with exponential backoff
- [ ] Circuit closes when all children healthy
- [ ] Deriv/actions skipped when circuit open
- [ ] Unit test coverage >80%

**Detailed Plan:** [phase-1-infrastructure-supervision.md](./phase-1-infrastructure-supervision.md)

---

#### **Phase 2: Async Action Executor** (Weeks 7-8, 260 LOC)

**Deliverables:**
- Global worker pool for async action execution
- Non-blocking action queueing (tick never blocks)
- Action timeout handling with context cancellation
- Per-child action queues (identified by child name)

**Files Created:**
- `pkg/fsmv2/executor/action_executor.go` - ActionExecutor implementation
- `pkg/fsmv2/executor/worker_pool.go` - Goroutine pool management
- `pkg/fsmv2/supervisor/supervisor.go` - ActionExecutor integration
- `pkg/fsmv2/executor/action_executor_test.go` - Executor tests

**Acceptance Criteria:**
- [ ] Actions execute in global worker pool (non-blocking)
- [ ] Tick never blocks on action execution
- [ ] Actions time out after configured duration
- [ ] Multiple workers execute actions concurrently
- [ ] Unit test coverage >80%

**Detailed Plan:** [phase-2-async-actions.md](./phase-2-async-actions.md)

---

#### **Phase 3: Integration & Edge Cases** (Weeks 9-10, 330 LOC)

**Deliverables:**
- Complete Supervisor.Tick() loop integrating all phases
- Edge case handling (action during child restart, observation during circuit open)
- Template rendering performance tests (100+ children)
- Variable flow tests (3-level hierarchy)
- Location hierarchy tests (ISA-95 path computation)
- ProtocolConverter end-to-end test (real-world migration scenario)

**Complete Tick Loop order:**
1. Infrastructure Health Check (Phase 1)
2. Derive Desired State with variable injection (Phase 0 + 0.5)
3. Reconcile Children (Phase 0)
4. Apply State Mapping (Phase 0)
5. Tick Children recursively (Phase 0)
6. Async Actions (Phase 2)

**Files Created:**
- `pkg/fsmv2/supervisor/supervisor.go` - Complete tick loop
- `pkg/fsmv2/integration/complete_tick_test.go` - Full integration tests
- `pkg/fsmv2/integration/edge_cases_test.go` - Edge case scenarios
- `pkg/fsmv2/integration/protocol_converter_test.go` - Real-world E2E test

**Acceptance Criteria:**
- [ ] Complete tick loop integrates all phases
- [ ] Actions cancelled/retried during child restart
- [ ] Observations collected during circuit open
- [ ] Templates render correctly at scale (100+ children)
- [ ] Variables propagate through 3-level hierarchy
- [ ] Location paths computed correctly
- [ ] ProtocolConverter end-to-end works
- [ ] Integration test coverage >90%

**Detailed Plan:** [phase-3-integration.md](./phase-3-integration.md)

---

#### **Phase 4: Monitoring & Observability** (Week 11, 75 LOC)

**Deliverables:**
- Prometheus metrics for circuit breaker, actions, children, templates
- Structured logging with error distinction (infrastructure vs application)
- UX enhancements (heartbeat logs, pre-escalation warnings, runbook embedding)
- Runbook for common troubleshooting scenarios

**Key Metrics:**
- Circuit breaker: `fsmv2_circuit_open`, `fsmv2_infrastructure_recovery_total`
- Actions: `fsmv2_action_execution_duration_seconds`, `fsmv2_action_timeout_total`
- Children: `fsmv2_children_count`, `fsmv2_reconciliation_duration_seconds`
- Templates: `fsmv2_template_rendering_duration_seconds`
- Worker pool: `fsmv2_worker_pool_utilization`

**UX Enhancements:**
- Heartbeat logs during circuit breaker recovery
- Pre-escalation warnings at retry attempt 4
- Runbook URLs embedded in escalation errors
- Error scope distinction (infrastructure vs worker)

**Files Created:**
- `pkg/fsmv2/metrics/prometheus.go` - Prometheus metrics
- `pkg/fsmv2/supervisor/supervisor.go` - Structured logging integration
- `docs/runbooks/fsmv2-troubleshooting.md` - Runbook documentation

**Acceptance Criteria:**
- [ ] All metrics exposed via Prometheus endpoint
- [ ] Structured logging at appropriate levels
- [ ] Runbook covers common scenarios
- [ ] Metrics collection adds <1ms latency

**Detailed Plan:** [phase-4-monitoring.md](./phase-4-monitoring.md)

---

### File Structure

**Total Code:** ~1,720 LOC production code + ~2,000+ LOC tests

**Files by Phase:**
- Phase 0: 4 files (410 LOC)
- Phase 0.5: 4 files (325 LOC)
- Phase 1: 3 files (320 LOC)
- Phase 2: 3 files (260 LOC)
- Phase 3: 4 files (330 LOC)
- Phase 4: 2 files (75 LOC)

**Total:** ~20 new production files across 6 phases

---

### Testing Strategy

**Unit Tests:**
- Phase 0: >90% coverage (foundational)
- Phase 0.5: >85% coverage (template rendering critical)
- Phases 1, 2: >80% coverage
- All phases: Ginkgo/Gomega BDD style

**Integration Tests:**
- 7+ scenarios per phase (minimum)
- Hierarchical composition scenarios
- Variable propagation scenarios
- Template rendering scenarios
- Circuit breaker scenarios
- Edge case scenarios

**System Tests:**
- ProtocolConverter end-to-end migration
- 3-level hierarchy performance tests
- Template rendering at scale (100+ children)

**Performance Benchmarks:**
- reconcileChildren() <10ms for 10 children
- Template rendering <5ms per child
- Tick propagation <5ms for 3-level hierarchy
- Action queueing <1ms

---

### Integration with Communicator

FSMv2 Communicator will be the first production FSMv2 component:

**States:**
1. StoppedState
2. AuthenticatingState
3. SyncingState
4. DegradedState (with exponential backoff)

**Actions:**
1. AuthenticateAction - POST /auth to get JWT
2. SyncAction - Delegates to orchestrator

**Transport Layer:**
- Uses existing umh-core channel-based push/pull protocol (not CSE)
- Inbound/Outbound channels for bidirectional communication
- Puller/Pusher goroutines with 10ms tick rate

**Goroutine Reduction:**
- **Before:** 32+ goroutines, 42 watchdog instances
- **After:** 2 goroutines (observation loop + tick loop)

**Dependencies Removed:**
- CSE (Central Storage Engine) sync protocol
- Package-level globals
- HTTP client singleton
- All 42 watchdog instances

---

### Documentation

**Phase Plans:**
- [README.md](./README.md) - Phase navigation
- [phase-0-hierarchical-composition.md](./phase-0-hierarchical-composition.md) - Phase 0 details
- [phase-0.5-templating-variables.md](./phase-0.5-templating-variables.md) - Phase 0.5 details
- [phase-1-infrastructure-supervision.md](./phase-1-infrastructure-supervision.md) - Phase 1 details
- [phase-2-async-actions.md](./phase-2-async-actions.md) - Phase 2 details
- [phase-3-integration.md](./phase-3-integration.md) - Phase 3 details
- [phase-4-monitoring.md](./phase-4-monitoring.md) - Phase 4 details

**Master Plan (with TDD examples):**
- [2025-11-02-fsmv2-supervision-and-async-actions.md](../2025-11-02-fsmv2-supervision-and-async-actions.md) - Complete TDD task breakdowns

---

### Overall Acceptance Criteria

**Framework Completeness (Primary Goal):**

- [ ] **Phase 0:** Hierarchical composition
  - Parent FSMs declare children via ChildSpec
  - reconcileChildren() adds/updates/removes children
  - StateMapping applied correctly
  - Recursive tick propagates to all children
  - Unit test coverage >90%

- [ ] **Phase 0.5:** Templating & variables
  - VariableBundle has User/Global/Internal namespaces
  - Flatten() promotes User variables to top-level
  - RenderTemplate() renders templates with strict mode
  - Location merging combines parent + child
  - Variables propagate through hierarchy
  - Unit test coverage >85%

- [ ] **Phase 1:** Infrastructure supervision
  - Circuit opens on infrastructure failure
  - Failed children restart with exponential backoff
  - Circuit closes when all children healthy
  - Deriv/actions skipped when circuit open
  - Unit test coverage >80%

- [ ] **Phase 2:** Async actions
  - Actions execute in global worker pool (non-blocking)
  - Tick never blocks on action execution
  - Actions time out after configured duration
  - Multiple workers execute actions concurrently
  - Unit test coverage >80%

- [ ] **Phase 3:** Integration & edge cases
  - Complete tick loop integrates all phases
  - Actions cancelled/retried during child restart
  - Observations collected during circuit open
  - Templates render correctly at scale (100+ children)
  - Variables propagate through 3-level hierarchy
  - Location paths computed correctly
  - Integration test coverage >90%

- [ ] **Phase 4:** Monitoring & observability
  - All metrics exposed via Prometheus endpoint
  - Structured logging at appropriate levels
  - Runbook covers common scenarios
  - Metrics collection adds <1ms latency

**Communicator Validation (Proves Framework Works):**

- [ ] **Communicator Worker Implementation:**
  - Worker implements Worker interface correctly
  - States: Stopped, Authenticating, Syncing, Degraded
  - Actions: AuthenticateAction, SyncAction
  - Transport layer uses channels (Puller/Pusher)
  - Goroutines reduced from 32+ to 2
  - All 42 watchdog instances eliminated
  - Backward compatible with Router/Subscriber
  - All Communicator integration tests pass

**Quality & Performance:**

- [ ] Overall test coverage >85% (framework + Communicator)
- [ ] All Ginkgo tests pass
- [ ] No focused specs
- [ ] golangci-lint and go vet pass
- [ ] Code review approved
- [ ] Performance benchmarks met:
  - Complete tick cycle <10ms (3-level hierarchy)
  - Template rendering <5ms per child
  - 100+ children supported
  - Action queueing <1ms

**Documentation:**

- [ ] All phase plans documented
- [ ] Runbook for common scenarios
- [ ] Architecture decision records updated
- [ ] Migration guide for other FSMs (future: Benthos, Redpanda, S6)

---

### Dependencies

**Blocks:**
- Phase 0 blocks all other phases (foundation)
- Complete FSMv2 framework blocks other FSM migrations

**Blocked By:**
- None (greenfield implementation)

**Related Issues:**
- [ENG-3764](https://linear.app/united-manufacturing-hub/issue/ENG-3764/add-watchdog-restart-capability-for-graceful-goroutine-recovery) - CANCELLED (parent issue)

**Related PRs:**
- PR #2302 - FSM v2 ↔ CSE interface contract (reference for patterns)
- PR #2235 - FSM v2 Container implementation (reference for patterns)

---

### Notes

**Framework-First Approach:**
- Build complete FSMv2 framework (Phases 0-4) using mock workers for testing
- After Phase 3, framework is "complete" and ready for validation
- Implement Communicator Worker to prove framework works with real production component
- This validates framework design before migrating other FSMs

**What "Complete FSMv2 (Except SQLite)" Means:**

✅ **Included (Complete Framework):**
- Phase 0: Hierarchical composition (ChildSpec, reconciliation, StateMapping, recursive tick)
- Phase 0.5: Templating & variables (VariableBundle, RenderTemplate, location computation)
- Phase 1: Infrastructure supervision (circuit breaker, health checks, exponential backoff)
- Phase 2: Async actions (ActionExecutor, worker pool, timeouts)
- Phase 3: Integration (complete tick loop, edge cases, performance validation)
- Phase 4: Monitoring (Prometheus metrics, structured logging, runbooks)
- In-memory TriangularStore (sufficient for production use)

❌ **Deferred (Can Come Later):**
- SQLite persistent store (framework uses Store interface, not specific implementation)
- Workers don't depend on SQLite - in-memory store is transparent
- Adding SQLite later doesn't change Worker API or framework behavior

**Why SQLite Can Wait:**
- Framework completeness = all phases working with in-memory store
- SQLite is a storage optimization, not a framework requirement
- Store interface already designed (ChildStore() method isolates child data)
- In-memory implementation proves framework works correctly

**Communicator as Proving Ground:**
- First production FSMv2 component demonstrates pattern viability
- Goroutine reduction (32+ → 2) proves supervision model works
- Watchdog elimination (42 → 0) proves circuit breaker works
- Real-world validation before migrating other FSMs (Benthos, Redpanda, S6)

**Phase 0 is Critical:**
- All other phases depend on hierarchical composition foundation
- Must be solid before proceeding (>90% test coverage requirement)
- Test with mock workers before implementing Communicator

**UX Standards Compliance:**
- Phase 4 monitoring implements UX-001 through UX-004 standards
- Heartbeat logs, pre-escalation warnings, runbook embedding for operational visibility

---

## Timeline Summary

| Phase | Duration | LOC | Component | Status |
|-------|----------|-----|-----------|--------|
| Phase 0: Hierarchical Composition | Weeks 1-2 | 410 | Framework | Not Started |
| Phase 0.5: Templating & Variables | Weeks 3-4 | 325 | Framework | Not Started |
| Phase 1: Infrastructure Supervision | Weeks 5-6 | 320 | Framework | Not Started |
| Phase 2: Async Action Executor | Weeks 7-8 | 260 | Framework | Not Started |
| Phase 3: Integration & Edge Cases | Weeks 9-10 | 330 | Framework | Not Started |
| **→ Communicator Worker Implementation** | **Week 10** | **~200** | **Validation** | **Not Started** |
| Phase 4: Monitoring & Observability | Week 11 | 75 | Framework | Not Started |
| **Framework Total** | **11 weeks** | **~1,720 LOC** | | **In Progress** |
| **Communicator Validation** | **Week 10** | **~200 LOC** | | **Not Started** |
| **Complete (Framework + Validation)** | **12 weeks** | **~1,920 LOC** | | **In Progress** |

**Key Milestones:**
- **Week 10:** Framework complete (Phases 0-3), Communicator Worker implemented
- **Week 11:** Monitoring added, final integration testing
- **Week 12:** Code review, polish, documentation finalization
