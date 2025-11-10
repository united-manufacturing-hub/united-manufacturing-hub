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

const StartActionName = "start"

// StartAction loads configuration and spawns children
type StartAction struct {
	dependencies snapshot.ParentDependencies
}

// NewStartAction creates a new start action
func NewStartAction(deps snapshot.ParentDependencies) *StartAction {
	return &StartAction{
		dependencies: deps,
	}
}

// Execute loads config and returns ChildrenSpecs for spawning
func (a *StartAction) Execute(ctx context.Context) error {
	logger := a.dependencies.GetLogger()
	logger.Info("Starting parent worker")

	return nil
}

func (a *StartAction) String() string {
	return StartActionName
}

func (a *StartAction) Name() string {
	return StartActionName
}
