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
	"fmt"
	"sort"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

// fsmv2SeriesFor returns every umh_fsmv2_* series labelled with hierarchyPath,
// keyed by metric name and labels. The value is a counter's or gauge's value,
// or a histogram's sample count.
func fsmv2SeriesFor(hierarchyPath string) map[string]float64 {
	families, err := prometheus.DefaultGatherer.Gather()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	series := map[string]float64{}

	for _, family := range families {
		if !strings.HasPrefix(family.GetName(), "umh_fsmv2_") {
			continue
		}

		for _, metric := range family.GetMetric() {
			var labels []string

			onPath := false

			for _, label := range metric.GetLabel() {
				if label.GetName() == "hierarchy_path" {
					onPath = label.GetValue() == hierarchyPath

					continue
				}

				labels = append(labels, label.GetName()+"="+label.GetValue())
			}

			if !onPath {
				continue
			}

			sort.Strings(labels)
			key := fmt.Sprintf("%s{%s}", family.GetName(), strings.Join(labels, ","))

			switch {
			case metric.GetCounter() != nil:
				series[key] = metric.GetCounter().GetValue()
			case metric.GetGauge() != nil:
				series[key] = metric.GetGauge().GetValue()
			case metric.GetHistogram() != nil:
				series[key] = float64(metric.GetHistogram().GetSampleCount())
			}
		}
	}

	return series
}

// twoWorkerSupervisor returns a supervisor running the mockIdentity worker
// first and a second worker after it, with both workers ticked once.
func twoWorkerSupervisor(first, second *mockState) (*supervisor.Supervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState], deps.Identity) {
	s := supervisorWithState(first)
	second.nextState = second
	secondIdentity := deps.Identity{ID: "second-worker", Name: "Second Worker", WorkerType: "test", HierarchyPath: "second-worker(test)"}

	ExpectWithOffset(1, s.AddWorker(secondIdentity, &mockWorker{initialState: second})).To(Succeed())
	ExpectWithOffset(1, s.TestTickAll(context.Background())).To(Succeed())

	return s, secondIdentity
}

var _ = Describe("Supervisor metrics", func() {
	It("record nothing under a removed worker's path while another worker keeps running", func() {
		first := &mockState{signal: fsmv2.SignalNone}
		s, secondIdentity := twoWorkerSupervisor(first, &mockState{signal: fsmv2.SignalNone})
		firstPath := mockIdentity().HierarchyPath

		first.signal = fsmv2.SignalNeedsRemoval

		Expect(s.TestTickAll(context.Background())).To(Succeed())
		Expect(s.ListWorkers()).To(ConsistOf(secondIdentity.ID))

		removedWorkerSeries := fsmv2SeriesFor(firstPath)
		secondBefore := fsmv2SeriesFor(secondIdentity.HierarchyPath)

		for range 3 {
			Expect(s.TestTick(context.Background())).To(Succeed())
		}

		Expect(fsmv2SeriesFor(firstPath)).To(Equal(removedWorkerSeries))
		Expect(fsmv2SeriesFor(secondIdentity.HierarchyPath)).NotTo(Equal(secondBefore),
			"the ticks must record metrics for the remaining worker")
	})

	It("record the circuit breaker on every worker of the supervisor", func() {
		s, secondIdentity := twoWorkerSupervisor(&mockState{signal: fsmv2.SignalNone}, &mockState{signal: fsmv2.SignalNone})
		s.TestSetStarted(true)
		s.TestSetCircuitOpen(true)

		Expect(s.TestTick(context.Background())).To(Succeed())
		Expect(s.TestIsInfraCircuitOpen()).To(BeFalse())

		for _, path := range []string{mockIdentity().HierarchyPath, secondIdentity.HierarchyPath} {
			Expect(fsmv2SeriesFor(path)).To(HaveKeyWithValue("umh_fsmv2_circuit_open{}", 0.0), path)
		}
	})

	It("record the open circuit breaker on a worker added while it is open", func() {
		s := supervisorWithState(&mockState{signal: fsmv2.SignalNone})
		s.TestSetStarted(true)

		openChild := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
			WorkerType: "open-child",
			Store:      newMockTriangularStore(),
			Logger:     deps.NewNopFSMLogger(),
		})
		openChild.TestSetCircuitOpen(true)
		s.TestLinkChild("open-child", openChild)

		Expect(s.TestTick(context.Background())).To(MatchError(supervisor.ErrInfraCircuitOpen))
		Expect(fsmv2SeriesFor(mockIdentity().HierarchyPath)).To(HaveKeyWithValue("umh_fsmv2_circuit_open{}", 1.0))

		lateIdentity := deps.Identity{ID: "late-worker", Name: "Late Worker", WorkerType: "test", HierarchyPath: "late-worker(test)"}
		lateState := &mockState{signal: fsmv2.SignalNone}
		lateState.nextState = lateState
		Expect(s.AddWorker(lateIdentity, &mockWorker{initialState: lateState})).To(Succeed())

		Expect(s.TestTick(context.Background())).To(MatchError(supervisor.ErrInfraCircuitOpen))
		Expect(fsmv2SeriesFor(lateIdentity.HierarchyPath)).To(HaveKeyWithValue("umh_fsmv2_circuit_open{}", 1.0))
	})
})
