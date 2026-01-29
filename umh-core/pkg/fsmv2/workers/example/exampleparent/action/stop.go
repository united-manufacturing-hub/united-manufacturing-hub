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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

const StopActionName = "stop"

// StopAction gracefully stops all children and cleans up resources.
type StopAction struct{}

// Execute stops all children gracefully.
func (a *StopAction) Execute(ctx context.Context, depsAny any) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if depsAny != nil {
		deps := depsAny.(deps.Dependencies)
		logger := deps.GetLogger()
		logger.Info("Stopping parent worker")
	}

	return nil
}

func (a *StopAction) String() string {
	return StopActionName
}

func (a *StopAction) Name() string {
	return StopActionName
}
