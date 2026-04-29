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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
)

// WorkerSnapshot is the typed snapshot passed to State.Next().
// Replaces the untyped fsmv2.Snapshot + helpers.ConvertSnapshot pattern.
// State files call ConvertWorkerSnapshot[TConfig, TStatus](snapAny) to obtain this.
//
// The snapshot exposes the nested form (Observed/Desired) which surfaces the
// observation and wrapped-desired structs directly so state files reach typed
// framework data without a parallel set of accessors.
type WorkerSnapshot[TConfig any, TStatus any] struct {
	// CollectedAt is when the underlying observation was taken. Mirrors
	// Observed.CollectedAt; kept top-level for direct access by persistence
	// helpers and consumers that don't need the rest of the observation.
	CollectedAt time.Time

	// Identity carries the worker's identity (ID, Name, WorkerType,
	// HierarchyPath). Stays top-level — it is wire-format identity, not a
	// property of the observation, and lives on the raw Snapshot envelope.
	Identity deps.Identity
	// Desired is the wrapped desired state (BaseDesiredState + TConfig +
	// ChildrenSpecs). Use Desired.Config for typed config and
	// Desired.IsShutdownRequested() for the merged shutdown signal.
	// Treat Desired as read-only; the framework does not observe writes back.
	Desired WrappedDesiredState[TConfig]

	// Observed is the full typed observation captured by the supervisor on
	// this tick. Use Observed.Status for developer business data and
	// Observed.LifecyclePhase() / Observed.State for FSM state introspection.
	// Treat Observed as read-only; the framework does not observe writes back,
	// and shared slice/map/pointer mutations (e.g. on LastActionResults,
	// ChildrenView's children slice) may corrupt supervisor state.
	Observed Observation[TStatus]
}

// ShouldStop returns true when the worker should stop.
// Covers the explicit shutdown signal (user stop or parent-driven stop merged by supervisor).
//
// CHANGE-1 final resolution (P3.7): body simplified to IsShutdownRequested only.
// The ParentMappedState OR-clause (active from P3.0 through P3.6) is now retired
// because parents communicate stop intent via BaseDesiredState.ShutdownRequested
// directly — the supervisor merges parent-disabled intent into ShutdownRequested
// via ChildSpec.Enabled before the child's reconciliation tick.
func (s WorkerSnapshot[TConfig, TStatus]) ShouldStop() bool {
	return s.Desired.IsShutdownRequested()
}

// ConvertWorkerSnapshot type-asserts the raw snapshot from State.Next() into a
// fully typed WorkerSnapshot. Panics with a descriptive message if the snapshot
// contains unexpected types.
func ConvertWorkerSnapshot[TConfig any, TStatus any](snapAny any) WorkerSnapshot[TConfig, TStatus] {
	snap, ok := snapAny.(Snapshot)
	if !ok {
		panic(fmt.Sprintf("ConvertWorkerSnapshot: expected fsmv2.Snapshot, got %T", snapAny))
	}

	var expectedObs Observation[TStatus]
	obs, ok := snap.Observed.(Observation[TStatus])
	if !ok {
		panic(fmt.Sprintf("ConvertWorkerSnapshot: expected %T, got %T", expectedObs, snap.Observed))
	}

	var expectedDes *WrappedDesiredState[TConfig]
	des, ok := snap.Desired.(*WrappedDesiredState[TConfig])
	if !ok {
		panic(fmt.Sprintf("ConvertWorkerSnapshot: expected %T, got %T", expectedDes, snap.Desired))
	}

	return WorkerSnapshot[TConfig, TStatus]{
		CollectedAt: obs.CollectedAt,
		Identity:    snap.Identity,
		Observed:    obs,
		Desired:     *des,
	}
}

// ExtractConfig type-asserts a DesiredState to *WrappedDesiredState[TConfig]
// and returns the developer's typed config. Used in CollectObservedState to
// access configuration from the desired state parameter.
// Panics with a descriptive message if the type does not match.
func ExtractConfig[TConfig any](desired DesiredState) TConfig {
	var expected *WrappedDesiredState[TConfig]
	wds, ok := desired.(*WrappedDesiredState[TConfig])
	if !ok {
		panic(fmt.Sprintf("ExtractConfig: expected %T, got %T", expected, desired))
	}

	return wds.Config
}
