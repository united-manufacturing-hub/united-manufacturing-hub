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

// Package example provides reference implementations for FSMv2 workers.
//
// For conceptual overview and framework documentation, see:
//   - pkg/fsmv2/README.md - Triangle Model, concepts, quick start
//   - pkg/fsmv2/doc.go - Implementation patterns, API contracts
//
// # Example Workers
//
// This package contains several example workers demonstrating FSMv2 patterns:
//
//   - exampleparent/ - Parent worker with child management (hierarchical composition)
//   - child/ - Child worker with states, actions, and dependencies
//   - failing/ - Worker that simulates failures (for testing retry logic)
//   - slow/ - Worker with slow operations (for testing timeouts)
//   - panic/ - Worker that panics (for testing panic recovery)
//
// # Directory Structure
//
// Each worker follows a consistent structure:
//
//	child/
//	├── worker.go           # Worker implementation (CollectObservedState, DeriveDesiredState)
//	├── dependencies.go     # Dependency injection struct
//	├── userspec.go         # User configuration parsing
//	├── snapshot/           # ObservedState and DesiredState types
//	├── state/              # State implementations (Next() methods)
//	│   ├── base.go         # Common state utilities
//	│   ├── state_*.go      # Individual state implementations
//	│   └── state_*_test.go # State transition tests
//	└── action/             # Action implementations (Execute() methods)
//	    ├── connect.go      # Example action
//	    └── connect_test.go # Idempotency tests
//
// # Key Patterns Demonstrated
//
// States: See child/state/ for:
//   - Shutdown handling (always check IsShutdownRequested() first)
//   - State naming conventions (TryingTo* for active, descriptive nouns for passive)
//   - State transitions returning (State, Signal, Action)
//
// Actions: See child/action/connect.go for:
//   - Empty struct pattern (no fields, deps injected via Execute)
//   - Context cancellation check (select on ctx.Done() first)
//   - Idempotency (check if work already done)
//
// Parent-Child: See exampleparent/worker.go for:
//   - Returning ChildrenSpecs in DeriveDesiredState()
//   - StateMapping for FSM state coordination
//   - VariableBundle for passing data to children
//
// Testing: See child/action/*_test.go for:
//   - Action idempotency verification
//   - State transition testing patterns
package example
