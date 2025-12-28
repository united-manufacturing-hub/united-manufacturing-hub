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

// Package helpers provides convenience utilities for FSMv2 workers.
//
// This package contains optional boilerplate reducers that help implement
// workers more easily. Unlike the core fsmv2 contracts (api.go, dependencies.go),
// these helpers are not required for FSM operation but reduce repetitive code.
//
// # BaseState
//
// BaseState provides automatic state name derivation from type names:
//
//	type RunningState struct {
//	    helpers.BaseState
//	}
//
//	func (s RunningState) String() string {
//	    return helpers.DeriveStateName(s)  // Returns "Running"
//	}
//
// # BaseWorker
//
// BaseWorker provides type-safe dependency access for workers:
//
//	type MyWorker struct {
//	    *helpers.BaseWorker[*MyDeps]
//	}
//
//	func NewMyWorker(deps *MyDeps) *MyWorker {
//	    return &MyWorker{BaseWorker: helpers.NewBaseWorker(deps)}
//	}
//
// # ConvertSnapshot
//
// ConvertSnapshot provides type-safe snapshot conversion for state transitions:
//
//	func (s *MyState) Next(snapAny any) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
//	    snap := helpers.ConvertSnapshot[MyObserved, *MyDesired](snapAny)
//	    // Direct typed access: snap.Observed.Field, snap.Desired.Method()
//	}
//
// # Why Internal?
//
// These helpers are placed in internal/ because they are implementation details
// that support the public fsmv2 API. The core contracts (Worker, State, Action,
// Dependencies) remain public in pkg/fsmv2/, while these convenience utilities
// are available within the package hierarchy.
package helpers
