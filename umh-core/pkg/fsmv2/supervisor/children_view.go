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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
)

// childInfoSlice returns a per-tick snapshot of every child supervisor as a
// slice of config.ChildInfo. The slice feeds config.NewChildrenView, which the
// collector injects onto parent ObservedState via the ChildrenViewConsumer
// capability interface.
//
// The RLock covers the children map snapshot only; per-child accessors run
// without holding the supervisor lock so AddWorker and RemoveWorker stay
// responsive. The map values are pointer interfaces, so the copied slice
// observes each child consistently for the duration of one tick.
func (s *Supervisor[TObserved, TDesired]) childInfoSlice() []config.ChildInfo {
	s.mu.RLock()

	childrenCopy := make(map[string]SupervisorInterface, len(s.children))
	for name, child := range s.children {
		childrenCopy[name] = child
	}

	s.mu.RUnlock()

	infos := make([]config.ChildInfo, 0, len(childrenCopy))

	for name, child := range childrenCopy {
		stateName, stateReason := child.GetCurrentStateNameAndReason()
		phase := child.GetLifecyclePhase()
		infos = append(infos, config.ChildInfo{
			Name:          name,
			WorkerType:    child.GetWorkerType(),
			StateName:     stateName,
			StateReason:   stateReason,
			IsHealthy:     phase.IsHealthy(),
			IsOperational: phase.IsOperational(),
			IsStopped:     phase.IsStopped(),
			HierarchyPath: child.GetHierarchyPath(),
			IsStale:       child.IsObservationStale(),
			IsCircuitOpen: child.IsCircuitOpen(),
		})
	}

	return infos
}
