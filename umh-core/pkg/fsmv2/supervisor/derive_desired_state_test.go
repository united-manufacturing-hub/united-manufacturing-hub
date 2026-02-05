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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

var _ = Describe("DeriveDesiredState saves to store", func() {
	var (
		store      *mockStore
		workerType string
		workerID   string
		ctx        context.Context
	)

	BeforeEach(func() {
		workerType = "test"
		workerID = "test-worker"
		ctx = context.Background()
		store = newMockStore()
	})

	It("should save derived state to store during tick", func() {
		// Track SaveDesired calls
		saveDesiredCount := 0
		var lastSavedDesired persistence.Document

		store.saveDesired = func(ctx context.Context, wt string, id string, desired persistence.Document) error {
			saveDesiredCount++
			lastSavedDesired = desired
			if store.desired[wt] == nil {
				store.desired[wt] = make(map[string]persistence.Document)
			}
			store.desired[wt][id] = desired

			return nil
		}

		// Create a simple worker
		worker := &mockWorker{
			observed: &mockObservedState{
				ID:          workerID,
				CollectedAt: time.Now(),
				Desired:     &mockDesiredState{},
			},
		}

		// Create supervisor
		s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
			WorkerType: workerType,
			Store:      store,
			Logger:     zap.NewNop().Sugar(),
			CollectorHealth: supervisor.CollectorHealthConfig{
				StaleThreshold: 10 * time.Second,
			},
		})

		identity := deps.Identity{
			ID:         workerID,
			Name:       "Test Worker",
			WorkerType: workerType,
		}

		// AddWorker calls DeriveDesiredState and saves initial desired state
		err := s.AddWorker(identity, worker)
		Expect(err).ToNot(HaveOccurred())

		initialSaveCount := saveDesiredCount

		// Tick should call DeriveDesiredState and save the derived state
		err = s.TestTick(ctx)
		Expect(err).ToNot(HaveOccurred())

		// Verify SaveDesired was called during tick
		Expect(saveDesiredCount).To(BeNumerically(">", initialSaveCount),
			"SaveDesired should be called during tick to save derived desired state")

		// Verify the saved document has the worker ID
		Expect(lastSavedDesired).ToNot(BeNil())
		Expect(lastSavedDesired["id"]).To(Equal(workerID))
	})

	It("should call SaveDesired before LoadSnapshot during tick", func() {
		// Track operation order
		operations := []string{}

		store.saveDesired = func(ctx context.Context, wt string, id string, desired persistence.Document) error {
			operations = append(operations, "SaveDesired")
			if store.desired[wt] == nil {
				store.desired[wt] = make(map[string]persistence.Document)
			}
			store.desired[wt][id] = desired

			return nil
		}

		store.loadSnapshot = func(ctx context.Context, wt string, id string) (*storage.Snapshot, error) {
			operations = append(operations, "LoadSnapshot")
			identity := persistence.Document{
				"id":         id,
				"name":       "Test Worker",
				"workerType": wt,
			}
			desired := persistence.Document{}
			if store.desired[wt] != nil && store.desired[wt][id] != nil {
				desired = store.desired[wt][id]
			}
			observed := persistence.Document{
				"id":          id,
				"collectedAt": time.Now(),
			}

			return &storage.Snapshot{
				Identity: identity,
				Desired:  desired,
				Observed: observed,
			}, nil
		}

		worker := &mockWorker{
			observed: &mockObservedState{
				ID:          workerID,
				CollectedAt: time.Now(),
				Desired:     &mockDesiredState{},
			},
		}

		s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
			WorkerType: workerType,
			Store:      store,
			Logger:     zap.NewNop().Sugar(),
			CollectorHealth: supervisor.CollectorHealthConfig{
				StaleThreshold: 10 * time.Second,
			},
		})

		identity := deps.Identity{
			ID:         workerID,
			Name:       "Test Worker",
			WorkerType: workerType,
		}

		err := s.AddWorker(identity, worker)
		Expect(err).ToNot(HaveOccurred())

		// Clear operations after AddWorker
		operations = []string{}

		err = s.TestTick(ctx)
		Expect(err).ToNot(HaveOccurred())

		// Verify both operations occurred
		Expect(operations).To(ContainElement("SaveDesired"))
		Expect(operations).To(ContainElement("LoadSnapshot"))

		// Find positions
		saveIdx := -1
		loadIdx := -1
		for i, op := range operations {
			if op == "SaveDesired" && saveIdx == -1 {
				saveIdx = i
			}
			if op == "LoadSnapshot" && loadIdx == -1 {
				loadIdx = i
			}
		}

		Expect(saveIdx).To(BeNumerically(">=", 0), "SaveDesired should be called")
		Expect(loadIdx).To(BeNumerically(">=", 0), "LoadSnapshot should be called")
		Expect(saveIdx).To(BeNumerically("<", loadIdx),
			"SaveDesired (index %d) should be called BEFORE LoadSnapshot (index %d)", saveIdx, loadIdx)
	})

	It("should save derived state with correct state field", func() {
		var savedState interface{}

		store.saveDesired = func(ctx context.Context, wt string, id string, desired persistence.Document) error {
			savedState = desired["state"]
			if store.desired[wt] == nil {
				store.desired[wt] = make(map[string]persistence.Document)
			}
			store.desired[wt][id] = desired

			return nil
		}

		// Provide a loadSnapshot that returns proper observed state
		store.loadSnapshot = func(ctx context.Context, wt string, id string) (*storage.Snapshot, error) {
			identity := persistence.Document{
				"id":         id,
				"name":       "Test Worker",
				"workerType": wt,
			}
			desired := persistence.Document{}
			if store.desired[wt] != nil && store.desired[wt][id] != nil {
				desired = store.desired[wt][id]
			}
			observed := persistence.Document{
				"id":          id,
				"collectedAt": time.Now(),
			}

			return &storage.Snapshot{
				Identity: identity,
				Desired:  desired,
				Observed: observed,
			}, nil
		}

		// Create a simple worker that returns "running" state
		worker := &mockWorker{
			observed: &mockObservedState{
				ID:          workerID,
				CollectedAt: time.Now(),
				Desired:     &mockDesiredState{},
			},
		}

		s := supervisor.NewSupervisor[*supervisor.TestObservedState, *supervisor.TestDesiredState](supervisor.Config{
			WorkerType: workerType,
			Store:      store,
			Logger:     zap.NewNop().Sugar(),
			CollectorHealth: supervisor.CollectorHealthConfig{
				StaleThreshold: 10 * time.Second,
			},
		})

		identity := deps.Identity{
			ID:         workerID,
			Name:       "Test Worker",
			WorkerType: workerType,
		}

		err := s.AddWorker(identity, worker)
		Expect(err).ToNot(HaveOccurred())

		err = s.TestTick(ctx)
		Expect(err).ToNot(HaveOccurred())

		// mockWorker.DeriveDesiredState returns config.DesiredState{State: "running"}
		Expect(savedState).To(Equal("running"))
	})
})
