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

package supervisor_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/metrics"
)

func cleanStateDurationSeries(hierarchyPath string) {
	for _, leftover := range stateDurationStates(hierarchyPath) {
		metrics.CleanupStateDuration(hierarchyPath, leftover)
	}
}

var _ = Describe("The state-duration series", func() {
	It("is one per worker, so removing one worker keeps the other's", func() {
		first := &mockState{signal: fsmv2.SignalNone}
		s := supervisorWithState(first)
		firstPath := mockIdentity().HierarchyPath

		second := &mockState{signal: fsmv2.SignalNone}
		second.nextState = second
		secondIdentity := deps.Identity{ID: "second-worker", Name: "Second Worker", WorkerType: "test", HierarchyPath: "second-worker(test)"}

		cleanStateDurationSeries(firstPath)
		cleanStateDurationSeries(secondIdentity.HierarchyPath)
		Expect(s.AddWorker(secondIdentity, &mockWorker{initialState: second})).To(Succeed())

		Expect(s.TestTickAll(context.Background())).To(Succeed())
		Expect(s.ListWorkers()).To(HaveLen(2))
		Expect(stateDurationStates(firstPath)).To(ConsistOf("MockState"))
		Expect(stateDurationStates(secondIdentity.HierarchyPath)).To(ConsistOf("MockState"))

		second.signal = fsmv2.SignalNeedsRemoval

		Expect(s.TestTickAll(context.Background())).To(Succeed())
		Expect(s.ListWorkers()).To(ConsistOf(mockIdentity().ID))
		Expect(stateDurationStates(firstPath)).To(ConsistOf("MockState"))
		Expect(stateDurationStates(secondIdentity.HierarchyPath)).To(BeEmpty())
	})
})
