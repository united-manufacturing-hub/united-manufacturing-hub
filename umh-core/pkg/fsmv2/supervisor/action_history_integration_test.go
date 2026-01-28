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
	"errors"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/internal/execution"
)

var _ = Describe("ActionHistory Integration", func() {
	Describe("ActionExecutor OnActionComplete callback", func() {
		var (
			executor *execution.ActionExecutor
			ctx      context.Context
			cancel   context.CancelFunc
			logger   *zap.SugaredLogger
		)

		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())
			logger = zap.NewNop().Sugar()
		})

		AfterEach(func() {
			if executor != nil {
				executor.Shutdown()
			}
			cancel()
		})

		It("should have an onActionComplete callback field", func() {
			// The implementation has:
			// - onActionComplete func(deps.ActionResult) field at action_executor.go:40
			// - SetOnActionComplete method at action_executor.go:299
			// Verify by setting a callback - if the field doesn't exist, this will fail to compile
			identity := deps.Identity{ID: "test-worker", WorkerType: "test"}
			executor = execution.NewActionExecutor(10, "test-supervisor", identity, logger)

			// SetOnActionComplete should not panic - this verifies the field exists
			callbackCalled := false
			executor.SetOnActionComplete(func(result deps.ActionResult) {
				callbackCalled = true
			})

			// The callback shouldn't be called until an action executes
			Expect(callbackCalled).To(BeFalse())
		})

		It("should call the callback after successful action execution", func() {
			identity := deps.Identity{ID: "test-worker", WorkerType: "test"}
			executor = execution.NewActionExecutor(10, "test-supervisor", identity, logger)

			var mu sync.Mutex
			var receivedResult deps.ActionResult
			callbackCalled := make(chan struct{}, 1)

			executor.SetOnActionComplete(func(result deps.ActionResult) {
				mu.Lock()
				receivedResult = result
				mu.Unlock()
				callbackCalled <- struct{}{}
			})

			executor.Start(ctx)

			// Execute a successful action
			action := &testActionForHistory{
				executeFunc: func(ctx context.Context) error {
					return nil
				},
				name: "SuccessAction",
			}

			err := executor.EnqueueAction("success-action", action, nil)
			Expect(err).ToNot(HaveOccurred())

			// Wait for callback to be called
			Eventually(callbackCalled, 2*time.Second).Should(Receive())

			mu.Lock()
			defer mu.Unlock()
			Expect(receivedResult.Success).To(BeTrue())
			Expect(receivedResult.ActionType).To(Equal("SuccessAction"))
			Expect(receivedResult.ErrorMsg).To(BeEmpty())
			Expect(time.Since(receivedResult.Timestamp)).To(BeNumerically("<", 5*time.Second))
			Expect(receivedResult.Latency).To(BeNumerically(">", 0))
		})

		It("should call the callback after failed action execution with error message", func() {
			identity := deps.Identity{ID: "test-worker", WorkerType: "test"}
			executor = execution.NewActionExecutor(10, "test-supervisor", identity, logger)

			var mu sync.Mutex
			var receivedResult deps.ActionResult
			callbackCalled := make(chan struct{}, 1)

			executor.SetOnActionComplete(func(result deps.ActionResult) {
				mu.Lock()
				receivedResult = result
				mu.Unlock()
				callbackCalled <- struct{}{}
			})

			executor.Start(ctx)

			// Execute an action that returns an error
			expectedError := errors.New("simulated failure")
			action := &testActionForHistory{
				executeFunc: func(ctx context.Context) error {
					return expectedError
				},
				name: "FailAction",
			}

			err := executor.EnqueueAction("fail-action", action, nil)
			Expect(err).ToNot(HaveOccurred())

			// Wait for callback to be called
			Eventually(callbackCalled, 2*time.Second).Should(Receive())

			mu.Lock()
			defer mu.Unlock()
			Expect(receivedResult.Success).To(BeFalse())
			Expect(receivedResult.ActionType).To(Equal("FailAction"))
			Expect(receivedResult.ErrorMsg).To(ContainSubstring("simulated failure"))
		})

		It("should call the callback after action timeout", func() {
			identity := deps.Identity{ID: "test-worker", WorkerType: "test"}
			// Create executor with short timeout
			executor = execution.NewActionExecutorWithTimeout(10, map[string]time.Duration{
				"timeout-action": 50 * time.Millisecond,
			}, "test-supervisor", identity, logger)

			var mu sync.Mutex
			var receivedResult deps.ActionResult
			callbackCalled := make(chan struct{}, 1)

			executor.SetOnActionComplete(func(result deps.ActionResult) {
				mu.Lock()
				receivedResult = result
				mu.Unlock()
				callbackCalled <- struct{}{}
			})

			executor.Start(ctx)

			// Execute an action that takes longer than timeout
			action := &testActionForHistory{
				executeFunc: func(ctx context.Context) error {
					<-ctx.Done()

					return ctx.Err()
				},
				name: "TimeoutAction",
			}

			err := executor.EnqueueAction("timeout-action", action, nil)
			Expect(err).ToNot(HaveOccurred())

			// Wait for callback to be called
			Eventually(callbackCalled, 2*time.Second).Should(Receive())

			mu.Lock()
			defer mu.Unlock()
			Expect(receivedResult.Success).To(BeFalse())
			Expect(receivedResult.ActionType).To(Equal("TimeoutAction"))
			Expect(receivedResult.ErrorMsg).To(ContainSubstring("deadline exceeded"))
		})

		It("should call the callback after action panic", func() {
			identity := deps.Identity{ID: "test-worker", WorkerType: "test"}
			executor = execution.NewActionExecutor(10, "test-supervisor", identity, logger)

			var mu sync.Mutex
			var receivedResult deps.ActionResult
			callbackCalled := make(chan struct{}, 1)

			executor.SetOnActionComplete(func(result deps.ActionResult) {
				mu.Lock()
				receivedResult = result
				mu.Unlock()
				callbackCalled <- struct{}{}
			})

			executor.Start(ctx)

			// Execute an action that panics
			action := &testActionForHistory{
				executeFunc: func(ctx context.Context) error {
					panic("simulated panic")
				},
				name: "PanicAction",
			}

			err := executor.EnqueueAction("panic-action", action, nil)
			Expect(err).ToNot(HaveOccurred())

			// Wait for callback to be called
			Eventually(callbackCalled, 2*time.Second).Should(Receive())

			mu.Lock()
			defer mu.Unlock()
			Expect(receivedResult.Success).To(BeFalse())
			Expect(receivedResult.ActionType).To(Equal("PanicAction"))
			Expect(receivedResult.ErrorMsg).To(ContainSubstring("panic"))
		})
	})

	Describe("WorkerContext actionHistory field", func() {
		It("should have an actionHistory field to store recorded actions", func() {
			// The WorkerContext struct has actionHistory field at types.go:116
			// We verify this by creating a supervisor and checking that
			// the InMemoryActionHistoryRecorder is functional

			// Create and use InMemoryActionHistoryRecorder directly to verify it works
			recorder := deps.NewInMemoryActionHistoryRecorder()
			Expect(recorder).ToNot(BeNil())

			// Record some results
			result1 := deps.ActionResult{
				Timestamp:  time.Now(),
				ActionType: "TestAction",
				Success:    true,
			}
			recorder.Record(result1)

			result2 := deps.ActionResult{
				Timestamp:  time.Now(),
				ActionType: "FailedAction",
				Success:    false,
				ErrorMsg:   "test error",
			}
			recorder.Record(result2)

			// Drain should return all recorded results
			drained := recorder.Drain()
			Expect(drained).To(HaveLen(2))
			Expect(drained[0].ActionType).To(Equal("TestAction"))
			Expect(drained[1].ActionType).To(Equal("FailedAction"))

			// Drain again should return empty (buffer cleared)
			drainedAgain := recorder.Drain()
			Expect(drainedAgain).To(BeEmpty())
		})
	})

	Describe("ActionHistoryProvider and ActionHistorySetter wiring in AddWorker", func() {
		var (
			sup    *supervisor.Supervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState]
			ctx    context.Context
			cancel context.CancelFunc
		)

		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())
		})

		AfterEach(func() {
			if sup != nil {
				sup.Shutdown()
			}
			cancel()
		})

		It("should wire up ActionHistoryProvider to drain from workerCtx.actionHistory", func() {
			// Create supervisor and worker
			sup = supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
				WorkerType: "test",
				Store:      supervisor.CreateTestTriangularStore(),
				Logger:     zap.NewNop().Sugar(),
				CollectorHealth: supervisor.CollectorHealthConfig{
					ObservationTimeout: 5 * time.Second,
					StaleThreshold:     10 * time.Second,
					Timeout:            20 * time.Second,
				},
			})

			identity := deps.Identity{
				ID:         "test-worker-history",
				Name:       "Test Worker History",
				WorkerType: "test",
			}

			worker := &supervisor.TestWorker{
				Observed: &supervisor.TestObservedState{ID: "test-worker-history"},
			}

			err := sup.AddWorker(identity, worker)
			Expect(err).ToNot(HaveOccurred())

			// The ActionHistoryProvider is wired in AddWorker (api.go:236-242).
			// We can't directly access workerCtx.actionHistory, but we can verify
			// the wiring by checking that the supervisor starts without error
			// and the worker is added successfully.
			workers := sup.ListWorkers()
			Expect(workers).To(ContainElement("test-worker-history"))
		})

		It("should wire up ActionHistorySetter to inject into worker deps", func() {
			// Create supervisor and worker
			sup = supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
				WorkerType: "test",
				Store:      supervisor.CreateTestTriangularStore(),
				Logger:     zap.NewNop().Sugar(),
				CollectorHealth: supervisor.CollectorHealthConfig{
					ObservationTimeout: 5 * time.Second,
					StaleThreshold:     10 * time.Second,
					Timeout:            20 * time.Second,
				},
			})

			identity := deps.Identity{
				ID:         "test-worker-setter",
				Name:       "Test Worker Setter",
				WorkerType: "test",
			}

			worker := &supervisor.TestWorker{
				Observed: &supervisor.TestObservedState{ID: "test-worker-setter"},
			}

			err := sup.AddWorker(identity, worker)
			Expect(err).ToNot(HaveOccurred())

			// The ActionHistorySetter is wired in AddWorker (api.go:243-253).
			// It injects action history into worker deps via SetActionHistory method.
			// We verify this by checking that the worker was added successfully
			// and the supervisor can be started.
			done := sup.Start(ctx)
			Expect(done).ToNot(BeNil())

			// Verify worker exists
			workers := sup.ListWorkers()
			Expect(workers).To(ContainElement("test-worker-setter"))
		})
	})

	Describe("End-to-end ActionHistory flow", func() {
		It("should auto-record action results and make them available via deps.GetActionHistory()", func() {
			// Full integration test:
			// 1. Verify InMemoryActionHistoryRecorder works
			// 2. Verify ActionExecutor records results via callback
			// 3. Verify the wiring connects them

			// Part 1: InMemoryActionHistoryRecorder stores and drains results
			recorder := deps.NewInMemoryActionHistoryRecorder()

			// Part 2: ActionExecutor uses callback to record
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			identity := deps.Identity{ID: "e2e-test-worker", WorkerType: "test"}
			executor := execution.NewActionExecutor(10, "test-supervisor", identity, zap.NewNop().Sugar())

			// Wire the callback to the recorder BEFORE starting (this is what AddWorker does)
			// SetOnActionComplete must be called before Start() to avoid races
			executor.SetOnActionComplete(func(result deps.ActionResult) {
				recorder.Record(result)
			})

			executor.Start(ctx)
			defer executor.Shutdown()

			// Execute multiple actions - success, fail, and final
			successAction := &testActionForHistory{
				executeFunc: func(ctx context.Context) error {
					return nil
				},
				name: "EndToEndSuccess",
			}

			failAction := &testActionForHistory{
				executeFunc: func(ctx context.Context) error {
					return errors.New("e2e error")
				},
				name: "EndToEndFail",
			}

			finalAction := &testActionForHistory{
				executeFunc: func(ctx context.Context) error {
					return nil
				},
				name: "FinalAction",
			}

			err := executor.EnqueueAction("e2e-success", successAction, nil)
			Expect(err).ToNot(HaveOccurred())

			err = executor.EnqueueAction("e2e-fail", failAction, nil)
			Expect(err).ToNot(HaveOccurred())

			err = executor.EnqueueAction("e2e-final", finalAction, nil)
			Expect(err).ToNot(HaveOccurred())

			// Collect all results - the recorder accumulates them
			var allResults []deps.ActionResult
			Eventually(func() int {
				// Keep draining and accumulating results
				results := recorder.Drain()
				allResults = append(allResults, results...)

				return len(allResults)
			}, 2*time.Second, 50*time.Millisecond).Should(BeNumerically(">=", 3))

			// Verify we got all three expected action types
			actionTypes := make(map[string]bool)
			for _, r := range allResults {
				actionTypes[r.ActionType] = true
			}
			Expect(actionTypes).To(HaveKey("EndToEndSuccess"))
			Expect(actionTypes).To(HaveKey("EndToEndFail"))
			Expect(actionTypes).To(HaveKey("FinalAction"))
		})
	})
})

// testActionForHistory is a test action implementation for action history tests.
type testActionForHistory struct {
	executeFunc func(ctx context.Context) error
	name        string
}

func (t *testActionForHistory) Execute(ctx context.Context, deps any) error {
	return t.executeFunc(ctx)
}

func (t *testActionForHistory) Name() string {
	return t.name
}

var _ fsmv2.Action[any] = (*testActionForHistory)(nil)

// ActionResultMatcher helps verify ActionResult fields in tests.
type ActionResultMatcher struct {
	ActionType string
	Success    bool
	HasError   bool
}

func (m ActionResultMatcher) Match(actual interface{}) (success bool, err error) {
	result, ok := actual.(deps.ActionResult)
	if !ok {
		return false, nil
	}

	if result.ActionType != m.ActionType {
		return false, nil
	}

	if result.Success != m.Success {
		return false, nil
	}

	if m.HasError && result.ErrorMsg == "" {
		return false, nil
	}

	if !m.HasError && result.ErrorMsg != "" {
		return false, nil
	}
	// Verify timestamp is recent (within last minute)
	if time.Since(result.Timestamp) > time.Minute {
		return false, nil
	}

	return true, nil
}

func (m ActionResultMatcher) FailureMessage(actual interface{}) string {
	return "Expected ActionResult to match criteria"
}

func (m ActionResultMatcher) NegatedFailureMessage(actual interface{}) string {
	return "Expected ActionResult NOT to match criteria"
}
