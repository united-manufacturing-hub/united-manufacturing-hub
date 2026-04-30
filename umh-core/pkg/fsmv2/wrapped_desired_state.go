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
}

// GetChildrenSpecs returns the children specifications.
// Implements config.ChildSpecProvider interface.
func (d *WrappedDesiredState[TConfig]) GetChildrenSpecs() []config.ChildSpec {
	return d.ChildrenSpecs
}
