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
	exampleparent "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/example/exampleparent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
)

// Returning nil on a RenderChildren error is NOT safe: the supervisor falls
// back to GetChildrenSpecs(), which is also nil for migrated workers, so
// reconcileChildren(nil) marks every resident child pendingRemoval (despawn).
// exampleparent's RenderChildren builds its specs literally and cannot fail
// today; the guard and log are future-proofing for when it can. The semantic
// fix (nil = keep last rendered children) is tracked in ENG-5115.
func childrenAlive(cfg exampleparent.ExampleparentConfig) []config.ChildSpec {
	specs, err := exampleparent.RenderChildren(cfg, true)
	if err != nil {
		logger.For("exampleparent").Errorf("RenderChildren(enabled=true) failed, all resident children will be despawned (ENG-5115): %v", err)

		return nil
	}

	return specs
}
