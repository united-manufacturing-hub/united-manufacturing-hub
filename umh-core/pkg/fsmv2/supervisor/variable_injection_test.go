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
	"bytes"
	"context"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
)

// TestWorker is a test double for fsmv2.Worker that allows capturing DeriveDesiredState calls.
type TestWorker struct {
	identity               deps.Identity
	initialState           fsmv2.State[any, any]
	deriveDesiredStateFunc func(spec config.UserSpec) (fsmv2.DesiredState, error)
}

func (t *TestWorker) CollectObservedState(ctx context.Context, _ fsmv2.DesiredState) (fsmv2.ObservedState, error) {
	return &TestObservedState{
		ID:          t.identity.ID,
		CollectedAt: time.Now(),
	}, nil
}

func (t *TestWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	// Convert interface{} to config.UserSpec for the test function
	userSpec, ok := spec.(config.UserSpec)
	if !ok {
		return &config.DesiredState{BaseDesiredState: config.BaseDesiredState{}}, nil
	}

	if t.deriveDesiredStateFunc != nil {
		return t.deriveDesiredStateFunc(userSpec)
	}

	return &config.DesiredState{BaseDesiredState: config.BaseDesiredState{}}, nil
}

func (t *TestWorker) GetInitialState() fsmv2.State[any, any] {
	return t.initialState
}

// TestObservedState is a test double for fsmv2.ObservedState.
type TestObservedState struct {
	ID          string    `json:"id"`
	CollectedAt time.Time `json:"collectedAt"`
}

func (t *TestObservedState) GetTimestamp() time.Time {
	return t.CollectedAt
}

// TestState is a test double for fsmv2.State.
type TestState struct {
	name   string
	reason string
}

func (t *TestState) Next(snapshot any) fsmv2.NextResult[any, any] {
	reason := t.reason
	if reason == "" {
		reason = t.name
	}

	return fsmv2.Result[any, any](t, fsmv2.SignalNone, nil, reason, nil)
}

func (t *TestState) String() string {
	return t.name
}

func (t *TestState) LifecyclePhase() config.LifecyclePhase {
	return config.PhaseRunningHealthy
}

var _ = Describe("Variable Injection", func() {
	var (
		ctx        context.Context
		store      storage.TriangularStoreInterface
		testWorker *TestWorker
		identity   deps.Identity
		s          *supervisor.Supervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState]
		logger     deps.FSMLogger
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = deps.NewNopFSMLogger()

		basicStore := memory.NewInMemoryStore()

		// Create collections for test worker type
		var err error
		err = basicStore.CreateCollection(ctx, "test_identity", nil)
		Expect(err).ToNot(HaveOccurred())
		err = basicStore.CreateCollection(ctx, "test_desired", nil)
		Expect(err).ToNot(HaveOccurred())
		err = basicStore.CreateCollection(ctx, "test_observed", nil)
		Expect(err).ToNot(HaveOccurred())

		store = storage.NewTriangularStore(basicStore, deps.NewNopFSMLogger())

		identity = deps.Identity{
			ID:         "test-worker-1",
			Name:       "Test Worker",
			WorkerType: "test",
		}

		testWorker = &TestWorker{
			identity: identity,
			initialState: &TestState{
				name: "Initial",
			},
		}

		s = supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
			WorkerType:   "test",
			Store:        store,
			Logger:       logger,
			TickInterval: 100 * time.Millisecond,
		})

		err = s.AddWorker(identity, testWorker)
		Expect(err).ToNot(HaveOccurred())

		// Save initial desired state (required for Tick to load snapshot)
		initialDesired := persistence.Document{
			"id":    identity.ID,
			"state": "running",
		}
		_, err = store.SaveDesired(ctx, "test", identity.ID, initialDesired)
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("Global variables in the spec", func() {
		It("passes the Global variables in the supervisor's spec to DeriveDesiredState", func() {
			globalVars := map[string]any{
				"api_endpoint": "https://api.example.com",
				"cluster_id":   "cluster-123",
			}

			s.TestUpdateUserSpec(config.UserSpec{Variables: config.VariableBundle{Global: globalVars}})

			// Capture the userSpec passed to DeriveDesiredState
			var capturedSpec config.UserSpec
			testWorker.deriveDesiredStateFunc = func(spec config.UserSpec) (fsmv2.DesiredState, error) {
				capturedSpec = spec

				return &config.DesiredState{BaseDesiredState: config.BaseDesiredState{}}, nil
			}

			err := s.TestTick(ctx)
			Expect(err).ToNot(HaveOccurred())

			Expect(capturedSpec.Variables.Global).To(Equal(globalVars))
		})

		It("passes no Global variables when the spec has none", func() {
			s.TestUpdateUserSpec(config.UserSpec{})

			var capturedSpec config.UserSpec
			testWorker.deriveDesiredStateFunc = func(spec config.UserSpec) (fsmv2.DesiredState, error) {
				capturedSpec = spec

				return &config.DesiredState{BaseDesiredState: config.BaseDesiredState{}}, nil
			}

			err := s.TestTick(ctx)
			Expect(err).ToNot(HaveOccurred())

			Expect(capturedSpec.Variables.Global).To(BeNil())
		})
	})

	Describe("Internal Variables Injection in Tick", func() {
		It("should inject Internal variables with id, _created_at, and empty parent_id for root supervisor", func() {
			var capturedSpec config.UserSpec
			testWorker.deriveDesiredStateFunc = func(spec config.UserSpec) (fsmv2.DesiredState, error) {
				capturedSpec = spec

				return &config.DesiredState{BaseDesiredState: config.BaseDesiredState{}}, nil
			}

			err := s.TestTick(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Verify Internal variables were injected (map[string]any).
			Expect(capturedSpec.Variables.Internal).ToNot(BeNil())
			Expect(capturedSpec.Variables.Internal["id"]).To(Equal(identity.ID))
			Expect(capturedSpec.Variables.Internal["_created_at"]).ToNot(BeZero())

			// For root supervisor, ParentID should be empty string.
			Expect(capturedSpec.Variables.Internal["parent_id"]).To(Equal(""))
		})

	})

	Describe("Variable Inheritance from Parent to Child", func() {
		It("should inherit parent User variables to child, and add the child's own", func() {
			// Setup parent supervisor with User variables
			parentUserVars := map[string]any{
				"IP":   "192.168.1.100",
				"PORT": 502,
			}

			parentUserSpec := config.UserSpec{
				Variables: config.VariableBundle{
					User: parentUserVars,
				},
			}

			s.TestUpdateUserSpec(parentUserSpec)

			// Create a child spec that does NOT define IP but defines DEVICE_ID
			childUserSpec := config.UserSpec{
				Variables: config.VariableBundle{
					User: map[string]any{
						"DEVICE_ID": "child-device",
					},
				},
			}

			// Capture the userSpec passed to child during reconciliation
			var capturedChildSpec config.UserSpec

			// Create a mock child supervisor to capture the userSpec
			// We'll use the TestReconcileWithCapture helper if available,
			// or check via the child's updateUserSpec
			// For now, we test via DeriveDesiredState which receives the merged spec

			testWorker.deriveDesiredStateFunc = func(spec config.UserSpec) (fsmv2.DesiredState, error) {
				// This will be called during tick, returning a ChildSpec
				return &config.DesiredState{
					BaseDesiredState: config.BaseDesiredState{},
					ChildrenSpecs: []config.ChildSpec{
						{
							Name:       "test-child",
							WorkerType: "test",
							UserSpec:   childUserSpec,
						},
					},
				}, nil
			}

			// Tick the parent to trigger reconcileChildren
			err := s.TestTick(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Get the child supervisor and check its userSpec
			children := s.GetChildren()
			Expect(children).To(HaveKey("test-child"))

			child := children["test-child"]
			Expect(child).ToNot(BeNil())

			// Get the child's userSpec through the test helper
			capturedChildSpec = child.TestGetUserSpec()

			// Verify inheritance: child should have parent's IP and PORT, plus its own DEVICE_ID
			Expect(capturedChildSpec.Variables.User).To(HaveKeyWithValue("IP", "192.168.1.100"))
			Expect(capturedChildSpec.Variables.User).To(HaveKeyWithValue("PORT", 502))
			Expect(capturedChildSpec.Variables.User).To(HaveKeyWithValue("DEVICE_ID", "child-device"))
		})

		It("keeps the parent's User value when the child's spec sets the same key", func() {
			// Setup parent supervisor with User variables
			parentUserVars := map[string]any{
				"IP":   "192.168.1.100",
				"PORT": 502,
			}

			parentUserSpec := config.UserSpec{
				Variables: config.VariableBundle{
					User: parentUserVars,
				},
			}

			s.TestUpdateUserSpec(parentUserSpec)

			childUserSpec := config.UserSpec{
				Variables: config.VariableBundle{
					User: map[string]any{
						"PORT":      503,
						"DEVICE_ID": "child-device",
					},
				},
			}

			testWorker.deriveDesiredStateFunc = func(spec config.UserSpec) (fsmv2.DesiredState, error) {
				return &config.DesiredState{
					BaseDesiredState: config.BaseDesiredState{},
					ChildrenSpecs: []config.ChildSpec{
						{
							Name:       "same-key-child",
							WorkerType: "test",
							UserSpec:   childUserSpec,
						},
					},
				}, nil
			}

			err := s.TestTick(ctx)
			Expect(err).ToNot(HaveOccurred())

			children := s.GetChildren()
			Expect(children).To(HaveKey("same-key-child"))

			child := children["same-key-child"]
			capturedChildSpec := child.TestGetUserSpec()

			Expect(capturedChildSpec.Variables.User).To(HaveKeyWithValue("IP", "192.168.1.100"))
			Expect(capturedChildSpec.Variables.User).To(HaveKeyWithValue("PORT", 502))
			Expect(capturedChildSpec.Variables.User).To(HaveKeyWithValue("DEVICE_ID", "child-device"))
		})
	})

	Describe("Variable conflicts", func() {
		var logs *bytes.Buffer

		BeforeEach(func() {
			logs = &bytes.Buffer{}
			s = supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
				WorkerType:   "test",
				Store:        store,
				Logger:       deps.NewJSONFSMLogger(logs, deps.LevelWarn),
				TickInterval: 100 * time.Millisecond,
			})
			Expect(s.AddWorker(identity, testWorker)).To(Succeed())
		})

		conflictLines := func() []string {
			var lines []string
			for _, line := range strings.Split(logs.String(), "\n") {
				if strings.Contains(line, `"msg":"child_variable_conflict"`) {
					lines = append(lines, line)
				}
			}
			return lines
		}

		// Each value is matched JSON-quoted, so a log line's timestamp
		// cannot match a numeric value.
		noValueInWarnings := func(values ...string) {
			all := strings.Join(conflictLines(), "\n")
			for _, value := range values {
				Expect(all).NotTo(ContainSubstring(strconv.Quote(value)))
			}
		}

		emitChild := func(vars config.VariableBundle) {
			testWorker.deriveDesiredStateFunc = func(spec config.UserSpec) (fsmv2.DesiredState, error) {
				return &config.DesiredState{
					BaseDesiredState: config.BaseDesiredState{},
					ChildrenSpecs: []config.ChildSpec{
						{Name: "conflict-child", WorkerType: "test", UserSpec: config.UserSpec{Variables: vars}},
					},
				}, nil
			}
		}

		It("warns once per child, namespace and key, not on every tick", func() {
			s.TestUpdateUserSpec(config.UserSpec{
				Variables: config.VariableBundle{
					User:   map[string]any{"IP": "192.168.1.100", "PORT": 502},
					Global: map[string]any{"cluster_id": "cluster-a"},
				},
			})
			emitChild(config.VariableBundle{
				User:   map[string]any{"PORT": 503, "DEVICE_ID": "child-device"},
				Global: map[string]any{"cluster_id": "cluster-b"},
			})

			// Tick 1 adds the child; ticks 2 and 3 update it.
			for range 3 {
				Expect(s.TestTick(ctx)).To(Succeed())
			}

			lines := conflictLines()
			Expect(lines).To(HaveLen(2), "one warning per (child, namespace, key), got:\n%s", logs.String())
			Expect(lines).To(ContainElement(And(
				ContainSubstring(`"child_name":"conflict-child"`),
				ContainSubstring(`"namespace":"User"`),
				ContainSubstring(`"key":"PORT"`),
			)))
			Expect(lines).To(ContainElement(And(
				ContainSubstring(`"child_name":"conflict-child"`),
				ContainSubstring(`"namespace":"Global"`),
				ContainSubstring(`"key":"cluster_id"`),
			)))
			// The warning names the key, never a value: variables can hold
			// credentials. The parent's dropped value is as sensitive as the
			// child's, so both must stay out of the logs.
			Expect(logs.String()).NotTo(ContainSubstring("cluster-b"))
			Expect(logs.String()).NotTo(ContainSubstring("cluster-a"))
			Expect(logs.String()).NotTo(ContainSubstring("192.168.1.100"))
			noValueInWarnings("502", "503")

			childSpec := s.GetChildren()["conflict-child"].TestGetUserSpec()
			Expect(childSpec.Variables.User).To(HaveKeyWithValue("PORT", 502))
			Expect(childSpec.Variables.Global).To(HaveKeyWithValue("cluster_id", "cluster-a"))
		})

		It("does not warn when the child only adds keys", func() {
			s.TestUpdateUserSpec(config.UserSpec{
				Variables: config.VariableBundle{User: map[string]any{"IP": "192.168.1.100"}},
			})
			emitChild(config.VariableBundle{User: map[string]any{"DEVICE_ID": "child-device"}})

			for range 3 {
				Expect(s.TestTick(ctx)).To(Succeed())
			}

			// The merge ran: without this, an empty conflictLines() could also
			// mean no child was ever created.
			childSpec := s.GetChildren()["conflict-child"].TestGetUserSpec()
			Expect(childSpec.Variables.User).To(HaveKeyWithValue("IP", "192.168.1.100"))
			Expect(childSpec.Variables.User).To(HaveKeyWithValue("DEVICE_ID", "child-device"))

			Expect(conflictLines()).To(BeEmpty())
			Expect(logs.String()).NotTo(ContainSubstring("child-device"))
			Expect(logs.String()).NotTo(ContainSubstring("192.168.1.100"))
		})

		It("warns for each child that sets the key", func() {
			s.TestUpdateUserSpec(config.UserSpec{
				Variables: config.VariableBundle{User: map[string]any{"PORT": 502}},
			})
			testWorker.deriveDesiredStateFunc = func(spec config.UserSpec) (fsmv2.DesiredState, error) {
				return &config.DesiredState{
					BaseDesiredState: config.BaseDesiredState{},
					ChildrenSpecs: []config.ChildSpec{
						{Name: "child-a", WorkerType: "test", UserSpec: config.UserSpec{Variables: config.VariableBundle{User: map[string]any{"PORT": 503}}}},
						{Name: "child-b", WorkerType: "test", UserSpec: config.UserSpec{Variables: config.VariableBundle{User: map[string]any{"PORT": 504}}}},
					},
				}, nil
			}

			for range 3 {
				Expect(s.TestTick(ctx)).To(Succeed())
			}

			lines := conflictLines()
			Expect(lines).To(HaveLen(2))
			Expect(lines).To(ContainElement(ContainSubstring(`"child_name":"child-a"`)))
			Expect(lines).To(ContainElement(ContainSubstring(`"child_name":"child-b"`)))
			noValueInWarnings("502", "503", "504")
		})

		It("warns when a child's spec first sets the key on a later tick", func() {
			s.TestUpdateUserSpec(config.UserSpec{
				Variables: config.VariableBundle{User: map[string]any{"PORT": 502}},
			})
			emitChild(config.VariableBundle{User: map[string]any{"DEVICE_ID": "d"}})
			Expect(s.TestTick(ctx)).To(Succeed())
			Expect(conflictLines()).To(BeEmpty())

			emitChild(config.VariableBundle{User: map[string]any{"PORT": 503}})
			// The parent's spec must change: tick caches the derived state by the
			// user-spec hash (lastUserSpecHash; see supervisor/doc.go), so a new
			// deriveDesiredStateFunc alone never re-derives.
			s.TestUpdateUserSpec(config.UserSpec{
				Variables: config.VariableBundle{User: map[string]any{"PORT": 502, "IP": "10.0.0.1"}},
			})
			Expect(s.TestTick(ctx)).To(Succeed())
			Expect(s.TestTick(ctx)).To(Succeed())

			Expect(conflictLines()).To(HaveLen(1))
			noValueInWarnings("502", "503", "10.0.0.1")
		})

		It("warns once per namespace for the same key", func() {
			s.TestUpdateUserSpec(config.UserSpec{
				Variables: config.VariableBundle{
					User:   map[string]any{"PORT": 502},
					Global: map[string]any{"PORT": 502},
				},
			})
			emitChild(config.VariableBundle{
				User:   map[string]any{"PORT": 503},
				Global: map[string]any{"PORT": 503},
			})

			Expect(s.TestTick(ctx)).To(Succeed())

			lines := conflictLines()
			Expect(lines).To(HaveLen(2), "one warning per (child, namespace, key), got:\n%s", logs.String())
			Expect(lines).To(ContainElement(And(
				ContainSubstring(`"namespace":"User"`),
				ContainSubstring(`"key":"PORT"`),
			)))
			Expect(lines).To(ContainElement(And(
				ContainSubstring(`"namespace":"Global"`),
				ContainSubstring(`"key":"PORT"`),
			)))
			noValueInWarnings("502", "503")
		})

		It("does not warn again when a removed child is re-added with the same conflict", func() {
			const removableType = "variable_conflict_child"
			_ = factory.RegisterFactoryByType(removableType, func(identity deps.Identity, _ deps.FSMLogger, _ deps.StateReader, _ map[string]any) fsmv2.Worker {
				return &supervisor.TestWorkerWithType{
					Worker:     supervisor.TestWorker{InitialState: shutdownHonoringState{}},
					WorkerType: removableType,
				}
			})
			_ = factory.RegisterSupervisorFactoryByType(removableType, func(cfg interface{}) interface{} {
				return supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](cfg.(supervisor.Config))
			})

			deriveChild := func(vars config.VariableBundle) {
				testWorker.deriveDesiredStateFunc = func(spec config.UserSpec) (fsmv2.DesiredState, error) {
					return &config.DesiredState{
						BaseDesiredState: config.BaseDesiredState{},
						ChildrenSpecs: []config.ChildSpec{
							{Name: "conflict-child", WorkerType: removableType, UserSpec: config.UserSpec{Variables: vars}},
						},
					}, nil
				}
			}
			deriveNoChild := func() {
				testWorker.deriveDesiredStateFunc = func(spec config.UserSpec) (fsmv2.DesiredState, error) {
					return &config.DesiredState{BaseDesiredState: config.BaseDesiredState{}}, nil
				}
			}

			s.TestUpdateUserSpec(config.UserSpec{
				Variables: config.VariableBundle{User: map[string]any{"PORT": 502}},
			})
			deriveChild(config.VariableBundle{User: map[string]any{"PORT": 503}})
			Expect(s.TestTick(ctx)).To(Succeed())
			Expect(conflictLines()).To(HaveLen(1))

			// Remove the child from the specs; the parent spec must change too,
			// or tick keeps the cached desired state.
			deriveNoChild()
			s.TestUpdateUserSpec(config.UserSpec{
				Variables: config.VariableBundle{User: map[string]any{"PORT": 502, "IP": "10.0.0.1"}},
			})
			Eventually(func() bool {
				Expect(s.TestTick(ctx)).To(Succeed())
				_, exists := s.GetChildren()["conflict-child"]
				return !exists
			}, 10*time.Second).Should(BeTrue())

			deriveChild(config.VariableBundle{User: map[string]any{"PORT": 503}})
			s.TestUpdateUserSpec(config.UserSpec{
				Variables: config.VariableBundle{User: map[string]any{"PORT": 502, "IP": "10.0.0.2"}},
			})
			Expect(s.TestTick(ctx)).To(Succeed())

			Expect(conflictLines()).To(HaveLen(1))
			noValueInWarnings("502", "503")
		})
	})

	Describe("User Variables Preservation", func() {
		It("should preserve existing User variables during injection", func() {
			// Set up userSpec with existing User variables
			// Note: PORT uses float64 because JSON deep cloning converts all numbers to float64
			existingUserVars := map[string]any{
				"IP":   "192.168.1.100",
				"PORT": float64(502),
			}

			globalVars := map[string]any{
				"api_endpoint": "https://api.example.com",
			}

			userSpec := config.UserSpec{
				Variables: config.VariableBundle{
					User:   existingUserVars,
					Global: globalVars,
				},
			}

			s.TestUpdateUserSpec(userSpec)

			var capturedSpec config.UserSpec
			testWorker.deriveDesiredStateFunc = func(spec config.UserSpec) (fsmv2.DesiredState, error) {
				capturedSpec = spec

				return &config.DesiredState{BaseDesiredState: config.BaseDesiredState{}}, nil
			}

			err := s.TestTick(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Verify User variables were preserved
			Expect(capturedSpec.Variables.User).To(Equal(existingUserVars))

			Expect(capturedSpec.Variables.Global).To(Equal(globalVars))

			// Verify Internal variables were added
			Expect(capturedSpec.Variables.Internal).ToNot(BeNil())
		})

		It("should handle case where userSpec has no existing variables", func() {
			// userSpec with no Variables set
			userSpec := config.UserSpec{}
			s.TestUpdateUserSpec(userSpec)

			var capturedSpec config.UserSpec
			testWorker.deriveDesiredStateFunc = func(spec config.UserSpec) (fsmv2.DesiredState, error) {
				capturedSpec = spec

				return &config.DesiredState{BaseDesiredState: config.BaseDesiredState{}}, nil
			}

			err := s.TestTick(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Should not panic, variables should be initialized
			Expect(capturedSpec.Variables.User).ToNot(BeNil())
			Expect(capturedSpec.Variables.Global).To(BeNil())
			Expect(capturedSpec.Variables.Internal).ToNot(BeNil())
		})
	})
})
