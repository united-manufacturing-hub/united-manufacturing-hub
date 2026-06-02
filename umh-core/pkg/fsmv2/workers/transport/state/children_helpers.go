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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/snapshot"
)

// yaml.Marshal of the ChildAuthUserSpec carrier cannot fail in practice; on the
// hypothetical error we still return nil so the supervisor falls back to
// GetChildrenSpecs() rather than emitting an empty children set (which
// would despawn all children).
func childrenAlive(cfg snapshot.TransportDesiredState, status snapshot.TransportStatus) []config.ChildSpec {
	specs, err := snapshot.RenderChildren(cfg, status, true)
	if err != nil {
		return nil
	}

	return specs
}

func childrenStopped(cfg snapshot.TransportDesiredState, status snapshot.TransportStatus) []config.ChildSpec {
	specs, err := snapshot.RenderChildren(cfg, status, false)
	if err != nil {
		return nil
	}

	return specs
}
