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
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// WorkerSnapshot is the typed snapshot passed to State.Next().
// Replaces the untyped fsmv2.Snapshot + helpers.ConvertSnapshot pattern.
// State files call ConvertWorkerSnapshot[TConfig, TStatus](snapAny) to obtain this.
type WorkerSnapshot[TConfig any, TStatus any] struct {
	CollectedAt         time.Time
	Config              TConfig
	Status              TStatus
	ChildrenView        any
	Identity            deps.Identity
	ParentMappedState   string
	LastActionResults   []deps.ActionResult
	ChildrenHealthy     int
	ChildrenUnhealthy   int
	IsShutdownRequested bool
}

// IsStopRequired returns true when the worker should transition to stopped.
// Covers both explicit shutdown requests and parent-driven stop signals.
func (s WorkerSnapshot[TConfig, TStatus]) IsStopRequired() bool {
	return s.IsShutdownRequested || s.ParentMappedState == config.DesiredStateStopped
}

// ConvertWorkerSnapshot type-asserts the raw snapshot from State.Next() into a
// fully typed WorkerSnapshot. Panics with a descriptive message if the snapshot
// contains unexpected types.
func ConvertWorkerSnapshot[TConfig any, TStatus any](snapAny any) WorkerSnapshot[TConfig, TStatus] {
	snap, ok := snapAny.(Snapshot)
	if !ok {
		panic(fmt.Sprintf("ConvertWorkerSnapshot: expected fsmv2.Snapshot, got %T", snapAny))
	}

	obs, ok := snap.Observed.(Observation[TStatus])
	if !ok {
		panic(fmt.Sprintf("ConvertWorkerSnapshot: expected Observation[TStatus], got %T", snap.Observed))
	}

	des, ok := snap.Desired.(*WrappedDesiredState[TConfig])
	if !ok {
		panic(fmt.Sprintf("ConvertWorkerSnapshot: expected *WrappedDesiredState[TConfig], got %T", snap.Desired))
	}

	return WorkerSnapshot[TConfig, TStatus]{
		Config:              des.Config,
		Status:              obs.Status,
		Identity:            snap.Identity,
		IsShutdownRequested: des.IsShutdownRequested(),
		ParentMappedState:   obs.ParentMappedState,
		CollectedAt:         obs.CollectedAt,
		LastActionResults:   obs.LastActionResults,
		ChildrenHealthy:     obs.ChildrenHealthy,
		ChildrenUnhealthy:   obs.ChildrenUnhealthy,
		ChildrenView:        obs.ChildrenView,
	}
}

// ExtractConfig type-asserts a DesiredState to *WrappedDesiredState[TConfig]
// and returns the developer's typed config. Used in CollectObservedState to
// access configuration from the desired state parameter.
// Panics with a descriptive message if the type does not match.
func ExtractConfig[TConfig any](desired DesiredState) TConfig {
	wds, ok := desired.(*WrappedDesiredState[TConfig])
	if !ok {
		panic(fmt.Sprintf("ExtractConfig: expected *WrappedDesiredState[TConfig], got %T", desired))
	}

	return wds.Config
}
