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
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/internal/collection"
)

var _ = Describe("Collector", func() {
	Context("when starting collector", func() {
		It("should start observation loop", func() {
			worker := &supervisor.TestWorker{Observed: supervisor.CreateTestObservedStateWithID("test-worker")}
			collector := collection.NewCollector[supervisor.TestObservedState](collection.CollectorConfig[supervisor.TestObservedState]{
				Worker:              worker,
				Identity:            supervisor.TestIdentity(),
				Store:               supervisor.CreateTestTriangularStore(),
				Logger:              deps.NewNopFSMLogger(),
				ObservationInterval: 1 * time.Second,
				ObservationTimeout:  3 * time.Second,
			})

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			err := collector.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Give it time to start
			time.Sleep(100 * time.Millisecond)

			// Should be running
			Expect(collector.IsRunning()).To(BeTrue())

			// Clean shutdown
			cancel()
			time.Sleep(100 * time.Millisecond)
			Expect(collector.IsRunning()).To(BeFalse())
		})
	})

	Context("when triggering immediate collection", func() {
		It("should signal observation loop to collect immediately", func() {
			worker := &supervisor.TestWorker{Observed: supervisor.CreateTestObservedStateWithID("test-worker")}
			collector := collection.NewCollector[supervisor.TestObservedState](collection.CollectorConfig[supervisor.TestObservedState]{
				Worker:              worker,
				Identity:            supervisor.TestIdentity(),
				Store:               supervisor.CreateTestTriangularStore(),
				Logger:              deps.NewNopFSMLogger(),
				ObservationInterval: 1 * time.Second,
				ObservationTimeout:  3 * time.Second,
			})

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			err := collector.Start(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(collector.IsRunning()).To(BeTrue())

			collector.TriggerNow()

			Expect(collector.IsRunning()).To(BeTrue())

			cancel()
			time.Sleep(100 * time.Millisecond)
			Expect(collector.IsRunning()).To(BeFalse())
		})
	})

	Context("when goroutine doesn't exit during TriggerNow (invariant I6 violation)", func() {
		It("should panic with clear message", func() {
			collector := &testCollectorWithHangingLoop{}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			err := collector.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			time.Sleep(50 * time.Millisecond)

			Expect(func() {
				collector.TriggerNow()
			}).To(Panic())
		})
	})

	Context("Invariant I8: Collector lifecycle validation", func() {
		It("should panic when Start() is called twice", func() {
			collector := collection.NewCollector[supervisor.TestObservedState](collection.CollectorConfig[supervisor.TestObservedState]{
				Worker:              &supervisor.TestWorker{},
				Identity:            supervisor.TestIdentity(),
				Store:               supervisor.CreateTestTriangularStore(),
				Logger:              deps.NewNopFSMLogger(),
				ObservationInterval: 1 * time.Second,
				ObservationTimeout:  3 * time.Second,
			})

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			err := collector.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			time.Sleep(50 * time.Millisecond)

			Expect(func() {
				_ = collector.Start(ctx)
			}).To(PanicWith(ContainSubstring("collector already started")))

			cancel()
			time.Sleep(100 * time.Millisecond)
		})

		It("should handle TriggerNow() gracefully when called before Start()", func() {
			collector := collection.NewCollector[supervisor.TestObservedState](collection.CollectorConfig[supervisor.TestObservedState]{
				Worker:              &supervisor.TestWorker{},
				Identity:            supervisor.TestIdentity(),
				Store:               supervisor.CreateTestTriangularStore(),
				Logger:              deps.NewNopFSMLogger(),
				ObservationInterval: 1 * time.Second,
				ObservationTimeout:  3 * time.Second,
			})

			// Should not panic - logs error and returns
			collector.TriggerNow()
		})

		It("should handle Stop() gracefully when called before Start()", func() {
			collector := collection.NewCollector[supervisor.TestObservedState](collection.CollectorConfig[supervisor.TestObservedState]{
				Worker:              &supervisor.TestWorker{},
				Identity:            supervisor.TestIdentity(),
				Store:               supervisor.CreateTestTriangularStore(),
				Logger:              deps.NewNopFSMLogger(),
				ObservationInterval: 1 * time.Second,
				ObservationTimeout:  3 * time.Second,
			})

			// Should not panic - logs warning and returns
			ctx := context.Background()
			collector.Stop(ctx)

			// Collector should still be in created state (not running)
			Expect(collector.IsRunning()).To(BeFalse())
		})

		It("should return false from IsRunning() before Start()", func() {
			collector := collection.NewCollector[supervisor.TestObservedState](collection.CollectorConfig[supervisor.TestObservedState]{
				Worker:              &supervisor.TestWorker{},
				Identity:            supervisor.TestIdentity(),
				Store:               supervisor.CreateTestTriangularStore(),
				Logger:              deps.NewNopFSMLogger(),
				ObservationInterval: 1 * time.Second,
				ObservationTimeout:  3 * time.Second,
			})

			Expect(collector.IsRunning()).To(BeFalse())
		})

		It("should track lifecycle correctly through normal flow", func() {
			collector := collection.NewCollector[supervisor.TestObservedState](collection.CollectorConfig[supervisor.TestObservedState]{
				Worker:              &supervisor.TestWorker{},
				Identity:            supervisor.TestIdentity(),
				Store:               supervisor.CreateTestTriangularStore(),
				Logger:              deps.NewNopFSMLogger(),
				ObservationInterval: 1 * time.Second,
				ObservationTimeout:  3 * time.Second,
			})

			Expect(collector.IsRunning()).To(BeFalse())

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			err := collector.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			time.Sleep(50 * time.Millisecond)
			Expect(collector.IsRunning()).To(BeTrue())

			cancel()
			time.Sleep(100 * time.Millisecond)
			Expect(collector.IsRunning()).To(BeFalse())
		})
	})

	Context("when calling Restart()", func() {
		It("should stop and restart the collector goroutine", func() {
			worker := &supervisor.TestWorker{Observed: supervisor.CreateTestObservedStateWithID("test-worker")}
			collector := collection.NewCollector[supervisor.TestObservedState](collection.CollectorConfig[supervisor.TestObservedState]{
				Worker:              worker,
				Identity:            supervisor.TestIdentity(),
				Store:               supervisor.CreateTestTriangularStore(),
				Logger:              deps.NewNopFSMLogger(),
				ObservationInterval: 1 * time.Second,
				ObservationTimeout:  3 * time.Second,
			})

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			err := collector.Start(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(collector.IsRunning()).To(BeTrue())

			// Restart should stop and start the collector goroutine
			collector.Restart()

			// After restart, collector should still be running
			Expect(collector.IsRunning()).To(BeTrue())

			// TriggerNow should still work after Restart
			collector.TriggerNow()
			Expect(collector.IsRunning()).To(BeTrue())

			cancel()
			time.Sleep(100 * time.Millisecond)
			Expect(collector.IsRunning()).To(BeFalse())
		})

		It("should handle Restart() gracefully when called before Start()", func() {
			collector := collection.NewCollector[supervisor.TestObservedState](collection.CollectorConfig[supervisor.TestObservedState]{
				Worker:              &supervisor.TestWorker{},
				Identity:            supervisor.TestIdentity(),
				Store:               supervisor.CreateTestTriangularStore(),
				Logger:              deps.NewNopFSMLogger(),
				ObservationInterval: 1 * time.Second,
				ObservationTimeout:  3 * time.Second,
			})

			// Should not panic - logs error and returns
			collector.Restart()
		})
	})

	Context("when context is cancelled during collection", func() {
		It("should stop gracefully and mark as not running", func() {
			worker := &supervisor.TestWorker{Observed: supervisor.CreateTestObservedStateWithID("test-worker")}
			collector := collection.NewCollector[supervisor.TestObservedState](collection.CollectorConfig[supervisor.TestObservedState]{
				Worker:              worker,
				Identity:            supervisor.TestIdentity(),
				Store:               supervisor.CreateTestTriangularStore(),
				Logger:              deps.NewNopFSMLogger(),
				ObservationInterval: 50 * time.Millisecond, // Short interval for fast test
				ObservationTimeout:  1 * time.Second,
			})

			ctx, cancel := context.WithCancel(context.Background())

			err := collector.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Wait for at least one collection cycle to start
			time.Sleep(100 * time.Millisecond)
			Expect(collector.IsRunning()).To(BeTrue())

			// Cancel context while collection loop is running
			cancel()

			// Collector should stop within reasonable time
			Eventually(func() bool {
				return collector.IsRunning()
			}, 2*time.Second, 50*time.Millisecond).Should(BeFalse())
		})

		It("should skip COS when context is cancelled after desired state load", func() {
			cosCalled := &atomic.Bool{}
			worker := &cosTrackingWorker{called: cosCalled}

			var cancelCollector context.CancelFunc

			collector := collection.NewCollector[supervisor.TestObservedState](collection.CollectorConfig[supervisor.TestObservedState]{
				Worker:              worker,
				Identity:            supervisor.TestIdentity(),
				Store:               supervisor.CreateTestTriangularStore(),
				Logger:              deps.NewNopFSMLogger(),
				ObservationInterval: 50 * time.Millisecond,
				ObservationTimeout:  5 * time.Second,
				DesiredStateProvider: func() fsmv2.DesiredState {
					// Cancel context inside DesiredStateProvider — after desired
					// state is loaded but before the framework ctx check.
					if cancelCollector != nil {
						cancelCollector()
					}

					return &config.DesiredState{BaseDesiredState: config.BaseDesiredState{}}
				},
			})

			ctx, cancel := context.WithCancel(context.Background())
			cancelCollector = cancel

			err := collector.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Wait for collector to process at least one tick.
			time.Sleep(200 * time.Millisecond)

			// COS should NOT have been called — the framework ctx check caught the cancellation.
			Expect(cosCalled.Load()).To(BeFalse())
		})

		It("should handle immediate cancellation after start", func() {
			worker := &supervisor.TestWorker{Observed: supervisor.CreateTestObservedStateWithID("test-worker")}
			collector := collection.NewCollector[supervisor.TestObservedState](collection.CollectorConfig[supervisor.TestObservedState]{
				Worker:              worker,
				Identity:            supervisor.TestIdentity(),
				Store:               supervisor.CreateTestTriangularStore(),
				Logger:              deps.NewNopFSMLogger(),
				ObservationInterval: 1 * time.Second,
				ObservationTimeout:  3 * time.Second,
			})

			ctx, cancel := context.WithCancel(context.Background())

			err := collector.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Cancel immediately after start (before first collection completes)
			cancel()

			// Collector should stop gracefully
			Eventually(func() bool {
				return collector.IsRunning()
			}, 2*time.Second, 50*time.Millisecond).Should(BeFalse())
		})
	})

	Describe("typed desired-state handoff (post-P1.5c)", func() {
		It("passes DesiredStateProvider's typed value into CollectObservedState verbatim", func() {
			// Locks the contract that replaced the pre-P1.5c duck-typed
			// observedDesiredState injection: the collector must pass the
			// same typed DesiredState pointer the DesiredStateProvider
			// returned to CollectObservedState, so workers can ExtractConfig
			// from it without a separate plumbing path.
			expected := &config.DesiredState{BaseDesiredState: config.BaseDesiredState{}}
			cosCalled := &atomic.Bool{}
			worker := &desiredCapturingWorker{called: cosCalled}

			collector := collection.NewCollector[supervisor.TestObservedState](collection.CollectorConfig[supervisor.TestObservedState]{
				Worker:               worker,
				Identity:             supervisor.TestIdentity(),
				Store:                supervisor.CreateTestTriangularStore(),
				Logger:               deps.NewNopFSMLogger(),
				ObservationInterval:  50 * time.Millisecond,
				ObservationTimeout:   3 * time.Second,
				DesiredStateProvider: func() fsmv2.DesiredState { return expected },
			})

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			Expect(collector.Start(ctx)).To(Succeed())

			Eventually(func() bool { return cosCalled.Load() }, 2*time.Second, 25*time.Millisecond).Should(BeTrue())

			got := worker.gotDesired.Load()
			Expect(got).NotTo(BeNil(), "collector must pass non-nil typed DesiredState into CollectObservedState")
			gotTyped, ok := got.(fsmv2.DesiredState)
			Expect(ok).To(BeTrue(), "captured value must satisfy fsmv2.DesiredState; collector must not box-and-unbox the type")
			Expect(gotTyped).To(BeIdenticalTo(fsmv2.DesiredState(expected)),
				"collector must hand the worker the same DesiredState pointer the provider returned")
		})
	})
})

type testCollectorWithHangingLoop struct {
	collection.Collector[supervisor.TestObservedState]
	parentCtx     context.Context
	ctx           context.Context
	cancel        context.CancelFunc
	goroutineDone chan struct{}
}

func (c *testCollectorWithHangingLoop) Start(ctx context.Context) error {
	c.parentCtx = ctx
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.goroutineDone = make(chan struct{})

	go func() {
		select {}
	}()

	return nil
}

func (c *testCollectorWithHangingLoop) IsRunning() bool {
	return true
}

func (c *testCollectorWithHangingLoop) TriggerNow() {
	if c.cancel != nil {
		c.cancel()
	}

	done := c.goroutineDone
	parentCtx := c.parentCtx

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		panic("Invariant I6 violated: observation loop goroutine did not exit after context cancellation within grace period (5s). This indicates the Worker does not properly handle context cancellation.")
	}

	_ = c.Start(parentCtx)
}

// cosTrackingWorker records whether CollectObservedState was called.
// Used to verify the framework-level ctx.Done() check in the collector.
type cosTrackingWorker struct {
	called *atomic.Bool
}

func (w *cosTrackingWorker) CollectObservedState(_ context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	w.called.Store(true)

	return supervisor.CreateTestObservedStateWithID("cos-tracking"), nil
}

// desiredCapturingWorker records the typed DesiredState pointer value the
// collector passes into CollectObservedState. Used by the typed-handoff
// regression spec — replaces the duck-typed observedDesiredState injection
// test that was removed in P1.5c Row 1. Locks the contract that the
// collector hands the worker the same DesiredState the DesiredStateProvider
// produced, so the worker can ExtractConfig from it directly.
type desiredCapturingWorker struct {
	gotDesired atomic.Value // holds fsmv2.DesiredState
	called     *atomic.Bool
}

func (w *desiredCapturingWorker) CollectObservedState(_ context.Context, desired fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	w.gotDesired.Store(desired)
	w.called.Store(true)

	return supervisor.CreateTestObservedStateWithID("desired-capture"), nil
}

func (w *desiredCapturingWorker) DeriveDesiredState(_ interface{}) (fsmv2.DesiredState, error) {
	return &config.DesiredState{BaseDesiredState: config.BaseDesiredState{}}, nil
}

func (w *desiredCapturingWorker) GetInitialState() fsmv2.State[any, any] {
	return &supervisor.TestState{}
}

func (w *cosTrackingWorker) DeriveDesiredState(_ interface{}) (fsmv2.DesiredState, error) {
	return &config.DesiredState{BaseDesiredState: config.BaseDesiredState{}}, nil
}

func (w *cosTrackingWorker) GetInitialState() fsmv2.State[any, any] {
	return &supervisor.TestState{}
}
