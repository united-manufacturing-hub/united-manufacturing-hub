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
// # WARNING: example/test workers only
//
// This package contains example workers for demonstrating FSM concepts and for use in tests.
// Do not copy these patterns into production code.
//
// Package-level variables (e.g., StoppedWaitDuration, RunningDuration) allow tests to
// override timing behavior. This is acceptable for test code but causes parallel test
// interference and hidden dependencies in production.
//
// # Production workers
//
// Use dependency injection instead:
//   - Pass durations/timeouts via constructor parameters
//   - Use configuration structs that can be customized per-instance
//   - Use interface-based time abstractions (like the Clock interface)
//
// # References
//
// pkg/fsmv2/doc.go references this package for:
//   - Parent-child hierarchical composition
//   - State coordination via ChildStartStates
//
// See workers/example/doc.go for full example worker documentation.
package state
