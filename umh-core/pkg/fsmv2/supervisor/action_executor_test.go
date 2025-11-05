package supervisor_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

var _ = Describe("ActionExecutor", func() {
	var (
		executor *supervisor.ActionExecutor
		ctx      context.Context
		cancel   context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		executor = supervisor.NewActionExecutor(10)
		executor.Start(ctx)
	})

	AfterEach(func() {
		cancel()
		executor.Shutdown()
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

			err := executor.EnqueueAction(actionID, action)
			Expect(err).ToNot(HaveOccurred())

			Eventually(executed).Should(Receive())
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
			for i := 0; i < totalCapacity+2; i++ {
				err := executor.EnqueueAction(fmt.Sprintf("action-%d", i), blockingAction)
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

			err := executor.EnqueueAction(actionID, action)
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

			err := executor.EnqueueAction(actionID, action)
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

			for i := 0; i < actionCount; i++ {
				err := executor.EnqueueAction(fmt.Sprintf("action-%d", i), action)
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

			err := executor.EnqueueAction(actionID, action)
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

			err := executor.EnqueueAction(actionID, action)
			Expect(err).ToNot(HaveOccurred())

			go executor.Shutdown()

			Eventually(completed).Should(Receive())
		})
	})

	Describe("Action Timeout Handling", func() {
		Context("when action times out", func() {
			It("should cancel action after configured timeout", func() {
				executor := supervisor.NewActionExecutorWithTimeout(10, map[string]time.Duration{
					"test-action": 100 * time.Millisecond,
				})
				executor.Start(ctx)

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

				err := executor.EnqueueAction("test-action", action)
				Expect(err).ToNot(HaveOccurred())

				Eventually(actionStarted).Should(Receive())
				Eventually(actionCancelled, 200*time.Millisecond).Should(Receive())
			})

			It("should clear in-progress status after timeout", func() {
				executor := supervisor.NewActionExecutorWithTimeout(10, map[string]time.Duration{
					"timeout-action": 50 * time.Millisecond,
				})
				executor.Start(ctx)

				action := &testAction{
					execute: func(ctx context.Context) error {
						<-ctx.Done()
						return ctx.Err()
					},
				}

				err := executor.EnqueueAction("timeout-action", action)
				Expect(err).ToNot(HaveOccurred())

				Expect(executor.HasActionInProgress("timeout-action")).To(BeTrue())

				Eventually(func() bool {
					return executor.HasActionInProgress("timeout-action")
				}, 200*time.Millisecond).Should(BeFalse())
			})
		})

		Context("when action completes before timeout", func() {
			It("should allow action to complete successfully", func() {
				executor := supervisor.NewActionExecutorWithTimeout(10, map[string]time.Duration{
					"fast-action": 1 * time.Second,
				})
				executor.Start(ctx)

				completed := make(chan bool, 1)
				action := &testAction{
					execute: func(ctx context.Context) error {
						time.Sleep(50 * time.Millisecond)
						completed <- true
						return nil
					},
				}

				err := executor.EnqueueAction("fast-action", action)
				Expect(err).ToNot(HaveOccurred())

				Eventually(completed).Should(Receive())
			})
		})

		Context("when action type is not configured", func() {
			It("should use default timeout for unconfigured action types", func() {
				executor := supervisor.NewActionExecutorWithTimeout(10, map[string]time.Duration{
					"configured": 1 * time.Second,
				})
				executor.Start(ctx)

				cancelled := make(chan bool, 1)
				action := &testAction{
					execute: func(ctx context.Context) error {
						<-ctx.Done()
						cancelled <- true
						return ctx.Err()
					},
				}

				err := executor.EnqueueAction("unconfigured-action", action)
				Expect(err).ToNot(HaveOccurred())

				Eventually(cancelled, 35*time.Second).Should(Receive())
			})
		})
	})

	Describe("Non-Blocking Guarantees", func() {
		Context("EnqueueAction non-blocking behavior", func() {
			It("should never block when enqueueing action (even when queue full)", func() {
				smallExecutor := supervisor.NewActionExecutor(2)
				smallExecutor.Start(ctx)
				defer smallExecutor.Shutdown()

				blockingAction := &testAction{
					execute: func(ctx context.Context) error {
						time.Sleep(1 * time.Second)
						return nil
					},
				}

				for i := 0; i < 100; i++ {
					start := time.Now()
					_ = smallExecutor.EnqueueAction(fmt.Sprintf("action-%d", i), blockingAction)
					duration := time.Since(start)

					Expect(duration).To(BeNumerically("<", 1*time.Millisecond),
						fmt.Sprintf("EnqueueAction took %v, expected <1ms (non-blocking)", duration))
				}
			})

			It("should handle 100+ concurrent actions without blocking enqueue", func() {
				executor := supervisor.NewActionExecutor(50)
				executor.Start(ctx)
				defer executor.Shutdown()

				completed := make(chan string, 150)

				for i := 0; i < 150; i++ {
					actionID := fmt.Sprintf("action-%d", i)
					action := &testAction{
						execute: func(ctx context.Context) error {
							time.Sleep(10 * time.Millisecond)
							completed <- actionID
							return nil
						},
					}

					start := time.Now()
					err := executor.EnqueueAction(actionID, action)
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
				executor := supervisor.NewActionExecutor(10)
				executor.Start(ctx)
				defer executor.Shutdown()

				for i := 0; i < 100; i++ {
					_ = executor.EnqueueAction(fmt.Sprintf("action-%d", i), &testAction{
						execute: func(ctx context.Context) error {
							time.Sleep(100 * time.Millisecond)
							return nil
						},
					})
				}

				for i := 0; i < 1000; i++ {
					start := time.Now()
					_ = executor.HasActionInProgress("action-0")
					duration := time.Since(start)

					Expect(duration).To(BeNumerically("<", 1*time.Millisecond),
						fmt.Sprintf("HasActionInProgress took %v, expected <1ms (non-blocking read)", duration))
				}
			})
		})
	})
})

type testAction struct {
	execute func(ctx context.Context) error
	name    string
}

func (t *testAction) Execute(ctx context.Context) error {
	return t.execute(ctx)
}

func (t *testAction) Name() string {
	if t.name == "" {
		return "test-action"
	}
	return t.name
}

var _ fsmv2.Action = (*testAction)(nil)
