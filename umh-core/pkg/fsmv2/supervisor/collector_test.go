// Copyright 2025 UMH Systems GmbH
package supervisor_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"go.uber.org/zap"
)

var _ = Describe("Collector", func() {
	Context("when starting collector", func() {
		It("should start observation loop", func() {
			collector := supervisor.NewCollector(supervisor.CollectorConfig{
				Worker:              &mockWorker{},
				Identity:            mockIdentity(),
				Store:               &mockStore{},
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
			collector := supervisor.NewCollector(supervisor.CollectorConfig{
				Worker:              &mockWorker{},
				Identity:            mockIdentity(),
				Store:               &mockStore{},
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
			collector := supervisor.NewCollector(supervisor.CollectorConfig{
				Worker:              &mockWorker{},
				Identity:            mockIdentity(),
				Store:               &mockStore{},
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
			collector := supervisor.NewCollector(supervisor.CollectorConfig{
				Worker:              &mockWorker{},
				Identity:            mockIdentity(),
				Store:               &mockStore{},
				Logger:              zap.NewNop().Sugar(),
				ObservationInterval: 1 * time.Second,
				ObservationTimeout:  3 * time.Second,
			})

			collector.Restart()
		})

		It("should return false from IsRunning() before Start()", func() {
			collector := supervisor.NewCollector(supervisor.CollectorConfig{
				Worker:              &mockWorker{},
				Identity:            mockIdentity(),
				Store:               &mockStore{},
				Logger:              zap.NewNop().Sugar(),
				ObservationInterval: 1 * time.Second,
				ObservationTimeout:  3 * time.Second,
			})

			Expect(collector.IsRunning()).To(BeFalse())
		})

		It("should track lifecycle correctly through normal flow", func() {
			collector := supervisor.NewCollector(supervisor.CollectorConfig{
				Worker:              &mockWorker{},
				Identity:            mockIdentity(),
				Store:               &mockStore{},
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
})

type testCollectorWithHangingLoop struct {
	supervisor.Collector
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
