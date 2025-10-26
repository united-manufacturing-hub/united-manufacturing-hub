# Architecture

This section contains technical architecture documentation for UMH Core internals.

## Documents

- **[FSM v2 ↔ CSE Interface Contract](fsm-v2-cse-contract.md)** - Contract between FSM v2 and CSE for data persistence
- **[SQLite Checkpoint Handling](sqlite-checkpoint-handling.md)** - WAL checkpoint strategy
- **[SQLite Maintenance Implementation](sqlite-maintenance-implementation.md)** - Database maintenance procedures

## Target Audience

These documents are intended for:
- UMH Core developers working on internal components
- Contributors adding new FSM v2 workers
- Developers integrating with CSE (Control Sync Engine)
- Anyone implementing new persistence patterns

For user-facing documentation, see [Usage](../usage/README.md) and [Reference](../reference/README.md).
