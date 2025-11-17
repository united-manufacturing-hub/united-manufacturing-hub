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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/example-parent/snapshot"
)

const StopActionName = "stop"

// StopAction gracefully stops all children and cleans up resources
// This is a stateless action - completely empty struct with no fields.
// Dependencies will be injected via Execute() in Phase 2C when the Action interface is updated.
type StopAction struct {
	// COMPLETELY EMPTY - no dependencies
}

// NewStopAction creates a new stop action
func NewStopAction() *StopAction {
	return &StopAction{}
}

// Execute stops all children gracefully
// Dependencies are injected via deps parameter, enabling full action functionality.
func (a *StopAction) Execute(ctx context.Context, depsAny any) error {
	deps := depsAny.(snapshot.ParentDependencies)
	logger := deps.GetLogger()
	logger.Info("Stopping parent worker")
	return nil
}

func (a *StopAction) String() string {
	return StopActionName
}

func (a *StopAction) Name() string {
	return StopActionName
}
