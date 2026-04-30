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

package supervisor

import (
	"sort"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// buildChildrenView constructs a serializable config.ChildrenView snapshot
// from the supervisor's live children map. It is called by the supervisor on
// each tick and the result is injected into ObservedState via the
// SetChildrenView setter.
//
// The aggregate predicates inside config.NewChildrenView short-circuit for
// empty children slices (AllHealthy, AllOperational, and AllStopped all return
// true) and per-child classification reads the cached LifecyclePhase populated
// by buildChildInfo.
//
// Children are emitted in name-sorted order so JSON serialization is stable
// across ticks and equality checks behave deterministically when comparing
// snapshots stored in CSE.
func buildChildrenView(children map[string]SupervisorInterface) config.ChildrenView {
	if len(children) == 0 {
		return config.NewChildrenView(nil)
	}

	names := make([]string, 0, len(children))
	for name := range children {
		names = append(names, name)
	}
	sort.Strings(names)

	infos := make([]config.ChildInfo, 0, len(names))
	for _, name := range names {
		infos = append(infos, buildChildInfo(name, children[name]))
	}

	return config.NewChildrenView(infos)
}

// buildChildInfo creates a ChildInfo struct from a child supervisor.
//
// The cached Phase field is the canonical source of truth for predicate
// classification inside ChildrenView. StateName is retained for display only:
// it carries the raw worker state name (e.g., "Connected"), which the
// prefix-based ParseLifecyclePhase cannot reliably classify.
func buildChildInfo(name string, child SupervisorInterface) config.ChildInfo {
	stateName, stateReason := child.GetCurrentStateNameAndReason()
	phase := child.GetLifecyclePhase()

	return config.ChildInfo{
		Name:          name,
		WorkerType:    child.GetWorkerType(),
		StateName:     stateName,
		StateReason:   stateReason,
		Phase:         phase,
		IsHealthy:     phase.IsHealthy(), // Only PhaseRunningHealthy is healthy
		ErrorMsg:      "",
		HierarchyPath: child.GetHierarchyPath(),
		IsStale:       child.IsObservationStale(),
		IsCircuitOpen: child.IsCircuitOpen(),
	}
}
