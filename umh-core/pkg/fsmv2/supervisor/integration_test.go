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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/supervisor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

var _ = Describe("DataFreshness Full Cycle Integration", func() {
	It("should validate complete 4-layer defense lifecycle", func() {
		shutdownRequested := false
		var snapshotTimestamp time.Time

		worker := &mockWorker{
			collectFunc: func(ctx context.Context) (fsmv2.ObservedState, error) {
				return &mockObservedState{CollectedAt: snapshotTimestamp}, nil
			},
		}

		worker.initialState = &mockState{}

		store := newMockStore()

		// Initialize the desired state in the store's internal map
		// This is required because RequestShutdown calls LoadDesired first
		if store.desired["test"] == nil {
			store.desired["test"] = make(map[string]persistence.Document)
		}
		store.desired["test"]["test-worker"] = persistence.Document{
			"id":                "test-worker",
			"ShutdownRequested": false,
		}

		// Initialize the observed state in the store's internal map
		// This is required because LoadObservedTyped reads from this map
		if store.observed["test"] == nil {
			store.observed["test"] = make(map[string]persistence.Document)
		}

		store.loadSnapshot = func(ctx context.Context, workerType string, id string) (*storage.Snapshot, error) {
			// Also update the observed map for LoadObservedTyped
			store.observed["test"]["test-worker"] = persistence.Document{
				"collectedAt": snapshotTimestamp,
			}
			identity := persistence.Document{
				"id":         "test-worker",
				"name":       "Test Worker",
				"workerType": workerType,
			}

			desired := persistence.Document{
				"id":                "test-worker",
				"ShutdownRequested": shutdownRequested,
			}

			observed := persistence.Document{
				"collectedAt": snapshotTimestamp,
			}

			return &storage.Snapshot{
				Identity: identity,
				Desired:  desired,
				Observed: observed,
			}, nil
		}
		store.saveDesired = func(ctx context.Context, workerType string, id string, desired persistence.Document) error {
			// Update both the internal map (for LoadDesired) and the closure variable
			if store.desired[workerType] == nil {
				store.desired[workerType] = make(map[string]persistence.Document)
			}
			store.desired[workerType][id] = desired

			// Update closure variable for test assertions
			if shutdown, ok := desired["ShutdownRequested"].(bool); ok {
				shutdownRequested = shutdown
			}

			return nil
		}

		s := newSupervisorWithWorker(worker, store, supervisor.CollectorHealthConfig{
			StaleThreshold:     5 * time.Second,  // Shortened for test
			Timeout:            10 * time.Second, // Shortened for test
			MaxRestartAttempts: 3,
		})

		ctx := context.Background()

		// Phase 1: Normal operation (fresh data)
		By("Phase 1: Normal operation with fresh data")
		snapshotTimestamp = time.Now()
		err := s.TestTick(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(s.TestGetRestartCount()).To(Equal(0))

		// Phase 2: Stale data (pause FSM, no restart yet)
		By("Phase 2: Stale data (pause FSM)")
		snapshotTimestamp = time.Now().Add(-7 * time.Second) // >5s but <10s
		err = s.TestTick(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(s.TestGetRestartCount()).To(Equal(0), "should not restart for stale (but not timeout) data")

		// Phase 3: Timeout (trigger restarts)
		By("Phase 3: Timeout triggers collector restarts")
		snapshotTimestamp = time.Now().Add(-15 * time.Second) // >10s timeout

		// First restart
		err = s.TestTick(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(s.TestGetRestartCount()).To(Equal(1))

		// Second restart
		err = s.TestTick(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(s.TestGetRestartCount()).To(Equal(2))

		// Third restart
		err = s.TestTick(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(s.TestGetRestartCount()).To(Equal(3))

		// Phase 4: Max restarts exceeded (trigger shutdown)
		By("Phase 4: Max restarts exceeded triggers shutdown")
		err = s.TestTick(ctx)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("collector unresponsive"))
		Expect(shutdownRequested).To(BeTrue(), "shutdown should be requested after max restart attempts")

		// Phase 5: Collector recovers (reset counter)
		By("Phase 5: Collector recovery resets counter")
		s.TestSetRestartCount(2) // Had previous failures
		shutdownRequested = false

		snapshotTimestamp = time.Now() // Fresh data!

		err = s.TestTick(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(s.TestGetRestartCount()).To(Equal(0), "restart counter should reset on recovery")
	})
})
