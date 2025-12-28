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

package action

import (
	"context"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplepanic/snapshot"
)

const ConnectActionName = "connect"

// ConnectAction is a stateless action that attempts to establish a connection.
// Configuration (shouldPanic) is read from dependencies, not struct fields.
type ConnectAction struct{}

func (a *ConnectAction) Execute(ctx context.Context, depsAny any) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	deps := depsAny.(snapshot.ExamplepanicDependencies)
	logger := deps.GetLogger()

	if deps.IsShouldPanic() {
		logger.Warn("Simulating panic in connect action")
		panic("simulated panic in connect action")
	}

	logger.Info("Attempting to connect (normal behavior)")

	// Mark as connected on success
	deps.SetConnected(true)

	return nil
}

func (a *ConnectAction) String() string {
	return ConnectActionName
}

func (a *ConnectAction) Name() string {
	return ConnectActionName
}
