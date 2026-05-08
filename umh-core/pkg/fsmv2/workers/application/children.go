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

package application

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// RenderChildren is the parent's children-set emitter for the application
// worker. The application worker is a YAML passthrough parent: children come
// from the user's YAML configuration, not from a worker-specific spec
// structure. RenderChildren takes the ChildrenSpecs that DeriveDesiredState
// already parsed and projected into WrappedDesiredState.ChildrenSpecs, and
// returns a copy.
//
// Pure, deterministic, idempotent: same input yields the same
// ChildSpec values across repeated calls.
func RenderChildren(src []config.ChildSpec) []config.ChildSpec {
	if len(src) == 0 {
		return []config.ChildSpec{}
	}

	out := make([]config.ChildSpec, len(src))
	copy(out, src)
	return out
}
