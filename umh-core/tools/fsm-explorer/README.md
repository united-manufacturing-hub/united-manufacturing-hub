# FSM v2 Explorer - System Health Monitoring Demo

An automated demonstration of FSM v2 behavior monitoring real system metrics (CPU, memory, disk).

## What This Demo Shows

The FSM Explorer demonstrates the **observation-driven FSM v2 pattern** through a 5-phase narrative:

**Phase 1: Initial Setup**
- Initializes FSM v2 System Health Monitor
- Monitors host system (Mac) resources
- Initializes TriangularStore persistence (SQLite)
- Registers worker with supervisor
- Shows initial FSM state

**Phase 2: First Observation Cycle**
- Worker observes container reality
- Supervisor evaluates desired vs observed state
- FSM transitions based on observation
- Database saves new state with incremented sync ID

**Phase 3: Normal Operation**
- Multiple observation cycles (steady state)
- Demonstrates continuous monitoring
- No state changes when desired = observed

**Phase 4: Shutdown Request**
- Injects shutdown into desired state
- Supervisor detects mismatch (desired=shutdown, observed=running)
- FSM transitions to shutting down
- Shows reconciliation logic

**Phase 5: Summary**
- Explains key FSM v2 concepts
- Highlights differences from traditional FSMs
- Cleans up resources

This is different from traditional FSMs where you send commands. In FSM v2, you **observe reality** and the supervisor decides what actions to take.

## Prerequisites

- Go 1.23 or later
- macOS or Linux (for system metrics collection)

## Quick Start

```bash
# From umh-core directory
cd tools/fsm-explorer

# Run the demo (builds and executes)
./run.sh

# Or run directly
go run main.go
```

## What to Watch For

1. **Narrative Explanation**: Demo explains each phase in human terms
2. **State Transitions**: Watch for "🎯 State Transition" messages showing how FSM moves between states
3. **Observation Loop**: Every second, the supervisor observes reality and compares it to desired state
4. **Database Persistence**: All state is saved to SQLite (`/tmp/explorer-test-*/test.db`)
5. **Real Metrics**: Actual macOS host CPU/memory/disk usage (not mocked)

## Understanding FSM v2

### Key Concepts

**Observation-Driven**: The supervisor doesn't react to commands - it observes reality and reconciles desired vs observed state.

**Tick-Based**: Every tick, the supervisor:
1. Collects observations from all workers
2. Compares desired vs observed state
3. Decides which actions to execute
4. Executes actions
5. Saves new observed state

**State Flow**:
```
NotCreated → Creating → Created → Starting → Running → ShuttingDown → Removing → Removed
```

### Under the Hood

When you run this explorer:

1. **Setup Phase**:
   - Initializes system health monitor
   - Initializes SQLite database with TriangularStore
   - Registers container worker with supervisor
   - Starts observation loop (every 1 second)

2. **Observation Loop**:
   - Worker collects host system metrics (CPU, memory, disk)
   - Reports observed state to supervisor
   - Supervisor saves to database (auto-increments `_sync_id`)

3. **Shutdown Phase**:
   - `InjectShutdown()` changes desired state
   - Supervisor sees desired=Removed, observed=Running
   - Executes RemoveAction
   - Monitoring is stopped

## For Future FSM Demos

This container demo uses a **copy-paste friendly pattern** for building future FSM demos (e.g., Benthos, Redpanda).

**Instructions are in the file header:**
```go
// FUTURE: When building 2nd FSM demo (benthos, redpanda, etc.):
// 1. Copy this file to tools/fsm-explorer-<name>/main.go
// 2. Find/replace "container" → "<name>"
// 3. Adapt metrics and phase logic
// 4. Notice what you're duplicating
// 5. After 3rd FSM demo, consider extracting common patterns
```

**Reusable patterns are marked:**
```go
// REUSABLE PATTERN: Demo header
// Copy this when building next FSM demo
func printDemoHeader() { ... }

// REUSABLE PATTERN: Phase header formatting
// Copy this when building next FSM demo
func printPhaseHeader(num int, title string) { ... }
```

**Philosophy**: Don't create abstractions until you have 3 examples (Rule of Three). Copy-paste is better than premature abstraction.

## Troubleshooting

**Error: "failed to create basic store"**
- Check `/tmp` directory is writable
- Try cleaning up old test directories

**Error: "Monitoring is not active"**
- System metrics may not be available on your platform
- Try running on macOS or Linux

## Advanced Usage

### Inspecting the Database

```bash
# Find the temp directory from logs
sqlite3 /tmp/explorer-test-*/test.db

# View stored states
SELECT * FROM container_observed;
SELECT * FROM container_desired;
SELECT * FROM container_identity;

# Check sync IDs
SELECT _sync_id, * FROM container_observed ORDER BY _sync_id DESC LIMIT 10;
```


## What Makes This FSM v2?

Compared to the old FSM pattern (`pkg/fsm/`), FSM v2 has:

1. **Database-backed state**: All state persisted to SQLite
2. **Type-safe interfaces**: Workers implement typed interfaces
3. **Generic supervisor**: One supervisor manages any worker type
4. **CSE integration**: Uses TriangularStore (Identity/Desired/Observed pattern)
5. **Idempotent actions**: Every action tested for idempotency
6. **Observable architecture**: External systems can query state history

## Files

- `main.go` - Automated demo with 5-phase narrative
- `run.sh` - Build and run script
- `../../pkg/fsmv2/explorer/container_scenario.go` - Scenario implementation
- `../../pkg/fsmv2/container/` - Container worker and actions
- `../../pkg/fsmv2/persistence/triangular_adapter.go` - Storage adapter

## Next Steps

After watching the demo:

1. Read `pkg/fsmv2/container/worker.go` to see how observations work
2. Check `pkg/fsmv2/container/action_*.go` to see idempotent actions
3. Look at `pkg/fsmv2/supervisor/supervisor.go` to understand tick logic
4. Inspect the database to see CSE Triangular Model pattern
5. **Build another demo** for a different component (Benthos, Redpanda) using the copy-paste pattern

## Questions?

See the main CLAUDE.md documentation in the umh-core root directory for FSM v2 architecture details.
