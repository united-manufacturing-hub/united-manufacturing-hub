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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

// RenderChildren returns the push and pull ChildSpecs with the auth session stamped onto both.
// status.AuthSession is embedded in a types.ChildAuthUserSpec carrier so push/pull can parse it
// from their snapshot without reaching into parent dependencies.
//
// enabled=false keeps both children resident in Stopped (not despawned).
// They hold connection buffers and retry state.
func RenderChildren(cfg TransportDesiredState, status TransportStatus, enabled bool) ([]config.ChildSpec, error) {
	_ = cfg // cfg currently unused; retained for API consistency and future field mapping
	carrier := types.ChildAuthUserSpec{AuthSession: status.AuthSession}

	pushSpec, err := config.NewChildSpec("push", "push", carrier, enabled)
	if err != nil {
		return nil, fmt.Errorf("transport RenderChildren: push: %w", err)
	}

	pullSpec, err := config.NewChildSpec("pull", "pull", carrier, enabled)
	if err != nil {
		return nil, fmt.Errorf("transport RenderChildren: pull: %w", err)
	}

	return []config.ChildSpec{pushSpec, pullSpec}, nil
}
