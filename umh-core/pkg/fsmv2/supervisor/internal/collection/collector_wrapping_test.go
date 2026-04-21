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

package collection_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/internal/collection"
)

// ---------------------------------------------------------------------------
// Test types for collector wrapping
// ---------------------------------------------------------------------------

// testWrappingObservedState satisfies ObservedState and implements all duck-type
// setters used by wrapNewObservation. Type name ends in "ObservedState" so
// DeriveWorkerType produces "testwrapping" — matching the CSE collection name.
type testWrappingObservedState struct {
	CollectedAt   time.Time             `json:"collected_at"`
	State         string                `json:"state"`
	Metrics       deps.MetricsContainer `json:"metrics,omitempty"`
	ActionResults []deps.ActionResult   `json:"last_action_results,omitempty"`
}

func (o testWrappingObservedState) GetTimestamp() time.Time { return o.CollectedAt }

func (o testWrappingObservedState) SetCollectedAt(t time.Time) fsmv2.ObservedState {
	o.CollectedAt = t
	return o
}

func (o testWrappingObservedState) SetFrameworkMetrics(fm deps.FrameworkMetrics) fsmv2.ObservedState {
	o.Metrics.Framework = fm
	return o
}

func (o testWrappingObservedState) SetActionHistory(h []deps.ActionResult) fsmv2.ObservedState {
	o.ActionResults = h
	return o
}

func (o testWrappingObservedState) SetWorkerMetrics(m deps.Metrics) fsmv2.ObservedState {
	o.Metrics.Worker = m
	return o
}

func (o testWrappingObservedState) SetState(s string) fsmv2.ObservedState {
	o.State = s
	return o
}

// wrappingTestWorker implements Worker + DependencyProvider for testing
// the NewObservation collector wrapping path.
type wrappingTestWorker struct {
	baseDeps *deps.BaseDependencies
	observed fsmv2.ObservedState
}

func (w *wrappingTestWorker) CollectObservedState(_ context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	return w.observed, nil
}

func (w *wrappingTestWorker) DeriveDesiredState(_ interface{}) (fsmv2.DesiredState, error) {
	return &config.DesiredState{BaseDesiredState: config.BaseDesiredState{State: "running"}}, nil
}

func (w *wrappingTestWorker) GetInitialState() fsmv2.State[any, any] {
	return &supervisor.TestState{}
}

func (w *wrappingTestWorker) GetDependenciesAny() any {
	return w.baseDeps
}

// wrappingTestWorkerNoDeps implements Worker but NOT DependencyProvider.
type wrappingTestWorkerNoDeps struct {
	observed fsmv2.ObservedState
}

func (w *wrappingTestWorkerNoDeps) CollectObservedState(_ context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	return w.observed, nil
}

func (w *wrappingTestWorkerNoDeps) DeriveDesiredState(_ interface{}) (fsmv2.DesiredState, error) {
	return &config.DesiredState{BaseDesiredState: config.BaseDesiredState{State: "running"}}, nil
}

func (w *wrappingTestWorkerNoDeps) GetInitialState() fsmv2.State[any, any] {
	return &supervisor.TestState{}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func wrappingTestIdentity(id string) deps.Identity {
	return deps.Identity{ID: id, WorkerType: "testwrapping", Name: id}
}

func wrappingDesiredProvider() func() fsmv2.DesiredState {
	return func() fsmv2.DesiredState {
		return &config.DesiredState{BaseDesiredState: config.BaseDesiredState{State: "running"}}
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

var _ = Describe("Collector post-COS wrapping", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() {
		cancel()
	})

	Context("NewObservation worker (zero CollectedAt)", func() {
		It("should set CollectedAt, framework metrics, action history, and accumulated worker metrics", func() {
			identity := wrappingTestIdentity("wrap-test")
			store := supervisor.CreateTestTriangularStoreForWorkerType("testwrapping")

			// Pre-seed CSE with previous observation containing worker metrics.
			prevObs := testWrappingObservedState{
				CollectedAt: time.Now().Add(-time.Second),
				Metrics: deps.MetricsContainer{
					Worker: deps.Metrics{
						Counters: map[string]int64{"existing_counter": 10},
						Gauges:   map[string]float64{"old_gauge": 2.0},
					},
				},
			}
			_, err := storage.SaveObservedTyped[testWrappingObservedState](store, ctx, identity.ID, prevObs)
			Expect(err).ToNot(HaveOccurred())

			// Set up worker with real BaseDependencies.
			bd := deps.NewBaseDependencies(deps.NewNopFSMLogger(), nil, identity)

			// Seed MetricsRecorder with current-tick data.
			bd.MetricsRecorder().IncrementCounter("existing_counter", 5)
			bd.MetricsRecorder().IncrementCounter("new_counter", 1)
			bd.MetricsRecorder().SetGauge("new_gauge", 3.14)

			expectedFM := &deps.FrameworkMetrics{TimeInCurrentStateMs: 42}
			expectedAH := []deps.ActionResult{{ActionType: "test-action", Success: true}}

			worker := &wrappingTestWorker{
				baseDeps: bd,
				observed: testWrappingObservedState{}, // Zero CollectedAt → triggers wrapping
			}

			c := collection.NewCollector[testWrappingObservedState](collection.CollectorConfig[testWrappingObservedState]{
				Worker:                  worker,
				Identity:                identity,
				Store:                   store,
				Logger:                  deps.NewNopFSMLogger(),
				ObservationInterval:     50 * time.Millisecond,
				ObservationTimeout:      3 * time.Second,
				DesiredStateProvider:     wrappingDesiredProvider(),
				FrameworkMetricsProvider: func() *deps.FrameworkMetrics { return expectedFM },
				FrameworkMetricsSetter:   func(fm *deps.FrameworkMetrics) { bd.SetFrameworkState(fm) },
				ActionHistoryProvider:    func() []deps.ActionResult { return expectedAH },
				ActionHistorySetter:      func(ah []deps.ActionResult) { bd.SetActionHistory(ah) },
				StateProvider:            func() string { return "running" },
			})

			err = c.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Wait until collector writes an observation with accumulated metrics.
			Eventually(func() int64 {
				var loaded testWrappingObservedState
				if loadErr := store.LoadObservedTyped(ctx, "testwrapping", identity.ID, &loaded); loadErr != nil {
					return 0
				}

				return loaded.Metrics.Worker.Counters["existing_counter"]
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(int64(15)))

			var loaded testWrappingObservedState
			Expect(store.LoadObservedTyped(ctx, "testwrapping", identity.ID, &loaded)).To(Succeed())

			// CollectedAt was set by wrapNewObservation.
			Expect(loaded.CollectedAt).ToNot(BeZero())

			// Framework metrics injected.
			Expect(loaded.Metrics.Framework.TimeInCurrentStateMs).To(Equal(int64(42)))

			// Action history injected.
			Expect(loaded.ActionResults).To(HaveLen(1))
			Expect(loaded.ActionResults[0].ActionType).To(Equal("test-action"))

			// Accumulated worker metrics:
			//   existing_counter: 10 (previous) + 5 (drain) = 15
			//   new_counter:       0 (no previous) + 1 (drain) = 1
			Expect(loaded.Metrics.Worker.Counters["new_counter"]).To(Equal(int64(1)))

			//   Gauges replace: new_gauge=3.14, old_gauge preserved from previous.
			Expect(loaded.Metrics.Worker.Gauges["new_gauge"]).To(Equal(3.14))
			Expect(loaded.Metrics.Worker.Gauges["old_gauge"]).To(Equal(2.0))

			// State injected by collector's SetState duck-type (separate from wrapping).
			Expect(loaded.State).To(Equal("running"))
		})
	})

	Context("Legacy worker (no duck-type setters)", func() {
		It("should run collection loop without panic", func() {
			worker := &supervisor.TestWorker{
				Observed: supervisor.CreateTestObservedStateWithID("legacy-worker"),
			}
			store := supervisor.CreateTestTriangularStore()

			c := collection.NewCollector[supervisor.TestObservedState](collection.CollectorConfig[supervisor.TestObservedState]{
				Worker:              worker,
				Identity:            supervisor.TestIdentity(),
				Store:               store,
				Logger:              deps.NewNopFSMLogger(),
				ObservationInterval: 50 * time.Millisecond,
				ObservationTimeout:  3 * time.Second,
				DesiredStateProvider: wrappingDesiredProvider(),
			})

			err := c.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Legacy workers lack duck-type setters, so wrapNewObservation
			// gracefully skips metric injection. The collector loop must
			// not panic.
			time.Sleep(200 * time.Millisecond)
			Expect(c.IsRunning()).To(BeTrue())
		})
	})

	Context("CSE read failure (no previous state)", func() {
		It("should start from empty metrics and drain correctly", func() {
			identity := wrappingTestIdentity("no-prev")
			store := supervisor.CreateTestTriangularStoreForWorkerType("testwrapping")
			// No pre-seeding — CSE load will fail.

			bd := deps.NewBaseDependencies(deps.NewNopFSMLogger(), nil, identity)
			bd.MetricsRecorder().IncrementCounter("fresh_counter", 7)
			bd.MetricsRecorder().SetGauge("fresh_gauge", 1.5)

			worker := &wrappingTestWorker{
				baseDeps: bd,
				observed: testWrappingObservedState{}, // Zero CollectedAt
			}

			c := collection.NewCollector[testWrappingObservedState](collection.CollectorConfig[testWrappingObservedState]{
				Worker:              worker,
				Identity:            identity,
				Store:               store,
				Logger:              deps.NewNopFSMLogger(),
				ObservationInterval: 50 * time.Millisecond,
				ObservationTimeout:  3 * time.Second,
				DesiredStateProvider: wrappingDesiredProvider(),
				FrameworkMetricsProvider: func() *deps.FrameworkMetrics {
					return &deps.FrameworkMetrics{}
				},
				FrameworkMetricsSetter: func(fm *deps.FrameworkMetrics) { bd.SetFrameworkState(fm) },
				ActionHistoryProvider:  func() []deps.ActionResult { return nil },
				ActionHistorySetter:    func(ah []deps.ActionResult) { bd.SetActionHistory(ah) },
			})

			err := c.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Metrics should come purely from the current drain.
			Eventually(func() int64 {
				var loaded testWrappingObservedState
				if loadErr := store.LoadObservedTyped(ctx, "testwrapping", identity.ID, &loaded); loadErr != nil {
					return 0
				}

				return loaded.Metrics.Worker.Counters["fresh_counter"]
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(int64(7)))

			var loaded testWrappingObservedState
			Expect(store.LoadObservedTyped(ctx, "testwrapping", identity.ID, &loaded)).To(Succeed())
			Expect(loaded.Metrics.Worker.Gauges["fresh_gauge"]).To(Equal(1.5))
		})
	})

	Context("Empty MetricsRecorder", func() {
		It("should not panic and save observation with empty worker metrics", func() {
			identity := wrappingTestIdentity("empty-mr")
			store := supervisor.CreateTestTriangularStoreForWorkerType("testwrapping")

			bd := deps.NewBaseDependencies(deps.NewNopFSMLogger(), nil, identity)
			// MetricsRecorder left empty — no IncrementCounter/SetGauge calls.

			worker := &wrappingTestWorker{
				baseDeps: bd,
				observed: testWrappingObservedState{}, // Zero CollectedAt
			}

			c := collection.NewCollector[testWrappingObservedState](collection.CollectorConfig[testWrappingObservedState]{
				Worker:              worker,
				Identity:            identity,
				Store:               store,
				Logger:              deps.NewNopFSMLogger(),
				ObservationInterval: 50 * time.Millisecond,
				ObservationTimeout:  3 * time.Second,
				DesiredStateProvider: wrappingDesiredProvider(),
				FrameworkMetricsProvider: func() *deps.FrameworkMetrics {
					return &deps.FrameworkMetrics{}
				},
				FrameworkMetricsSetter: func(fm *deps.FrameworkMetrics) { bd.SetFrameworkState(fm) },
				ActionHistoryProvider:  func() []deps.ActionResult { return nil },
				ActionHistorySetter:    func(ah []deps.ActionResult) { bd.SetActionHistory(ah) },
			})

			err := c.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Should complete without panic and save a valid observation.
			Eventually(func() bool {
				var loaded testWrappingObservedState
				if loadErr := store.LoadObservedTyped(ctx, "testwrapping", identity.ID, &loaded); loadErr != nil {
					return false
				}

				return !loaded.CollectedAt.IsZero()
			}, 2*time.Second, 50*time.Millisecond).Should(BeTrue())
		})
	})

	Context("No DependencyProvider", func() {
		It("should set CollectedAt but skip metrics injection", func() {
			identity := wrappingTestIdentity("no-deps")
			store := supervisor.CreateTestTriangularStoreForWorkerType("testwrapping")

			worker := &wrappingTestWorkerNoDeps{
				observed: testWrappingObservedState{}, // Zero CollectedAt, no DependencyProvider
			}

			c := collection.NewCollector[testWrappingObservedState](collection.CollectorConfig[testWrappingObservedState]{
				Worker:              worker,
				Identity:            identity,
				Store:               store,
				Logger:              deps.NewNopFSMLogger(),
				ObservationInterval: 50 * time.Millisecond,
				ObservationTimeout:  3 * time.Second,
				DesiredStateProvider: wrappingDesiredProvider(),
			})

			err := c.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// CollectedAt should be set (wrapNewObservation step 1 succeeds).
			Eventually(func() bool {
				var loaded testWrappingObservedState
				if loadErr := store.LoadObservedTyped(ctx, "testwrapping", identity.ID, &loaded); loadErr != nil {
					return false
				}

				return !loaded.CollectedAt.IsZero()
			}, 2*time.Second, 50*time.Millisecond).Should(BeTrue())

			// Worker metrics should be empty (no DependencyProvider → no accumulation).
			var loaded testWrappingObservedState
			Expect(store.LoadObservedTyped(ctx, "testwrapping", identity.ID, &loaded)).To(Succeed())
			Expect(loaded.Metrics.Worker.Counters).To(BeEmpty())
			Expect(loaded.Metrics.Worker.Gauges).To(BeEmpty())
		})
	})
})
