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
	"github.com/prometheus/client_golang/prometheus"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/metrics"
)

const stateDurationMetric = "umh_fsmv2_state_duration_seconds"

// stateDurationStates returns the states that have a state-duration series
// for hierarchyPath in the default Prometheus registry.
func stateDurationStates(hierarchyPath string) []string {
	families, err := prometheus.DefaultGatherer.Gather()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	var states []string

	for _, family := range families {
		if family.GetName() != stateDurationMetric {
			continue
		}

		for _, metric := range family.GetMetric() {
			labels := map[string]string{}
			for _, label := range metric.GetLabel() {
				labels[label.GetName()] = label.GetValue()
			}

			if labels["hierarchy_path"] == hierarchyPath {
				states = append(states, labels["state"])
			}
		}
	}

	return states
}

// supervisorWithState builds a supervisor whose one worker stays in state.
// Any state-duration series other tests left for the same hierarchy path is
// deleted first, so the spec starts clean.
func supervisorWithState(state *mockState) *supervisor.Supervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState] {
	state.nextState = state

	s := newSupervisorWithWorkerAndLogger(&mockWorker{initialState: state}, newMockTriangularStore(), supervisor.CollectorHealthConfig{}, deps.NewNopFSMLogger())

	workerPath := mockIdentity().HierarchyPath
	for _, leftover := range stateDurationStates(workerPath) {
		metrics.CleanupStateDuration(workerPath, leftover)
	}

	return s
}

var _ = Describe("The state-duration metric of a worker", func() {
	It("exists while the worker runs and is gone after it is removed for good", func() {
		state := &mockState{signal: fsmv2.SignalNone}
		s := supervisorWithState(state)
		hierarchyPath := s.GetHierarchyPath()

		Expect(s.TestTick(context.Background())).To(Succeed())
		Expect(s.ListWorkers()).To(HaveLen(1))
		Expect(stateDurationStates(hierarchyPath)).To(ConsistOf("MockState"))

		state.signal = fsmv2.SignalNeedsRemoval

		Expect(s.TestTick(context.Background())).To(Succeed())
		Expect(s.ListWorkers()).To(BeEmpty())
		Expect(stateDurationStates(hierarchyPath)).To(BeEmpty())
	})
})
