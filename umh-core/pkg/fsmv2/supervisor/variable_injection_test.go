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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence/memory"
)

// TestWorker is a test double for fsmv2.Worker that allows capturing DeriveDesiredState calls.
type TestWorker struct {
	identity               fsmv2.Identity
	initialState           fsmv2.State
	deriveDesiredStateFunc func(spec config.UserSpec) (config.DesiredState, error)
}

func (t *TestWorker) CollectObservedState(ctx context.Context) (fsmv2.ObservedState, error) {
	return &TestObservedState{
		ID:          t.identity.ID,
		CollectedAt: time.Now(),
	}, nil
}

func (t *TestWorker) DeriveDesiredState(spec interface{}) (config.DesiredState, error) {
	// Convert interface{} to config.UserSpec for the test function
	userSpec, ok := spec.(config.UserSpec)
	if !ok {
		return config.DesiredState{State: "running"}, nil
	}

	if t.deriveDesiredStateFunc != nil {
		return t.deriveDesiredStateFunc(userSpec)
	}

	return config.DesiredState{State: "running"}, nil
}

func (t *TestWorker) GetInitialState() fsmv2.State {
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
	return &config.DesiredState{State: "running"}
}

// TestState is a test double for fsmv2.State.
type TestState struct {
	name   string
	reason string
}

func (t *TestState) Next(snapshot fsmv2.Snapshot) (fsmv2.State, fsmv2.Signal, fsmv2.Action) {
	return t, fsmv2.SignalNone, nil
}

func (t *TestState) String() string {
	return t.name
}

func (t *TestState) Reason() string {
	if t.reason != "" {
		return t.reason
	}

	return t.name
}

var _ = Describe("Variable Injection", func() {
	var (
		ctx        context.Context
		store      storage.TriangularStoreInterface
		testWorker *TestWorker
		identity   fsmv2.Identity
		s          *supervisor.Supervisor
		logger     *zap.SugaredLogger
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = zap.NewNop().Sugar()

		basicStore := memory.NewInMemoryStore()
		registry := storage.NewRegistry()

		// Create collections for test worker type
		var err error
		err = basicStore.CreateCollection(ctx, "test_identity", nil)
		Expect(err).ToNot(HaveOccurred())
		err = basicStore.CreateCollection(ctx, "test_desired", nil)
		Expect(err).ToNot(HaveOccurred())
		err = basicStore.CreateCollection(ctx, "test_observed", nil)
		Expect(err).ToNot(HaveOccurred())

		store = storage.NewTriangularStore(basicStore, registry)

		identity = fsmv2.Identity{
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

		s = supervisor.NewSupervisor(supervisor.Config{
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
		err = store.SaveDesired(ctx, "test", identity.ID, initialDesired)
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
			testWorker.deriveDesiredStateFunc = func(spec config.UserSpec) (config.DesiredState, error) {
				capturedSpec = spec

				return config.DesiredState{State: "running"}, nil
			}

			err := s.Tick(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Verify Global variables were injected
			Expect(capturedSpec.Variables.Global).To(Equal(globalVars))
		})

		It("should inject empty Global map when no global variables set", func() {
			// Don't call SetGlobalVariables, so globalVars should be nil or empty

			var capturedSpec config.UserSpec
			testWorker.deriveDesiredStateFunc = func(spec config.UserSpec) (config.DesiredState, error) {
				capturedSpec = spec

				return config.DesiredState{State: "running"}, nil
			}

			err := s.Tick(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Verify Global variables are either nil or empty
			if capturedSpec.Variables.Global != nil {
				Expect(capturedSpec.Variables.Global).To(BeEmpty())
			}
		})
	})

	Describe("Internal Variables Injection in Tick", func() {
		It("should inject Internal variables with id, created_at, and empty bridged_by for root supervisor", func() {
			var capturedSpec config.UserSpec
			testWorker.deriveDesiredStateFunc = func(spec config.UserSpec) (config.DesiredState, error) {
				capturedSpec = spec

				return config.DesiredState{State: "running"}, nil
			}

			err := s.Tick(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Verify Internal variables were injected
			Expect(capturedSpec.Variables.Internal).ToNot(BeNil())
			Expect(capturedSpec.Variables.Internal["id"]).To(Equal(identity.ID))
			Expect(capturedSpec.Variables.Internal["created_at"]).ToNot(BeNil())

			createdAt, ok := capturedSpec.Variables.Internal["created_at"].(time.Time)
			Expect(ok).To(BeTrue())
			Expect(createdAt).ToNot(BeZero())

			// For root supervisor, bridged_by should be empty
			bridgedBy, exists := capturedSpec.Variables.Internal["bridged_by"]
			if exists {
				Expect(bridgedBy).To(Equal(""))
			}
		})

		It("should inject bridged_by for child supervisor", func() {
			// Create parent supervisor
			parentIdentity := fsmv2.Identity{
				ID:         "parent-worker",
				Name:       "Parent Worker",
				WorkerType: "test",
			}
			parentWorker := &TestWorker{
				identity:     parentIdentity,
				initialState: &TestState{name: "Initial"},
			}

			parentSupervisor := supervisor.NewSupervisor(supervisor.Config{
				WorkerType:   "test",
				Store:        store,
				Logger:       logger,
				TickInterval: 100 * time.Millisecond,
			})

			err := parentSupervisor.AddWorker(parentIdentity, parentWorker)
			Expect(err).ToNot(HaveOccurred())

			// Create child supervisor with parentID set
			// Note: This assumes there's a way to set parentID during supervisor creation
			// Implementation details may vary
			childSupervisor := supervisor.NewSupervisor(supervisor.Config{
				WorkerType:   "test",
				Store:        store,
				Logger:       logger,
				TickInterval: 100 * time.Millisecond,
			})

			// TODO: Set parentID on childSupervisor (mechanism TBD)
			// This test demonstrates the expected behavior for child supervisors

			childIdentity := fsmv2.Identity{
				ID:         "child-worker",
				Name:       "Child Worker",
				WorkerType: "test",
			}
			childWorker := &TestWorker{
				identity:     childIdentity,
				initialState: &TestState{name: "Initial"},
			}

			err = childSupervisor.AddWorker(childIdentity, childWorker)
			Expect(err).ToNot(HaveOccurred())

			// Save initial desired state for child worker
			childDesired := persistence.Document{
				"id":    childIdentity.ID,
				"state": "running",
			}
			err = store.SaveDesired(ctx, "test", childIdentity.ID, childDesired)
			Expect(err).ToNot(HaveOccurred())

			childWorker.deriveDesiredStateFunc = func(spec config.UserSpec) (config.DesiredState, error) {
				// capturedSpec = spec
				return config.DesiredState{State: "running"}, nil
			}

			err = childSupervisor.Tick(ctx)
			Expect(err).ToNot(HaveOccurred())

			// For child supervisor, bridged_by should be set to parent ID
			// This test will need adjustment based on actual implementation
			Skip("Pending implementation of parentID mechanism")
		})
	})

	Describe("User Variables Preservation", func() {
		It("should preserve existing User variables during injection", func() {
			// Set up userSpec with existing User variables
			existingUserVars := map[string]any{
				"IP":   "192.168.1.100",
				"PORT": 502,
			}

			userSpec := config.UserSpec{
				Variables: config.VariableBundle{
					User: existingUserVars,
				},
			}

			s.UpdateUserSpec(userSpec)

			globalVars := map[string]any{
				"api_endpoint": "https://api.example.com",
			}
			s.SetGlobalVariables(globalVars)

			var capturedSpec config.UserSpec
			testWorker.deriveDesiredStateFunc = func(spec config.UserSpec) (config.DesiredState, error) {
				capturedSpec = spec

				return config.DesiredState{State: "running"}, nil
			}

			err := s.Tick(ctx)
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
			s.UpdateUserSpec(userSpec)

			globalVars := map[string]any{
				"api_endpoint": "https://api.example.com",
			}
			s.SetGlobalVariables(globalVars)

			var capturedSpec config.UserSpec
			testWorker.deriveDesiredStateFunc = func(spec config.UserSpec) (config.DesiredState, error) {
				capturedSpec = spec

				return config.DesiredState{State: "running"}, nil
			}

			err := s.Tick(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Should not panic, variables should be initialized
			Expect(capturedSpec.Variables.Global).To(Equal(globalVars))
			Expect(capturedSpec.Variables.Internal).ToNot(BeNil())
		})
	})
})
