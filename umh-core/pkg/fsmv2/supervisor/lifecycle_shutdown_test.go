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

// Package supervisor internal tests for graceful-shutdown drain regression.
package supervisor

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// shutdownTestSyncBuffer is a goroutine-safe bytes.Buffer for log capture in these tests.
type shutdownTestSyncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *shutdownTestSyncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *shutdownTestSyncBuffer) Sync() error { return nil }

func (b *shutdownTestSyncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// containsLogEvent returns true if any JSON log line has the given msg value.
func containsLogEvent(logOutput, msg string) bool {
	for _, line := range strings.Split(logOutput, "\n") {
		if line == "" {
			continue
		}
		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry["msg"] == msg {
			return true
		}
	}
	return false
}

// findLogEvent returns the first log entry with the given msg, or nil.
func findLogEvent(logOutput, msg string) map[string]interface{} {
	for _, line := range strings.Split(logOutput, "\n") {
		if line == "" {
			continue
		}
		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry["msg"] == msg {
			return entry
		}
	}
	return nil
}

// shutdownHonoringState returns SignalNeedsRemoval when the snapshot's Desired
// implements fsmv2.DesiredState and IsShutdownRequested is true; otherwise
// SignalNone. Mirrors testutil.State's signature.
//
// State must be non-nil in NextResult; tickWorker reads it.
type shutdownHonoringState struct{}

func (shutdownHonoringState) String() string { return "running" }

func (shutdownHonoringState) LifecyclePhase() config.LifecyclePhase {
	return config.PhaseRunningHealthy
}

func (s shutdownHonoringState) Next(snapshot any) fsmv2.NextResult[any, any] {
	if snap, ok := snapshot.(fsmv2.Snapshot); ok {
		if ds, ok := snap.Desired.(fsmv2.DesiredState); ok {
			if ds.IsShutdownRequested() {
				return fsmv2.NextResult[any, any]{Signal: fsmv2.SignalNeedsRemoval, State: s, Reason: "shutdown requested"}
			}
		}
	}
	return fsmv2.NextResult[any, any]{Signal: fsmv2.SignalNone, State: shutdownHonoringState{}, Reason: "running"}
}

var _ = Describe("Shutdown drain regression", func() {
	const (
		workerType = "container"
		workerID   = "drain-test-worker"
	)

	var buf *shutdownTestSyncBuffer

	BeforeEach(func() {
		buf = &shutdownTestSyncBuffer{}
	})

	newStore := func() *storage.TriangularStore {
		return CreateTestTriangularStoreForWorkerType(workerType)
	}

	seedDesiredState := func(s *Supervisor[*TestObservedState, *TestDesiredState]) {
		ctx := context.Background()
		desiredDoc := persistence.Document{
			FieldID:             workerID,
			"ShutdownRequested": false,
			"state":             "running",
		}
		_, err := s.store.SaveDesired(ctx, workerType, workerID, desiredDoc)
		Expect(err).ToNot(HaveOccurred())
	}

	Describe("TestShutdown_DrainsWorkersViaReconciliation", func() {
		It("should complete Shutdown() in under 500ms when worker honors ShutdownRequested", func() {
			triangularStore := newStore()
			logger := deps.NewJSONFSMLogger(buf, deps.LevelDebug)

			s := NewSupervisor[*TestObservedState, *TestDesiredState](Config{
				WorkerType:              workerType,
				Store:                   triangularStore,
				Logger:                  logger,
				TickInterval:            100 * time.Millisecond,
				GracefulShutdownTimeout: 1 * time.Second,
			})

			identity := deps.Identity{
				ID:         workerID,
				Name:       "Drain Test Worker",
				WorkerType: workerType,
			}
			worker := &TestWorker{
				InitialState: shutdownHonoringState{},
			}
			Expect(s.AddWorker(identity, worker)).To(Succeed())

			seedDesiredState(s)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			_ = s.Start(ctx)

			// Sanity: worker must be alive before Shutdown.
			Eventually(func() int {
				s.mu.RLock()
				defer s.mu.RUnlock()
				return len(s.workers)
			}, 500*time.Millisecond, 10*time.Millisecond).Should(Equal(1))

			start := time.Now()
			s.Shutdown()
			elapsed := time.Since(start)

			// Assertion 1: elapsed well under GracefulShutdownTimeout (1s).
			// Pre-fix: drain waits full 1s, elapsed ≈ 1s → FAILS.
			// Post-fix: drain exits via remaining==0 at ~100–200ms → PASSES.
			Expect(elapsed).To(BeNumerically("<", 500*time.Millisecond),
				"Shutdown took %v; expected drain in <500ms (GracefulShutdownTimeout=1s). "+
					"This confirms the drain deadlock: tickLoop is cancelled before drain polls s.workers.", elapsed)

			// Assertion 2: worker map empty after Shutdown returns.
			s.mu.RLock()
			remaining := len(s.workers)
			s.mu.RUnlock()
			Expect(remaining).To(Equal(0), "expected s.workers to be empty after graceful drain")

			// Assertion 3: graceful_shutdown_timeout warn must NOT be emitted.
			logOutput := buf.String()
			Expect(containsLogEvent(logOutput, "graceful_shutdown_timeout")).To(BeFalse(),
				"graceful_shutdown_timeout was logged, indicating the timeout fired instead of draining")
		})
	})

	Describe("TestShutdown_TimeoutPathStillFires", func() {
		It("should fire graceful_shutdown_timeout when worker does not honor ShutdownRequested", func() {
			triangularStore := newStore()
			logger := deps.NewJSONFSMLogger(buf, deps.LevelDebug)

			s := NewSupervisor[*TestObservedState, *TestDesiredState](Config{
				WorkerType:              workerType,
				Store:                   triangularStore,
				Logger:                  logger,
				TickInterval:            50 * time.Millisecond,
				GracefulShutdownTimeout: 200 * time.Millisecond,
			})

			identity := deps.Identity{
				ID:         workerID,
				Name:       "Timeout Test Worker",
				WorkerType: workerType,
			}
			// Default TestWorker uses testutil.State which returns SignalNone indefinitely.
			worker := &TestWorker{}
			Expect(s.AddWorker(identity, worker)).To(Succeed())

			seedDesiredState(s)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			_ = s.Start(ctx)

			Eventually(func() int {
				s.mu.RLock()
				defer s.mu.RUnlock()
				return len(s.workers)
			}, 500*time.Millisecond, 10*time.Millisecond).Should(Equal(1))

			start := time.Now()
			s.Shutdown()
			elapsed := time.Since(start)

			// Assertion 1: upper bound only — generous slop.
			Expect(elapsed).To(BeNumerically("<", 500*time.Millisecond),
				"Shutdown blocked far beyond GracefulShutdownTimeout=200ms: %v", elapsed)

			// Assertion 2: worker not drained (timeout path — correct pre-fix and post-fix behaviour).
			s.mu.RLock()
			remaining := len(s.workers)
			s.mu.RUnlock()
			Expect(remaining).To(BeNumerically(">", 0),
				"expected worker still present after timeout-path Shutdown (worker never honored shutdown)")

			// Assertion 3: graceful_shutdown_timeout warn IS present, with remaining_worker_count > 0.
			logOutput := buf.String()
			entry := findLogEvent(logOutput, "graceful_shutdown_timeout")
			Expect(entry).ToNot(BeNil(), "expected graceful_shutdown_timeout warn to be logged")
			if entry != nil {
				count, ok := entry["remaining_worker_count"]
				Expect(ok).To(BeTrue(), "expected remaining_worker_count field in warn log")
				Expect(count).To(BeNumerically(">", 0), "expected remaining_worker_count > 0")
			}
		})
	})

	Describe("TestRestart_DemotedToDuringDrain", func() {
		It("should demote SignalNeedsRestart to removal when supervisor is draining (started=false)", func() {
			// This test verifies the processSignal restart gating introduced in
			// Commit 3: shouldRestart = s.started.Load() && s.pendingRestart[workerID].
			// When Shutdown() sets started=false before drain, a pending restart
			// must NOT spawn a new worker — it must be treated as a removal.
			triangularStore := newStore()
			logger := deps.NewJSONFSMLogger(buf, deps.LevelDebug)

			s := NewSupervisor[*TestObservedState, *TestDesiredState](Config{
				WorkerType:              workerType,
				Store:                   triangularStore,
				Logger:                  logger,
				TickInterval:            50 * time.Millisecond,
				GracefulShutdownTimeout: 500 * time.Millisecond,
			})

			// shutdownHonoringState immediately emits SignalNeedsRemoval when
			// ShutdownRequested is true, so the worker drains on the first tick
			// after Shutdown() sets started=false.
			identity := deps.Identity{
				ID:         workerID,
				Name:       "Demotion Test Worker",
				WorkerType: workerType,
			}
			worker := &TestWorker{
				InitialState: shutdownHonoringState{},
			}
			Expect(s.AddWorker(identity, worker)).To(Succeed())

			seedDesiredState(s)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			_ = s.Start(ctx)

			// Wait for the worker to be registered as alive.
			Eventually(func() int {
				s.mu.RLock()
				defer s.mu.RUnlock()
				return len(s.workers)
			}, 300*time.Millisecond, 10*time.Millisecond).Should(Equal(1))

			// Mark the worker as pendingRestart BEFORE calling Shutdown().
			// This simulates a restart signal arriving just before drain starts.
			s.TestSetPendingRestart(identity.ID)

			// Shutdown() stores started=false (Phase 1) then drains (Phase 3).
			// The drain loop will call processSignal on the worker.
			// Because started=false, shouldRestart evaluates to false and the
			// worker is removed rather than restarted.
			s.Shutdown()

			// After Shutdown returns, the worker must be gone — NOT re-added.
			s.mu.RLock()
			remaining := len(s.workers)
			s.mu.RUnlock()
			Expect(remaining).To(Equal(0),
				"worker should be removed during drain, not restarted (started=false gating)")

			// pendingRestart must have been cleaned up.
			Expect(s.TestIsPendingRestart(identity.ID)).To(BeFalse(),
				"pendingRestart entry should be cleared after demotion")
		})
	})

	Describe("ForceExit short-circuits the drain", func() {
		It("breaks the Phase 3 drain when the ForceExit channel closes", func() {
			triangularStore := newStore()
			logger := deps.NewJSONFSMLogger(buf, deps.LevelDebug)
			forceExit := make(chan struct{})

			s := NewSupervisor[*TestObservedState, *TestDesiredState](Config{
				WorkerType: workerType,
				Store:      triangularStore,
				Logger:     logger,
				// Long timeout so the test must hit forceExit, not the timer.
				TickInterval:            50 * time.Millisecond,
				GracefulShutdownTimeout: 5 * time.Second,
				ForceExit:               forceExit,
			})

			identity := deps.Identity{
				ID:         workerID,
				Name:       "Force Exit Test Worker",
				WorkerType: workerType,
			}
			// Worker never honours ShutdownRequested -> drain would otherwise
			// run to its full 5-second timeout. ForceExit must cut it short.
			worker := &TestWorker{}
			Expect(s.AddWorker(identity, worker)).To(Succeed())

			seedDesiredState(s)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			_ = s.Start(ctx)

			Eventually(func() int {
				s.mu.RLock()
				defer s.mu.RUnlock()
				return len(s.workers)
			}, 500*time.Millisecond, 10*time.Millisecond).Should(Equal(1))

			// Close forceExit shortly after Shutdown enters Phase 3 drain.
			go func() {
				time.Sleep(100 * time.Millisecond)
				close(forceExit)
			}()

			start := time.Now()
			s.Shutdown()
			elapsed := time.Since(start)

			// Assertion 1: elapsed must be far below GracefulShutdownTimeout=5s.
			// A successful forceExit cuts Phase 3 within ~150ms of the close.
			Expect(elapsed).To(BeNumerically("<", 1*time.Second),
				"Shutdown took %v; expected ForceExit to cut Phase 3 well before "+
					"GracefulShutdownTimeout=5s", elapsed)

			// Assertion 2: the dedicated force-exit log event fires.
			logOutput := buf.String()
			Expect(containsLogEvent(logOutput, "graceful_shutdown_force_exit")).To(BeTrue(),
				"expected graceful_shutdown_force_exit log; ForceExit branch did not fire")

			// Assertion 3: the timeout path must NOT have fired (forceExit wins).
			Expect(containsLogEvent(logOutput, "graceful_shutdown_timeout")).To(BeFalse(),
				"graceful_shutdown_timeout fired; ForceExit should have won the race")
		})
	})
})
