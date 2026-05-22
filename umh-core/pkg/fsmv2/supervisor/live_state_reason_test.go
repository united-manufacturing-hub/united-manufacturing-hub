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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// Tests for ENG-4991: live state reason propagation (symptom from ENG-4983).

var _ = Describe("Live state reason propagation (ENG-4991)", func() {
	var (
		s            *supervisor.Supervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState]
		initialState *mockState
	)

	getWorkerDebug := func() supervisor.WorkerDebugInfo {
		raw := s.GetDebugInfo()
		info, ok := raw.(supervisor.SupervisorDebugInfo)
		Expect(ok).To(BeTrue(), "GetDebugInfo should return SupervisorDebugInfo")
		Expect(info.Workers).To(HaveLen(1), "getWorkerDebug: expected exactly one worker")

		return info.Workers[0]
	}

	Context("self-return with a live reason", func() {
		BeforeEach(func() {
			initialState = &mockState{}
			initialState.nextState = initialState
			initialState.reason = "auth backoff: 42 errors (cloudflare_challenge), delay 60s"

			s = newSupervisorWithWorker(&mockWorker{initialState: initialState}, nil, supervisor.CollectorHealthConfig{
				StaleThreshold: 10 * time.Second,
			})
		})

		It("persists the live reason into workerContext.currentStateReason", func() {
			err := s.TestTick(context.Background())
			Expect(err).ToNot(HaveOccurred())

			debug := getWorkerDebug()
			Expect(debug.StateReason).To(Equal("auth backoff: 42 errors (cloudflare_challenge), delay 60s"))

			_, reason := s.GetCurrentStateNameAndReason()
			Expect(reason).To(Equal("auth backoff: 42 errors (cloudflare_challenge), delay 60s"))
		})

		It("leaves transition-gated fields unchanged on a self-return", func() {
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
		It("reflects an updated reason after multiple self-returns", func() {
			initialState = &mockState{}
			initialState.nextState = initialState
			initialState.reason = "auth backoff: 1 errors (cloudflare_challenge), delay 2s"

			s = newSupervisorWithWorker(&mockWorker{initialState: initialState}, nil, supervisor.CollectorHealthConfig{
				StaleThreshold: 10 * time.Second,
			})

			Expect(s.TestTick(context.Background())).To(Succeed())

			initialState.reason = "auth backoff: 47 errors (cloudflare_challenge), delay 60s"

			Expect(s.TestTick(context.Background())).To(Succeed())

			debug := getWorkerDebug()
			Expect(debug.StateReason).To(Equal("auth backoff: 47 errors (cloudflare_challenge), delay 60s"))
		})
	})

	Context("state transitions still update the reason and the gated fields together", func() {
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
		// Empty reason from worker.Next() is a real signal, not a bug to mask.
		It("overwrites a stale non-empty reason with an empty one", func() {
			initialState = &mockState{}
			initialState.nextState = initialState
			initialState.reason = "first reason"

			s = newSupervisorWithWorker(&mockWorker{initialState: initialState}, nil, supervisor.CollectorHealthConfig{
				StaleThreshold: 10 * time.Second,
			})

			Expect(s.TestTick(context.Background())).To(Succeed())
			Expect(getWorkerDebug().StateReason).To(Equal("first reason"))

			initialState.reason = ""

			Expect(s.TestTick(context.Background())).To(Succeed())
			Expect(getWorkerDebug().StateReason).To(Equal(""), "empty reason from Next() must overwrite the stale value — Next() is authoritative")
		})
	})

	Context("propagating the live reason from a child to its parent (P5)", func() {
		const childWorkerType = "p5-child"

		var (
			parentSuper *supervisor.Supervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState]
			mockStore   *mockTriangularStore
			childState  *mockState
		)

		BeforeEach(func() {
			mockStore = newMockTriangularStore()

			childState = &mockState{}
			childState.nextState = childState
			childState.reason = "degraded (47 errors, 12 pending), backoff 8s"

			err := factory.RegisterFactoryByType(childWorkerType,
				func(_ deps.Identity, _ deps.FSMLogger, _ deps.StateReader, _ map[string]any) fsmv2.Worker {
					return &mockWorker{initialState: childState}
				},
			)
			Expect(err).ToNot(HaveOccurred())

			err = factory.RegisterSupervisorFactoryByType(childWorkerType,
				func(cfg interface{}) interface{} {
					return supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](
						cfg.(supervisor.Config),
					)
				},
			)
			Expect(err).ToNot(HaveOccurred())

			parentWorker := &hierarchicalWorker{
				id: "parent-worker",
				logger: newTickLogger(),
				observed: &mockObservedState{
					ID:          "parent-worker",
					CollectedAt: time.Now(),
					Desired:     &mockDesiredState{},
				},
				childrenSpecs: []config.ChildSpec{
					{
						Name:       "child1",
						WorkerType: childWorkerType,
						UserSpec:   config.UserSpec{Config: "child-config"},
					},
				},
			}

			parentSuper = supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
				WorkerType: "parent",
				Logger:     deps.NewNopFSMLogger(),
				Store:      mockStore,
			})

			identity := deps.Identity{
				ID:         "parent-worker",
				Name:       "Parent Worker",
				WorkerType: "parent",
			}
			Expect(parentSuper.AddWorker(identity, parentWorker)).To(Succeed())

			parentSuper.TestUpdateUserSpec(config.UserSpec{Config: "parent-config"})

			desiredDoc := persistence.Document{
				"id":                identity.ID,
				"ShutdownRequested": false,
			}
			_, err = mockStore.SaveDesired(context.Background(), "parent", identity.ID, desiredDoc)
			Expect(err).ToNot(HaveOccurred())

			mockStore.Observed["parent"] = map[string]interface{}{
				"parent-worker": persistence.Document{
					"id":          "parent-worker",
					"collectedAt": time.Now(),
				},
			}
		})

		It("parent observes the child's live self-return reason after a tick", func() {
			Expect(parentSuper.TestTick(context.Background())).To(Succeed())

			raw := parentSuper.GetDebugInfo()
			parentDebug, ok := raw.(supervisor.SupervisorDebugInfo)
			Expect(ok).To(BeTrue(), "GetDebugInfo should return SupervisorDebugInfo")
			Expect(parentDebug.Children).ToNot(BeEmpty(), "parent must have at least one child after tick")

			childDebug, exists := parentDebug.Children["child1"]
			Expect(exists).To(BeTrue(), "child named 'child1' must appear in parent's Children map")
			Expect(childDebug.Workers).ToNot(BeEmpty(), "child supervisor must have at least one worker")

			Expect(childDebug.Workers[0].StateReason).To(
				Equal("degraded (47 errors, 12 pending), backoff 8s"),
				"parent's view of child must carry the child's live per-tick reason, not the stale entry reason",
			)
		})

		It("parent reflects updated child reasons across multiple ticks", func() {
			Expect(parentSuper.TestTick(context.Background())).To(Succeed())

			raw := parentSuper.GetDebugInfo()
			parentDebug, ok := raw.(supervisor.SupervisorDebugInfo)
			Expect(ok).To(BeTrue())
			Expect(parentDebug.Children["child1"].Workers[0].StateReason).To(
				Equal("degraded (47 errors, 12 pending), backoff 8s"),
			)

			childState.reason = "degraded (89 errors, 5 pending), backoff 120s"

			Expect(parentSuper.TestTick(context.Background())).To(Succeed())

			raw = parentSuper.GetDebugInfo()
			parentDebug, ok = raw.(supervisor.SupervisorDebugInfo)
			Expect(ok).To(BeTrue())
			Expect(parentDebug.Children["child1"].Workers[0].StateReason).To(
				Equal("degraded (89 errors, 5 pending), backoff 120s"),
				"parent must reflect the child's updated live reason after a subsequent tick",
			)
		})
	})

})
