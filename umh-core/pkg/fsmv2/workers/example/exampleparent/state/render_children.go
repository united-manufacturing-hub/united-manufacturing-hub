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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent/snapshot"
)

// RenderChildren is the state-package mirror of the canonical
// workers/example/exampleparent.RenderChildren emitter. The mirror exists to
// break the package import cycle: the worker package directly references
// state/StoppedState{} from GetInitialState (worker.go line ~143), so state/
// cannot import the exampleparent worker package without a cycle.
//
// Per P2.2 option (a) decision (see lab report): exampleparent intentionally
// preserves the divergent RenderChildren(spec *ParentUserSpec) signature in
// the worker package as a teaching example for the OLD typed-config /
// helpers.ConvertSnapshot pattern. The snapshot's typed Desired
// (*ExampleparentDesiredState) does not yet carry the ParentUserSpec fields
// (ChildrenCount, ChildWorkerType, ChildConfig) needed to drive the canonical
// emitter from the snapshot alone, so the mirror cannot reproduce the
// canonical body deterministically during the migration window.
//
// The mirror returns nil ("no opinion" per NextResult.Children godoc at
// fsmv2/api.go:140-153) so the supervisor continues to reconcile
// exampleparent's children via the DDS-derived path. nil is the only safe
// return here — the alternative non-nil empty slice ([]ChildSpec{}) is the
// authoritative "I want zero children" signal that, once P2.4 cuts the
// supervisor over to NextResult.Children, would silently despawn
// exampleparent's teaching children (child-0/1/2 from worker_test.go).
// This deferral remains in effect until a future P-step extends
// ExampleparentDesiredState to carry ParentUserSpec fields and the canonical
// worker.go RenderChildren(&parentSpec) is replaceable with a snapshot-driven
// body. At that future step, the mirror's return type can flip from nil to a
// real ChildSpec slice atomically with the canonical body migration.
//
// Idempotent (Design Intent §16), pure, deterministic, and AST-detectable
// by P1.8 architecture test #6 (RenderChildrenCalledAtTopOfStateNext); the
// heuristic only checks call presence, not return value, so the nil return
// satisfies Test #6 unchanged.
func RenderChildren(snap helpers.TypedSnapshot[snapshot.ExampleparentObservedState, *snapshot.ExampleparentDesiredState]) []config.ChildSpec {
	_ = snap
	return nil
}
