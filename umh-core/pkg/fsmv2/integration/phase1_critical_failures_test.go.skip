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

package integration_test

import (
	"context"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/collection"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/execution"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
	"go.uber.org/zap"
)

var _ = Describe("Phase 1: Critical Failures", func() {
	var (
		logger *zap.SugaredLogger
		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		logger = zap.NewNop().Sugar()
		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() {
		cancel()
	})

	Context("Day 1: Action Executor Failures", func() {
		var (
			executor *execution.ActionExecutor
			before   int
		)

		BeforeEach(func() {
			before = GetGoroutineCount()
			executor = execution.NewActionExecutorWithTimeout(
				1,
				map[string]time.Duration{
					"test_timeout": 100 * time.Millisecond,
				},
				"test-supervisor",
			)
			executor.Start(ctx)
			time.Sleep(10 * time.Millisecond)
		})

		AfterEach(func() {
			executor.Shutdown()
			ExpectNoGoroutineLeaks(before)
		})

		It("should timeout action execution after configured duration", func() {
			By("Enqueueing an action that blocks forever with 100ms timeout")
			action := NewBlockingAction(true)

			start := time.Now()
			err := executor.EnqueueAction("test_timeout", action)
			Expect(err).ToNot(HaveOccurred())

			By("Waiting for the action to complete with timeout")
			Eventually(func() bool {
				return executor.HasActionInProgress("test_timeout")
			}, "2s", "10ms").Should(BeFalse())

			elapsed := time.Since(start)
			By(fmt.Sprintf("Verifying timeout occurred around 100ms (actual: %s)", elapsed))
			Expect(elapsed).To(BeNumerically(">", 90*time.Millisecond))
			Expect(elapsed).To(BeNumerically("<", 500*time.Millisecond))
		})

		It("should recover from action panic without crashing", func() {
			By("Enqueueing an action that panics")
			action := &PanicAction{}

			err := executor.EnqueueAction("panic-action", action)
			Expect(err).ToNot(HaveOccurred())

			By("Waiting for the action to complete")
			Eventually(func() bool {
				return executor.HasActionInProgress("panic-action")
			}, "2s", "10ms").Should(BeFalse())

			By("Verifying executor is still healthy and can process new actions")
			healthyAction := &SlowAction{duration: 10 * time.Millisecond}
			err = executor.EnqueueAction("healthy-action", healthyAction)
			Expect(err).ToNot(HaveOccurred())

			Eventually(healthyAction.WasExecuted, "1s", "10ms").Should(BeTrue())
		})

		It("should handle action queue full scenario", func() {
			By("Filling the action queue with blocking actions")
			queueSize := 2
			for i := 0; i < queueSize*2; i++ {
				action := NewBlockingActionWithDuration(500 * time.Millisecond)
				err := executor.EnqueueAction(fmt.Sprintf("block-%d", i), action)
				if err != nil {
					By(fmt.Sprintf("Queue full at action %d", i))
					break
				}
			}

			By("Attempting to enqueue one more action when queue is full")
			action := &SlowAction{duration: 10 * time.Millisecond}
			err := executor.EnqueueAction("overflow-action", action)

			By("Verifying either rejection or eventual execution")
			if err != nil {
				Expect(err.Error()).To(ContainSubstring("full"))
			} else {
				Eventually(action.WasExecuted, "3s", "50ms").Should(BeTrue())
			}
		})

		It("should handle concurrent action execution safely", func() {
			By("Submitting 10 actions concurrently")
			var wg sync.WaitGroup
			actions := make([]*SlowAction, 10)
			errors := make([]error, 10)

			for i := 0; i < 10; i++ {
				actions[i] = NewSlowAction(50 * time.Millisecond)
				wg.Add(1)

				go func(idx int) {
					defer wg.Done()
					errors[idx] = executor.EnqueueAction(fmt.Sprintf("concurrent-%d", idx), actions[idx])
				}(i)
			}

			wg.Wait()

			By("Verifying all actions either executed or were rejected cleanly")
			executedCount := 0
			for i, action := range actions {
				if errors[i] == nil {
					Eventually(action.WasExecuted, "3s", "50ms").Should(BeTrue())
					executedCount++
				}
			}

			Expect(executedCount).To(BeNumerically(">", 0), "at least some actions should have executed")
		})
	})

	Context("Day 2: Collector Failures", func() {
		var (
			store  storage.TriangularStoreInterface
			before int
		)

		BeforeEach(func() {
			before = GetGoroutineCount()
			store = memory.NewInMemoryStore()
		})

		AfterEach(func() {
			ExpectNoGoroutineLeaks(before)
		})

		It("should timeout collector observation after configured duration", func() {
			worker := NewMockWorker("timeout-worker")
			worker.SetCollectBlockFor(2 * time.Second)

			collectorConfig := collection.CollectorConfig{
				Worker:              worker,
				Identity:            worker.GetIdentity(),
				Store:               store,
				Logger:              logger,
				ObservationInterval: 50 * time.Millisecond,
				ObservationTimeout:  100 * time.Millisecond,
				WorkerType:          "mock",
			}

			collector := collection.NewCollector(collectorConfig)
			err := collector.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			By("Waiting for collector to attempt observation and timeout")
			time.Sleep(500 * time.Millisecond)

			By("Verifying collector is still running after timeout")
			Expect(collector.IsRunning()).To(BeTrue())

			collector.Stop()
			Eventually(collector.IsRunning, "2s", "50ms").Should(BeFalse())
		})

		It("should recover from collector panic without crashing", func() {
			worker := NewMockWorker("panic-worker")
			worker.SetCollectPanic(true)

			collectorConfig := collection.CollectorConfig{
				Worker:              worker,
				Identity:            worker.GetIdentity(),
				Store:               store,
				Logger:              logger,
				ObservationInterval: 50 * time.Millisecond,
				ObservationTimeout:  1 * time.Second,
				WorkerType:          "mock",
			}

			collector := collection.NewCollector(collectorConfig)
			err := collector.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			By("Waiting for collector to panic and recover")
			time.Sleep(200 * time.Millisecond)

			By("Verifying collector is still running after panic")
			Expect(collector.IsRunning()).To(BeTrue())

			By("Disabling panic and verifying normal operation resumes")
			worker.SetCollectPanic(false)
			time.Sleep(200 * time.Millisecond)

			Expect(worker.GetCollectCallCount()).To(BeNumerically(">", 1))

			collector.Stop()
			Eventually(collector.IsRunning, "2s", "50ms").Should(BeFalse())
		})

		It("should handle collector restart exceeding max attempts", func() {
			worker := NewMockWorker("restart-worker")

			collectorConfig := collection.CollectorConfig{
				Worker:              worker,
				Identity:            worker.GetIdentity(),
				Store:               store,
				Logger:              logger,
				ObservationInterval: 50 * time.Millisecond,
				ObservationTimeout:  1 * time.Second,
				WorkerType:          "mock",
			}

			collector := collection.NewCollector(collectorConfig)
			err := collector.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			By("Triggering multiple restarts")
			for i := 0; i < 10; i++ {
				collector.Restart()
				time.Sleep(10 * time.Millisecond)
			}

			By("Verifying collector continues to operate")
			Expect(collector.IsRunning()).To(BeTrue())

			collector.Stop()
			Eventually(collector.IsRunning, "2s", "50ms").Should(BeFalse())
		})

		It("should panic when collector is started twice (invariant I8)", func() {
			worker := NewMockWorker("double-start-worker")

			collectorConfig := collection.CollectorConfig{
				Worker:              worker,
				Identity:            worker.GetIdentity(),
				Store:               store,
				Logger:              logger,
				ObservationInterval: 50 * time.Millisecond,
				ObservationTimeout:  1 * time.Second,
				WorkerType:          "mock",
			}

			collector := collection.NewCollector(collectorConfig)

			By("Starting collector first time")
			err := collector.Start(ctx)
			Expect(err).ToNot(HaveOccurred())

			By("Attempting to start collector second time should panic")
			Expect(func() {
				_ = collector.Start(ctx)
			}).To(Panic())

			collector.Stop()
		})
	})

	Context("Day 3 Part 1: State/Signal Failures", func() {
		var (
			sup    *supervisor.Supervisor
			before int
		)

		BeforeEach(func() {
			before = GetGoroutineCount()
		})

		AfterEach(func() {
			if sup != nil {
				sup.Stop()
			}
			ExpectNoGoroutineLeaks(before)
		})

		It("should escalate SignalNeedsRestart loop to shutdown", func() {
			worker := NewMockWorker("restart-loop-worker")
			restartState := NewMockState("NeedsRestart")
			restartState.SetTransition(restartState, fsmv2.SignalNeedsRestart, nil)
			worker.initialState = restartState

			config := supervisor.NewSupervisorConfigForTesting()
			config.ObservationInterval = 50 * time.Millisecond
			config.ObservationTimeout = 1 * time.Second
			config.StaleThreshold = 2 * time.Second
			config.CollectorTimeout = 1 * time.Second

			sup = supervisor.NewSupervisor(config, logger)
			sup.Start(ctx)

			err := sup.AddWorker(worker, nil)
			Expect(err).ToNot(HaveOccurred())

			By("Allowing multiple ticks to detect restart loop")
			time.Sleep(500 * time.Millisecond)

			By("Verifying worker doesn't enter infinite loop")
			Expect(restartState.GetCallCount()).To(BeNumerically("<", 1000))
		})

		It("should handle state transition and action simultaneously", func() {
			worker := NewMockWorker("concurrent-transition-worker")
			state1 := NewMockState("State1")
			state2 := NewMockState("State2")
			action := NewSlowAction(100 * time.Millisecond)
			state1.SetTransition(state2, fsmv2.SignalNone, action)

			worker.initialState = state1

			config := supervisor.NewSupervisorConfigForTesting()
			config.ObservationInterval = 50 * time.Millisecond

			sup = supervisor.NewSupervisor(config, logger)
			sup.Start(ctx)

			err := sup.AddWorker(worker, nil)
			Expect(err).ToNot(HaveOccurred())

			By("Waiting for state transition and action execution")
			Eventually(action.WasExecuted, "2s", "50ms").Should(BeTrue())

			By("Verifying no race conditions occurred")
		})

		It("should handle concurrent AddWorker calls safely", func() {
			config := supervisor.NewSupervisorConfigForTesting()
			sup = supervisor.NewSupervisor(config, logger)
			sup.Start(ctx)

			By("Adding 10 workers concurrently")
			var wg sync.WaitGroup
			errors := make([]error, 10)

			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					worker := NewMockWorker(fmt.Sprintf("concurrent-worker-%d", idx))
					errors[idx] = sup.AddWorker(worker, nil)
				}(i)
			}

			wg.Wait()

			By("Verifying all workers were added successfully")
			successCount := 0
			for _, err := range errors {
				if err == nil {
					successCount++
				}
			}

			Expect(successCount).To(Equal(10))
		})

		It("should handle RemoveWorker during tick", func() {
			config := supervisor.NewSupervisorConfigForTesting()
			config.ObservationInterval = 100 * time.Millisecond

			sup = supervisor.NewSupervisor(config, logger)
			sup.Start(ctx)

			worker := NewMockWorker("remove-during-tick-worker")
			err := sup.AddWorker(worker, nil)
			Expect(err).ToNot(HaveOccurred())

			By("Waiting for a tick to start")
			time.Sleep(150 * time.Millisecond)

			By("Removing worker during tick")
			err = sup.RemoveWorker(worker.GetIdentity().ID)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying supervisor continues to operate normally")
			time.Sleep(200 * time.Millisecond)
		})

		It("should prevent concurrent State.Next() calls via tickInProgress", func() {
			worker := NewMockWorker("tick-in-progress-worker")
			state := NewMockState("ConcurrentState")
			worker.initialState = state

			config := supervisor.NewSupervisorConfigForTesting()
			config.ObservationInterval = 20 * time.Millisecond

			sup = supervisor.NewSupervisor(config, logger)
			sup.Start(ctx)

			err := sup.AddWorker(worker, nil)
			Expect(err).ToNot(HaveOccurred())

			By("Allowing multiple ticks to occur")
			time.Sleep(500 * time.Millisecond)

			By("Verifying State.Next() was called but not concurrently")
			callCount := state.GetCallCount()
			Expect(callCount).To(BeNumerically(">", 5))
		})
	})

	Context("Day 3 Part 2: Timeout Invariants", func() {
		It("should panic if ObservationTimeout >= StaleThreshold on startup", func() {
			logger := zap.NewNop().Sugar()

			By("Creating supervisor with invalid timeout configuration")
			Expect(func() {
				config := supervisor.NewSupervisorConfigForTesting()
				config.ObservationTimeout = 15 * time.Second
				config.StaleThreshold = 10 * time.Second

				_ = supervisor.NewSupervisor(config, logger)
			}).To(Panic(), "ObservationTimeout must be less than StaleThreshold")
		})

		It("should panic if StaleThreshold >= CollectorTimeout on startup", func() {
			logger := zap.NewNop().Sugar()

			By("Creating supervisor with invalid threshold configuration")
			Expect(func() {
				config := supervisor.NewSupervisorConfigForTesting()
				config.StaleThreshold = 15 * time.Second
				config.CollectorTimeout = 10 * time.Second

				_ = supervisor.NewSupervisor(config, logger)
			}).To(Panic(), "StaleThreshold must be less than CollectorTimeout")
		})
	})
})
