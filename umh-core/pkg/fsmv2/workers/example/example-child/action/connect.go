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
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-child/snapshot"
)

const ConnectActionName = "connect"

// ConnectAction establishes a connection to an external resource
type ConnectAction struct {
	dependencies  snapshot.ChildDependencies
	failureCount  int
	maxFailures   int
}

// NewConnectAction creates a new connect action
func NewConnectAction(deps snapshot.ChildDependencies) *ConnectAction {
	return &ConnectAction{
		dependencies: deps,
		maxFailures:  0,
	}
}

// NewConnectActionWithFailures creates a connect action that will fail N times before succeeding
func NewConnectActionWithFailures(deps snapshot.ChildDependencies, failCount int) *ConnectAction {
	return &ConnectAction{
		dependencies: deps,
		failureCount: failCount,
		maxFailures:  failCount,
	}
}

// Execute attempts to acquire a connection from the pool
func (a *ConnectAction) Execute(ctx context.Context) error {
	logger := a.dependencies.GetLogger()
	logger.Info("Attempting to connect")

	if a.failureCount > 0 {
		a.failureCount--
		return fmt.Errorf("transient connection error (attempt %d/%d)", a.maxFailures-a.failureCount, a.maxFailures+1)
	}

	logger.Info("Connection established successfully")
	return nil
}

func (a *ConnectAction) String() string {
	return ConnectActionName
}

func (a *ConnectAction) Name() string {
	return ConnectActionName
}
