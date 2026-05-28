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
// # Patterns to copy
//
// The state-machine shape and the RenderChildren-based parent-child pattern
// in this package are the intended templates for new workers. See
// pkg/fsmv2/doc.go § Parent-child workers for the contract.
//
// # WARNING: do NOT copy the timing-variable test pattern
//
// Package-level variables (StoppedWaitDuration, RunningDuration) exist so
// tests can override timing behavior. Production workers should not copy
// this. Use dependency injection instead:
//   - Pass durations/timeouts via constructor parameters
//   - Use configuration structs that can be customized per-instance
//   - Use interface-based time abstractions (like the Clock interface)
//
// # References
//
// pkg/fsmv2/doc.go § Parent-child workers points at this package as the
// despawn example (children are stateless). For the resident-disable
// counterpart, see workers/example/examplechild/state/state_stopped.go and
// workers/transport.
//
// See workers/example/doc.go for full example worker documentation.
package state
