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

package push_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	depspkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push/state"
)

var _ fsmv2.Worker = (*push.PushWorker)(nil)

var _ = Describe("PushWorker", func() {
	var (
		worker     *push.PushWorker
		logger     depspkg.FSMLogger
		identity   depspkg.Identity
		parentDeps *transport.TransportDependencies
	)

	BeforeEach(func() {
		logger = depspkg.NewNopFSMLogger()
		identity = depspkg.Identity{ID: "test-push", Name: "Test Push"}
		transport.SetChannelProvider(newTestChannelProvider())
		parentDeps = createParentDeps(logger)
		transport.SetChildDeps(parentDeps)
	})

	AfterEach(func() {
		transport.ClearChildDeps()
		transport.ClearChannelProvider()
	})

	Describe("Compile-time interface check", func() {
		It("should implement fsmv2.Worker interface", func() {
			var _ fsmv2.Worker = (*push.PushWorker)(nil)
		})
	})

	Describe("NewPushWorker", func() {
		It("should create a worker with valid args", func() {
			w, err := push.NewPushWorker(identity, logger, nil, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(w).NotTo(BeNil())
			var ok bool
			worker, ok = w.(*push.PushWorker)
			Expect(ok).To(BeTrue())
			Expect(worker).NotTo(BeNil())
		})

		It("should reject nil logger", func() {
			_, err := push.NewPushWorker(identity, nil, nil, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("logger"))
		})

		It("should reject missing parent transport ChildDeps", func() {
			transport.ClearChildDeps()
			_, err := push.NewPushWorker(identity, logger, nil, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("transport.ChildDeps()"))
		})
	})

	Describe("CollectObservedState", func() {
		BeforeEach(func() {
			w, err := push.NewPushWorker(identity, logger, nil, nil)
			Expect(err).ToNot(HaveOccurred())
			worker = w.(*push.PushWorker)
		})

		It("should return observed state (NewObservation with zero timestamp for collector)", func() {
			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx, nil)

			Expect(err).ToNot(HaveOccurred())
			Expect(observed).NotTo(BeNil())
			// NewObservation returns zero timestamp — the collector sets it after COS returns
			Expect(observed.GetTimestamp()).To(BeZero())
		})

		It("should handle context cancellation", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			_, err := worker.CollectObservedState(ctx, nil)

			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(context.Canceled))
		})

		It("should report transport availability from parent deps", func() {
			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx, nil)

			Expect(err).ToNot(HaveOccurred())
			typedObs, ok := observed.(fsmv2.Observation[push.PushStatus])
			Expect(ok).To(BeTrue())
			Expect(typedObs.Status.HasTransport).To(BeTrue())
		})

		It("should report JWT token availability from parent deps", func() {
			parentDeps.SetJWT("test-token", time.Now().Add(time.Hour))

			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx, nil)

			Expect(err).ToNot(HaveOccurred())
			typedObs, ok := observed.(fsmv2.Observation[push.PushStatus])
			Expect(ok).To(BeTrue())
			Expect(typedObs.Status.HasValidToken).To(BeTrue())
		})

		It("should report HasValidToken false for expired token", func() {
			parentDeps.SetJWT("expired-token", time.Now().Add(-1*time.Hour))

			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx, nil)

			Expect(err).ToNot(HaveOccurred())
			typedObs, ok := observed.(fsmv2.Observation[push.PushStatus])
			Expect(ok).To(BeTrue())
			Expect(typedObs.Status.HasValidToken).To(BeFalse())
		})

		It("should report consecutive errors from child deps", func() {
			worker.GetDependencies().RecordError()
			worker.GetDependencies().RecordError()

			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx, nil)

			Expect(err).ToNot(HaveOccurred())
			typedObs, ok := observed.(fsmv2.Observation[push.PushStatus])
			Expect(ok).To(BeTrue())
			Expect(typedObs.Status.ConsecutiveErrors).To(Equal(2))
		})

		It("should return Observation[PushStatus] with zero metrics (collector handles metrics)", func() {
			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx, nil)

			Expect(err).ToNot(HaveOccurred())
			typedObs, ok := observed.(fsmv2.Observation[push.PushStatus])
			Expect(ok).To(BeTrue())
			// NewObservation returns zero Metrics — the collector handles metric accumulation
			Expect(typedObs.Metrics.Worker.Counters).To(BeNil())
		})
	})

	Describe("DeriveDesiredState", func() {
		BeforeEach(func() {
			w, err := push.NewPushWorker(identity, logger, nil, nil)
			Expect(err).ToNot(HaveOccurred())
			worker = w.(*push.PushWorker)
		})

		It("should return running state for nil spec", func() {
			desired, err := worker.DeriveDesiredState(nil)

			Expect(err).ToNot(HaveOccurred())
			Expect(desired).NotTo(BeNil())
			Expect(desired.GetState()).To(Equal("running"))
		})

		It("should return correct state for valid spec", func() {
			spec := fsmv2config.UserSpec{
				Config:    `state: stopped`,
				Variables: fsmv2config.VariableBundle{},
			}

			desired, err := worker.DeriveDesiredState(spec)

			Expect(err).ToNot(HaveOccurred())
			Expect(desired).NotTo(BeNil())
			Expect(desired.GetState()).To(Equal("stopped"))
		})

		It("should return running state for empty config", func() {
			spec := fsmv2config.UserSpec{
				Config:    "",
				Variables: fsmv2config.VariableBundle{},
			}

			desired, err := worker.DeriveDesiredState(spec)

			Expect(err).ToNot(HaveOccurred())
			Expect(desired.GetState()).To(Equal("running"))
		})

		It("should be deterministic", func() {
			spec := fsmv2config.UserSpec{
				Config:    `state: running`,
				Variables: fsmv2config.VariableBundle{},
			}

			desired1, err1 := worker.DeriveDesiredState(spec)
			desired2, err2 := worker.DeriveDesiredState(spec)

			Expect(err1).ToNot(HaveOccurred())
			Expect(err2).ToNot(HaveOccurred())
			Expect(desired1.GetState()).To(Equal(desired2.GetState()))
		})

		It("should return error for invalid spec type", func() {
			_, err := worker.DeriveDesiredState("invalid-string-spec")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid spec type"))
		})
	})

	Describe("GetInitialState", func() {
		BeforeEach(func() {
			w, err := push.NewPushWorker(identity, logger, nil, nil)
			Expect(err).ToNot(HaveOccurred())
			worker = w.(*push.PushWorker)
		})

		It("should return StoppedState", func() {
			initialState := worker.GetInitialState()

			Expect(initialState).To(BeAssignableToTypeOf(&state.StoppedState{}))
		})
	})

	Describe("Factory registration", func() {
		It("should be registered in the factory as 'push'", func() {
			registeredTypes := factory.ListRegisteredTypes()
			Expect(registeredTypes).To(ContainElement("push"))
		})

		It("should have both worker and supervisor factories registered", func() {
			workerOnly, supervisorOnly := factory.ValidateRegistryConsistency()
			Expect(workerOnly).NotTo(ContainElement("push"))
			Expect(supervisorOnly).NotTo(ContainElement("push"))
		})

		It("should read parent deps from transport.ChildDeps() when instantiated via factory", func() {
			factoryIdentity := depspkg.Identity{ID: "factory-push", Name: "Factory Push", WorkerType: "push"}

			w, err := factory.NewWorkerByType("push", factoryIdentity, logger, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(w).NotTo(BeNil())

			pw, ok := w.(*push.PushWorker)
			Expect(ok).To(BeTrue())
			Expect(pw).NotTo(BeNil())

			// The constructed worker must reflect the parent deps seeded via SetChildDeps
			// in BeforeEach (parentDeps has a mockTransport attached, so HasTransport is true).
			ctx := context.Background()
			observed, err := pw.CollectObservedState(ctx, nil)
			Expect(err).ToNot(HaveOccurred())
			typedObs, ok := observed.(fsmv2.Observation[push.PushStatus])
			Expect(ok).To(BeTrue())
			Expect(typedObs.Status.HasTransport).To(BeTrue())
		})
	})
})
