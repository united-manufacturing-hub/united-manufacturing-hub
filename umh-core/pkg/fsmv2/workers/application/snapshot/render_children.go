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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// RenderChildren is the snapshot-package emitter consumed by application
// state.Next implementations. It lives in snapshot/ (the shared leaf imported
// by both state/ and the worker package) so state/ can call it without
// pulling in the worker package and creating an import cycle.
//
// Per §4-C YAML-passthrough exception, every declared child runs (Enabled
// forced to true). Pure, deterministic, idempotent: same snapshot input
// yields the same ChildSpec values across repeated calls.
func RenderChildren(snap fsmv2.WorkerSnapshot[ApplicationConfig, ApplicationStatus]) []config.ChildSpec {
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
