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

package state

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/application/snapshot"
)

// RenderChildren is the state-package mirror of the canonical
// workers/application.RenderChildren emitter. It exists to break the package
// import cycle: the worker package blank-imports state/ to trigger
// RegisterInitialState (see stopped_state.go init), so state/ cannot import
// the application worker package without a cycle. Mirroring the body here
// keeps state.Next calls AST-detectable by P1.8 architecture test #6
// (RenderChildrenCalledAtTopOfStateNext) without the cycle.
//
// This function MUST stay byte-for-byte equivalent to the production
// workers/application.RenderChildren in workers/application/children.go.
// Any drift between the two is a §4-C exception trap re-emerging — the
// architecture suite's Test 5 / Test 13 layer 2 registry walk anchors the
// canonical worker-package emitter and is unchanged; this mirror feeds
// state.Next only.
//
// Per §4-C YAML-passthrough exception, every declared child runs (Enabled
// forced to true). Pure, deterministic, idempotent: same snapshot input
// yields the same ChildSpec values across repeated calls (idempotency
// property exercised by P1.8 architecture test #7 via the worker-package
// emitter; the local mirror inherits identical semantics).
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
