# UMH Core Log Analyzer

A specialized tool for analyzing UMH Core logs, particularly focusing on FSM (Finite State Machine) state transitions and tick-based reconciliation events.

## Features

- **Tick Timeline Analysis**: View all events organized by tick number
- **FSM State Tracking**: Track individual FSM journeys through state transitions
- **Visual Timeline**: ASCII visualization of concurrent FSM activities
- **Stuck FSM Detection**: Identify FSMs that may be stuck in failed transitions
- **Error Analysis**: Group and analyze errors by type and frequency
- **Multiple Starts Detection**: Automatically detect multiple UMH Core restarts in logs
- **Session Analysis**: Track errors and activity across different run sessions
- **Interactive Mode**: Explore logs interactively with various commands

## Building

From the `umh-core` directory:

```bash
cd tools/loganalyzer
go build -o loganalyzer
```

## Usage

### Basic Usage

```bash
# Show summary of log file
./loganalyzer -f /path/to/umh-core/current

# Run specific command
./loganalyzer -f /path/to/umh-core/current -c timeline -start 1 -end 50

# Interactive mode
./loganalyzer -f /path/to/umh-core/current -i
```

### Command Line Options

- `-f <file>`: Path to the log file (required)
- `-i`: Run in interactive mode
- `-c <command>`: Run specific command (summary, timeline, fsm, list-fsms, stuck)
- `-fsm <name>`: FSM name for fsm command
- `-start <tick>`: Start tick for timeline (default: 1)
- `-end <tick>`: End tick for timeline (default: 100)

### Interactive Commands

- `summary` - Show overall analysis summary with multiple starts detection
- `timeline [start end]` - Show tick timeline (default: 1-100)
- `fsm <name>` - Show history of a specific FSM
- `list-fsms` - List all FSMs
- `stuck` - Find potentially stuck FSMs
- `filter <pattern>` - Filter FSMs by name pattern
- `visual [start end]` - Show visual FSM timeline (default: 1-20)
- `concurrent` - Show concurrent FSM activity analysis
- `errors` - Show error analysis and grouping
- `errors-tick <tick>` - Show errors at a specific tick
- `help` - Show available commands
- `quit` - Exit the program

## Understanding the Output

### Timeline View

Shows events grouped by tick:
- **Desired State Changes**: FSMs being told to change state
- **FSM Transitions**: Actual state transitions (✓ success, → attempt, ✗ failed)
- **Reconciliations**: Components reconciling their state
- **Actions**: Service actions (→ starting, ✓ completed)
- **Errors**: Any errors that occurred

### Visual Timeline

Provides a grid view showing FSM states over time:
- Each row represents an FSM
- Each column represents a tick
- State symbols: C=created, S=stopped, R=running, A=active, etc.
- Transition symbols: → = transition, ✗ = failed attempt

### FSM History

Shows the complete journey of a single FSM through its states, including:
- State transitions with tick numbers
- Success/failure of each transition
- Current state

### Stuck FSM Detection

Identifies FSMs that have:
- Multiple consecutive failed transition attempts
- Not reached their desired state
- May need manual intervention

### Error Analysis

The `errors` command provides:
- Grouped errors by pattern/type
- Error frequency and timing
- First and last occurrence of each error type
- Errors distributed across sessions
- Ticks with the most errors

### Multiple Starts Detection

The summary automatically detects when UMH Core was restarted:
- Shows number of sessions
- Duration of each session
- Error count per session
- Tick range for each session

## Example Output

```
=== Tick Timeline (Ticks 1-10) ===

Tick 7 (13:16:07.938):
  Desired State Changes:
    • redpanda → active
    • generate-1 → active
  FSM Transitions:
    ✓ Core: creating → monitoring_stopped (event: create_done)
    → redpanda: to_be_created → ? (event: create)
  Reconciliations:
    • ProtocolConverterServicegenerate-1
    • StreamProcessorServicetest
  Actions:
    → Starting redpanda service
    ✓ Redpanda service started
```

## Tips

1. Start with `summary` to get an overview
2. Use `stuck` to quickly identify problematic FSMs
3. Use `visual` for a quick overview of system activity
4. Filter FSMs by pattern to focus on specific components
5. Check `concurrent` to identify performance bottlenecks