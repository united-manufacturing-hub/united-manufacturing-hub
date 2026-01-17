# UMH Core Changelog

Release notes for the UMH Core edge gateway.

---

## v0.15.x - January 2026

*Released: 2026-01-15*

### New Features

- **Improved S7 Protocol Support** - Enhanced handling of S7 string data types with proper NULL-padding trimming.

- **Data Model Validation** - Data models are now validated at the UNS output, ensuring payload conformance before publishing.

- **Audit Logging** - Added audit trail for configuration changes with timestamp and source tracking.

### Improvements

- **FSM State Resilience** - State machines now recover gracefully from transient errors without requiring full restart.

- **Resource Limiting** - Improved CPU and memory management with configurable limits per bridge.

- **Communicator Reliability** - Enhanced message queue handling to prevent channel overflow during network instability.

<details>
<summary>Technical Notes</summary>

- Updated benthos-umh to v0.11.5
- Refactored FSM reconciliation loop for better error handling
- Added p95/p99 latency metrics to status messages

</details>

---

## v0.14.x - December 2025

*Released: 2025-12-01*

### New Features

- **Stream Processor Templates** - New template system for stream processors with variable substitution.

- **Topic Browser API** - GraphQL API for browsing the Unified Namespace programmatically.

### Bug Fixes

- Fixed port conflict issues when multiple bridges start simultaneously
- Resolved memory leak in long-running Modbus connections
- Fixed S6 service directory cleanup on bridge deletion

### Breaking Changes

- Configuration format for `dataFlows` section updated. See [Migration from Classic](../production/migration-from-classic.md) for details.
