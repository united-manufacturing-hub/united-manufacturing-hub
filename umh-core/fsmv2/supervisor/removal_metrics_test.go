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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/persistence/memory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/supervisor/metrics"
)

const (
	stateDurationMetric = "umh_fsmv2_state_duration_seconds"

	removalTestGauge         = "removal_test_gauge"
	removalTestGaugeMetric   = "umh_fsmv2_worker_" + removalTestGauge
	removalTestCounter       = "removal_test_counter"
	removalTestCounterMetric = "umh_fsmv2_worker_" + removalTestCounter
)

// seriesLabels returns the label sets of metricName's series for hierarchyPath.
func seriesLabels(helperFramesAboveSpec int, metricName, hierarchyPath string) []map[string]string {
	families, err := prometheus.DefaultGatherer.Gather()
	ExpectWithOffset(helperFramesAboveSpec+1, err).NotTo(HaveOccurred())

	var matches []map[string]string

	for _, family := range families {
		if family.GetName() != metricName {
			continue
		}

		for _, metric := range family.GetMetric() {
			labels := map[string]string{}
			for _, label := range metric.GetLabel() {
				labels[label.GetName()] = label.GetValue()
			}

			if labels["hierarchy_path"] == hierarchyPath {
				matches = append(matches, labels)
			}
		}
	}

	return matches
}

// stateDurationStates returns the states that have a state-duration series
// for hierarchyPath in the default Prometheus registry.
func stateDurationStates(hierarchyPath string) []string {
	var states []string

	for _, labels := range seriesLabels(1, stateDurationMetric, hierarchyPath) {
		states = append(states, labels["state"])
	}

	return states
}

// exportTestWorkerMetrics exports the way the collector does after it saves an observation.
func exportTestWorkerMetrics(handle *metrics.WorkerMetricsHandle, gaugeValue float64) {
	workerMetrics := deps.Metrics{
		Counters: map[string]int64{removalTestCounter: 3},
		Gauges:   map[string]float64{removalTestGauge: gaugeValue},
	}
	observed := fsmv2.Observation[struct{}]{}.SetWorkerMetrics(workerMetrics)

	handle.Export(observed)
}

// gaugeValues returns the values of every series of metricName for hierarchyPath.
func gaugeValues(metricName, hierarchyPath string) []float64 {
	families, err := prometheus.DefaultGatherer.Gather()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	var values []float64

	for _, family := range families {
		if family.GetName() != metricName {
			continue
		}

		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				if label.GetName() == "hierarchy_path" && label.GetValue() == hierarchyPath {
					values = append(values, metric.GetGauge().GetValue())
				}
			}
		}
	}

	return values
}

// supervisorWithState builds a supervisor whose one worker stays in state. Any
// state-duration series other tests left for the same hierarchy path is deleted
// first, so the spec starts clean.
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

var _ = Describe("The worker metrics of a worker", func() {
	It("are gone after the worker is removed for good", func() {
		state := &mockState{signal: fsmv2.SignalNone}
		s := supervisorWithState(state)
		hierarchyPath := s.GetHierarchyPath()

		Expect(s.TestTick(context.Background())).To(Succeed())
		exportTestWorkerMetrics(s.TestWorkerMetrics(mockIdentity().ID), 1)
		Expect(seriesLabels(0, removalTestGaugeMetric, hierarchyPath)).To(HaveLen(1))
		Expect(seriesLabels(0, removalTestCounterMetric, hierarchyPath)).To(HaveLen(1))

		state.signal = fsmv2.SignalNeedsRemoval

		Expect(s.TestTick(context.Background())).To(Succeed())
		Expect(s.ListWorkers()).To(BeEmpty())
		Expect(seriesLabels(0, removalTestGaugeMetric, hierarchyPath)).To(BeEmpty())
		Expect(seriesLabels(0, removalTestCounterMetric, hierarchyPath)).To(BeEmpty())
	})

	It("do not come back from a collection that returns after RemoveWorkerForRestart", func() {
		state := &mockState{signal: fsmv2.SignalNone}
		s := supervisorWithState(state)
		hierarchyPath := s.GetHierarchyPath()

		Expect(s.TestTick(context.Background())).To(Succeed())
		removedHandle := s.TestWorkerMetrics(mockIdentity().ID)
		exportTestWorkerMetrics(removedHandle, 1)

		Expect(s.RemoveWorkerForRestart(context.Background(), mockIdentity().ID)).To(Succeed())
		exportTestWorkerMetrics(removedHandle, 1)

		Expect(seriesLabels(0, removalTestGaugeMetric, hierarchyPath)).To(BeEmpty())
		Expect(seriesLabels(0, removalTestCounterMetric, hierarchyPath)).To(BeEmpty())
	})

	It("of a worker added again at the same path keep their values when the removed worker's collection returns", func() {
		state := &mockState{signal: fsmv2.SignalNone}
		s := supervisorWithState(state)
		hierarchyPath := s.GetHierarchyPath()

		Expect(s.TestTick(context.Background())).To(Succeed())
		removedHandle := s.TestWorkerMetrics(mockIdentity().ID)

		Expect(s.RemoveWorkerForRestart(context.Background(), mockIdentity().ID)).To(Succeed())
		Expect(s.AddWorker(mockIdentity(), &mockWorker{initialState: state})).To(Succeed())
		exportTestWorkerMetrics(s.TestWorkerMetrics(mockIdentity().ID), 1)

		exportTestWorkerMetrics(removedHandle, 99)

		Expect(gaugeValues(removalTestGaugeMetric, hierarchyPath)).To(ConsistOf(1.0))
	})
})

// stuckRunningState never asks for removal, so Shutdown stops it as a registered worker.
type stuckRunningState struct{}

func (stuckRunningState) String() string { return "running" }

func (stuckRunningState) LifecyclePhase() config.LifecyclePhase { return config.PhaseRunningHealthy }

func (s stuckRunningState) Next(_ any) fsmv2.NextResult[any, any] {
	return fsmv2.NextResult[any, any]{Signal: fsmv2.SignalNone, State: s, Reason: "running"}
}

type stuckRunningWorker struct {
	id string
}

func (w *stuckRunningWorker) CollectObservedState(_ context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	return &supervisor.TestObservedState{ID: w.id, CollectedAt: time.Now(), Desired: &supervisor.TestDesiredState{}}, nil
}

func (w *stuckRunningWorker) DeriveDesiredState(_ interface{}) (fsmv2.DesiredState, error) {
	return &config.DesiredState{State: "running"}, nil
}

func (w *stuckRunningWorker) GetInitialState() fsmv2.State[any, any] {
	return stuckRunningState{}
}

const removalRunningType = "removalmetrics"

var _ = Describe("Shutdown", func() {
	It("deletes the worker metrics of a worker that never asked for removal", func() {
		ctx := context.Background()
		identity := deps.Identity{
			ID:            "removal-metrics-shutdown",
			Name:          "removal-metrics-shutdown",
			WorkerType:    removalRunningType,
			HierarchyPath: "removal-metrics-shutdown(" + removalRunningType + ")",
		}

		basicStore := memory.NewInMemoryStore()
		for _, suffix := range []string{"_identity", "_desired", "_observed"} {
			Expect(basicStore.CreateCollection(ctx, removalRunningType+suffix, nil)).To(Succeed())
		}
		triangularStore := storage.NewTriangularStore(basicStore, deps.NewNopFSMLogger())

		root := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
			WorkerType:              removalRunningType,
			Store:                   triangularStore,
			Logger:                  deps.NewNopFSMLogger(),
			TickInterval:            20 * time.Millisecond,
			GracefulShutdownTimeout: 200 * time.Millisecond,
		})
		Expect(root.AddWorker(identity, &stuckRunningWorker{id: identity.ID})).To(Succeed())

		_, err := triangularStore.SaveDesired(ctx, removalRunningType, identity.ID, persistence.Document{
			supervisor.FieldID:  identity.ID,
			"ShutdownRequested": false,
			"state":             "running",
		})
		Expect(err).NotTo(HaveOccurred())

		runCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		_ = root.Start(runCtx)

		Eventually(root.ListWorkers, 5*time.Second).Should(HaveLen(1))
		exportTestWorkerMetrics(root.TestWorkerMetrics(identity.ID), 1)
		Expect(seriesLabels(0, removalTestGaugeMetric, identity.HierarchyPath)).To(HaveLen(1))

		root.Shutdown()

		Expect(root.ListWorkers()).To(HaveLen(1))
		Expect(seriesLabels(0, removalTestGaugeMetric, identity.HierarchyPath)).To(BeEmpty())
		Expect(seriesLabels(0, removalTestCounterMetric, identity.HierarchyPath)).To(BeEmpty())
	})
})
