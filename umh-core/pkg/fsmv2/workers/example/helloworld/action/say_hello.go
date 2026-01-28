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

// Package action contains the actions for the helloworld worker.
package action

import (
	"context"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/helloworld/snapshot"
)

const SayHelloActionName = "say_hello"

// SayHelloAction prints a greeting and marks hello as said.
//
// ACTION PATTERN:
//   - Check context cancellation first
//   - Check if action is already done (IDEMPOTENCY)
//   - Perform the action
//   - Update dependencies state
//
// IDEMPOTENCY: Actions must be safe to call multiple times.
// The state machine may retry actions, so always check if already done.
type SayHelloAction struct{}

// Execute performs the say hello action.
func (a *SayHelloAction) Execute(ctx context.Context, depsAny any) error {
	// 1. Check context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	deps := depsAny.(snapshot.HelloworldDependencies)
	logger := deps.ActionLogger(SayHelloActionName)

	// 2. Check if already done (IDEMPOTENCY)
	if deps.HasSaidHello() {
		logger.Debug("already_said_hello")

		return nil
	}

	// 3. Perform the action
	logger.Info("hello_world")

	// 4. Update dependencies state (observed on next collection)
	deps.SetHelloSaid(true)

	return nil
}

// String returns the action name for logging.
func (a *SayHelloAction) String() string {
	return SayHelloActionName
}

// Name returns the action name for metrics and history.
func (a *SayHelloAction) Name() string {
	return SayHelloActionName
}
