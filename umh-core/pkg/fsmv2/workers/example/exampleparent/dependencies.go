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

package exampleparent

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent/dependency"
	"go.uber.org/zap"
)

// ParentDependencies provides access to tools needed by parent worker actions.
type ParentDependencies struct {
	*deps.BaseDependencies
	stateTracker dependency.StateTracker
}

// NewParentDependencies creates new dependencies for the parent worker.
func NewParentDependencies(logger *zap.SugaredLogger, stateReader deps.StateReader, identity deps.Identity) *ParentDependencies {
	return &ParentDependencies{
		BaseDependencies: deps.NewBaseDependencies(logger, stateReader, identity),
		stateTracker:     dependency.NewDefaultStateTracker(nil), // Uses real clock
	}
}

// GetStateTracker returns the state tracker tool for tracking state transitions.
func (d *ParentDependencies) GetStateTracker() dependency.StateTracker {
	return d.stateTracker
}
