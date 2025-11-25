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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-child/snapshot"
)

const ConnectActionName = "connect"

// ConnectAction establishes a connection to an external resource
// This is a stateless action - completely empty struct with no fields.
// Dependencies will be injected via Execute() in Phase 2C when the Action interface is updated.
type ConnectAction struct {
	// COMPLETELY EMPTY - no dependencies, no state
}

// NewConnectActionWithFailures creates a connect action that will fail N times before succeeding
// Note: Retry logic will be handled by ActionExecutor in Phase 2C, not by the action itself.
func NewConnectActionWithFailures(failCount int) *ConnectAction {
	return &ConnectAction{}
}

// Execute attempts to acquire a connection from the pool
// Dependencies are injected via deps parameter, enabling full action functionality.
func (a *ConnectAction) Execute(ctx context.Context, depsAny any) error {
	deps := depsAny.(snapshot.ChildDependencies)
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
