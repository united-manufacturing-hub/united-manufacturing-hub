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

package hello_world

import (
	"sync"

	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// HelloworldDependencies provides resources needed by worker actions.
//
// Actions interact with external systems (databases, APIs, hardware) through
// dependencies. The dependencies struct stores state that actions modify
// and CollectObservedState reads.
//
// Thread safety: Use sync.RWMutex to protect mutable state since actions
// and observation collection may run concurrently.
type HelloworldDependencies struct {
	*deps.BaseDependencies

	mu        sync.RWMutex
	helloSaid bool
}

// NewHelloworldDependencies creates dependencies for the helloworld worker.
func NewHelloworldDependencies(logger *zap.SugaredLogger, stateReader deps.StateReader, identity deps.Identity) *HelloworldDependencies {
	return &HelloworldDependencies{
		BaseDependencies: deps.NewBaseDependencies(logger, stateReader, identity),
		helloSaid:        false,
	}
}

// SetHelloSaid marks that the hello action has been executed.
// Called by SayHelloAction.
func (d *HelloworldDependencies) SetHelloSaid(said bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.helloSaid = said
}

// HasSaidHello returns whether hello has been said.
// Called by CollectObservedState.
func (d *HelloworldDependencies) HasSaidHello() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.helloSaid
}
