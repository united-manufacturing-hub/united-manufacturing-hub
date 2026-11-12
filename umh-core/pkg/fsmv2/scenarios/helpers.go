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

package scenarios

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2"
)

// timesEntered returns how many times the worker has entered state. The count
// keeps its value after the worker leaves the state, so a wait still sees a
// state the worker has already left.
func timesEntered[TStatus any](obs fsmv2.Observation[TStatus], state string) int64 {
	return obs.Metrics.Framework.TransitionsByState[state]
}
