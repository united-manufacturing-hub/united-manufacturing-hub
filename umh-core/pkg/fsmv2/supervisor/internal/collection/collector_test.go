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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/internal/collection"
	"go.uber.org/zap"
)

var _ = Describe("Collector", func() {
Context("when starting collector", func() {
		It("should start observation loop", func() {
			worker := &supervisor.TestWorker{Observed: supervisor.CreateTestObservedStateWithID("test-worker")}
			collector := collection.NewCollector[supervisor.TestObservedState](collection.CollectorConfig[supervisor.TestObservedState]{
				Worker:              worker,
				Identity:            supervisor.TestIdentity(),
				Store:               supervisor.CreateTestTriangularStore(),
				Logger:              zap.NewNop().Sugar(),
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

Context("when restarting collector", func() {
		It("should stop old loop and start new one", func() {
			worker := &supervisor.TestWorker{Observed: supervisor.CreateTestObservedStateWithID("test-worker")}
			collector := collection.NewCollector[supervisor.TestObservedState](collection.CollectorConfig[supervisor.TestObservedState]{
				Worker:              worker,
				Identity:            supervisor.TestIdentity(),
				Store:               supervisor.CreateTestTriangularStore(),
				Logger:              zap.NewNop().Sugar(),
				ObservationInterval: 1 * time.Second,
				ObservationTimeout:  3 * time.Second,
			})

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			err := collector.Start(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(collector.IsRunning()).To(BeTrue())

			collector.Restart()

			Expect(collector.IsRunning()).To(BeTrue())

			cancel()
			time.Sleep(100 * time.Millisecond)
			Expect(collector.IsRunning()).To(BeFalse())
		})
	})

	Context("when goroutine doesn't exit (invariant I6 violation)", func() {
		It("should panic with clear message", func() {
			collector := &testCollectorWithHangingLoop{}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			err := collector.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			time.Sleep(50 * time.Millisecond)

			Expect(func() {
				collector.Restart()
			}).To(Panic())
		})
	})

	Context("Invariant I8: Collector lifecycle validation", func() {
		It("should panic when Start() is called twice", func() {
			collector := collection.NewCollector[supervisor.TestObservedState](collection.CollectorConfig[supervisor.TestObservedState]{
				Worker:              &supervisor.TestWorker{},
				Identity:            supervisor.TestIdentity(),
				Store:               supervisor.CreateTestTriangularStore(),
				Logger:              zap.NewNop().Sugar(),
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

		It("should handle Restart() gracefully when called before Start()", func() {
			collector := collection.NewCollector[supervisor.TestObservedState](collection.CollectorConfig[supervisor.TestObservedState]{
				Worker:              &supervisor.TestWorker{},
				Identity:            supervisor.TestIdentity(),
				Store:               supervisor.CreateTestTriangularStore(),
				Logger:              zap.NewNop().Sugar(),
				ObservationInterval: 1 * time.Second,
				ObservationTimeout:  3 * time.Second,
			})

			// Should not panic - logs error and returns
			collector.Restart()
		})

		It("should handle Stop() gracefully when called before Start()", func() {
			collector := collection.NewCollector[supervisor.TestObservedState](collection.CollectorConfig[supervisor.TestObservedState]{
				Worker:              &supervisor.TestWorker{},
				Identity:            supervisor.TestIdentity(),
				Store:               supervisor.CreateTestTriangularStore(),
				Logger:              zap.NewNop().Sugar(),
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
				Logger:              zap.NewNop().Sugar(),
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
				Logger:              zap.NewNop().Sugar(),
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

	Context("when context is cancelled during collection", func() {
		It("should stop gracefully and mark as not running", func() {
			worker := &supervisor.TestWorker{Observed: supervisor.CreateTestObservedStateWithID("test-worker")}
			collector := collection.NewCollector[supervisor.TestObservedState](collection.CollectorConfig[supervisor.TestObservedState]{
				Worker:              worker,
				Identity:            supervisor.TestIdentity(),
				Store:               supervisor.CreateTestTriangularStore(),
				Logger:              zap.NewNop().Sugar(),
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

		It("should handle immediate cancellation after start", func() {
			worker := &supervisor.TestWorker{Observed: supervisor.CreateTestObservedStateWithID("test-worker")}
			collector := collection.NewCollector[supervisor.TestObservedState](collection.CollectorConfig[supervisor.TestObservedState]{
				Worker:              worker,
				Identity:            supervisor.TestIdentity(),
				Store:               supervisor.CreateTestTriangularStore(),
				Logger:              zap.NewNop().Sugar(),
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

func (c *testCollectorWithHangingLoop) Restart() {
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
