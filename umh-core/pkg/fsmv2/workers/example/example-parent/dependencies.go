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

package example_parent

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"go.uber.org/zap"
)

// ParentDependencies provides access to tools needed by parent worker actions.
type ParentDependencies struct {
	*fsmv2.BaseDependencies
}

// NewParentDependencies creates new dependencies for the parent worker.
func NewParentDependencies(logger *zap.SugaredLogger, identity fsmv2.Identity) *ParentDependencies {
	return &ParentDependencies{
		BaseDependencies: fsmv2.NewBaseDependencies(logger, identity),
	}
}
