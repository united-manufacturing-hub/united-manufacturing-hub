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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application/snapshot"
)

// RenderChildren is the parent's children-set emitter for the application
// worker. The application worker is a YAML passthrough parent: children come
// from the user's YAML configuration, not from a worker-specific spec
// structure. RenderChildren reads the children that DeriveDesiredState
// already parsed and projected into snap.Desired.ChildrenSpecs, and returns
// them with Enabled forced to true.
//
// §4-C exception: the regular zero-value-false rule is opted out of here in
// favor of "every declared child runs" — the load-bearing semantic for the
// application worker (see the §4-C exception block in DeriveDesiredState).
// Callers that want a stopped child must use ShutdownRequested, not Enabled.
//
// Pure, deterministic, idempotent: same snapshot input yields the same
// ChildSpec values across repeated calls (idempotency property exercised by
// P1.8 architecture test #7). Per §4-C LOCKED, Enabled is set explicitly to
// true so the F4⊕G1 trap detector in P1.8 architecture test #13 (registry
// walk, layer 2) accepts the emitted slice.
func RenderChildren(snap fsmv2.WorkerSnapshot[snapshot.ApplicationConfig, snapshot.ApplicationStatus]) []config.ChildSpec {
	src := snap.Desired.ChildrenSpecs
	if len(src) == 0 {
		return []config.ChildSpec{}
	}

	out := make([]config.ChildSpec, len(src))
	for i, child := range src {
		child.Enabled = true
		out[i] = child
	}
	return out
}
