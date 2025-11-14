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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

var _ = Describe("Tick with Data Freshness", func() {
	var (
		s            *supervisor.Supervisor
		initialState *mockState
		nextState    *mockState
		store        *mockStore
	)

	BeforeEach(func() {
		initialState = &mockState{}
		nextState = &mockState{}
		initialState.nextState = nextState
	})

	Context("when data is stale", func() {
		It("should skip state transition", func() {
			store = newMockStore()
			store.loadSnapshot = func(ctx context.Context, workerType string, id string) (*storage.Snapshot, error) {
				identity := mockIdentity()

				return &storage.Snapshot{
					Identity: persistence.Document{
						"id":         identity.ID,
						"name":       identity.Name,
						"workerType": identity.WorkerType,
					},
					Desired: persistence.Document{},
					Observed: persistence.Document{
						"collectedAt": time.Now().Add(-15 * time.Second),
					},
				}, nil
			}

			s = newSupervisorWithWorker(&mockWorker{initialState: initialState}, store, supervisor.CollectorHealthConfig{
				StaleThreshold: 10 * time.Second,
			})

			err := s.TestTick(context.Background())
			Expect(err).ToNot(HaveOccurred())

			Expect(s.GetCurrentState()).To(Equal(initialState.String()))
		})
	})

	Context("when data is fresh", func() {
		It("should progress with state transition", func() {
			store = newMockStore()
			store.loadSnapshot = func(ctx context.Context, workerType string, id string) (*storage.Snapshot, error) {
				identity := mockIdentity()

				return &storage.Snapshot{
					Identity: persistence.Document{
						"id":         identity.ID,
						"name":       identity.Name,
						"workerType": identity.WorkerType,
					},
					Desired: persistence.Document{},
					Observed: persistence.Document{
						"collectedAt": time.Now(),
					},
				}, nil
			}

			s = newSupervisorWithWorker(&mockWorker{initialState: initialState}, store, supervisor.CollectorHealthConfig{
				StaleThreshold: 10 * time.Second,
			})

			err := s.TestTick(context.Background())
			Expect(err).ToNot(HaveOccurred())

			Expect(s.GetCurrentState()).To(Equal(nextState.String()))
		})
	})

	Context("when ObservedState is nil (Invariant I16)", func() {
		It("should panic with clear error message about nil ObservedState", func() {
			identity := mockIdentity()
			worker := &mockWorker{
				initialState: initialState,
				observed:     &mockObservedState{CollectedAt: time.Now()},
			}

			store = newMockStore()
			store.loadSnapshot = func(ctx context.Context, workerType string, id string) (*storage.Snapshot, error) {

				return &storage.Snapshot{
					Identity: persistence.Document{
						"id":         identity.ID,
						"name":       identity.Name,
						"workerType": identity.WorkerType,
					},
					Desired:  persistence.Document{},
					Observed: nil,
				}, nil
			}

			s = newSupervisorWithWorker(worker, store, supervisor.CollectorHealthConfig{
				StaleThreshold: 10 * time.Second,
			})

			Expect(func() {
				_ = s.TestTick(context.Background())
			}).Should(PanicWith(MatchRegexp("Invariant I16 violated.*nil ObservedState")))
		})
	})
})
