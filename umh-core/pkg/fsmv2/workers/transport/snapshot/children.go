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

// RenderChildren returns the ChildSpec set for the transport worker.
//
// Transport manages two dependency-configured children — push and pull.
// Their configuration arrives exclusively via injected dependencies (JWT token,
// transport interface), so each ChildSpec carries an empty config.BaseUserSpec{};
// the child's DeriveDesiredState reads an empty spec and defaults to "running".
//
// Push and pull packages import the top-level transport package, which in turn
// imports this snapshot package. Importing push or pull here would create an
// import cycle, so config.BaseUserSpec{} is used directly. Both PushUserSpec and
// PullUserSpec embed only BaseUserSpec with no additional fields, so the marshalled
// YAML output is identical.
//
// The enabled parameter controls whether children should be active (true = alive
// trajectory) or resident-disabled (false = stop trajectory, pause-not-delete).
// Children are never despawned for transport: buffer holders stay resident.
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
