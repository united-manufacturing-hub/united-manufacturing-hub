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
	"strings"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor/internal/execution"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

var _ = Describe("Stuck Action Detection", func() {
	Context("when an action ignores context cancellation and blocks beyond 2x timeout", func() {
		It("should detect and log stuck action", func() {
			observedCore, observedLogs := observer.New(zapcore.DebugLevel)
			logger := deps.NewFSMLogger(zap.New(observedCore).Sugar())

			identity := deps.Identity{ID: "test-worker", WorkerType: "test", HierarchyPath: "test/stuck"}

			executor := execution.NewActionExecutorWithTimeout(2, map[string]time.Duration{
				"stuck-action": 100 * time.Millisecond,
			}, "test-supervisor", identity, logger)
			executor.TestSetMetricsInterval(200 * time.Millisecond)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			executor.Start(ctx)
			defer executor.Shutdown()

			blockChan := make(chan struct{})
			defer close(blockChan)

			action := &testAction{
				name: "stuck-action",
				execute: func(ctx context.Context) error {
					<-blockChan

					return nil
				},
			}

			err := executor.EnqueueAction("stuck-action", action, nil)
			Expect(err).ToNot(HaveOccurred())

			// Wait for stuck detection:
			// - Action timeout: 100ms
			// - 2x threshold: 200ms
			// - Metrics interval: 200ms
			// So by ~400ms the first metrics check after 2x should fire
			time.Sleep(600 * time.Millisecond)

			stuckLogs := filterStuckActionLogs(observedLogs, "stuck_action_detected")
			Expect(stuckLogs).ToNot(BeEmpty(), "Expected stuck_action_detected log entry")

			stuckLog := stuckLogs[0]
			Expect(stuckLog.ContextMap()).To(HaveKey("action_name"))
			Expect(stuckLog.ContextMap()["action_name"]).To(Equal("stuck-action"))
			Expect(stuckLog.ContextMap()).To(HaveKey("elapsed_ms"))
			Expect(stuckLog.ContextMap()).To(HaveKey("timeout_ms"))
		})

		It("should not flag actions that complete within timeout", func() {
			observedCore, observedLogs := observer.New(zapcore.DebugLevel)
			logger := deps.NewFSMLogger(zap.New(observedCore).Sugar())

			identity := deps.Identity{ID: "test-worker", WorkerType: "test", HierarchyPath: "test/normal"}

			executor := execution.NewActionExecutorWithTimeout(2, map[string]time.Duration{
				"fast-action": 1 * time.Second,
			}, "test-supervisor", identity, logger)
			executor.TestSetMetricsInterval(200 * time.Millisecond)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			executor.Start(ctx)
			defer executor.Shutdown()

			action := &testAction{
				name: "fast-action",
				execute: func(ctx context.Context) error {
					time.Sleep(50 * time.Millisecond)

					return nil
				},
			}

			err := executor.EnqueueAction("fast-action", action, nil)
			Expect(err).ToNot(HaveOccurred())

			time.Sleep(500 * time.Millisecond)

			stuckLogs := filterStuckActionLogs(observedLogs, "stuck_action_detected")
			Expect(stuckLogs).To(BeEmpty(), "No stuck action logs expected for fast action")
		})
	})
})

var _ = Describe("Stuck Action Force-Removal", func() {
	It("should force-remove stuck entry after 3x timeout", func() {
		observedCore, observedLogs := observer.New(zapcore.DebugLevel)
		logger := deps.NewFSMLogger(zap.New(observedCore).Sugar())

		identity := deps.Identity{ID: "test-worker", WorkerType: "test", HierarchyPath: "test/force-remove"}

		executor := execution.NewActionExecutorWithTimeout(2, map[string]time.Duration{
			"stuck-force": 100 * time.Millisecond,
		}, "test-supervisor", identity, logger)
		executor.TestSetMetricsInterval(100 * time.Millisecond)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		executor.Start(ctx)
		defer executor.Shutdown()

		blockChan := make(chan struct{})
		defer close(blockChan)

		action := &testAction{
			name: "stuck-force",
			execute: func(ctx context.Context) error {
				<-blockChan

				return nil
			},
		}

		err := executor.EnqueueAction("stuck-force", action, nil)
		Expect(err).ToNot(HaveOccurred())

		Expect(executor.HasActionInProgress("stuck-force")).To(BeTrue())

		// Wait for force-removal: 3x timeout = 300ms, plus metrics interval headroom
		time.Sleep(600 * time.Millisecond)

		Expect(executor.HasActionInProgress("stuck-force")).To(BeFalse(),
			"Stuck entry should be force-removed after 3x timeout")

		removedLogs := filterStuckActionLogs(observedLogs, "stuck_action_force_removed")
		Expect(removedLogs).ToNot(BeEmpty(), "Expected stuck_action_force_removed log entry")
		removedLog := removedLogs[0]
		Expect(removedLog.ContextMap()).To(HaveKey("action_name"))
		Expect(removedLog.ContextMap()).To(HaveKey("elapsed_ms"))
		Expect(removedLog.ContextMap()).To(HaveKey("timeout_ms"))
	})

	It("should allow re-enqueue after force-removal", func() {
		observedCore, _ := observer.New(zapcore.DebugLevel)
		logger := deps.NewFSMLogger(zap.New(observedCore).Sugar())

		identity := deps.Identity{ID: "test-worker", WorkerType: "test", HierarchyPath: "test/re-enqueue"}

		executor := execution.NewActionExecutorWithTimeout(2, map[string]time.Duration{
			"stuck-reenqueue": 100 * time.Millisecond,
		}, "test-supervisor", identity, logger)
		executor.TestSetMetricsInterval(100 * time.Millisecond)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		executor.Start(ctx)
		defer executor.Shutdown()

		blockChan := make(chan struct{})
		defer close(blockChan)

		action := &testAction{
			name: "stuck-reenqueue",
			execute: func(ctx context.Context) error {
				<-blockChan

				return nil
			},
		}

		err := executor.EnqueueAction("stuck-reenqueue", action, nil)
		Expect(err).ToNot(HaveOccurred())

		// Wait for force-removal
		time.Sleep(600 * time.Millisecond)
		Expect(executor.HasActionInProgress("stuck-reenqueue")).To(BeFalse())

		// Re-enqueue should succeed
		completed := make(chan bool, 1)
		normalAction := &testAction{
			name: "stuck-reenqueue",
			execute: func(ctx context.Context) error {
				completed <- true

				return nil
			},
		}
		err = executor.EnqueueAction("stuck-reenqueue", normalAction, nil)
		Expect(err).ToNot(HaveOccurred())

		Eventually(completed, 2*time.Second).Should(Receive())
	})

	It("should not invoke callback when orphaned goroutine finishes after force-removal", func() {
		observedCore, _ := observer.New(zapcore.DebugLevel)
		logger := deps.NewFSMLogger(zap.New(observedCore).Sugar())

		identity := deps.Identity{ID: "test-worker", WorkerType: "test", HierarchyPath: "test/orphan-callback"}

		executor := execution.NewActionExecutorWithTimeout(2, map[string]time.Duration{
			"orphan-cb": 100 * time.Millisecond,
		}, "test-supervisor", identity, logger)
		executor.TestSetMetricsInterval(100 * time.Millisecond)

		var callbackCount int32
		executor.SetOnActionComplete(func(result deps.ActionResult) {
			atomic.AddInt32(&callbackCount, 1)
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		executor.Start(ctx)
		defer executor.Shutdown()

		releaseChan := make(chan struct{})

		action := &testAction{
			name: "orphan-cb",
			execute: func(ctx context.Context) error {
				<-releaseChan

				return nil
			},
		}

		err := executor.EnqueueAction("orphan-cb", action, nil)
		Expect(err).ToNot(HaveOccurred())

		// Wait for force-removal (metricsReporter fires callback for force-removed actions)
		time.Sleep(600 * time.Millisecond)
		Expect(executor.HasActionInProgress("orphan-cb")).To(BeFalse())
		Expect(atomic.LoadInt32(&callbackCount)).To(Equal(int32(1)),
			"Force-removal should invoke callback with failure result")

		// Release the orphaned goroutine
		close(releaseChan)
		time.Sleep(200 * time.Millisecond)

		Expect(atomic.LoadInt32(&callbackCount)).To(Equal(int32(1)),
			"Orphaned goroutine should NOT invoke callback after force-removal")
	})

	It("should not delete re-enqueued entry when orphaned goroutine finishes", func() {
		observedCore, _ := observer.New(zapcore.DebugLevel)
		logger := deps.NewFSMLogger(zap.New(observedCore).Sugar())

		identity := deps.Identity{ID: "test-worker", WorkerType: "test", HierarchyPath: "test/orphan-delete"}

		executor := execution.NewActionExecutorWithTimeout(2, map[string]time.Duration{
			"orphan-del": 100 * time.Millisecond,
		}, "test-supervisor", identity, logger)
		executor.TestSetMetricsInterval(100 * time.Millisecond)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		executor.Start(ctx)
		defer executor.Shutdown()

		releaseChan := make(chan struct{})

		stuckAction := &testAction{
			name: "orphan-del",
			execute: func(ctx context.Context) error {
				<-releaseChan

				return nil
			},
		}

		err := executor.EnqueueAction("orphan-del", stuckAction, nil)
		Expect(err).ToNot(HaveOccurred())

		// Wait for force-removal
		time.Sleep(600 * time.Millisecond)
		Expect(executor.HasActionInProgress("orphan-del")).To(BeFalse())

		// Re-enqueue with a long-running action
		blockChan := make(chan struct{})
		defer close(blockChan)
		newAction := &testAction{
			name: "orphan-del",
			execute: func(ctx context.Context) error {
				<-blockChan

				return nil
			},
		}
		err = executor.EnqueueAction("orphan-del", newAction, nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(executor.HasActionInProgress("orphan-del")).To(BeTrue())

		// Release the orphaned goroutine
		close(releaseChan)
		time.Sleep(200 * time.Millisecond)

		// The re-enqueued entry should still be in progress (orphan didn't delete it)
		Expect(executor.HasActionInProgress("orphan-del")).To(BeTrue(),
			"Re-enqueued entry should NOT be deleted by orphaned goroutine")
	})
})

var _ = Describe("Stuck Action Deduplication", func() {
	It("should log stuck exactly once across multiple metrics ticks", func() {
		observedCore, observedLogs := observer.New(zapcore.DebugLevel)
		logger := deps.NewFSMLogger(zap.New(observedCore).Sugar())

		identity := deps.Identity{ID: "test-worker", WorkerType: "test", HierarchyPath: "test/dedup"}

		executor := execution.NewActionExecutorWithTimeout(2, map[string]time.Duration{
			"stuck-dedup": 100 * time.Millisecond,
		}, "test-supervisor", identity, logger)
		executor.TestSetMetricsInterval(100 * time.Millisecond)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		executor.Start(ctx)
		defer executor.Shutdown()

		blockChan := make(chan struct{})
		defer close(blockChan)

		action := &testAction{
			name: "stuck-dedup",
			execute: func(ctx context.Context) error {
				<-blockChan

				return nil
			},
		}

		err := executor.EnqueueAction("stuck-dedup", action, nil)
		Expect(err).ToNot(HaveOccurred())

		// Wait for several metrics ticks after the stuck threshold (2x100ms=200ms)
		// Metrics fires every 100ms, so after 800ms we'd have ~6 ticks past the threshold
		time.Sleep(800 * time.Millisecond)

		stuckLogs := filterStuckActionLogs(observedLogs, "stuck_action_detected")
		Expect(stuckLogs).To(HaveLen(1), "Expected exactly 1 stuck_action_detected log, got %d", len(stuckLogs))
	})

	It("should report separately for different action IDs", func() {
		observedCore, observedLogs := observer.New(zapcore.DebugLevel)
		logger := deps.NewFSMLogger(zap.New(observedCore).Sugar())

		identity := deps.Identity{ID: "test-worker", WorkerType: "test", HierarchyPath: "test/multi"}

		executor := execution.NewActionExecutorWithTimeout(4, map[string]time.Duration{
			"stuck-a": 100 * time.Millisecond,
			"stuck-b": 100 * time.Millisecond,
		}, "test-supervisor", identity, logger)
		executor.TestSetMetricsInterval(100 * time.Millisecond)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		executor.Start(ctx)
		defer executor.Shutdown()

		blockChan := make(chan struct{})
		defer close(blockChan)

		for _, id := range []string{"stuck-a", "stuck-b"} {
			action := &testAction{
				name: id,
				execute: func(ctx context.Context) error {
					<-blockChan

					return nil
				},
			}
			err := executor.EnqueueAction(id, action, nil)
			Expect(err).ToNot(HaveOccurred())
		}

		time.Sleep(800 * time.Millisecond)

		stuckLogs := filterStuckActionLogs(observedLogs, "stuck_action_detected")
		Expect(stuckLogs).To(HaveLen(2), "Expected exactly 2 stuck_action_detected logs (one per action)")

		actionNames := map[string]bool{}
		for _, log := range stuckLogs {
			name, _ := log.ContextMap()["action_name"].(string)
			actionNames[name] = true
		}
		Expect(actionNames).To(HaveKey("stuck-a"))
		Expect(actionNames).To(HaveKey("stuck-b"))
	})
})

func filterStuckActionLogs(logs *observer.ObservedLogs, message string) []observer.LoggedEntry {
	var filtered []observer.LoggedEntry

	for _, entry := range logs.All() {
		if strings.Contains(entry.Message, message) {
			filtered = append(filtered, entry)
		}
	}

	return filtered
}
