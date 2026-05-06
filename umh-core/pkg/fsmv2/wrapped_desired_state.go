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

package fsmv2

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// WrappedDesiredState wraps a developer's TConfig into the full DesiredState
// required by the supervisor. BaseDesiredState promotion provides
// IsShutdownRequested and SetShutdownRequested for free.
//
// The framework constructs this internally during DeriveDesiredState;
// developers only provide TConfig via WorkerBase helpers.
type WrappedDesiredState[TConfig any] struct {
	config.BaseDesiredState
	Config        TConfig            `json:"config"`
	ChildrenSpecs []config.ChildSpec `json:"childrenSpecs,omitempty"`
	Disabled      bool               `json:"isDisabled,omitempty"`
}

// GetChildrenSpecs returns the children specifications.
// Implements config.ChildSpecProvider interface.
func (d *WrappedDesiredState[TConfig]) GetChildrenSpecs() []config.ChildSpec {
	return d.ChildrenSpecs
}

// IsDisabled reports whether this child is currently disabled by its parent.
// Set by the CHANGE-19 reducer when ChildSpec.Enabled=false. Transient — the
// parent may flip Enabled=true to resume the child without removing it.
//
// Compare with IsShutdownRequested (permanent removal; renamed IsBeingRemoved
// in PR5.3) and Config.GetState()=="stopped" (user-driven stop). See
// WorkerSnapshot.ShouldStop for the umbrella semantic.
func (d *WrappedDesiredState[TConfig]) IsDisabled() bool {
	return d.Disabled
}

// SetDisabled sets the disabled flag. Called by the supervisor reducer.
func (d *WrappedDesiredState[TConfig]) SetDisabled(v bool) {
	d.Disabled = v
}
