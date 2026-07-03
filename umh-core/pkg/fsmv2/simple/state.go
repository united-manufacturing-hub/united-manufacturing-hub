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

package simple

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/internal/helpers"
)

// runningState is the shared steady state of every simple worker. It is a
// framework-owned singleton registered per worker type by Register. Rung 3 adds
// the degraded branch and rung 5 splits the states into sub-files and derives
// their cadence; for now the worker stays running once observed.
type runningState struct {
	helpers.RunningHealthyBase
}

// Next keeps the worker running. Lifecycle and health branching arrive in later
// rungs; the reason string is static until the state reads the verdict.
func (s *runningState) Next(_ any) fsmv2.NextResult[any, any] {
	return fsmv2.Transition(s, fsmv2.SignalNone, nil, "simple worker running", nil)
}

// String returns the observed-state name, derived from the type name.
func (s *runningState) String() string {
	return helpers.DeriveStateName(s)
}
