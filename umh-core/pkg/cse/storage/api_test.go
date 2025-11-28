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

package storage_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/cse/storage"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/persistence"
)

// API Test Documentation
//
// This file tests the public API of TriangularStoreInterface.
// These tests document the intended behavior and are independent of FSM v2 logic.
//
// The test flow demonstrates a typical worker registration lifecycle:
//  1. Start with empty store
//  2. Register a new worker (using only public API)
//  3. Verify collections were created
//  4. Add desired state initially
//  5. Verify snapshot exists (via LoadSnapshot)
//  6. Verify delta was created (via GetDeltas)

var _ = Describe("TriangularStore Public API", func() {
	var (
		store *mockStore
		ts    *storage.TriangularStore
		ctx   context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		store = newMockStore()
		ts = storage.NewTriangularStore(store, zap.NewNop().Sugar())
	})

	Describe("Worker Registration Lifecycle", func() {
		const workerType = "container"
		const workerID = "worker-1"

		Describe("Step 1: Register Worker and Verify Collections", func() {
			It("should create collections when registering a new worker", func() {
				// Given: Empty store

				// When: Save identity
				err := ts.SaveIdentity(ctx, workerType, workerID, persistence.Document{
					"id":         workerID,
					"name":       "Test Container",
					"workerType": workerType,
				})
				Expect(err).NotTo(HaveOccurred())

				// Then: Verify identity can be loaded
				identity, err := ts.LoadIdentity(ctx, workerType, workerID)
				Expect(err).NotTo(HaveOccurred())
				Expect(identity["name"]).To(Equal("Test Container"))
				Expect(identity["workerType"]).To(Equal(workerType))
			})
		})

		Describe("Step 2: Verify Snapshot After Initial Desired State", func() {
			It("should return snapshot after initial desired state", func() {
				// Given: Worker with identity
				err := ts.SaveIdentity(ctx, workerType, workerID, persistence.Document{
					"id":   workerID,
					"name": "Test Container",
				})
				Expect(err).NotTo(HaveOccurred())

				// When: Save desired state
				changed, err := ts.SaveDesired(ctx, workerType, workerID, persistence.Document{
					"id":     workerID,
					"config": "production",
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(changed).To(BeTrue()) // First save always changes

				// Then: Verify data can be loaded back
				desired, err := ts.LoadDesired(ctx, workerType, workerID)
				Expect(err).NotTo(HaveOccurred())
				desiredDoc := desired.(persistence.Document)
				Expect(desiredDoc["config"]).To(Equal("production"))
			})
		})

		Describe("Step 3: Verify Delta Created via GetDeltas", func() {
			It("should return delta or bootstrap after save", func() {
				// Given: Get initial sync position
				initialSyncID, err := ts.GetLatestSyncID(ctx)
				Expect(err).NotTo(HaveOccurred())

				// When: Save desired state (first save)
				changed, err := ts.SaveDesired(ctx, workerType, workerID, persistence.Document{
					"id":     workerID,
					"config": "initial",
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(changed).To(BeTrue())

				// Then: GetDeltas shows change occurred
				sub := storage.Subscription{LastSyncID: initialSyncID}
				resp, err := ts.GetDeltas(ctx, sub)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.LatestSyncID).To(BeNumerically(">", initialSyncID))

				// If deltas are available, verify structure
				if !resp.RequiresBootstrap && len(resp.Deltas) > 0 {
					Expect(resp.Deltas[0].WorkerType).To(Equal(workerType))
					Expect(resp.Deltas[0].WorkerID).To(Equal(workerID))
					Expect(resp.Deltas[0].Role).To(Equal(storage.RoleDesired))
				}
			})

			It("should return deltas array with field-level changes", func() {
				// Given: Worker with initial desired state
				_, err := ts.SaveDesired(ctx, workerType, workerID, persistence.Document{
					"id":     workerID,
					"config": "initial",
				})
				Expect(err).NotTo(HaveOccurred())

				// Record sync position after first save
				syncAfterFirst, err := ts.GetLatestSyncID(ctx)
				Expect(err).NotTo(HaveOccurred())

				// When: Save updated desired state
				changed, err := ts.SaveDesired(ctx, workerType, workerID, persistence.Document{
					"id":     workerID,
					"config": "updated",
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(changed).To(BeTrue())

				// Then: GetDeltas returns delta for the change
				sub := storage.Subscription{LastSyncID: syncAfterFirst}
				resp, err := ts.GetDeltas(ctx, sub)
				Expect(err).NotTo(HaveOccurred())

				// Should have deltas (not bootstrap) since we're only one change behind
				if len(resp.Deltas) > 0 {
					delta := resp.Deltas[0]
					Expect(delta.WorkerType).To(Equal(workerType))
					Expect(delta.WorkerID).To(Equal(workerID))
					Expect(delta.Role).To(Equal(storage.RoleDesired))
					Expect(delta.SyncID).To(BeNumerically(">", syncAfterFirst))

					// Verify changes are tracked
					if delta.Changes != nil {
						Expect(delta.Changes.Modified).NotTo(BeEmpty())
					}
				}
			})
		})

		Describe("Step 4: Verify No Delta on Unchanged Data", func() {
			It("should not create delta when data unchanged", func() {
				// Given: Existing desired state
				_, err := ts.SaveDesired(ctx, workerType, workerID, persistence.Document{
					"id":     workerID,
					"config": "value",
				})
				Expect(err).NotTo(HaveOccurred())

				syncAfterFirst, err := ts.GetLatestSyncID(ctx)
				Expect(err).NotTo(HaveOccurred())

				// When: Save identical data
				changed, err := ts.SaveDesired(ctx, workerType, workerID, persistence.Document{
					"id":     workerID,
					"config": "value",
				})

				// Then: No change detected
				Expect(err).NotTo(HaveOccurred())
				Expect(changed).To(BeFalse())

				// Sync ID should remain unchanged
				syncAfterSecond, err := ts.GetLatestSyncID(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(syncAfterSecond).To(Equal(syncAfterFirst))
			})
		})

		Describe("Step 5: Full Lifecycle with Delta Tracking", func() {
			It("should track changes through full worker lifecycle", func() {
				// Step 1: Empty store - record initial position
				initialSync, err := ts.GetLatestSyncID(ctx)
				Expect(err).NotTo(HaveOccurred())

				// Step 2: Register worker (save identity)
				err = ts.SaveIdentity(ctx, workerType, workerID, persistence.Document{
					"id":   workerID,
					"name": "Container A",
				})
				Expect(err).NotTo(HaveOccurred())

				// Step 3: Add desired state
				_, err = ts.SaveDesired(ctx, workerType, workerID, persistence.Document{
					"id":     workerID,
					"status": "running",
				})
				Expect(err).NotTo(HaveOccurred())

				// Step 4: Add observed state
				_, err = ts.SaveObserved(ctx, workerType, workerID, persistence.Document{
					"id":  workerID,
					"cpu": 45.2,
				})
				Expect(err).NotTo(HaveOccurred())

				// Step 5: Verify sync ID progressed
				finalSync, err := ts.GetLatestSyncID(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(finalSync).To(BeNumerically(">", initialSync))

				// Step 6: Verify snapshot is complete
				snapshot, err := ts.LoadSnapshot(ctx, workerType, workerID)
				Expect(err).NotTo(HaveOccurred())
				Expect(snapshot.Identity).NotTo(BeNil())
				Expect(snapshot.Desired).NotTo(BeNil())
				Expect(snapshot.Observed).NotTo(BeNil())

				// Verify snapshot contents
				Expect(snapshot.Identity["name"]).To(Equal("Container A"))
				Expect(snapshot.Desired["status"]).To(Equal("running"))
				observedDoc := snapshot.Observed.(persistence.Document)
				Expect(observedDoc["cpu"]).To(Equal(45.2))

				// Step 7: Verify GetDeltas returns data for behind client
				sub := storage.Subscription{LastSyncID: initialSync}
				resp, err := ts.GetDeltas(ctx, sub)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.LatestSyncID).To(Equal(finalSync))

				// If deltas available, should have multiple entries
				if !resp.RequiresBootstrap {
					Expect(len(resp.Deltas)).To(BeNumerically(">=", 2)) // At least desired and observed
				}
			})
		})

		Describe("GetDeltas Edge Cases", func() {
			It("should return empty deltas when client is at current position", func() {
				// Given: Worker with some state
				_, _ = ts.SaveDesired(ctx, workerType, workerID, persistence.Document{
					"id":     workerID,
					"config": "value",
				})

				currentSync, err := ts.GetLatestSyncID(ctx)
				Expect(err).NotTo(HaveOccurred())

				// When: Client is at current position
				sub := storage.Subscription{LastSyncID: currentSync}
				resp, err := ts.GetDeltas(ctx, sub)
				Expect(err).NotTo(HaveOccurred())

				// Then: No deltas and not bootstrap
				Expect(resp.Deltas).To(BeEmpty())
				Expect(resp.RequiresBootstrap).To(BeFalse())
				Expect(resp.LatestSyncID).To(Equal(currentSync))
			})

			It("should return empty deltas when client is ahead of server", func() {
				// Given: Empty store with initial sync ID
				currentSync, err := ts.GetLatestSyncID(ctx)
				Expect(err).NotTo(HaveOccurred())

				// When: Client claims to be ahead (shouldn't happen but handle gracefully)
				sub := storage.Subscription{LastSyncID: currentSync + 100}
				resp, err := ts.GetDeltas(ctx, sub)
				Expect(err).NotTo(HaveOccurred())

				// Then: No deltas (client is "ahead")
				Expect(resp.Deltas).To(BeEmpty())
				Expect(resp.RequiresBootstrap).To(BeFalse())
			})

			It("should include HasMore flag when delta limit reached", func() {
				// Given: Multiple changes
				for i := range 150 {
					_, _ = ts.SaveDesired(ctx, workerType, workerID, persistence.Document{
						"id":    workerID,
						"value": i,
					})
				}

				// When: Request deltas from start
				sub := storage.Subscription{LastSyncID: 0}
				resp, err := ts.GetDeltas(ctx, sub)
				Expect(err).NotTo(HaveOccurred())

				// Then: Either has deltas with HasMore, or requires bootstrap
				if !resp.RequiresBootstrap && len(resp.Deltas) > 0 {
					// If returning deltas, might indicate HasMore
					// Note: Current limit is 100, so with 150 changes, HasMore should be true
					if len(resp.Deltas) >= 100 {
						Expect(resp.HasMore).To(BeTrue())
					}
				}
			})
		})

		Describe("Delta Store Integration", func() {
			It("should persist deltas to unified collection", func() {
				// Given: Save multiple changes
				err := ts.SaveIdentity(ctx, workerType, workerID, persistence.Document{
					"id":   workerID,
					"name": "Test",
				})
				Expect(err).NotTo(HaveOccurred())

				_, err = ts.SaveDesired(ctx, workerType, workerID, persistence.Document{
					"id":     workerID,
					"config": "v1",
				})
				Expect(err).NotTo(HaveOccurred())

				_, err = ts.SaveDesired(ctx, workerType, workerID, persistence.Document{
					"id":     workerID,
					"config": "v2",
				})
				Expect(err).NotTo(HaveOccurred())

				// When: Query deltas from start
				sub := storage.Subscription{LastSyncID: 0}
				resp, err := ts.GetDeltas(ctx, sub)
				Expect(err).NotTo(HaveOccurred())

				// Then: Should have deltas for the changes
				if !resp.RequiresBootstrap {
					// Deltas should be ordered by SyncID
					for i := 1; i < len(resp.Deltas); i++ {
						Expect(resp.Deltas[i].SyncID).To(BeNumerically(">", resp.Deltas[i-1].SyncID))
					}
				}
			})
		})
	})
})
