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
	"fmt"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// RenderChildren returns the push and pull ChildSpecs.
// Both carry an empty config.BaseUserSpec{} because their config arrives via
// injected dependencies (JWT token, transport interface), not via UserSpec.
//
// We use BaseUserSpec{} directly instead of PushUserSpec{} / PullUserSpec{}
// because importing those packages here would create an import cycle (both
// import the top-level transport package, which imports this one). Both embed
// only BaseUserSpec with no extra fields, so the YAML output is identical.
//
// enabled=false keeps both children resident in Stopped (not despawned).
// They hold connection buffers and retry state.
func RenderChildren(cfg TransportDesiredState, enabled bool) ([]config.ChildSpec, error) {
	_ = cfg // cfg currently unused; retained for API consistency and future field mapping

	pushSpec, err := config.NewChildSpec("push", "push", config.BaseUserSpec{}, enabled)
	if err != nil {
		return nil, fmt.Errorf("transport RenderChildren: push: %w", err)
	}

	pullSpec, err := config.NewChildSpec("pull", "pull", config.BaseUserSpec{}, enabled)
	if err != nil {
		return nil, fmt.Errorf("transport RenderChildren: pull: %w", err)
	}

	return []config.ChildSpec{pushSpec, pullSpec}, nil
}
