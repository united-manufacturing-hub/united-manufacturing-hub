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

package pull_test

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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/pull"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/pull/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/pull/state"
)

var _ fsmv2.Worker = (*pull.PullWorker)(nil)

var _ = Describe("PullWorker", func() {
	var (
		worker     *pull.PullWorker
		logger     depspkg.FSMLogger
		identity   depspkg.Identity
		parentDeps *transport.TransportDependencies
	)

	BeforeEach(func() {
		logger = depspkg.NewNopFSMLogger()
		identity = depspkg.Identity{ID: "test-pull", Name: "Test Pull"}
		transport.SetChannelProvider(newTestChannelProvider())
		parentDeps = createParentDeps(logger)
	})

	AfterEach(func() {
		transport.ClearChannelProvider()
	})

	Describe("Compile-time interface check", func() {
		It("should implement fsmv2.Worker interface", func() {
			var _ fsmv2.Worker = (*pull.PullWorker)(nil)
		})
	})

	Describe("NewPullWorker", func() {
		It("should create a worker with valid args", func() {
			var err error
			worker, err = pull.NewPullWorker(identity, logger, nil, parentDeps)
			Expect(err).ToNot(HaveOccurred())
			Expect(worker).NotTo(BeNil())
		})

		It("should reject nil logger", func() {
			_, err := pull.NewPullWorker(identity, nil, nil, parentDeps)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("logger"))
		})

		It("should reject nil parentDeps", func() {
			_, err := pull.NewPullWorker(identity, logger, nil, nil)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("CollectObservedState", func() {
		BeforeEach(func() {
			var err error
			worker, err = pull.NewPullWorker(identity, logger, nil, parentDeps)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return non-nil observed state of correct type", func() {
			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx, nil)

			Expect(err).ToNot(HaveOccurred())
			Expect(observed).NotTo(BeNil())
			// Timestamp is zero from NewObservation — the collector fills it post-COS.
			// Only assert the return is non-nil and of the correct type.
			_, ok := observed.(fsmv2.Observation[snapshot.PullStatus])
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
			typedObs, ok := observed.(fsmv2.Observation[snapshot.PullStatus])
			Expect(ok).To(BeTrue())
			Expect(typedObs.Status.HasTransport).To(BeTrue())
		})

		It("should report JWT token availability from parent deps", func() {
			parentDeps.SetJWT("test-token", time.Now().Add(time.Hour))

			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx, nil)

			Expect(err).ToNot(HaveOccurred())
			typedObs, ok := observed.(fsmv2.Observation[snapshot.PullStatus])
			Expect(ok).To(BeTrue())
			Expect(typedObs.Status.HasValidToken).To(BeTrue())
		})

		It("should report consecutive errors from child deps", func() {
			worker.GetDependencies().RecordError()
			worker.GetDependencies().RecordError()

			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx, nil)

			Expect(err).ToNot(HaveOccurred())
			typedObs, ok := observed.(fsmv2.Observation[snapshot.PullStatus])
			Expect(ok).To(BeTrue())
			Expect(typedObs.Status.ConsecutiveErrors).To(Equal(2))
		})

		It("should report pending message count", func() {
			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx, nil)

			Expect(err).ToNot(HaveOccurred())
			typedObs, ok := observed.(fsmv2.Observation[snapshot.PullStatus])
			Expect(ok).To(BeTrue())
			Expect(typedObs.Status.PendingMessageCount).To(Equal(0))
		})

		It("should report backpressure state", func() {
			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx, nil)

			Expect(err).ToNot(HaveOccurred())
			typedObs, ok := observed.(fsmv2.Observation[snapshot.PullStatus])
			Expect(ok).To(BeTrue())
			Expect(typedObs.Status.IsBackpressured).To(BeFalse())
		})

		It("should return Observation type (framework fields filled by collector)", func() {
			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx, nil)

			Expect(err).ToNot(HaveOccurred())
			// NewObservation returns Observation[T] — collector post-processes it.
			// We only verify the return type here; metrics are filled post-COS.
			_, ok := observed.(fsmv2.Observation[snapshot.PullStatus])
			Expect(ok).To(BeTrue())
		})
	})

	Describe("DeriveDesiredState", func() {
		BeforeEach(func() {
			var err error
			worker, err = pull.NewPullWorker(identity, logger, nil, parentDeps)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return running state for nil spec", func() {
			desired, err := worker.DeriveDesiredState(nil)

			Expect(err).ToNot(HaveOccurred())
			Expect(desired).NotTo(BeNil())
			typed, ok := desired.(*fsmv2.WrappedDesiredState[snapshot.PullConfig])
			Expect(ok).To(BeTrue())
			Expect(typed.GetState()).To(Equal("running"))
		})

		It("should return correct state for valid spec", func() {
			spec := fsmv2config.UserSpec{
				Config:    `state: stopped`,
				Variables: fsmv2config.VariableBundle{},
			}

			desired, err := worker.DeriveDesiredState(spec)

			Expect(err).ToNot(HaveOccurred())
			Expect(desired).NotTo(BeNil())
			typedStopped, okStopped := desired.(*fsmv2.WrappedDesiredState[snapshot.PullConfig])
			Expect(okStopped).To(BeTrue())
			Expect(typedStopped.GetState()).To(Equal("stopped"))
		})

		It("should return running state for empty config", func() {
			spec := fsmv2config.UserSpec{
				Config:    "",
				Variables: fsmv2config.VariableBundle{},
			}

			desired, err := worker.DeriveDesiredState(spec)

			Expect(err).ToNot(HaveOccurred())
			typedRunning, okRunning := desired.(*fsmv2.WrappedDesiredState[snapshot.PullConfig])
			Expect(okRunning).To(BeTrue())
			Expect(typedRunning.GetState()).To(Equal("running"))
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
			pull1, pullOk1 := desired1.(*fsmv2.WrappedDesiredState[snapshot.PullConfig])
			Expect(pullOk1).To(BeTrue())
			pull2, pullOk2 := desired2.(*fsmv2.WrappedDesiredState[snapshot.PullConfig])
			Expect(pullOk2).To(BeTrue())
			Expect(pull1.GetState()).To(Equal(pull2.GetState()))
		})

		It("should return error for invalid spec type", func() {
			_, err := worker.DeriveDesiredState("invalid-string-spec")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid spec type"))
		})

		It("should return error for malformed YAML", func() {
			spec := fsmv2config.UserSpec{
				Config:    "state: [invalid yaml: {",
				Variables: fsmv2config.VariableBundle{},
			}

			_, err := worker.DeriveDesiredState(spec)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("GetInitialState", func() {
		BeforeEach(func() {
			var err error
			worker, err = pull.NewPullWorker(identity, logger, nil, parentDeps)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return StoppedState", func() {
			initialState := worker.GetInitialState()

			Expect(initialState).To(BeAssignableToTypeOf(&state.StoppedState{}))
		})
	})

	Describe("Factory registration", func() {
		It("should be registered in the factory as 'pull'", func() {
			registeredTypes := factory.ListRegisteredTypes()
			Expect(registeredTypes).To(ContainElement("pull"))
		})

		It("should have both worker and supervisor factories registered", func() {
			workerOnly, supervisorOnly := factory.ValidateRegistryConsistency()
			Expect(workerOnly).NotTo(ContainElement("pull"))
			Expect(supervisorOnly).NotTo(ContainElement("pull"))
		})
	})
})
