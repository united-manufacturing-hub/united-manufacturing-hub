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

package transport

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// RenderChildren is the parent's children-set emitter for the transport
// worker. Pure function of the typed snapshot: same input yields the same
// ChildSpec values (and ChildSpec.Hash output) across repeated calls
// (idempotency property exercised by P1.8 architecture test #7).
//
// The transport parent always emits two children — push and pull — that run
// whenever the parent is enabled. Children run regardless of the parent's
// current FSM state, including Degraded, to prevent the oscillation loop
// where a child stops on parent degradation (caused by the child being
// unhealthy), the parent recovers (no unhealthy children), the child
// restarts, and the cycle repeats.
//
// Per §4-C LOCKED, Enabled MUST be set explicitly to true; the F4⊕G1 trap
// detector in P1.8 architecture test #13 (registry walk, layer 2) catches
// forgotten-Enabled in renderChildren bodies.
//
// State.Next emits this set via NextResult.Children (wired in P2.2 and made
// authoritative for the supervisor in P2.4); the legacy DDS-derived path was
// retired in P2.5.
func RenderChildren(snap fsmv2.WorkerSnapshot[TransportConfig, TransportStatus]) []config.ChildSpec {
	rawSpec := snapshotUserSpec(snap)

	return []config.ChildSpec{
		{
			Name:       "push",
			WorkerType: "push",
			UserSpec:   config.UserSpec{Config: rawSpec.Config, Variables: rawSpec.Variables},
			Enabled:    true,
		},
		{
			Name:       "pull",
			WorkerType: "pull",
			UserSpec:   config.UserSpec{Config: rawSpec.Config, Variables: rawSpec.Variables},
			Enabled:    true,
		},
	}
}

// snapshotUserSpec extracts the children's UserSpec source from the parent
// snapshot. Both children inherit the parent's raw spec fields (Config +
// Variables) unchanged. Falls back to zero-value UserSpec on the nil-spec
// startup path.
func snapshotUserSpec(snap fsmv2.WorkerSnapshot[TransportConfig, TransportStatus]) config.UserSpec {
	if len(snap.Desired.ChildrenSpecs) > 0 {
		return snap.Desired.ChildrenSpecs[0].UserSpec
	}
	return config.UserSpec{}
}
