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
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
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

func (t *TestWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	return &TestObservedState{
		ID:          t.identity.ID,
		CollectedAt: time.Now(),
	}, nil
}

func (t *TestWorker) DeriveDesiredState(spec interface{}) (fsmv2.DesiredState, error) {
	// Convert interface{} to config.UserSpec for the test function
	userSpec, ok := spec.(config.UserSpec)
	if !ok {
		return &config.DesiredState{BaseDesiredState: config.BaseDesiredState{State: "running"}}, nil
	}

	if t.deriveDesiredStateFunc != nil {
		return t.deriveDesiredStateFunc(userSpec)
	}

	return &config.DesiredState{BaseDesiredState: config.BaseDesiredState{State: "running"}}, nil
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

func (t *TestObservedState) GetObservedDesiredState() fsmv2.DesiredState {
	return &config.DesiredState{BaseDesiredState: config.BaseDesiredState{State: "running"}}
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

	return fsmv2.Result[any, any](t, fsmv2.SignalNone, nil, reason)
}

func (t *TestState) String() string {
	return t.name
}

var _ = Describe("Variable Injection", func() {
	var (
		ctx        context.Context
		store      storage.TriangularStoreInterface
		testWorker *TestWorker
		identity   deps.Identity
		s          *supervisor.Supervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState]
		logger     *zap.SugaredLogger
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = zap.NewNop().Sugar()

		basicStore := memory.NewInMemoryStore()

		// Create collections for test worker type
		var err error
		err = basicStore.CreateCollection(ctx, "test_identity", nil)
		Expect(err).ToNot(HaveOccurred())
		err = basicStore.CreateCollection(ctx, "test_desired", nil)
		Expect(err).ToNot(HaveOccurred())
		err = basicStore.CreateCollection(ctx, "test_observed", nil)
		Expect(err).ToNot(HaveOccurred())

		store = storage.NewTriangularStore(basicStore, zap.NewNop().Sugar())

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

	Describe("SetGlobalVariables", func() {
		It("should store global variables correctly", func() {
			globalVars := map[string]any{
				"api_endpoint": "https://api.example.com",
				"cluster_id":   "cluster-123",
				"region":       "us-east-1",
			}

			s.SetGlobalVariables(globalVars)

			// Note: We can't directly access s.globalVars as it's private,
			// but we can verify it works by checking the injection during Tick()
			// This test will fail until implementation is complete
		})

		It("should handle nil global variables", func() {
			s.SetGlobalVariables(nil)

			// Should not panic or error
		})

		It("should handle empty global variables map", func() {
			s.SetGlobalVariables(map[string]any{})

			// Should not panic or error
		})
	})

	Describe("Global Variables Injection in Tick", func() {
		It("should inject Global variables into userSpec.Variables.Global", func() {
			globalVars := map[string]any{
				"api_endpoint": "https://api.example.com",
				"cluster_id":   "cluster-123",
			}

			s.SetGlobalVariables(globalVars)

			// Capture the userSpec passed to DeriveDesiredState
			var capturedSpec config.UserSpec
			testWorker.deriveDesiredStateFunc = func(spec config.UserSpec) (fsmv2.DesiredState, error) {
				capturedSpec = spec

				return &config.DesiredState{BaseDesiredState: config.BaseDesiredState{State: "running"}}, nil
			}

			err := s.TestTick(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Verify Global variables were injected
			Expect(capturedSpec.Variables.Global).To(Equal(globalVars))
		})

		It("should inject empty Global map when no global variables set", func() {
			// Don't call SetGlobalVariables, so globalVars should be nil or empty

			var capturedSpec config.UserSpec
			testWorker.deriveDesiredStateFunc = func(spec config.UserSpec) (fsmv2.DesiredState, error) {
				capturedSpec = spec

				return &config.DesiredState{BaseDesiredState: config.BaseDesiredState{State: "running"}}, nil
			}

			err := s.TestTick(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Verify Global variables are either nil or empty
			if capturedSpec.Variables.Global != nil {
				Expect(capturedSpec.Variables.Global).To(BeEmpty())
			}
		})
	})

	Describe("Internal Variables Injection in Tick", func() {
		It("should inject Internal variables with id, _created_at, and empty parent_id for root supervisor", func() {
			var capturedSpec config.UserSpec
			testWorker.deriveDesiredStateFunc = func(spec config.UserSpec) (fsmv2.DesiredState, error) {
				capturedSpec = spec

				return &config.DesiredState{BaseDesiredState: config.BaseDesiredState{State: "running"}}, nil
			}

			err := s.TestTick(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Verify Internal variables were injected
			Expect(capturedSpec.Variables.Internal).ToNot(BeNil())
			Expect(capturedSpec.Variables.Internal[supervisor.FieldID]).To(Equal(identity.ID))
			Expect(capturedSpec.Variables.Internal[storage.FieldCreatedAt]).ToNot(BeNil())

			createdAt, ok := capturedSpec.Variables.Internal[storage.FieldCreatedAt].(time.Time)
			Expect(ok).To(BeTrue())
			Expect(createdAt).ToNot(BeZero())

			// For root supervisor, parent_id should be empty
			parentID, exists := capturedSpec.Variables.Internal[supervisor.FieldParentID]
			if exists {
				Expect(parentID).To(Equal(""))
			}
		})

	})

	Describe("Variable Inheritance from Parent to Child", func() {
		It("should inherit parent User variables to child, with child vars overriding parent", func() {
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
					BaseDesiredState: config.BaseDesiredState{State: "running"},
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

		It("should allow child User variables to override parent User variables", func() {
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

			// Create a child spec that overrides PORT
			childUserSpec := config.UserSpec{
				Variables: config.VariableBundle{
					User: map[string]any{
						"PORT":      503, // Override parent's PORT
						"DEVICE_ID": "child-device",
					},
				},
			}

			testWorker.deriveDesiredStateFunc = func(spec config.UserSpec) (fsmv2.DesiredState, error) {
				return &config.DesiredState{
					BaseDesiredState: config.BaseDesiredState{State: "running"},
					ChildrenSpecs: []config.ChildSpec{
						{
							Name:       "override-child",
							WorkerType: "test",
							UserSpec:   childUserSpec,
						},
					},
				}, nil
			}

			err := s.TestTick(ctx)
			Expect(err).ToNot(HaveOccurred())

			children := s.GetChildren()
			Expect(children).To(HaveKey("override-child"))

			child := children["override-child"]
			capturedChildSpec := child.TestGetUserSpec()

			// Verify: IP inherited, PORT overridden by child, DEVICE_ID from child
			Expect(capturedChildSpec.Variables.User).To(HaveKeyWithValue("IP", "192.168.1.100"))
			Expect(capturedChildSpec.Variables.User).To(HaveKeyWithValue("PORT", 503)) // Child's value
			Expect(capturedChildSpec.Variables.User).To(HaveKeyWithValue("DEVICE_ID", "child-device"))
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

			userSpec := config.UserSpec{
				Variables: config.VariableBundle{
					User: existingUserVars,
				},
			}

			s.TestUpdateUserSpec(userSpec)

			globalVars := map[string]any{
				"api_endpoint": "https://api.example.com",
			}
			s.SetGlobalVariables(globalVars)

			var capturedSpec config.UserSpec
			testWorker.deriveDesiredStateFunc = func(spec config.UserSpec) (fsmv2.DesiredState, error) {
				capturedSpec = spec

				return &config.DesiredState{BaseDesiredState: config.BaseDesiredState{State: "running"}}, nil
			}

			err := s.TestTick(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Verify User variables were preserved
			Expect(capturedSpec.Variables.User).To(Equal(existingUserVars))

			// Verify Global variables were added
			Expect(capturedSpec.Variables.Global).To(Equal(globalVars))

			// Verify Internal variables were added
			Expect(capturedSpec.Variables.Internal).ToNot(BeNil())
		})

		It("should handle case where userSpec has no existing variables", func() {
			// userSpec with no Variables set
			userSpec := config.UserSpec{}
			s.TestUpdateUserSpec(userSpec)

			globalVars := map[string]any{
				"api_endpoint": "https://api.example.com",
			}
			s.SetGlobalVariables(globalVars)

			var capturedSpec config.UserSpec
			testWorker.deriveDesiredStateFunc = func(spec config.UserSpec) (fsmv2.DesiredState, error) {
				capturedSpec = spec

				return &config.DesiredState{BaseDesiredState: config.BaseDesiredState{State: "running"}}, nil
			}

			err := s.TestTick(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Should not panic, variables should be initialized
			Expect(capturedSpec.Variables.Global).To(Equal(globalVars))
			Expect(capturedSpec.Variables.Internal).ToNot(BeNil())
		})
	})
})
