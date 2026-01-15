// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package state provides FSM state implementations for the example parent worker.
//
// # WARNING: EXAMPLE/TEST WORKERS ONLY
//
// This package contains EXAMPLE workers for demonstrating FSM concepts and for use in tests.
// These are NOT production code patterns to copy blindly.
//
// Specifically, this package uses package-level variables (e.g., StoppedWaitDuration,
// RunningDuration) that can be overridden by tests. This pattern is acceptable here because:
//   - These are example/test workers, not production code
//   - It allows tests to run quickly without real time delays
//   - The trade-off of global mutable state is acceptable in test contexts
//
// # DO NOT COPY THIS PATTERN FOR PRODUCTION WORKERS
//
// Real production workers should use proper dependency injection:
//   - Pass durations/timeouts via constructor parameters
//   - Use configuration structs that can be customized per-instance
//   - Use interface-based time abstractions (like the Clock interface)
//
// Package-level mutable variables in production code cause:
//   - Test interference when tests run in parallel
//   - Difficulty reasoning about code behavior
//   - Hidden dependencies that make refactoring harder
//
// # Documentation
//
// This package is referenced by pkg/fsmv2/doc.go as an example of:
//   - Parent-child hierarchical composition
//   - State coordination via ChildStartStates
//
// When modifying these files, verify doc.go references remain accurate.
//
// See workers/example/doc.go for the full example worker documentation.
package state
