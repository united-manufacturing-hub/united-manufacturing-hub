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

// Package supervisor internal test for ENG-4971. When a worker is mid-action at
// Shutdown(), actionPending==true blocks the early-return at
// reconciliation.go:286 so IsShutdownRequested() is never read; the fix adds
// `&& !isShutdownRequested` to that condition. With a frozen observation
// timestamp the gating predicate currentObsTime.After(lastActionObsTime) is
// false forever, so before the fix Next() stayed unreachable, the worker was
// never reaped, and the Shutdown() loop ran its full budget and warned
// graceful_shutdown_timeout. The fix matches the stale-data freshness gate,
// which already skipped its early-return on shutdown.
//
// Two specs constrain the one-line change from both sides: the first proves a
// mid-action worker reaps under shutdown; the second proves the gate still
// holds without shutdown, so the bypass is the reason the first reaps.
package supervisor_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// gateNoopAction is a stable-named, instantly-completing action. A state that
// dispatches it once flips workerCtx.actionPending=true, arming the
// action-observation gate. Its fixed Name lets HasActionInProgress de-dupe.
type gateNoopAction struct{}

func (gateNoopAction) Name() string { return "gate-noop-action" }

func (gateNoopAction) Execute(_ context.Context, _ any) error { return nil }

// gatedActionState dispatches gateNoopAction while alive (arming the
// action-observation gate) and, when ShutdownRequested is observed, returns
// SignalNeedsRemoval so the supervisor reaps it. The shutdown check is FIRST,
// so once the bypass makes Next() reachable again removal is the next result.
// State XOR action holds: the shutdown branch returns a signal with no action;
// the alive branch returns an action and self-state.
type gatedActionState struct{}

func (gatedActionState) String() string { return "running" }

func (gatedActionState) LifecyclePhase() config.LifecyclePhase {
	return config.PhaseRunningHealthy
}

func (s gatedActionState) Next(snapshot any) fsmv2.NextResult[any, any] {
	if snap, ok := snapshot.(fsmv2.Snapshot); ok {
		if ds, ok := snap.Desired.(fsmv2.DesiredState); ok && ds.IsShutdownRequested() {
			return fsmv2.NextResult[any, any]{
				Signal: fsmv2.SignalNeedsRemoval,
				State:  s,
				Reason: "shutdown requested",
			}
		}
	}

	// Alive: dispatch an action every time Next() runs. The frozen observation
	// timestamp means the gate never clears after the first dispatch, so without
	// the fix this branch runs exactly once and then Next() is unreachable.
	return fsmv2.NextResult[any, any]{
		Signal: fsmv2.SignalNone,
		State:  s,
		Action: gateNoopAction{},
		Reason: "running, dispatching action",
	}
}

var _ = Describe("Shutdown reap past action gating (ENG-4971)", func() {
	const (
		workerType = "container"
		workerID   = "gated-drain-worker"
	)

	var buf *shutdownTestSyncBuffer

	BeforeEach(func() {
		buf = &shutdownTestSyncBuffer{}
	})

	It("reaps a mid-action worker during the Shutdown() loop instead of timing out", func() {
		triangularStore := supervisor.CreateTestTriangularStoreForWorkerType(workerType)
		logger := deps.NewJSONFSMLogger(buf, deps.LevelDebug)

		// Short graceful timeout: without the bypass the Shutdown() loop runs to
		// this whole budget and warns, because the gate blocks the shutdown check;
		// with the bypass the worker is reaped within a tick and the warn never
		// fires.
		const gracefulTimeout = 500 * time.Millisecond

		s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
			WorkerType:              workerType,
			Store:                   triangularStore,
			Logger:                  logger,
			TickInterval:            20 * time.Millisecond,
			GracefulShutdownTimeout: gracefulTimeout,
		})

		identity := deps.Identity{
			ID:         workerID,
			Name:       "Gated Drain Worker",
			WorkerType: workerType,
		}

		// FROZEN observation timestamp: GetTimestamp() returns the same instant
		// on every collection, so the gating predicate
		// currentObsTime.After(lastActionObsTime) is false forever once an
		// action is pending. The gate then stays armed deterministically, with
		// no sleep-racing.
		frozen := time.Now()
		worker := &supervisor.TestWorker{
			InitialState: gatedActionState{},
			CollectFunc: func(_ context.Context) (fsmv2.ObservedState, error) {
				return &supervisor.TestObservedState{
					ID:          workerID,
					CollectedAt: frozen,
					Desired:     &supervisor.TestDesiredState{},
				}, nil
			},
		}
		Expect(s.AddWorker(identity, worker)).To(Succeed())

		ctx := context.Background()
		desiredDoc := persistence.Document{
			supervisor.FieldID:  workerID,
			"ShutdownRequested": false,
			"state":             "running",
		}
		_, err := triangularStore.SaveDesired(ctx, workerType, workerID, desiredDoc)
		Expect(err).ToNot(HaveOccurred())

		runCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		_ = s.Start(runCtx)
		defer s.Shutdown() // Idempotent; reaps if an assertion fails early.

		// Worker must be alive first.
		Eventually(s.TestWorkerCount, 1*time.Second, 10*time.Millisecond).Should(Equal(1))

		// Sequence the action BEFORE shutdown: wait until the worker has
		// dispatched its action and the gate is pending. The frozen timestamp
		// guarantees the gate then stays pinned, so this is ordering only, not a
		// timing race on the outcome.
		Eventually(func() bool {
			return containsLogEvent(buf.String(), "action_gating_pending")
		}, 1*time.Second, 10*time.Millisecond).Should(BeTrue(),
			"worker never dispatched an action, so the action-gating path under test was never armed")

		s.Shutdown()

		// Assertion 1: worker map empty, so the worker was reaped, not leaked.
		// Without the bypass the action gate blocks Next(), the shutdown check
		// never runs, and the worker is left resident when the Shutdown() loop
		// returns.
		remaining := s.TestWorkerCount()
		Expect(remaining).To(Equal(0),
			"expected the mid-action worker to be reaped during the Shutdown() loop (got %d remaining)", remaining)

		// Assertion 2: the Shutdown() loop exited via the worker-reap branch.
		// lifecycle.go logs graceful_shutdown_workers_removed only when
		// remaining==0 is reached before the timer, distinct from the
		// timeout/forceExit branches. This confirms the reap took the normal
		// drain-loop path positively, not by inference.
		Expect(containsLogEvent(buf.String(), "graceful_shutdown_workers_removed")).To(BeTrue(),
			"expected the Shutdown() loop to exit via the worker-reap branch (remaining==0), confirming the bypass let Next() emit SignalNeedsRemoval")

		// Assertion 3: the timeout warn must NOT fire. The Shutdown() loop logs
		// graceful_shutdown_timeout and runs its full budget only when a worker
		// never reaches SignalNeedsRemoval, which is exactly the blocked-gate case.
		Expect(containsLogEvent(buf.String(), "graceful_shutdown_timeout")).To(BeFalse(),
			"graceful_shutdown_timeout was logged: the Shutdown() loop timed out because the gated worker never ran its shutdown check")
	})

	It("holds the action gate when no shutdown is requested so the worker does not progress", func() {
		triangularStore := supervisor.CreateTestTriangularStoreForWorkerType(workerType)
		logger := deps.NewJSONFSMLogger(buf, deps.LevelDebug)

		// EnableTraceLogging makes the blocked-gate path observable:
		// action_gating_blocked is logged via logTrace, which only emits when
		// trace logging is on. The first spec does not need it; this one asserts
		// the blocked-path log line directly, so it must enable it.
		s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
			WorkerType:         workerType,
			Store:              triangularStore,
			Logger:             logger,
			TickInterval:       20 * time.Millisecond,
			EnableTraceLogging: true,
		})

		identity := deps.Identity{
			ID:         workerID,
			Name:       "Gated Drain Worker",
			WorkerType: workerType,
		}

		// Same frozen observation timestamp as the first spec: once an action is
		// pending the gating predicate currentObsTime.After(lastActionObsTime)
		// stays false, so the gate holds for as many ticks as we run.
		frozen := time.Now()
		worker := &supervisor.TestWorker{
			InitialState: gatedActionState{},
			CollectFunc: func(_ context.Context) (fsmv2.ObservedState, error) {
				return &supervisor.TestObservedState{
					ID:          workerID,
					CollectedAt: frozen,
					Desired:     &supervisor.TestDesiredState{},
				}, nil
			},
		}
		Expect(s.AddWorker(identity, worker)).To(Succeed())

		ctx := context.Background()
		desiredDoc := persistence.Document{
			supervisor.FieldID:  workerID,
			"ShutdownRequested": false,
			"state":             "running",
		}
		_, err := triangularStore.SaveDesired(ctx, workerType, workerID, desiredDoc)
		Expect(err).ToNot(HaveOccurred())

		runCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		_ = s.Start(runCtx)
		defer s.Shutdown()

		// Worker must be alive first.
		Eventually(s.TestWorkerCount, 1*time.Second, 10*time.Millisecond).Should(Equal(1))

		// Wait until the action is dispatched and the gate is armed. The frozen
		// timestamp then keeps it armed every following tick.
		Eventually(func() bool {
			return containsLogEvent(buf.String(), "action_gating_pending")
		}, 1*time.Second, 10*time.Millisecond).Should(BeTrue(),
			"worker never dispatched an action, so the action-gating path under test was never armed")

		// Without shutdown the gate must engage and block Next(): the blocked-path
		// log line is the positive proof the gate held. Deleting the gate (or
		// neutralizing it to `if false`) skips this whole block, so this line
		// never appears and this assertion fails. That is the falsifiability the
		// first spec lacks: the first spec passes even with the gate gone, this
		// one does not.
		Eventually(func() bool {
			return containsLogEvent(buf.String(), "action_gating_blocked")
		}, 1*time.Second, 10*time.Millisecond).Should(BeTrue(),
			"action_gating_blocked never logged: the gate did not engage, so the worker would progress past a pending action without shutdown")

		// The worker must stay resident and never reap: with no shutdown the
		// gatedActionState shutdown branch is unreachable, so SignalNeedsRemoval
		// is never emitted while the gate holds.
		Consistently(s.TestWorkerCount, 300*time.Millisecond, 20*time.Millisecond).Should(Equal(1),
			"the worker reaped without a shutdown request, which means the gate did not hold")
	})
})
