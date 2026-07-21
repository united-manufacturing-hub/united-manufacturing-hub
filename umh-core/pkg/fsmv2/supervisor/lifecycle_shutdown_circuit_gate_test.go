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

// Package supervisor internal test for ENG-4971. The infrastructure circuit
// breaker is the THIRD pre-Next() gate in tick() (after the stale-data
// freshness gate at reconciliation.go ~:216 and the action-observation gate at
// ~:286). When CheckChildConsistency fails the gate returns ErrInfraCircuitOpen
// before tick() reaches the worker, so the worker never runs Next(), never
// honors IsShutdownRequested, never emits SignalNeedsRemoval, and is never
// reaped. The freshness and action gates already bypass themselves on shutdown;
// before the fix the circuit gate did not, so an unhealthy subtree at SIGTERM
// stalled the parent worker's reap until the drain burned its full budget and
// warned graceful_shutdown_timeout (in prod, base 2s, risking SIGKILL).
//
// The fix wraps the whole circuit-breaker section (the CheckChildConsistency
// failure return AND the recovery/Store(false) branch) in `if s.started.Load()`
// — shutdown clears started at Shutdown() entry (lifecycle.go ~:230), so the
// drain skips infra-health gating and the worker ticks to its reap. Gating the
// whole section (not just the failure return) keeps a false
// circuit_breaker_closed / infrastructure_recovered from firing during
// shutdown.
//
// Determinism: a real child supervisor with its circuit forced open is injected
// into the parent's children map. CheckChildConsistency fails on it every tick.
// Before the fix tick() returns at the circuit gate and never reaches the
// reconcileChildren(nil) despawn that would clear the bad child, so the open
// circuit is self-sustaining and the drain rides its full budget — no
// sleep-racing, the outcome is forced by the gate, not by scheduling.
package supervisor_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

var _ = Describe("Shutdown reap past an open infrastructure circuit (ENG-4971)", func() {
	const (
		circuitWorkerType = "container"
		circuitWorkerID   = "circuit-drain-worker"
		badChildName      = "unhealthy-child"
		badChildType      = "container"
	)

	var buf *shutdownTestSyncBuffer

	BeforeEach(func() {
		buf = &shutdownTestSyncBuffer{}
	})

	It("reaps its worker during Shutdown() even when a child's circuit is open", func() {
		triangularStore := supervisor.CreateTestTriangularStoreForWorkerType(circuitWorkerType)
		logger := deps.NewJSONFSMLogger(buf, deps.LevelDebug)

		// Short graceful timeout: without the bypass the drain rides this whole
		// budget and warns, because the circuit gate blocks the worker tick;
		// with the bypass the worker reaps within a tick and the warn never
		// fires.
		const gracefulTimeout = 500 * time.Millisecond

		s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
			WorkerType:              circuitWorkerType,
			Store:                   triangularStore,
			Logger:                  logger,
			TickInterval:            20 * time.Millisecond,
			GracefulShutdownTimeout: gracefulTimeout,
		})

		identity := deps.Identity{
			ID:         circuitWorkerID,
			Name:       "Circuit Drain Worker",
			WorkerType: circuitWorkerType,
		}
		// shutdownHonoringState (lifecycle_shutdown_test.go) emits
		// SignalNeedsRemoval the moment its Next() observes ShutdownRequested.
		// The only thing that can keep it from reaping is a gate upstream of the
		// worker tick — here, the open infrastructure circuit.
		worker := &supervisor.TestWorker{
			InitialState: shutdownHonoringState{},
		}
		Expect(s.AddWorker(identity, worker)).To(Succeed())

		ctx := context.Background()
		desiredDoc := persistence.Document{
			supervisor.FieldID:  circuitWorkerID,
			"ShutdownRequested": false,
			"state":             "running",
		}
		_, err := triangularStore.SaveDesired(ctx, circuitWorkerType, circuitWorkerID, desiredDoc)
		Expect(err).ToNot(HaveOccurred())

		runCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		_ = s.Start(runCtx)
		defer s.Shutdown() // Idempotent; reaps if an assertion fails early.

		// Worker must be alive first.
		Eventually(func() int {
			return s.TestWorkerCount()
		}, 1*time.Second, 10*time.Millisecond).Should(Equal(1))

		// Inject a real child supervisor whose circuit is forced open.
		// CheckChildConsistency reads child.isCircuitOpen() and returns a
		// ChildHealthError for any open child, so the parent's circuit gate trips
		// every tick. The child has no worker of its own, so once the parent
		// reaches its shutdown reconcile (post-fix) the bad child reaps
		// immediately (ListWorkers() is empty).
		badChild := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
			WorkerType:              badChildType,
			Store:                   triangularStore,
			Logger:                  logger,
			TickInterval:            20 * time.Millisecond,
			GracefulShutdownTimeout: gracefulTimeout,
		})
		badChild.TestSetCircuitOpen(true)

		s.TestLinkChild(badChildName, badChild)

		// Sequence the open circuit BEFORE Shutdown: wait until the parent's own
		// circuit has opened off the bad child, proving CheckChildConsistency is
		// failing. The child's circuit stays open (the parent's tick returns at
		// the gate before it can reconcile the bad child away), so this is
		// ordering only, not a timing race on the outcome.
		Eventually(func() bool {
			return s.TestIsInfraCircuitOpen()
		}, 1*time.Second, 10*time.Millisecond).Should(BeTrue(),
			"the parent's circuit never opened, so CheckChildConsistency did not fail on the injected child and the gate under test was never armed")

		s.Shutdown()

		// Assertion 1: the worker was reaped, not leaked. Without the bypass the
		// circuit gate returns ErrInfraCircuitOpen before the worker tick, the
		// shutdown check never runs, and the worker is left resident when the
		// Shutdown() loop returns.
		remaining := s.TestWorkerCount()
		Expect(remaining).To(Equal(0),
			"expected the worker to be reaped during the Shutdown() loop despite the open circuit (got %d remaining)", remaining)

		// Assertion 2: the Shutdown() loop exited via the worker-reap branch.
		// lifecycle.go logs graceful_shutdown_workers_removed only when
		// remaining==0 is reached before the timer, distinct from the
		// timeout/forceExit branches.
		Expect(containsLogEvent(buf.String(), "graceful_shutdown_workers_removed")).To(BeTrue(),
			"expected the Shutdown() loop to exit via the worker-reap branch (remaining==0), confirming the bypass let the worker tick past the open circuit")

		// Assertion 3: the timeout warn must NOT fire. The drain logs
		// graceful_shutdown_timeout and runs its full budget only when a worker
		// never reaches SignalNeedsRemoval — exactly the blocked-gate case.
		Expect(containsLogEvent(buf.String(), "graceful_shutdown_timeout")).To(BeFalse(),
			"graceful_shutdown_timeout was logged: the drain timed out because the open circuit blocked the worker's shutdown check")

		// Assertion 4: no false recovery log. Gating the whole circuit-breaker
		// section (not just the failure return) is what keeps the recovery branch
		// from logging circuit_breaker_closed / infrastructure_recovered during
		// shutdown — infra did not recover, the gate was simply skipped.
		Expect(containsLogEvent(buf.String(), "circuit_breaker_closed")).To(BeFalse(),
			"circuit_breaker_closed was logged during shutdown: the recovery branch ran even though infrastructure never recovered")
	})
})
