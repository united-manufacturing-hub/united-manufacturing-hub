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

// Package fsm holds what the FSMv1 state machines share: the base manager,
// the interfaces and the system snapshot. FSMv1 is the legacy framework. New logic is built as an FSMv2 worker in pkg/fsmv2.
//
// Each component (benthos, redpanda, s6, ...) has a subpackage with the
// files below. The connection subpackage has no actions.go.
//
//   - machine.go defines the states and transitions.
//   - fsm_callbacks.go holds the transition callbacks. They run synchronously
//     and only log, so they must not fail.
//   - actions.go holds the operations that can fail, such as file or network
//     I/O. Reconcile retries a failed action after a backoff, so every action
//     is idempotent and respects context cancellation.
//   - reconcile.go holds Reconcile, the single-threaded control loop. Only
//     Reconcile changes the state, by sending events to the machine.
//   - models.go holds the types.
package fsm
