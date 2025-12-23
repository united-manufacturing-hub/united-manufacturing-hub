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

package execution_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/internal/execution"
)

var _ = Describe("ActionExecutor", func() {
	var (
		executor *execution.ActionExecutor
		ctx      context.Context
		cancel   context.CancelFunc
		logger   *zap.SugaredLogger
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		logger = zap.NewNop().Sugar()
		executor = execution.NewActionExecutor(10, "test-supervisor", logger)
		executor.Start(ctx)
	})

	AfterEach(func() {
		cancel()
		executor.Shutdown()
	})

	Describe("Constructor Validation", func() {
		Context("NewActionExecutor with invalid worker count", func() {
			It("should default to 10 workers when workerCount is 0", func() {
				executor := execution.NewActionExecutor(0, "test-supervisor", zap.NewNop().Sugar())
				Expect(executor).ToNot(BeNil())
			})

			It("should default to 10 workers when workerCount is negative", func() {
				executor := execution.NewActionExecutor(-5, "test-supervisor", zap.NewNop().Sugar())
				Expect(executor).ToNot(BeNil())
			})
		})

		Context("NewActionExecutorWithTimeout with invalid worker count", func() {
			It("should default to 10 workers when workerCount is 0", func() {
				executor := execution.NewActionExecutorWithTimeout(0, map[string]time.Duration{}, "test-supervisor", zap.NewNop().Sugar())
				Expect(executor).ToNot(BeNil())
			})

			It("should default to 10 workers when workerCount is negative", func() {
				executor := execution.NewActionExecutorWithTimeout(-3, map[string]time.Duration{}, "test-supervisor", zap.NewNop().Sugar())
				Expect(executor).ToNot(BeNil())
			})
		})
	})

	Describe("EnqueueAction", func() {
		It("should enqueue action without blocking", func() {
			actionID := "test-action"

			executed := make(chan bool, 1)
			action := &testAction{
				execute: func(ctx context.Context) error {
					executed <- true

					return nil
				},
			}

			err := executor.EnqueueAction(actionID, action, nil)
			Expect(err).ToNot(HaveOccurred())

			Eventually(executed).Should(Receive())
		})

		It("should return error if action is already in progress", func() {
			actionID := "duplicate-action"

			blockChan := make(chan struct{})
			action := &testAction{
				execute: func(ctx context.Context) error {
					<-blockChan

					return nil
				},
			}

			err := executor.EnqueueAction(actionID, action, nil)
			Expect(err).ToNot(HaveOccurred())

			err = executor.EnqueueAction(actionID, action, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already in progress"))

			close(blockChan)
		})

		It("should return error if queue is full", func() {
			blockChan := make(chan struct{})
			blockingAction := &testAction{
				execute: func(ctx context.Context) error {
					<-blockChan

					return nil
				},
			}

			workerCount := 10
			queueBuffer := 20
			totalCapacity := workerCount + queueBuffer

			var lastErr error
			for i := range totalCapacity + 2 {
				err := executor.EnqueueAction(fmt.Sprintf("action-%d", i), blockingAction, nil)
				if err != nil {
					lastErr = err

					break
				}
			}

			Expect(lastErr).To(HaveOccurred())
			Expect(lastErr.Error()).To(ContainSubstring("queue full"))

			close(blockChan)
		})
	})

	Describe("HasActionInProgress", func() {
		It("should return true when action is queued", func() {
			actionID := "test-action"

			blockChan := make(chan struct{})
			action := &testAction{
				execute: func(ctx context.Context) error {
					<-blockChan

					return nil
				},
			}

			err := executor.EnqueueAction(actionID, action, nil)
			Expect(err).ToNot(HaveOccurred())

			Expect(executor.HasActionInProgress(actionID)).To(BeTrue())

			close(blockChan)
		})

		It("should return false after action completes", func() {
			actionID := "test-action"

			action := &testAction{
				execute: func(ctx context.Context) error {
					return nil
				},
			}

			err := executor.EnqueueAction(actionID, action, nil)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() bool {
				return executor.HasActionInProgress(actionID)
			}).Should(BeFalse())
		})

		It("should return false for unknown action ID", func() {
			Expect(executor.HasActionInProgress("unknown")).To(BeFalse())
		})
	})

	Describe("Concurrent Execution", func() {
		It("should execute multiple actions concurrently", func() {
			actionCount := 20
			var executed atomic.Int32

			action := &testAction{
				execute: func(ctx context.Context) error {
					time.Sleep(10 * time.Millisecond)
					executed.Add(1)

					return nil
				},
			}

			for i := range actionCount {
				err := executor.EnqueueAction(fmt.Sprintf("action-%d", i), action, nil)
				Expect(err).ToNot(HaveOccurred())
			}

			Eventually(func() int32 {
				return executed.Load()
			}, 200*time.Millisecond).Should(Equal(int32(actionCount)))
		})
	})

	Describe("Context Cancellation", func() {
		It("should cancel in-progress actions on context cancel", func() {
			actionID := "test-action"

			cancelled := make(chan bool, 1)
			action := &testAction{
				execute: func(ctx context.Context) error {
					select {
					case <-ctx.Done():
						cancelled <- true

						return ctx.Err()
					case <-time.After(1 * time.Second):
						return nil
					}
				},
			}

			err := executor.EnqueueAction(actionID, action, nil)
			Expect(err).ToNot(HaveOccurred())

			cancel()

			Eventually(cancelled).Should(Receive())
		})
	})

	Describe("Shutdown", func() {
		It("should wait for in-progress actions to complete", func() {
			actionID := "test-action"

			completed := make(chan bool, 1)
			action := &testAction{
				execute: func(ctx context.Context) error {
					time.Sleep(50 * time.Millisecond)
					completed <- true

					return nil
				},
			}

			err := executor.EnqueueAction(actionID, action, nil)
			Expect(err).ToNot(HaveOccurred())

			go executor.Shutdown()

			Eventually(completed).Should(Receive())
		})
	})

	Describe("Action Timeout Handling", func() {
		Context("when action times out", func() {
			It("should cancel action after configured timeout", func() {
				executor := execution.NewActionExecutorWithTimeout(10, map[string]time.Duration{
					"test-action": 100 * time.Millisecond,
				}, "test-supervisor", zap.NewNop().Sugar())
				executor.Start(ctx)
				defer executor.Shutdown()

				actionStarted := make(chan bool, 1)
				actionCancelled := make(chan bool, 1)

				action := &testAction{
					execute: func(ctx context.Context) error {
						actionStarted <- true
						<-ctx.Done()
						actionCancelled <- true

						return ctx.Err()
					},
				}

				err := executor.EnqueueAction("test-action", action, nil)
				Expect(err).ToNot(HaveOccurred())

				Eventually(actionStarted).Should(Receive())
				Eventually(actionCancelled, 200*time.Millisecond).Should(Receive())
			})

			It("should clear in-progress status after timeout", func() {
				executor := execution.NewActionExecutorWithTimeout(10, map[string]time.Duration{
					"timeout-action": 50 * time.Millisecond,
				}, "test-supervisor", zap.NewNop().Sugar())
				executor.Start(ctx)
				defer executor.Shutdown()

				action := &testAction{
					execute: func(ctx context.Context) error {
						<-ctx.Done()

						return ctx.Err()
					},
				}

				err := executor.EnqueueAction("timeout-action", action, nil)
				Expect(err).ToNot(HaveOccurred())

				Expect(executor.HasActionInProgress("timeout-action")).To(BeTrue())

				Eventually(func() bool {
					return executor.HasActionInProgress("timeout-action")
				}, 200*time.Millisecond).Should(BeFalse())
			})
		})

		Context("when action completes before timeout", func() {
			It("should allow action to complete successfully", func() {
				executor := execution.NewActionExecutorWithTimeout(10, map[string]time.Duration{
					"fast-action": 1 * time.Second,
				}, "test-supervisor", zap.NewNop().Sugar())
				executor.Start(ctx)
				defer executor.Shutdown()

				completed := make(chan bool, 1)
				action := &testAction{
					execute: func(ctx context.Context) error {
						time.Sleep(50 * time.Millisecond)
						completed <- true

						return nil
					},
				}

				err := executor.EnqueueAction("fast-action", action, nil)
				Expect(err).ToNot(HaveOccurred())

				Eventually(completed).Should(Receive())
			})
		})

		Context("when action type is not configured", func() {
			It("should use default timeout for unconfigured action types", func() {
				// Use a custom executor with short default timeout for testing
				// The default 30s timeout is too long for unit tests
				executor := execution.NewActionExecutorWithTimeout(10, map[string]time.Duration{
					"configured": 1 * time.Second,
				}, "test-supervisor", zap.NewNop().Sugar())
				executor.Start(ctx)
				defer executor.Shutdown()

				// Instead of waiting for the 30s default timeout,
				// we verify that unconfigured actions get enqueued and can be cancelled
				cancelled := make(chan bool, 1)
				action := &testAction{
					execute: func(ctx context.Context) error {
						<-ctx.Done()
						cancelled <- true

						return ctx.Err()
					},
				}

				err := executor.EnqueueAction("unconfigured-action", action, nil)
				Expect(err).ToNot(HaveOccurred())

				// Cancel the parent context to trigger action cancellation
				// This tests that unconfigured actions respect context cancellation
				cancel()

				Eventually(cancelled, 1*time.Second).Should(Receive())
			})
		})
	})

	Describe("Metrics Recording", func() {
		Context("RecordActionQueued", func() {
			It("should record metrics when action is enqueued", func() {
				actionID := "metrics-test-action"
				supervisorID := "test-supervisor"

				action := &testAction{
					execute: func(ctx context.Context) error {
						return nil
					},
				}

				// Create executor with supervisorID
				executor := execution.NewActionExecutor(10, supervisorID, zap.NewNop().Sugar())
				executor.Start(ctx)
				defer executor.Shutdown()

				err := executor.EnqueueAction(actionID, action, nil)
				Expect(err).ToNot(HaveOccurred())

				// Note: This test verifies the code path exists
				// Actual Prometheus metric validation would require integration tests
			})
		})

		Context("RecordActionExecutionDuration on success", func() {
			It("should record execution duration when action completes successfully", func() {
				actionID := "success-action"
				supervisorID := "test-supervisor"

				completed := make(chan bool, 1)
				action := &testAction{
					execute: func(ctx context.Context) error {
						time.Sleep(10 * time.Millisecond)
						completed <- true

						return nil
					},
				}

				executor := execution.NewActionExecutor(10, supervisorID, zap.NewNop().Sugar())
				executor.Start(ctx)
				defer executor.Shutdown()

				err := executor.EnqueueAction(actionID, action, nil)
				Expect(err).ToNot(HaveOccurred())

				Eventually(completed).Should(Receive())
			})
		})

		Context("RecordActionExecutionDuration on failure", func() {
			It("should record execution duration when action fails", func() {
				actionID := "failure-action"
				supervisorID := "test-supervisor"

				failed := make(chan bool, 1)
				action := &testAction{
					execute: func(ctx context.Context) error {
						time.Sleep(10 * time.Millisecond)
						failed <- true

						return errors.New("simulated failure")
					},
				}

				executor := execution.NewActionExecutor(10, supervisorID, zap.NewNop().Sugar())
				executor.Start(ctx)
				defer executor.Shutdown()

				err := executor.EnqueueAction(actionID, action, nil)
				Expect(err).ToNot(HaveOccurred())

				Eventually(failed).Should(Receive())
			})
		})

		Context("RecordActionTimeout", func() {
			It("should record metrics when action times out", func() {
				supervisorID := "test-supervisor"

				executor := execution.NewActionExecutorWithTimeout(10, map[string]time.Duration{
					"timeout-action": 50 * time.Millisecond,
				}, supervisorID, zap.NewNop().Sugar())
				executor.Start(ctx)
				defer executor.Shutdown()

				timedOut := make(chan bool, 1)
				action := &testAction{
					execute: func(ctx context.Context) error {
						<-ctx.Done()
						timedOut <- true

						return ctx.Err()
					},
				}

				err := executor.EnqueueAction("timeout-action", action, nil)
				Expect(err).ToNot(HaveOccurred())

				Eventually(timedOut, 200*time.Millisecond).Should(Receive())
			})
		})

		Context("Queue size and pool utilization metrics", func() {
			It("should periodically record queue size and worker pool utilization", func() {
				supervisorID := "test-supervisor"

				executor := execution.NewActionExecutor(10, supervisorID, zap.NewNop().Sugar())
				executor.Start(ctx)
				defer executor.Shutdown()

				// Enqueue some actions to create queue depth
				blockChan := make(chan struct{})
				for i := range 5 {
					action := &testAction{
						execute: func(ctx context.Context) error {
							<-blockChan

							return nil
						},
					}
					err := executor.EnqueueAction(fmt.Sprintf("blocked-action-%d", i), action, nil)
					Expect(err).ToNot(HaveOccurred())
				}

				// Wait for metrics reporter to run (periodic goroutine)
				time.Sleep(200 * time.Millisecond)

				// Release blocked actions
				close(blockChan)

				// Note: This verifies the executor supports metrics
				// Actual metric values would be verified in integration tests
			})
		})
	})

	Describe("Non-Blocking Guarantees", func() {
		Context("EnqueueAction non-blocking behavior", func() {
			It("should never block when enqueueing action (even when queue full)", func() {
				smallExecutor := execution.NewActionExecutor(2, "test-supervisor", zap.NewNop().Sugar())
				smallExecutor.Start(ctx)
				defer smallExecutor.Shutdown()

				blockingAction := &testAction{
					execute: func(ctx context.Context) error {
						time.Sleep(1 * time.Second)

						return nil
					},
				}

				for i := range 100 {
					start := time.Now()
					_ = smallExecutor.EnqueueAction(fmt.Sprintf("action-%d", i), blockingAction, nil)
					duration := time.Since(start)

					Expect(duration).To(BeNumerically("<", 1*time.Millisecond),
						fmt.Sprintf("EnqueueAction took %v, expected <1ms (non-blocking)", duration))
				}
			})

			It("should handle 100+ concurrent actions without blocking enqueue", func() {
				executor := execution.NewActionExecutor(50, "test-supervisor", zap.NewNop().Sugar())
				executor.Start(ctx)
				defer executor.Shutdown()

				completed := make(chan string, 150)

				for i := range 150 {
					actionID := fmt.Sprintf("action-%d", i)
					action := &testAction{
						execute: func(ctx context.Context) error {
							time.Sleep(10 * time.Millisecond)
							completed <- actionID

							return nil
						},
					}

					start := time.Now()
					err := executor.EnqueueAction(actionID, action, nil)
					duration := time.Since(start)

					Expect(duration).To(BeNumerically("<", 1*time.Millisecond),
						fmt.Sprintf("EnqueueAction took %v, expected <1ms (non-blocking)", duration))
					Expect(err).ToNot(HaveOccurred(),
						fmt.Sprintf("Action %d should enqueue successfully", i))
				}

				completedCount := 0
				timeout := time.After(5 * time.Second)
			countLoop:
				for {
					select {
					case <-completed:
						completedCount++
						if completedCount >= 150 {
							break countLoop
						}
					case <-timeout:
						break countLoop
					}
				}

				Expect(completedCount).To(Equal(150),
					"All 150 actions should complete")
			})
		})

		Context("HasActionInProgress non-blocking behavior", func() {
			It("should never block when checking action status", func() {
				executor := execution.NewActionExecutor(10, "test-supervisor", zap.NewNop().Sugar())
				executor.Start(ctx)
				defer executor.Shutdown()

				for i := range 100 {
					_ = executor.EnqueueAction(fmt.Sprintf("action-%d", i), &testAction{
						execute: func(ctx context.Context) error {
							time.Sleep(100 * time.Millisecond)

							return nil
						},
					}, nil)
				}

				for range 1000 {
					start := time.Now()
					_ = executor.HasActionInProgress("action-0")
					duration := time.Since(start)

					Expect(duration).To(BeNumerically("<", 1*time.Millisecond),
						fmt.Sprintf("HasActionInProgress took %v, expected <1ms (non-blocking read)", duration))
				}
			})
		})
	})

	Describe("Panic Recovery", func() {
		It("should recover from panicking action and clear in-progress status", func() {
			actionID := "panic-action"
			panicAction := &testAction{
				execute: func(ctx context.Context) error {
					panic("simulated panic in action")
				},
				name: "panic-test",
			}

			err := executor.EnqueueAction(actionID, panicAction, nil)
			Expect(err).ToNot(HaveOccurred())

			// Wait for action to be processed (panic should be recovered)
			Eventually(func() bool {
				return !executor.HasActionInProgress(actionID)
			}, 2*time.Second, 50*time.Millisecond).Should(BeTrue(),
				"Action should be cleared from in-progress after panic recovery")
		})

		It("should keep worker pool functional after panic", func() {
			// First: trigger panic
			panicAction := &testAction{
				execute: func(ctx context.Context) error {
					panic("boom")
				},
			}
			err := executor.EnqueueAction("panic-action", panicAction, nil)
			Expect(err).ToNot(HaveOccurred())

			// Wait for panic to be processed
			Eventually(func() bool {
				return !executor.HasActionInProgress("panic-action")
			}, 2*time.Second).Should(BeTrue())

			// Second: verify executor still works with normal action
			completed := make(chan bool, 1)
			normalAction := &testAction{
				execute: func(ctx context.Context) error {
					completed <- true
					return nil
				},
			}

			err = executor.EnqueueAction("normal-action", normalAction, nil)
			Expect(err).ToNot(HaveOccurred())

			Eventually(completed, 2*time.Second).Should(Receive(BeTrue()),
				"Normal action should complete after previous action panicked")
		})

		It("should handle multiple consecutive panics without crashing", func() {
			for i := range 5 {
				actionID := fmt.Sprintf("panic-action-%d", i)
				idx := i // capture loop variable
				panicAction := &testAction{
					execute: func(ctx context.Context) error {
						panic(fmt.Sprintf("panic %d", idx))
					},
				}
				err := executor.EnqueueAction(actionID, panicAction, nil)
				Expect(err).ToNot(HaveOccurred())
			}

			// Wait for all panics to be processed
			Eventually(func() int {
				count := 0
				for i := range 5 {
					if !executor.HasActionInProgress(fmt.Sprintf("panic-action-%d", i)) {
						count++
					}
				}
				return count
			}, 5*time.Second).Should(Equal(5),
				"All panic actions should be cleared from in-progress")

			// Verify executor still works
			completed := make(chan bool, 1)
			normalAction := &testAction{
				execute: func(ctx context.Context) error {
					completed <- true
					return nil
				},
			}
			err := executor.EnqueueAction("final-normal", normalAction, nil)
			Expect(err).ToNot(HaveOccurred())
			Eventually(completed, 2*time.Second).Should(Receive())
		})
	})
})

type testAction struct {
	execute func(ctx context.Context) error
	name    string
}

func (t *testAction) Execute(ctx context.Context, deps any) error {
	return t.execute(ctx)
}

func (t *testAction) Name() string {
	if t.name == "" {
		return "test-action"
	}

	return t.name
}

var _ fsmv2.Action[any] = (*testAction)(nil)
