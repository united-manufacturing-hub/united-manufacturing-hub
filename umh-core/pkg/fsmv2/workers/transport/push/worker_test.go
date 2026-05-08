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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push/snapshot"
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
	})

	AfterEach(func() {
		transport.ClearChannelProvider()
	})

	Describe("Compile-time interface check", func() {
		It("should implement fsmv2.Worker interface", func() {
			var _ fsmv2.Worker = (*push.PushWorker)(nil)
		})
	})

	Describe("NewPushWorker", func() {
		It("should create a worker with valid args", func() {
			var err error
			worker, err = push.NewPushWorker(identity, logger, nil, parentDeps)
			Expect(err).ToNot(HaveOccurred())
			Expect(worker).NotTo(BeNil())
		})

		It("should reject nil logger", func() {
			_, err := push.NewPushWorker(identity, nil, nil, parentDeps)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("logger"))
		})

		It("should reject nil parentDeps", func() {
			_, err := push.NewPushWorker(identity, logger, nil, nil)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("CollectObservedState", func() {
		BeforeEach(func() {
			var err error
			worker, err = push.NewPushWorker(identity, logger, nil, parentDeps)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return non-nil observed state", func() {
			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx, nil)

			Expect(err).ToNot(HaveOccurred())
			Expect(observed).NotTo(BeNil())
			// Timestamp is zero from NewObservation  - the collector fills it post-COS.
			// Only assert the return is non-nil and of the correct type.
			_, ok := observed.(fsmv2.Observation[snapshot.PushStatus])
			Expect(ok).To(BeTrue())
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
			typedObs, ok := observed.(fsmv2.Observation[snapshot.PushStatus])
			Expect(ok).To(BeTrue())
			Expect(typedObs.Status.HasTransport).To(BeTrue())
		})

		It("should report JWT token availability from parent deps", func() {
			parentDeps.SetJWT("test-token", time.Now().Add(time.Hour))

			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx, nil)

			Expect(err).ToNot(HaveOccurred())
			typedObs, ok := observed.(fsmv2.Observation[snapshot.PushStatus])
			Expect(ok).To(BeTrue())
			Expect(typedObs.Status.HasValidToken).To(BeTrue())
		})

		It("should report HasValidToken false for expired token", func() {
			parentDeps.SetJWT("expired-token", time.Now().Add(-1*time.Hour))

			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx, nil)

			Expect(err).ToNot(HaveOccurred())
			typedObs, ok := observed.(fsmv2.Observation[snapshot.PushStatus])
			Expect(ok).To(BeTrue())
			Expect(typedObs.Status.HasValidToken).To(BeFalse())
		})

		It("should report consecutive errors from child deps", func() {
			worker.GetDependencies().RecordError()
			worker.GetDependencies().RecordError()

			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx, nil)

			Expect(err).ToNot(HaveOccurred())
			typedObs, ok := observed.(fsmv2.Observation[snapshot.PushStatus])
			Expect(ok).To(BeTrue())
			Expect(typedObs.Status.ConsecutiveErrors).To(Equal(2))
		})

		It("should return Observation type (framework fields filled by collector)", func() {
			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx, nil)

			Expect(err).ToNot(HaveOccurred())
			// NewObservation returns Observation[T]  - collector post-processes it.
			// We only verify the return type here; metrics are filled post-COS.
			_, ok := observed.(fsmv2.Observation[snapshot.PushStatus])
			Expect(ok).To(BeTrue())
		})
	})

	Describe("DeriveDesiredState", func() {
		BeforeEach(func() {
			var err error
			worker, err = push.NewPushWorker(identity, logger, nil, parentDeps)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return running state for nil spec", func() {
			desired, err := worker.DeriveDesiredState(nil)

			Expect(err).ToNot(HaveOccurred())
			Expect(desired).NotTo(BeNil())
			typed, ok := desired.(*fsmv2.WrappedDesiredState[snapshot.PushConfig])
			Expect(ok).To(BeTrue())
			Expect(typed.State).To(Equal("running"))
		})

		It("should return correct state for valid spec", func() {
			spec := fsmv2config.UserSpec{
				Config:    `state: stopped`,
				Variables: fsmv2config.VariableBundle{},
			}

			desired, err := worker.DeriveDesiredState(spec)

			Expect(err).ToNot(HaveOccurred())
			Expect(desired).NotTo(BeNil())
			typedStopped, okStopped := desired.(*fsmv2.WrappedDesiredState[snapshot.PushConfig])
			Expect(okStopped).To(BeTrue())
			Expect(typedStopped.State).To(Equal("stopped"))
		})

		It("should return running state for empty config", func() {
			spec := fsmv2config.UserSpec{
				Config:    "",
				Variables: fsmv2config.VariableBundle{},
			}

			desired, err := worker.DeriveDesiredState(spec)

			Expect(err).ToNot(HaveOccurred())
			typedRunning, okRunning := desired.(*fsmv2.WrappedDesiredState[snapshot.PushConfig])
			Expect(okRunning).To(BeTrue())
			Expect(typedRunning.State).To(Equal("running"))
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
			pd1, pdOk1 := desired1.(*fsmv2.WrappedDesiredState[snapshot.PushConfig])
			Expect(pdOk1).To(BeTrue())
			pd2, pdOk2 := desired2.(*fsmv2.WrappedDesiredState[snapshot.PushConfig])
			Expect(pdOk2).To(BeTrue())
			Expect(pd1.State).To(Equal(pd2.State))
		})

		It("should return error for invalid spec type", func() {
			_, err := worker.DeriveDesiredState("invalid-string-spec")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid spec type"))
		})
	})

	Describe("GetInitialState", func() {
		BeforeEach(func() {
			var err error
			worker, err = push.NewPushWorker(identity, logger, nil, parentDeps)
			Expect(err).ToNot(HaveOccurred())
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
	})
})
