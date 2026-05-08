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
// IsShutdownRequested and SetShutdownRequested for free. The State field
// carries the desired lifecycle state ("running"/"stopped") set by
// DeriveDesiredState from the user spec's BaseUserSpec.GetState().
//
// The framework constructs this during DeriveDesiredState. Developers define
// their TConfig type and call the typed DeriveDesiredState helpers to produce it.
type WrappedDesiredState[TConfig any] struct {
	config.BaseDesiredState
	Config        TConfig            `json:"config"`
	ChildrenSpecs []config.ChildSpec `json:"childrenSpecs,omitempty"`
	State         string             `json:"state"             yaml:"state"` // "stopped" or "running" - desired lifecycle state
}

// GetState returns the desired lifecycle state, defaulting to "running" if empty.
func (d *WrappedDesiredState[TConfig]) GetState() string {
	if d.State == "" {
		return config.DesiredStateRunning
	}

	return d.State
}

// GetChildrenSpecs returns the children specifications.
// Implements config.ChildSpecProvider interface.
func (d *WrappedDesiredState[TConfig]) GetChildrenSpecs() []config.ChildSpec {
	return d.ChildrenSpecs
}
