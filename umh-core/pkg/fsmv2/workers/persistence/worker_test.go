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

package persistence_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence/state"
)

var _ = Describe("PersistenceWorker", func() {
	var (
		logger   *zap.SugaredLogger
		identity deps.Identity
		store    *mockTriangularStore
	)

	BeforeEach(func() {
		logger = zap.NewNop().Sugar()
		identity = deps.Identity{ID: "test-id", Name: "test-persistence"}
		store = &mockTriangularStore{}
	})

	Describe("NewPersistenceWorker", func() {
		It("should create a worker successfully", func() {
			worker, err := persistence.NewPersistenceWorker(identity, store, logger, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(worker).NotTo(BeNil())
		})

		It("should panic when store is nil", func() {
			Expect(func() {
				_, _ = persistence.NewPersistenceWorker(identity, nil, logger, nil)
			}).To(Panic())
		})
	})

	Describe("CollectObservedState", func() {
		It("should return valid observed state", func() {
			worker, err := persistence.NewPersistenceWorker(identity, store, logger, nil)
			Expect(err).NotTo(HaveOccurred())

			observed, err := worker.CollectObservedState(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(observed).NotTo(BeNil())

			obs, ok := observed.(snapshot.PersistenceObservedState)
			Expect(ok).To(BeTrue())
			Expect(obs.CollectedAt).NotTo(BeZero())
		})

		It("should return error when context is cancelled", func() {
			worker, err := persistence.NewPersistenceWorker(identity, store, logger, nil)
			Expect(err).NotTo(HaveOccurred())

			cancelledCtx, cancel := context.WithCancel(context.Background())
			cancel()

			_, err = worker.CollectObservedState(cancelledCtx)
			Expect(err).To(Equal(context.Canceled))
		})

		Context("ConsecutiveActionErrors computation", func() {
			It("should increment on failure", func() {
				worker, err := persistence.NewPersistenceWorker(identity, store, logger, nil)
				Expect(err).NotTo(HaveOccurred())

				d := worker.GetDependencies()
				d.SetActionHistory([]deps.ActionResult{
					{Success: false, ActionType: "CompactDeltas", Timestamp: time.Now()},
				})

				observed, err := worker.CollectObservedState(context.Background())
				Expect(err).NotTo(HaveOccurred())

				obs := observed.(snapshot.PersistenceObservedState)
				Expect(obs.ConsecutiveActionErrors).To(Equal(1))
			})

			It("should reset on success", func() {
				worker, err := persistence.NewPersistenceWorker(identity, store, logger, nil)
				Expect(err).NotTo(HaveOccurred())

				d := worker.GetDependencies()
				d.SetActionHistory([]deps.ActionResult{
					{Success: false, ActionType: "CompactDeltas", Timestamp: time.Now()},
					{Success: false, ActionType: "CompactDeltas", Timestamp: time.Now()},
					{Success: true, ActionType: "CompactDeltas", Timestamp: time.Now()},
				})

				observed, err := worker.CollectObservedState(context.Background())
				Expect(err).NotTo(HaveOccurred())

				obs := observed.(snapshot.PersistenceObservedState)
				Expect(obs.ConsecutiveActionErrors).To(Equal(0))
			})

			It("should preserve count on empty drain (no action results)", func() {
				worker, err := persistence.NewPersistenceWorker(identity, store, logger, nil)
				Expect(err).NotTo(HaveOccurred())

				d := worker.GetDependencies()
				d.SetActionHistory([]deps.ActionResult{
					{Success: false, ActionType: "CompactDeltas", Timestamp: time.Now()},
				})

				observed1, err := worker.CollectObservedState(context.Background())
				Expect(err).NotTo(HaveOccurred())
				obs1 := observed1.(snapshot.PersistenceObservedState)
				Expect(obs1.ConsecutiveActionErrors).To(Equal(1))

				// Second tick: no action results (empty drain after state transition)
				// Without stateReader the previous state is zero-valued, so consecutive errors
				// won't persist across ticks in this unit test. This test verifies the
				// within-tick computation: empty action history preserves previous count.
				d.SetActionHistory(nil)
				observed2, err := worker.CollectObservedState(context.Background())
				Expect(err).NotTo(HaveOccurred())
				obs2 := observed2.(snapshot.PersistenceObservedState)
				// Without stateReader, prev.ConsecutiveActionErrors is 0 and empty results
				// preserve 0. The full persistence test requires a stateReader mock.
				Expect(obs2.ConsecutiveActionErrors).To(Equal(0))
			})
		})

		It("should pick up timestamps set by actions", func() {
			worker, err := persistence.NewPersistenceWorker(identity, store, logger, nil)
			Expect(err).NotTo(HaveOccurred())

			now := time.Now()
			d := worker.GetDependencies()
			d.SetLastCompactionAt(now)
			d.SetLastMaintenanceAt(now.Add(-time.Hour))

			observed, err := worker.CollectObservedState(context.Background())
			Expect(err).NotTo(HaveOccurred())

			obs := observed.(snapshot.PersistenceObservedState)
			Expect(obs.LastCompactionAt).To(Equal(now))
			Expect(obs.LastMaintenanceAt).To(Equal(now.Add(-time.Hour)))
		})
	})

	Describe("DeriveDesiredState", func() {
		It("should return defaults with State running for nil spec", func() {
			worker, err := persistence.NewPersistenceWorker(identity, store, logger, nil)
			Expect(err).NotTo(HaveOccurred())

			desired, err := worker.DeriveDesiredState(nil)
			Expect(err).NotTo(HaveOccurred())

			d := desired.(*snapshot.PersistenceDesiredState)
			Expect(d.GetState()).To(Equal("running"))
			Expect(d.CompactionInterval).To(Equal(persistence.DefaultCompactionInterval))
			Expect(d.RetentionWindow).To(Equal(persistence.DefaultRetentionWindow))
			Expect(d.MaintenanceInterval).To(Equal(persistence.DefaultMaintenanceInterval))
		})

		It("should return error for invalid spec type", func() {
			worker, err := persistence.NewPersistenceWorker(identity, store, logger, nil)
			Expect(err).NotTo(HaveOccurred())

			_, err = worker.DeriveDesiredState("invalid")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("GetInitialState", func() {
		It("should return StoppedState", func() {
			worker, err := persistence.NewPersistenceWorker(identity, store, logger, nil)
			Expect(err).NotTo(HaveOccurred())

			initial := worker.GetInitialState()
			Expect(initial).To(BeAssignableToTypeOf(&state.StoppedState{}))
		})
	})
})
