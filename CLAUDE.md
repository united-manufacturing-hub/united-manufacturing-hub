# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

The United Manufacturing Hub (UMH) is an Industrial IoT platform for manufacturing data ingestion and management. It has two main components:

1. **UMH Core** (`umh-core/`) - Modern single-container edge gateway
2. **UMH Classic** (`deployment/united-manufacturing-hub/`) - Full Kubernetes deployment with Helm charts

## Essential Commands

### Building and Running

```bash
# Build Docker image
make build              # Standard build
make build-debug        # Build with debug support  
make build-pprof        # Build with profiling support

# Run tests
make test               # Run all tests
make unit-test          # Run unit tests only
make integration-test   # Run integration tests
make benchmark          # Run performance benchmarks

# Development tools
make test-graphql       # Start GraphQL server with simulator data (port 8090)
make pod-shell          # Shell into running container
make stop-all-pods      # Stop all UMH Core containers
make cleanup-all        # Clean all Docker resources

# Linting and checks (MUST run before completing any task)
golangci-lint run       # Run linter
go vet ./...           # Run static analysis
```

### Git Workflow

```bash
# Default PR target branch
git checkout staging

# Pre-commit hooks (via Lefthook) will run automatically:
# - gofmt (code formatting)
# - go vet (static analysis) 
# - License header checks

# Pre-push hooks will run:
# - nilaway (nil pointer analysis)
# - golangci-lint (comprehensive linting)
```

## High-Level Architecture

### UMH Core Architecture

The system follows a **microservices pattern with FSM-based reconciliation**:

```
┌─────────────────────┐
│   Agent (Go)        │ ← Reads /data/config.yaml, manages lifecycle
├─────────────────────┤
│   FSM Controllers   │ ← State machines for each component
│   - Benthos FSM     │   
│   - Redpanda FSM    │   
│   - S6 FSM          │   
├─────────────────────┤
│   Service Layer     │
│   - Benthos-UMH     │ ← Stream processing (Data Flow Components)
│   - Redpanda        │ ← Kafka-compatible message broker
│   - GraphQL API     │ ← Data access API
└─────────────────────┘
```

### FSM Pattern

Each major component (Benthos, Redpanda, S6) uses a Finite State Machine pattern:

1. **State Types**:
   - **Lifecycle States**: `to_be_created → creating → created`, `to_be_removed → removing → removed`
   - **Operational States**: `stopped → starting → running → stopping → stopped`

2. **File Organization**:
   - `machine.go`: FSM definition (states, events, transitions)
   - `fsm_callbacks.go`: Quick, fail-free callbacks (logging only)
   - `actions.go`: Heavy operations that can fail (must be idempotent)
   - `reconcile.go`: Single-threaded control loop

3. **Key Rules**:
   - Lifecycle states always take precedence over operational states
   - All state transitions go through the reconciliation loop
   - Actions must be idempotent (they will be retried on failure)
   - Only one goroutine modifies FSM state (deterministic)

### Data Flow Architecture

Data flows through configurable pipelines called **Data Flow Components (DFCs)**:

1. **Input** → Protocol adapters (OPC UA, MQTT, etc.)
2. **Processing** → Benthos processors (transform, filter, enrich)
3. **Output** → Destinations (Kafka topics, databases, APIs)

Each DFC is defined in YAML and managed by the Benthos FSM.

## Testing Guidelines

- **Framework**: Ginkgo v2 with Gomega matchers
- **Integration Tests**: Use Testcontainers for Docker-based testing
- **Unit Tests**: Focus on business logic, avoid mocking FSM internals
- **Key Test Patterns**:
  ```go
  // Use focused specs during development
  FIt("should handle state transition", func() {
      // Test implementation
  })
  
  // Test FSM transitions explicitly
  Eventually(func() State {
      return fsm.Current()
  }).Should(Equal(StateRunning))
  ```

## Code Style Requirements

1. **Do NOT add comments unless explicitly requested**
2. **Prefer editing existing files over creating new ones**
3. **Follow existing patterns in the codebase**
4. **Struct field alignment**: Order fields by decreasing size
5. **Error handling**: Return errors up the stack, handle in reconciliation loop
6. **No direct FSM state changes**: Always go through reconciliation

## Critical Patterns

### Reconciliation Loop Pattern
```go
func (r *Reconciler) Reconcile(ctx context.Context) error {
    // 1. Detect external changes
    // 2. Check backoff for failed transitions
    // 3. Compare current vs desired state
    // 4. Issue events or call actions
    // 5. Handle errors with exponential backoff
}
```

### Idempotent Action Pattern
```go
func (s *Service) StartBenthos(ctx context.Context) error {
    // Check if already running
    if s.isRunning() {
        return nil // Idempotent - no error
    }
    // Perform start operation
}
```

## Common Development Tasks

### Adding a New Data Flow Component
1. Define the component in YAML under `examples/`
2. Add validation in `pkg/datamodel/`
3. Update the Benthos FSM to handle the new component type
4. Add integration tests in `integration/`

### Debugging FSM Issues
1. Enable debug logging: `LOG_LEVEL=debug`
2. Check FSM transitions in logs
3. Use `make test-debug` for isolated testing
4. Examine backoff manager for retry patterns

### GraphQL API Development
1. Modify schema in `pkg/communicator/graphql/schema/`
2. Run `make generate` to update resolvers
3. Test with `make test-graphql` (playground at http://localhost:8090/)

## Important Notes

- **Default branch for PRs**: `staging` (not main)
- **Focused tests**: Don't remove `FIt()` or `FDescribe()` - they're intentional
- **FSM callbacks**: Keep them fail-free (logging only)
- **Actions**: Must be idempotent and handle context cancellation
- **Exponential backoff**: System automatically retries failed transitions
- **Resource Limiting**: Bridge creation is blocked when resources are constrained (controlled by `agent.enableResourceLimitBlocking` feature flag)