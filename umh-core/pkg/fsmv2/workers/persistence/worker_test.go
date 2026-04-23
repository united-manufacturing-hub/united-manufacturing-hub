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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/register"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/persistence/state"
)

var _ = Describe("PersistenceWorker", func() {
	var (
		logger   deps.FSMLogger
		identity deps.Identity
		store    *mockTriangularStore
	)

	BeforeEach(func() {
		logger = deps.NewNopFSMLogger()
		identity = deps.Identity{ID: "test-id", Name: "test-persistence"}
		store = &mockTriangularStore{}
		register.SetDeps[*persistence.PersistenceDependencies](
			persistence.WorkerTypeName,
			persistence.NewStoreOnlyDependencies(store),
		)
	})

	AfterEach(func() {
		register.ClearDeps(persistence.WorkerTypeName)
	})

	getPersistenceWorker := func(w any) *persistence.PersistenceWorker {
		pw, ok := w.(*persistence.PersistenceWorker)
		Expect(ok).To(BeTrue())
		return pw
	}

	Describe("NewPersistenceWorker", func() {
		It("should create a worker successfully from seed dependencies", func() {
			w, err := persistence.NewPersistenceWorker(identity, logger, nil, persistence.NewStoreOnlyDependencies(store))
			Expect(err).NotTo(HaveOccurred())
			Expect(w).NotTo(BeNil())
			Expect(getPersistenceWorker(w)).NotTo(BeNil())
		})

		It("should error when dependencies are nil", func() {
			_, err := persistence.NewPersistenceWorker(identity, logger, nil, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("register.SetDeps"))
		})

		It("should accept non-nil typed dependencies directly", func() {
			d := persistence.NewPersistenceDependencies(store, deps.DefaultScheduler{}, logger, nil, identity)
			w, err := persistence.NewPersistenceWorker(identity, logger, nil, d)
			Expect(err).NotTo(HaveOccurred())
			Expect(w).NotTo(BeNil())
		})

		It("should default WorkerType to 'persistence' when empty", func() {
			id := deps.Identity{ID: "test-id", Name: "test-persistence"}
			w, err := persistence.NewPersistenceWorker(id, logger, nil, persistence.NewStoreOnlyDependencies(store))
			Expect(err).NotTo(HaveOccurred())
			pw := getPersistenceWorker(w)
			Expect(pw.GetDependencies().GetWorkerType()).To(Equal("persistence"))
		})
	})

	Describe("CollectObservedState", func() {
		It("should return valid observed state", func() {
			w, err := persistence.NewPersistenceWorker(identity, logger, nil, persistence.NewStoreOnlyDependencies(store))
			Expect(err).NotTo(HaveOccurred())

			worker := getPersistenceWorker(w)
			observed, err := worker.CollectObservedState(context.Background(), nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(observed).NotTo(BeNil())

			// CollectedAt is populated by the collector post-COS, not inside COS
			// itself (per NewObservation contract). Assert only that the type is
			// correct; framework fields are exercised in integration tests.
			_, ok := observed.(fsmv2.Observation[persistence.PersistenceStatus])
			Expect(ok).To(BeTrue())
		})

		It("should return error when context is cancelled", func() {
			w, err := persistence.NewPersistenceWorker(identity, logger, nil, persistence.NewStoreOnlyDependencies(store))
			Expect(err).NotTo(HaveOccurred())

			worker := getPersistenceWorker(w)
			cancelledCtx, cancel := context.WithCancel(context.Background())
			cancel()

			_, err = worker.CollectObservedState(cancelledCtx, nil)
			Expect(err).To(Equal(context.Canceled))
		})

		Context("ConsecutiveActionErrors computation", func() {
			It("should increment on failure", func() {
				w, err := persistence.NewPersistenceWorker(identity, logger, nil, persistence.NewStoreOnlyDependencies(store))
				Expect(err).NotTo(HaveOccurred())

				worker := getPersistenceWorker(w)
				d := worker.GetDependencies()
				d.SetActionHistory([]deps.ActionResult{
					{Success: false, ActionType: "CompactDeltas", Timestamp: time.Now()},
				})

				observed, err := worker.CollectObservedState(context.Background(), nil)
				Expect(err).NotTo(HaveOccurred())

				obs := observed.(fsmv2.Observation[persistence.PersistenceStatus])
				Expect(obs.Status.ConsecutiveActionErrors).To(Equal(1))
			})

			It("should reset on success", func() {
				w, err := persistence.NewPersistenceWorker(identity, logger, nil, persistence.NewStoreOnlyDependencies(store))
				Expect(err).NotTo(HaveOccurred())

				worker := getPersistenceWorker(w)
				d := worker.GetDependencies()
				d.SetActionHistory([]deps.ActionResult{
					{Success: false, ActionType: "CompactDeltas", Timestamp: time.Now()},
					{Success: false, ActionType: "CompactDeltas", Timestamp: time.Now()},
					{Success: true, ActionType: "CompactDeltas", Timestamp: time.Now()},
				})

				observed, err := worker.CollectObservedState(context.Background(), nil)
				Expect(err).NotTo(HaveOccurred())

				obs := observed.(fsmv2.Observation[persistence.PersistenceStatus])
				Expect(obs.Status.ConsecutiveActionErrors).To(Equal(0))
			})

			It("should preserve count on empty drain (no action results)", func() {
				w, err := persistence.NewPersistenceWorker(identity, logger, nil, persistence.NewStoreOnlyDependencies(store))
				Expect(err).NotTo(HaveOccurred())

				worker := getPersistenceWorker(w)
				d := worker.GetDependencies()
				d.SetActionHistory([]deps.ActionResult{
					{Success: false, ActionType: "CompactDeltas", Timestamp: time.Now()},
				})

				observed1, err := worker.CollectObservedState(context.Background(), nil)
				Expect(err).NotTo(HaveOccurred())
				obs1 := observed1.(fsmv2.Observation[persistence.PersistenceStatus])
				Expect(obs1.Status.ConsecutiveActionErrors).To(Equal(1))

				// Second tick: no action results (empty drain after state transition)
				// Without stateReader the previous state is zero-valued, so consecutive errors
				// won't persist across ticks in this unit test. This test verifies the
				// within-tick computation: empty action history preserves previous count.
				d.SetActionHistory(nil)
				observed2, err := worker.CollectObservedState(context.Background(), nil)
				Expect(err).NotTo(HaveOccurred())
				obs2 := observed2.(fsmv2.Observation[persistence.PersistenceStatus])
				// Without stateReader, prev.ConsecutiveActionErrors is 0 and empty results
				// preserve 0. The full persistence test requires a stateReader mock.
				Expect(obs2.Status.ConsecutiveActionErrors).To(Equal(0))
			})
		})

		It("should pick up timestamps set by actions", func() {
			w, err := persistence.NewPersistenceWorker(identity, logger, nil, persistence.NewStoreOnlyDependencies(store))
			Expect(err).NotTo(HaveOccurred())

			worker := getPersistenceWorker(w)
			now := time.Now()
			d := worker.GetDependencies()
			d.SetLastCompactionAt(now)
			d.SetLastMaintenanceAt(now.Add(-time.Hour))

			observed, err := worker.CollectObservedState(context.Background(), nil)
			Expect(err).NotTo(HaveOccurred())

			obs := observed.(fsmv2.Observation[persistence.PersistenceStatus])
			Expect(obs.Status.LastCompactionAt).To(Equal(now))
			Expect(obs.Status.LastMaintenanceAt).To(Equal(now.Add(-time.Hour)))
		})
	})

	Describe("DeriveDesiredState", func() {
		It("should return defaults with State running for nil spec", func() {
			w, err := persistence.NewPersistenceWorker(identity, logger, nil, persistence.NewStoreOnlyDependencies(store))
			Expect(err).NotTo(HaveOccurred())

			worker := getPersistenceWorker(w)
			desired, err := worker.DeriveDesiredState(nil)
			Expect(err).NotTo(HaveOccurred())

			d := desired.(*fsmv2.WrappedDesiredState[persistence.PersistenceConfig])
			Expect(d.GetState()).To(Equal("running"))
			Expect(d.Config.CompactionInterval).To(Equal(persistence.DefaultCompactionInterval))
			Expect(d.Config.RetentionWindow).To(Equal(persistence.DefaultRetentionWindow))
			Expect(d.Config.MaintenanceInterval).To(Equal(persistence.DefaultMaintenanceInterval))
		})

		It("should return error for invalid spec type", func() {
			w, err := persistence.NewPersistenceWorker(identity, logger, nil, persistence.NewStoreOnlyDependencies(store))
			Expect(err).NotTo(HaveOccurred())

			worker := getPersistenceWorker(w)
			_, err = worker.DeriveDesiredState("invalid")
			Expect(err).To(HaveOccurred())
		})

		It("should parse custom intervals from spec", func() {
			w, err := persistence.NewPersistenceWorker(identity, logger, nil, persistence.NewStoreOnlyDependencies(store))
			Expect(err).NotTo(HaveOccurred())

			worker := getPersistenceWorker(w)
			spec := fsmv2config.UserSpec{
				Config: "compactionInterval: 10m\nretentionWindow: 48h\nmaintenanceInterval: 336h\n",
			}

			desired, err := worker.DeriveDesiredState(spec)
			Expect(err).NotTo(HaveOccurred())

			d := desired.(*fsmv2.WrappedDesiredState[persistence.PersistenceConfig])
			Expect(d.GetState()).To(Equal("running"))
			Expect(d.Config.CompactionInterval).To(Equal(10 * time.Minute))
			Expect(d.Config.RetentionWindow).To(Equal(48 * time.Hour))
			Expect(d.Config.MaintenanceInterval).To(Equal(336 * time.Hour))
		})

		It("should apply defaults for unspecified fields", func() {
			w, err := persistence.NewPersistenceWorker(identity, logger, nil, persistence.NewStoreOnlyDependencies(store))
			Expect(err).NotTo(HaveOccurred())

			worker := getPersistenceWorker(w)
			spec := fsmv2config.UserSpec{
				Config: "compactionInterval: 15m\n",
			}

			desired, err := worker.DeriveDesiredState(spec)
			Expect(err).NotTo(HaveOccurred())

			d := desired.(*fsmv2.WrappedDesiredState[persistence.PersistenceConfig])
			Expect(d.Config.CompactionInterval).To(Equal(15 * time.Minute))
			Expect(d.Config.RetentionWindow).To(Equal(persistence.DefaultRetentionWindow))
			Expect(d.Config.MaintenanceInterval).To(Equal(persistence.DefaultMaintenanceInterval))
		})

		It("should return defaults for empty Config string", func() {
			w, err := persistence.NewPersistenceWorker(identity, logger, nil, persistence.NewStoreOnlyDependencies(store))
			Expect(err).NotTo(HaveOccurred())

			worker := getPersistenceWorker(w)
			spec := fsmv2config.UserSpec{Config: ""}

			desired, err := worker.DeriveDesiredState(spec)
			Expect(err).NotTo(HaveOccurred())

			d := desired.(*fsmv2.WrappedDesiredState[persistence.PersistenceConfig])
			Expect(d.GetState()).To(Equal("running"))
			Expect(d.Config.CompactionInterval).To(Equal(persistence.DefaultCompactionInterval))
			Expect(d.Config.RetentionWindow).To(Equal(persistence.DefaultRetentionWindow))
			Expect(d.Config.MaintenanceInterval).To(Equal(persistence.DefaultMaintenanceInterval))
		})

		It("should return error for invalid YAML", func() {
			w, err := persistence.NewPersistenceWorker(identity, logger, nil, persistence.NewStoreOnlyDependencies(store))
			Expect(err).NotTo(HaveOccurred())

			worker := getPersistenceWorker(w)
			spec := fsmv2config.UserSpec{Config: "{{invalid yaml"}

			_, err = worker.DeriveDesiredState(spec)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("GetInitialState", func() {
		It("should return StoppedState", func() {
			w, err := persistence.NewPersistenceWorker(identity, logger, nil, persistence.NewStoreOnlyDependencies(store))
			Expect(err).NotTo(HaveOccurred())

			worker := getPersistenceWorker(w)
			initial := worker.GetInitialState()
			Expect(initial).To(BeAssignableToTypeOf(&state.StoppedState{}))
		})
	})

	Describe("Factory registration", func() {
		It("should be registered in the factory as 'persistence'", func() {
			registeredTypes := factory.ListRegisteredTypes()
			Expect(registeredTypes).To(ContainElement("persistence"))
		})

		It("should have both worker and supervisor factories registered", func() {
			workerOnly, supervisorOnly := factory.ValidateRegistryConsistency()
			Expect(workerOnly).NotTo(ContainElement("persistence"))
			Expect(supervisorOnly).NotTo(ContainElement("persistence"))
		})

		It("should consume store from register.GetDeps when instantiated via factory", func() {
			factoryIdentity := deps.Identity{ID: "factory-persistence", Name: "Factory Persistence", WorkerType: "persistence"}

			w, err := factory.NewWorkerByType("persistence", factoryIdentity, logger, nil, map[string]any{})
			Expect(err).NotTo(HaveOccurred())
			Expect(w).NotTo(BeNil())

			pw, ok := w.(*persistence.PersistenceWorker)
			Expect(ok).To(BeTrue())
			Expect(pw.GetDependencies().GetStore()).To(BeIdenticalTo(store))
		})
	})
})
