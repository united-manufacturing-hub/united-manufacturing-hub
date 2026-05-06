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
// IsBeingRemoved and SetBeingRemoved for free.
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

// IsDisabled reports whether this child is currently transient-paused by its
// parent (case B in the six-cases enumeration). Set by the CHANGE-19 reducer
// in supervisor/reconciliation.go when the parent's ChildSpec.Enabled=false
// for this child; cleared when the parent flips back to Enabled=true. The
// reducer runs at the start of reconcileChildren, before the child's tick,
// so child state files see a consistent value for the whole tick.
//
// Transient: the worker stays resident in s.children. ShouldStop returns true
// (the umbrella OR), so non-Stopped states route to Stopping → Stopped via
// the standard stop path. The Stopped state stays put (helpers.StoppedNext
// "stay resident" branch) and resumes to TryingToStart when IsDisabled clears.
//
// Distinct from IsBeingRemoved (permanent removal — cases C, D, E, F: parent
// omits spec, SignalNeedsRestart, graceful process shutdown, supervisor
// self-protection) and Config.GetState()=="stopped" (transient user-driven
// stop — case A). See WorkerSnapshot.ShouldStop for the umbrella OR over all
// three, and pkg/fsmv2/CLAUDE.md "Stopping a worker — six cases, three
// signals" for the case table.
func (d *WrappedDesiredState[TConfig]) IsDisabled() bool {
	return d.Disabled
}

// SetDisabled sets the transient-disable flag. Called by the CHANGE-19 reducer
// in supervisor/reconciliation.go from the parent's ChildSpec.Enabled flag.
// Worker code should not call this directly.
func (d *WrappedDesiredState[TConfig]) SetDisabled(v bool) {
	d.Disabled = v
}
