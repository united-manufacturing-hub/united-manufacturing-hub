# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

The United Manufacturing Hub (UMH) is an Industrial IoT platform for manufacturing data ingestion and management. It has two main components:

1. **UMH Core** (`umh-core/`) - Modern single-container edge gateway
2. **UMH Classic** (`deployment/united-manufacturing-hub/`) - Full Kubernetes deployment with Helm charts

## Terminology

- **Bridge** (UI) = `protocolConverter:` (YAML) = Protocol Converter (legacy)
- **Stand-alone Flow** (UI) = `dataFlow:` (YAML) = Data Flow Component/DFC (legacy)
- **Stream Processor** = `dataFlow:` with `sources:[]` array (aggregates multiple topics)
- **Data Contract** = underscore-prefixed type (`_raw`, `_pump_v1`, `_maintenance_v1`)
- **Virtual Path** = optional organizational segments in topics (e.g., `motor.electrical`)
- **Tag** = single data point/sensor (industrial term)
- **_raw** → **_devicemodel_v1** → **_businessmodel_v1** (data progression)

## Non-Intuitive Patterns

- **Variable flattening**: `variables.IP` → `{{ .IP }}` (nested becomes top-level)
- **S6 logs**: `.s` = clean rotation, `.u` = unfinished (container killed), `current` = active log
- **Empty FSMState**: `''` means S6 returns nothing (directory missing/corrupted)
- **FSM precedence**: Lifecycle states ALWAYS override operational states
- **One tag, one topic**: Never combine sensors in one payload (avoids timing/merge issues)
- **Bridge = Connection + Source Flow + Sink Flow**: Connection only monitors network availability
- **Data validation**: Happens at UNS output plugin, not at source
- **Bridge states**: `starting_failed_dfc_missing` = no data flow configured yet
- **Resource limiting**: Controlled by `agent.enableResourceLimitBlocking` and related settings. Default: ≤70% CPU; ~5 bridges per CPU core after reserving 1 for Redpanda
- **Template variables**: `{{ .IP }}`, `{{ .PORT }}` auto-injected from Connection config
- **Location computation**: Agent location + bridge location = `{{ .location_path }}`

## Essential Commands

**Build**: `make build` (standard), `make build-debug` (debug), `make build-pprof` (profiling)

**Test**: `make test` (all), `make unit-test`, `make integration-test`, `make benchmark`

**Dev**: `make test-graphql` (port 8090), `make pod-shell`, `make test-no-copy` (use current config)

**Clean** (destructive): `make stop-all-pods`, `make cleanup-all`

**MUST run before completing tasks**: `golangci-lint run`, `go vet ./...`, check no focused tests with `ginkgo -r --fail-on-focused`

**Git**: Default branch is `staging`. Lefthook runs gofmt, go vet, license checks on commit; nilaway, golangci-lint on push.

## Architecture & Key Decisions

### System Architecture

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

**Files**: `machine.go` (states) → `reconcile.go` (control loop) → `actions.go` (operations)

**State precedence**: Lifecycle (`to_be_created`, `removing`) > Operational (`running`, `stopped`)

**Key rules**:
- Actions must be idempotent (will retry on failure)
- Only reconciliation loop modifies state (deterministic)
- FSM callbacks fail-free (logging only)

### Data Architecture Decisions

**Two-layer model**:
- **Device models** (`_pump_v1`): Equipment internals, sites control
- **Business models** (`_maintenance_v1`): Enterprise KPIs, aggregated views

**UNS principles**:
- **Publish regardless**: Producers don't wait for consumers
- **Entry/exit via bridges**: All data validated at gateway
- **Location path**: WHERE in organization (enterprise.site.area)
- **Device model**: WHAT data exists (temperature, pressure)
- **Virtual path**: HOW to organize within model (motor.electrical)

## Testing Guidelines

- **Framework**: Ginkgo v2 with Gomega matchers
- **Integration Tests**: Use Testcontainers for Docker-based testing
- **Unit Tests**: Focus on business logic, avoid mocking FSM internals
- **Key Test Patterns**:
  ```go
  // Use focused specs during development (CI fails if any are present)
  // CI runs with: ginkgo -r -p --fail-on-focused
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


## Common Development Tasks

**Adding a Data Flow Component**: Define in YAML → Add validation in `pkg/datamodel/` → Update Benthos FSM → Add tests

**Debugging FSM**: Enable `LOG_LEVEL=debug` → Check transitions in logs → Use `make test-debug`

**GraphQL changes**: Modify schema → `make generate` → Test with `make test-graphql`

## Issue Investigation Workflow

When investigating FSM or service issues, follow this systematic approach:

### 1. Gather Context

**First, always ask the user for:**
- Action logs from UI/Management Console (if applicable)
- Description of what they were trying to do
- Timestamps and error messages

**Then check Linear context:**
- Read main issue and ALL sub-issues
- Review related PRs and their comments
- Understanding previous fix attempts is crucial

### 2. Analyze Logs Systematically

Start broad, then narrow:
```bash
# Recent errors and warnings (with human-readable timestamps)
tai64nlocal < /data/logs/umh-core/current | tail -1000 | grep -E "ERROR|WARN"

# FSM state changes for specific service
grep "service-name.*currentFSM" /data/logs/umh-core/current

# Find when issue started
grep -n "first-error-pattern" /data/logs/umh-core/* | head -1
```

**Key patterns to look for:**
- Empty FSMState (`FSMState=''`)
- Rapid retry loops (same timestamp repeating)
- State transitions that don't complete
- "Not existing" or "service does not exist" errors

### 3. Trace Code Paths

Map symptoms to source code:
1. Find where log messages originate: `grep -r "log message" pkg/`
2. Trace back through the FSM reconciliation loop
3. Identify blocking conditions or failed transitions

**FSM Structure (always the same):**
- `machine.go` - States and transitions
- `reconcile.go` - Control loop logic
- `actions.go` - Operations that can fail
- `models.go` - Data structures

### 4. Analyze Service State

```bash
# Build and run S6 analyzer for deep inspection
cd tools/s6-analyzer && go build && ./s6-analyzer /data/services/service-name

# Quick directory check
ls -la /data/services/*service-name*/
```

**Look for anomalies:**
- Down files blocking startup
- Missing supervise directories
- Timestamp mismatches
- Zombie services (directory exists but S6 returns empty)

### 5. Check Configuration

Verify the configuration chain:
- Template exists and is valid
- Variables are properly substituted
- References match actual resources

### 6. Build Timeline

Create a clear sequence:
1. User action → System response
2. FSM state transitions with timestamps (ISO-8601 with timezone, e.g., 2025-09-22T14:37:05Z)
3. Where it got stuck and why
4. Evidence from logs (quote exact lines)

### 7. Document Findings

Use the Linear template but focus on:
- **Root cause** (one clear sentence)
- **Evidence** (logs, config, S6 state)
- **Reproduction steps** (minimal and clear)
- **Why existing fixes didn't work** (if applicable)

## Code Path Analysis

When tracing issues through code:

### Understanding FSM Flow
Every FSM follows: **Event → State Change → Reconcile → Action → New State**

To trace issues:
1. Find the stuck state in reconcile.go
2. Check what condition prevents progress
3. Trace back to what updates that condition
4. Find why the update fails

### Common Patterns

**FSM Stuck**: Current state ≠ Desired state, reconciliation blocking
**Service Won't Start**: Check preconditions in reconcile, verify Create/Start idempotent
**Rollback Issues**: Timeout triggers rollback, cleanup assumes service exists

## Key Investigation Principles

1. **Start with user perspective** - What were they trying to do?
2. **Follow the data** - Logs don't lie, but they may be incomplete
3. **Verify assumptions** - Check if service actually exists before trying to stop it
4. **Consider timing** - Race conditions often appear as intermittent issues
5. **Check the full stack** - Protocol Converter → Benthos → S6 → Filesystem

## When to Dig Deeper

- Multiple customers report similar issues → Systematic problem
- Issue reoccurs after fix → Incomplete understanding
- Logs show impossible states → Race condition or corrupted state
- Rollback creates more problems → Non-idempotent operations

Remember: Every FSM issue has a trigger, a stuck state, and a missing transition. Find all three.

## UI Testing with Playwright MCP

When reproducing or testing UI-related issues, use Playwright MCP for browser automation:

### Setup and Navigation
```bash
# Start test environment
make test-no-copy  # Use current config without copying

# Navigate to Management Console
mcp__playwright__browser_navigate url: "https://management.umh.app"

# Take screenshots for documentation
mcp__playwright__browser_take_screenshot fullPage: true, filename: "before-deployment.png"
```

### Collaborative Workflow
The most effective approach combines human and AI capabilities:

1. **Human prepares context**: User creates initial setup, navigates to relevant page
2. **AI traces actions**: Uses browser_snapshot to understand current state
3. **Human provides credentials**: Login, sensitive data entry
4. **AI performs repetitive tasks**: Clicking through deployment flows, waiting for timeouts
5. **Both observe results**: Human confirms visual state, AI analyzes logs

### Key Capabilities
- **State observation**: `browser_snapshot` provides accessibility tree for navigation
- **Action automation**: Click buttons, fill forms, wait for conditions
- **Evidence collection**: Screenshots (though not automatically saved to PR)
- **Multi-tab handling**: Track deployment dialogs and logs simultaneously

### Testing Protocol Converter Deployments
```yaml
# Example reproduction workflow:
1. Navigate to Data Flows page
2. Click on protocol converter to edit
3. Change protocol type (e.g., S7 → Generate)
4. Click "Save & Deploy"
5. Monitor deployment dialog for status changes
6. Wait for timeout/success
7. Check logs for FSM state transitions
8. Screenshot final state for documentation
```

### Best Practices
- **Always screenshot before/after**: Provides visual evidence for reports
- **Monitor both UI and logs**: Deployment dialog + backend FSM states
- **Document timing**: Note when "not existing" states appear
- **Capture error messages**: Exact text from UI alerts and dialogs
- **Test multiple scenarios**: Failed deployments, successful deployments, rollbacks

### Limitations and Improvements
- Screenshots aren't automatically attached to PRs (manual step needed)
- Browser console errors should be checked with `browser_console_messages`
- Network requests can be monitored with `browser_network_requests`
- For complex forms, use `browser_fill_form` for batch field updates


## Important Notes

- **Default branch for PRs**: `staging` (not main)
- **Focused tests**: Do not commit focused specs. CI runs with `--fail-on-focused` and will fail if any are present
- **FSM callbacks**: Keep them fail-free (logging only)
- **Actions**: Must be idempotent and handle context cancellation
- **Exponential backoff**: System automatically retries failed transitions
- **Resource Limiting**: Bridge creation is blocked when resources are constrained (controlled by `agent.enableResourceLimitBlocking` feature flag)
