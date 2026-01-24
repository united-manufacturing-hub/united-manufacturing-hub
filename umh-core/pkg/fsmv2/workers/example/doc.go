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
// # Example workers
//
// This package contains example workers demonstrating FSMv2 patterns:
//
//   - exampleparent/ - Parent worker with child management (hierarchical composition)
//   - examplechild/ - Child worker with states, actions, and dependencies
//   - examplefailing/ - Worker that simulates failures (for testing retry logic)
//   - exampleslow/ - Worker with slow operations (for testing timeouts)
//   - examplepanic/ - Worker that panics (for testing panic recovery)
//
// # Directory structure
//
// Each worker follows this structure:
//
//	examplechild/
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
// # Creating a worker
//
// To create a worker named "myworker":
//
//  1. Create the directory structure:
//
//     workers/myworker/
//     ├── worker.go           # Implement Worker interface
//     ├── dependencies.go     # Define dependencies struct
//     ├── userspec.go         # Parse user configuration
//     ├── snapshot/
//     │   └── snapshot.go     # Define ObservedState and DesiredState types
//     ├── state/
//     │   ├── base.go         # Common state utilities
//     │   └── state_initial.go # Initial state implementation
//     └── action/
//     └── (action files)  # Action implementations
//
//  2. Register the worker in init():
//
//     func init() {
//     factory.RegisterWorkerType[snapshot.MyworkerObservedState, *snapshot.MyworkerDesiredState](
//     workerFactory, supervisorFactory)
//     }
//
//  3. Copy from examplechild/ as a starting point.
//
// See factory/README.md for worker type derivation rules.
//
// # Patterns demonstrated
//
// For pattern documentation and requirements, see pkg/fsmv2/doc.go.
//
// States:
//   - examplechild/state/ implements child worker states.
//   - exampleparent/state/ implements parent worker states.
//
// Actions:
//   - examplechild/action/connect.go shows the empty struct and idempotency patterns.
//
// Parent-child:
//   - exampleparent/worker.go shows ChildrenSpecs and VariableBundle usage.
//
// Testing:
//   - examplechild/action/*_test.go shows action idempotency verification.
//   - examplechild/state/*_test.go shows state transition testing.
package example
