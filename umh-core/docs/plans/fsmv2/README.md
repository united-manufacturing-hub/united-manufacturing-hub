# FSMv2 Implementation Plans

**Status:** Ready for Implementation
**Timeline:** 12 weeks (10 weeks development + 2 weeks buffer)
**Scope:** Hierarchical composition, templating, infrastructure supervision, async actions

---

## Quick Navigation

### Implementation Phases

**Foundation** (Weeks 1-4):
- [Phase 0: Hierarchical Composition](phase-0-hierarchical-composition.md) - 2 weeks, 410 LOC
- [Phase 0.5: Templating & Variables](phase-0.5-templating-variables.md) - 2 weeks, 325 LOC

**Core Features** (Weeks 5-8):
- [Phase 1: Infrastructure Supervision](phase-1-infrastructure-supervision.md) - 2 weeks, 320 LOC
- [Phase 2: Async Action Executor](phase-2-async-actions.md) - 2 weeks, 260 LOC

**Integration** (Weeks 9-10):
- [Phase 3: Integration & Edge Cases](phase-3-integration.md) - 2 weeks, 330 LOC

**Observability** (Week 11):
- [Phase 4: Monitoring & Observability](phase-4-monitoring.md) - 1 week, 75 LOC

---

## Design Documents

Core architecture decisions:

1. **[Child Specs in Desired State](design/fsmv2-child-specs-in-desired-state.md)**
   - ChildSpec structure: Name, WorkerType, UserSpec, StateMapping
   - Parent creates children via DeriveDesiredState() return value
   - Supervisor reconciles ChildrenSpecs (add/remove/update)
   - WorkerFactory pattern for dynamic worker creation

2. **[Idiomatic Templating & Variables](design/fsmv2-idiomatic-templating-and-variables.md)**
   - VariableBundle: User/Global/Internal namespaces
   - RenderTemplate() with strict mode
   - Location computation and merging
   - Template execution distributed at worker level

3. **[DeriveDesiredState Complete Definition](design/fsmv2-derive-desired-state-complete-definition.md)**
   - Complete API specification
   - Return type includes State + ChildrenSpecs
   - StateMapping application by supervisor
   - Hybrid control: Declarative mapping + child autonomy

**Supporting Design Documents:**
- [Children as Desired State](design/fsmv2-children-as-desired-state.md)
- [Child Removal Automation](design/fsmv2-child-removal-auto.md)
- [Combined vs Separate Methods](design/fsmv2-combined-vs-separate-methods.md)
- [Config Changes & Dynamic Children](design/fsmv2-config-changes-and-dynamic-children.md)
- [Developer Expectations](design/fsmv2-developer-expectations-current-api.md)
- [Phase 0 Worker-Child API](design/fsmv2-phase0-worker-child-api.md)
- [Templating vs StateMapping](design/fsmv2-templating-vs-statemapping.md)
- [UserSpec-Based Child Updates](design/fsmv2-userspec-based-child-updates.md)

---

## Archive

Historical planning documents:

- **[Gap Analysis](archive/fsmv2-master-plan-gap-analysis.md)** - Identified missing architecture
- **[Change Proposal](archive/fsmv2-master-plan-change-proposal.md)** - Proposed updates to master plan
- **Change Proposal Sections:**
  - [Section 3: Phase 0.5 Tasks](archive/fsmv2-change-proposal-section3.md)
  - [Section 4-5: Modified Tasks](archive/fsmv2-change-proposal-section4-5.md)
  - [Section 6-7: Timeline & Architecture](archive/fsmv2-change-proposal-section6-7.md)
  - [Section 8-11: Testing, Migration, Risks](archive/fsmv2-change-proposal-section8-11.md)

---

## Project Overview

### What is FSMv2?

FSMv2 is the second generation of UMH's Finite State Machine framework, adding:
- **Hierarchical composition**: Parents manage children (e.g., ProtocolConverter → Connection + DataFlows)
- **Template rendering**: Configuration reuse via Go templates
- **Variable propagation**: Parent context flows to children
- **Robust supervision**: Circuit breaker pattern for infrastructure health
- **Async actions**: Non-blocking operations with global worker pool

### Key Patterns

**ChildSpec in DesiredState** (Kubernetes-inspired):
```go
type DesiredState struct {
    State         string      // Operational state ("running", "stopped")
    ChildrenSpecs []ChildSpec // Children to manage
}

type ChildSpec struct {
    Name         string            // Child instance ID
    WorkerType   string            // Worker factory type
    UserSpec     UserSpec          // Child configuration
    StateMapping map[string]string // Parent state → child state
}
```

**Variable System** (Three-tier namespaces):
```go
type VariableBundle struct {
    User     map[string]any // User-defined + parent state + computed
    Global   map[string]any // Fleet-wide settings
    Internal map[string]any // Runtime metadata (not serialized)
}
```

**Supervisor Tick Loop**:
```
1. Infrastructure Health Check → Circuit breaker if unhealthy
2. Derive Desired State → Worker returns State + ChildrenSpecs
3. Reconcile Children → Add/update/remove based on specs
4. Apply State Mapping → Parent state → child states
5. Tick Children → Recursive tick propagation
6. Async Actions → Queue non-blocking operations
```

### Dependencies

**Phase Dependencies:**
- Phase 0 → Phase 0.5, 1, 2
- Phase 0.5 → Benthos worker implementation
- Phases 0, 0.5, 1, 2 → Phase 3
- Phase 3 → Phase 4

**Critical Path:**
Phase 0 must complete first (blocks all other phases). Phase 0.5 must complete before Benthos workers can work.

### Success Criteria

**Functionality:**
- Parent FSMs can declare and manage children
- Templates render correctly with variable propagation
- Circuit breaker pattern prevents cascading failures
- Async actions execute without blocking tick loop

**Performance:**
- Tick loop: <1ms per supervisor
- Child reconciliation: <10ms per operation
- Template rendering: <5ms per template
- Action execution: Non-blocking, <100ms queuing latency

**Quality:**
- Unit test coverage: >80% (>90% for Phase 0)
- Integration tests: 15+ hierarchical scenarios
- System tests: Real-world ProtocolConverter migration
- Documentation: Complete API docs and migration guide

---

## Getting Started

1. **Read design documents** (design/ folder) to understand architecture
2. **Start with Phase 0** - Foundational, blocks everything else
3. **Follow TDD approach** - RED → GREEN → REFACTOR for each task
4. **Review at phase boundaries** - Code review checkpoints after each phase
5. **Integrate incrementally** - Don't wait until Phase 4 to test integration

---

## Questions or Issues?

See the design documents for detailed rationale. For implementation questions, refer to the specific phase plan file.
