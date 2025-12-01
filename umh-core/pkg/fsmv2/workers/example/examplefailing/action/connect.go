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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/examplefailing/snapshot"
)

const ConnectActionName = "connect"

// ConnectAction establishes a connection to an external resource.
// This action can be configured to fail predictably for testing FSM error handling.
type ConnectAction struct {
}

// Execute attempts to acquire a connection from the pool.
func (a *ConnectAction) Execute(ctx context.Context, depsAny any) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	deps := depsAny.(snapshot.ExamplefailingDependencies)
	logger := deps.GetLogger()

	logger.Info("Attempting to connect")
	return nil
}

func (a *ConnectAction) String() string {
	return ConnectActionName
}

func (a *ConnectAction) Name() string {
	return ConnectActionName
}
