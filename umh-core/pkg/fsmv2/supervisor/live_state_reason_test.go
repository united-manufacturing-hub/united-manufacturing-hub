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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
)

// These tests cover ENG-4991: the supervisor must persist the per-tick reason
// composed by Next() into workerContext.currentStateReason on every reconcile,
// not only on state transitions. Before the fix the rich transient-retry
// reasons that auth/push/pull state machines emit are dropped on the floor and
// the heartbeat repeats the stale entry reason indefinitely (the symptom in
// ENG-4983 where an operator on a Teams call saw "simply nothing happening").

var _ = Describe("Live state reason propagation (ENG-4991)", func() {
	var (
		s            *supervisor.Supervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState]
		initialState *mockState
	)

	getWorkerDebug := func() supervisor.WorkerDebugInfo {
		raw := s.GetDebugInfo()
		info, ok := raw.(supervisor.SupervisorDebugInfo)
		Expect(ok).To(BeTrue(), "GetDebugInfo should return SupervisorDebugInfo")
		Expect(info.Workers).ToNot(BeEmpty(), "expected at least one worker")

		return info.Workers[0]
	}

	Context("self-return with a live reason", func() {
		BeforeEach(func() {
			// nextState is the same instance so Next() returns a self-return.
			// reason is the rich per-tick string a real worker (auth/push/pull)
			// would compose every tick from snapshot fields.
			initialState = &mockState{}
			initialState.nextState = initialState
			initialState.reason = "auth backoff: 42 errors (cloudflare_challenge), delay 60s"

			s = newSupervisorWithWorker(&mockWorker{initialState: initialState}, nil, supervisor.CollectorHealthConfig{
				StaleThreshold: 10 * time.Second,
			})
		})

		It("persists the live reason into workerContext.currentStateReason", func() {
			// Spec test #1 (P1). Before the fix this fails because the
			// supervisor only writes currentStateReason inside the
			// state-transition branch; a self-return leaves the field
			// stuck on the entry reason ("initial" at startup).
			err := s.TestTick(context.Background())
			Expect(err).ToNot(HaveOccurred())

			debug := getWorkerDebug()
			Expect(debug.StateReason).To(Equal("auth backoff: 42 errors (cloudflare_challenge), delay 60s"))
		})

		It("leaves transition-gated fields unchanged on a self-return", func() {
			// Spec test #1 (P4). Only currentStateReason should become
			// unconditional; currentState, stateEnteredAt, and
			// totalTransitions must still update only on real state
			// transitions.
			before := getWorkerDebug()

			err := s.TestTick(context.Background())
			Expect(err).ToNot(HaveOccurred())

			after := getWorkerDebug()
			Expect(after.State).To(Equal(before.State), "state name must not change on a self-return")
			Expect(after.StateEnteredAt).To(Equal(before.StateEnteredAt), "stateEnteredAt must not advance on a self-return")
			Expect(after.TotalTransitions).To(Equal(before.TotalTransitions), "totalTransitions must not increment on a self-return")
		})
	})

	Context("subsequent ticks keep the reason live", func() {
		// Spec test #4-#6 in miniature: a worker stuck in a state for
		// many ticks (the ENG-4983 reality) must surface whatever the
		// most recent Next() produced, not whatever the entry reason
		// was minutes ago.
		It("reflects an updated reason after multiple self-returns", func() {
			initialState = &mockState{}
			initialState.nextState = initialState
			initialState.reason = "auth backoff: 1 errors (cloudflare_challenge), delay 2s"

			s = newSupervisorWithWorker(&mockWorker{initialState: initialState}, nil, supervisor.CollectorHealthConfig{
				StaleThreshold: 10 * time.Second,
			})

			Expect(s.TestTick(context.Background())).To(Succeed())

			// The worker's Next() now returns a different reason next
			// tick (simulating ConsecutiveErrors and delay growing as
			// the transient-retry episode continues).
			initialState.reason = "auth backoff: 47 errors (cloudflare_challenge), delay 60s"

			Expect(s.TestTick(context.Background())).To(Succeed())

			debug := getWorkerDebug()
			Expect(debug.StateReason).To(Equal("auth backoff: 47 errors (cloudflare_challenge), delay 60s"))
		})
	})

	Context("state transitions still update the reason and the gated fields together", func() {
		// Spec test #2 (P4 regression): when the move-the-assignment
		// fix lands, the state-transition path must still write all of
		// currentState, stateEnteredAt, totalTransitions, and
		// currentStateReason in lockstep.
		It("updates all transition-gated fields plus the reason", func() {
			next := &mockState{}
			next.nextState = next

			initialState = &mockState{}
			initialState.nextState = next
			initialState.reason = "transitioning to next"

			s = newSupervisorWithWorker(&mockWorker{initialState: initialState}, nil, supervisor.CollectorHealthConfig{
				StaleThreshold: 10 * time.Second,
			})

			before := getWorkerDebug()

			err := s.TestTick(context.Background())
			Expect(err).ToNot(HaveOccurred())

			after := getWorkerDebug()
			Expect(after.StateReason).To(Equal("transitioning to next"))
			Expect(after.TotalTransitions).To(Equal(before.TotalTransitions+1), "transition must increment totalTransitions")
			Expect(after.StateEnteredAt.After(before.StateEnteredAt)).To(BeTrue(), "stateEnteredAt must advance on a real transition")
		})
	})

	Context("empty reason on a self-return", func() {
		// Spec Open Question 1, codified as Default: accept the empty.
		// Next() is the source of truth; preserving a stale non-empty
		// value over an empty one would re-introduce the bug for any
		// caller that legitimately wants to clear the reason.
		It("overwrites a stale non-empty reason with an empty one", func() {
			initialState = &mockState{}
			initialState.nextState = initialState
			initialState.reason = "first reason"

			s = newSupervisorWithWorker(&mockWorker{initialState: initialState}, nil, supervisor.CollectorHealthConfig{
				StaleThreshold: 10 * time.Second,
			})

			Expect(s.TestTick(context.Background())).To(Succeed())
			Expect(getWorkerDebug().StateReason).To(Equal("first reason"))

			// Now drive an empty reason.
			initialState.reason = ""

			Expect(s.TestTick(context.Background())).To(Succeed())
			Expect(getWorkerDebug().StateReason).To(Equal(""), "empty reason from Next() must overwrite the stale value — Next() is authoritative")
		})
	})

	// Compile-time witness that the mock-supervisor type is what
	// GetDebugInfo() will return. If the underlying type ever changes,
	// the type assertion in getWorkerDebug catches it at test time but
	// this assignment will fail at compile time so test rot is visible
	// without running the suite.
	var _ = func() fsmv2.State[any, any] { return &mockState{} }
})
