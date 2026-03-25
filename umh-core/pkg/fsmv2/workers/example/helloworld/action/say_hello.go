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

// Package action defines the actions for the helloworld worker.
package action

import (
	"context"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// SayHelloActionName is the name used for logging and metrics.
const SayHelloActionName = "say_hello"

// HelloDeps declares the dependency interface required by SayHelloAction.
// Satisfied by *HelloworldDependencies via structural typing, avoiding
// circular imports between the action and worker packages.
type HelloDeps interface {
	ActionLogger(actionType string) deps.FSMLogger
	SetHelloSaid(said bool)
	HasSaidHello() bool
}

// SayHelloAction prints a greeting and marks hello as said.
//
// The state machine retries actions, so check if already done.
type SayHelloAction struct{}

// Execute performs the say hello action.
func (a *SayHelloAction) Execute(ctx context.Context, depsAny any) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	deps := depsAny.(HelloDeps)
	logger := deps.ActionLogger(SayHelloActionName)

	if deps.HasSaidHello() {
		logger.Debug("already_said_hello")

		return nil
	}

	logger.Info("hello_world")

	deps.SetHelloSaid(true)

	return nil
}

// String returns the action name for logging (implements fmt.Stringer).
func (a *SayHelloAction) String() string {
	return SayHelloActionName
}

// Name returns the action name for metrics and history.
func (a *SayHelloAction) Name() string {
	return SayHelloActionName
}
