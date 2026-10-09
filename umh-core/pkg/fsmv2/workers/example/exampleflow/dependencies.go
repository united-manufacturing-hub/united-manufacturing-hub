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

package exampleflow

import (
	"sync/atomic"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// ExampleflowDependencies holds the flow worker's observable start state: the
// start action flips Started, the observed state reads it back, so
// TryingToStart can tell a start that ran from one that did not.
type ExampleflowDependencies struct {
	*deps.BaseDependencies

	started atomic.Bool
}

// Started reports whether the start action ran.
func (d *ExampleflowDependencies) Started() bool {
	return d.started.Load()
}

// MarkStarted records that the start action ran.
func (d *ExampleflowDependencies) MarkStarted() {
	d.started.Store(true)
}
