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
	"sync"
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

// seriesLabels returns the label sets of every series of metricName for
// hierarchyPath in the default Prometheus registry. callerDepth is how many
// helper frames sit between the spec and this function.
func seriesLabels(callerDepth int, metricName, hierarchyPath string) []map[string]string {
	families, err := prometheus.DefaultGatherer.Gather()
	ExpectWithOffset(callerDepth+1, err).NotTo(HaveOccurred())

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

// exportTestWorkerMetrics exports one worker gauge and one worker counter for
// hierarchyPath, as the collector does after it saves an observation.
func exportTestWorkerMetrics(hierarchyPath string) {
	workerMetrics := deps.Metrics{
		Counters: map[string]int64{removalTestCounter: 3},
		Gauges:   map[string]float64{removalTestGauge: 1},
	}
	observed := fsmv2.Observation[struct{}]{}.SetWorkerMetrics(workerMetrics)

	metrics.ExportWorkerMetrics(hierarchyPath, observed)
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

var _ = Describe("The worker metrics of a worker", func() {
	It("are gone after the worker is removed for good", func() {
		state := &mockState{signal: fsmv2.SignalNone}
		s := supervisorWithState(state)
		hierarchyPath := s.GetHierarchyPath()

		Expect(s.TestTick(context.Background())).To(Succeed())
		exportTestWorkerMetrics(hierarchyPath)
		Expect(seriesLabels(0, removalTestGaugeMetric, hierarchyPath)).To(HaveLen(1))
		Expect(seriesLabels(0, removalTestCounterMetric, hierarchyPath)).To(HaveLen(1))

		state.signal = fsmv2.SignalNeedsRemoval

		Expect(s.TestTick(context.Background())).To(Succeed())
		Expect(s.ListWorkers()).To(BeEmpty())
		Expect(seriesLabels(0, removalTestGaugeMetric, hierarchyPath)).To(BeEmpty())
		Expect(seriesLabels(0, removalTestCounterMetric, hierarchyPath)).To(BeEmpty())
	})
})

// blockableWorker is a running worker whose collections block once block is
// called, until release is called.
type blockableWorker struct {
	initialState      fsmv2.State[any, any]
	blocked           chan struct{}
	collectionStarted chan struct{}
	released          chan struct{}
	startedOnce       sync.Once
	id                string
}

func newBlockableWorker(id string, initialState fsmv2.State[any, any]) *blockableWorker {
	return &blockableWorker{
		id:                id,
		initialState:      initialState,
		blocked:           make(chan struct{}),
		collectionStarted: make(chan struct{}),
		released:          make(chan struct{}),
	}
}

func (w *blockableWorker) block()   { close(w.blocked) }
func (w *blockableWorker) release() { close(w.released) }

func (w *blockableWorker) CollectObservedState(_ context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	select {
	case <-w.blocked:
		w.startedOnce.Do(func() { close(w.collectionStarted) })
		<-w.released
	default:
	}

	return &supervisor.TestObservedState{ID: w.id, CollectedAt: time.Now(), Desired: &supervisor.TestDesiredState{}}, nil
}

func (w *blockableWorker) DeriveDesiredState(_ interface{}) (fsmv2.DesiredState, error) {
	return &config.DesiredState{State: "running"}, nil
}

func (w *blockableWorker) GetInitialState() fsmv2.State[any, any] {
	return w.initialState
}

// stuckRunningState never asks for removal, so Shutdown reaches its last step
// with the worker still registered.
type stuckRunningState struct{}

func (stuckRunningState) String() string { return "running" }

func (stuckRunningState) LifecyclePhase() config.LifecyclePhase { return config.PhaseRunningHealthy }

func (s stuckRunningState) Next(_ any) fsmv2.NextResult[any, any] {
	return fsmv2.NextResult[any, any]{Signal: fsmv2.SignalNone, State: s, Reason: "running"}
}

const removalRunningType = "removalmetrics"

func removalRunningIdentity(id string) deps.Identity {
	return deps.Identity{
		ID:            id,
		Name:          id,
		WorkerType:    removalRunningType,
		HierarchyPath: id + "(" + removalRunningType + ")",
	}
}

// startRemovalSupervisor starts a supervisor that runs worker and returns once
// the worker is registered. Shutdown runs after the spec.
func startRemovalSupervisor(identity deps.Identity, worker fsmv2.Worker) *supervisor.Supervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState] {
	ctx := context.Background()

	basicStore := memory.NewInMemoryStore()
	for _, suffix := range []string{"_identity", "_desired", "_observed"} {
		ExpectWithOffset(1, basicStore.CreateCollection(ctx, removalRunningType+suffix, nil)).To(Succeed())
	}
	triangularStore := storage.NewTriangularStore(basicStore, deps.NewNopFSMLogger())

	root := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
		WorkerType:              removalRunningType,
		Store:                   triangularStore,
		Logger:                  deps.NewNopFSMLogger(),
		TickInterval:            20 * time.Millisecond,
		GracefulShutdownTimeout: 200 * time.Millisecond,
	})
	ExpectWithOffset(1, root.AddWorker(identity, worker)).To(Succeed())

	_, err := triangularStore.SaveDesired(ctx, removalRunningType, identity.ID, persistence.Document{
		supervisor.FieldID:  identity.ID,
		"ShutdownRequested": false,
		"state":             "running",
	})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	runCtx, cancel := context.WithCancel(ctx)
	_ = root.Start(runCtx)

	DeferCleanup(func() {
		root.Shutdown()
		cancel()
	})

	EventuallyWithOffset(1, root.ListWorkers, 5*time.Second).Should(HaveLen(1))

	return root
}

func cancelledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	return ctx
}

var _ = Describe("The worker metrics of a running worker", func() {
	It("are gone after RemoveWorkerForRestart, once a collection that outlived Stop returns", func() {
		identity := removalRunningIdentity("removal-metrics-late-collection")
		worker := newBlockableWorker(identity.ID, stuckRunningState{})
		root := startRemovalSupervisor(identity, worker)

		exportTestWorkerMetrics(identity.HierarchyPath)
		worker.block()
		Eventually(worker.collectionStarted, 5*time.Second).Should(BeClosed())

		Expect(root.RemoveWorkerForRestart(cancelledContext(), identity.ID)).To(Succeed())

		Consistently(func() []map[string]string {
			return seriesLabels(1, removalTestGaugeMetric, identity.HierarchyPath)
		}, 100*time.Millisecond).Should(HaveLen(1))

		worker.release()

		Eventually(func() []map[string]string {
			return seriesLabels(1, removalTestGaugeMetric, identity.HierarchyPath)
		}, 5*time.Second).Should(BeEmpty())
		Expect(seriesLabels(0, removalTestCounterMetric, identity.HierarchyPath)).To(BeEmpty())
	})

	It("stay when a worker is added again at the same path before the late collection returns", func() {
		identity := removalRunningIdentity("removal-metrics-readded")
		worker := newBlockableWorker(identity.ID, stuckRunningState{})
		root := startRemovalSupervisor(identity, worker)

		worker.block()
		Eventually(worker.collectionStarted, 5*time.Second).Should(BeClosed())

		Expect(root.RemoveWorkerForRestart(cancelledContext(), identity.ID)).To(Succeed())
		Expect(root.AddWorker(identity, newBlockableWorker(identity.ID, stuckRunningState{}))).To(Succeed())
		exportTestWorkerMetrics(identity.HierarchyPath)

		worker.release()

		Consistently(func() []map[string]string {
			return seriesLabels(1, removalTestGaugeMetric, identity.HierarchyPath)
		}, 300*time.Millisecond).Should(HaveLen(1))
	})

	It("are gone after Shutdown stops a worker that never asked for removal", func() {
		identity := removalRunningIdentity("removal-metrics-shutdown")
		root := startRemovalSupervisor(identity, newBlockableWorker(identity.ID, stuckRunningState{}))

		exportTestWorkerMetrics(identity.HierarchyPath)
		Expect(seriesLabels(0, removalTestGaugeMetric, identity.HierarchyPath)).To(HaveLen(1))

		root.Shutdown()

		Expect(root.ListWorkers()).To(HaveLen(1))
		Expect(seriesLabels(0, removalTestGaugeMetric, identity.HierarchyPath)).To(BeEmpty())
		Expect(seriesLabels(0, removalTestCounterMetric, identity.HierarchyPath)).To(BeEmpty())
	})
})
