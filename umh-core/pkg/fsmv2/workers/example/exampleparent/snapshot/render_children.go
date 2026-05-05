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

package snapshot

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
)

// RenderChildren is the snapshot-package emitter consumed by exampleparent
// state.Next implementations. It lives in snapshot/ (the shared leaf imported
// by both state/ and the worker package) so state/ can call it without
// pulling in the worker package and creating an import cycle.
//
// Exampleparent intentionally preserves the divergent
// RenderChildren(spec *ExampleparentConfig) signature in the worker package as a
// teaching example for the OLD typed-config / helpers.ConvertSnapshot pattern.
// ExampleparentDesiredState does not carry the ExampleparentConfig fields
// (ChildrenCount, ChildWorkerType, ChildConfig) needed to drive the canonical
// emitter from the snapshot alone, so this function cannot reproduce the
// canonical body deterministically.
//
// Returns nil ("no opinion" per NextResult.Children godoc at
// fsmv2/api.go:140-153) so the supervisor reconciles exampleparent's children
// via the DDS-derived path. nil is the only safe return — a non-nil empty
// slice ([]ChildSpec{}) is the authoritative "I want zero children" signal
// and would silently despawn exampleparent's teaching children
// (child-0/1/2 from worker_test.go) once the supervisor cuts over to
// NextResult.Children.
//
// Idempotent (Design Intent §16), pure, deterministic.
func RenderChildren(snap helpers.TypedSnapshot[ExampleparentObservedState, *ExampleparentDesiredState]) []config.ChildSpec {
	_ = snap
	return nil
}
