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

// Package action provides idempotent actions for the example child worker.
//
// For core FSM concepts including state machine definitions, tick loop mechanics,
// and the worker hierarchy, see workers/example/doc.go.
//
// # Action implementation patterns
//
// This package demonstrates FSM v2 action patterns:
//   - Use empty structs with no fields. Inject dependencies via Execute.
//   - Check context cancellation first with select on ctx.Done().
//   - Check if work is already done before acting.
//
// # Error handling
//
// When an action returns an error, the state remains unchanged. On the next
// tick, state.Next() may return the same action, retrying through state
// re-evaluation.
//
// ## When to return an error
//
// Return an error in these situations:
//   - The operation fails due to transient issues like network timeouts or
//     connection refusals.
//   - A retriable condition blocks progress, such as pool exhaustion or rate
//     limiting.
//   - You deliberately simulate failures for testing purposes.
//
// ## When not to return an error
//
// Do not return an error in these situations:
//   - Validation fails. Validate in state.Next() before you emit the action.
//   - The failure is permanent. Return SignalNeedsRestart from state.Next()
//     instead.
//   - The condition is expected. For example, "already connected" is success,
//     not an error.
//
// # Idempotency
//
// For idempotency requirements, see pkg/fsmv2/doc.go "Actions" section.
//
// Example patterns:
//
//	// Good: idempotent - checks if work is already done
//	func (a *ConnectAction) Execute(ctx context.Context, depsAny any) error {
//	    deps := depsAny.(*MyDependencies)  // Type assertion
//	    if deps.IsConnected() {
//	        return nil  // Already done
//	    }
//	    return deps.Connect()
//	}
//
//	// Bad: not idempotent - state changes on every call
//	func (a *CounterAction) Execute(ctx context.Context, depsAny any) error {
//	    counter++  // Increments on every retry
//	    return nil
//	}
//
// # Context cancellation
//
// Check context cancellation at the start of every action:
//
//	func (a *MyAction) Execute(ctx context.Context, depsAny any) error {
//	    select {
//	    case <-ctx.Done():
//	        return ctx.Err()
//	    default:
//	    }
//	    // Action logic here
//	}
//
// See workers/example/doc.go for the full example worker documentation.
package action
