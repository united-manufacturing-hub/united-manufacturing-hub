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
	"gopkg.in/yaml.v3"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2config "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	depspkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push/state"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

var _ fsmv2.Worker = (*push.PushWorker)(nil)

var _ = Describe("PushWorker", func() {
	var (
		worker     *push.PushWorker
		logger     depspkg.FSMLogger
		identity   depspkg.Identity
		parentDeps *transport.TransportDependencies
		pushDeps   *push.PushDependencies
	)

	BeforeEach(func() {
		logger = depspkg.NewNopFSMLogger()
		identity = depspkg.Identity{ID: "test-push", Name: "Test Push"}
		transport.SetChannelProvider(newTestChannelProvider())
		parentDeps = createParentDeps(logger)

		var err error
		pushDeps, err = push.NewPushDependencies(parentDeps, depspkg.NewBaseDependencies(logger, nil, identity))
		Expect(err).ToNot(HaveOccurred())
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
			worker, err = push.NewPushWorker(identity, logger, nil, pushDeps)
			Expect(err).ToNot(HaveOccurred())
			Expect(worker).NotTo(BeNil())
		})

		It("should reject nil logger", func() {
			_, err := push.NewPushWorker(identity, nil, nil, pushDeps)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("logger"))
		})

		It("should reject nil dependencies", func() {
			_, err := push.NewPushWorker(identity, logger, nil, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("non-nil dependencies"))
		})
	})

	Describe("CollectObservedState", func() {
		BeforeEach(func() {
			var err error
			worker, err = push.NewPushWorker(identity, logger, nil, pushDeps)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return valid observed state", func() {
			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx, nil)

			Expect(err).ToNot(HaveOccurred())
			Expect(observed).NotTo(BeNil())
		})

		It("should return Observation[PushStatus] type", func() {
			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx, nil)

			Expect(err).ToNot(HaveOccurred())
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

		It("HasValidToken is true when the desired AuthSession token is present and unexpired", func() {
			desired := &fsmv2.WrappedDesiredState[snapshot.PushDesiredState]{
				Config: snapshot.PushDesiredState{AuthSession: types.AuthSession{Token: "t", Expiry: time.Now().Add(time.Hour)}},
			}
			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx, desired)

			Expect(err).ToNot(HaveOccurred())
			typedObs, ok := observed.(fsmv2.Observation[snapshot.PushStatus])
			Expect(ok).To(BeTrue())
			Expect(typedObs.Status.HasValidToken).To(BeTrue())
		})

		It("HasValidToken is false when the desired AuthSession token is expired", func() {
			desired := &fsmv2.WrappedDesiredState[snapshot.PushDesiredState]{
				Config: snapshot.PushDesiredState{AuthSession: types.AuthSession{Token: "expired-token", Expiry: time.Now().Add(-1 * time.Hour)}},
			}
			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx, desired)

			Expect(err).ToNot(HaveOccurred())
			typedObs, ok := observed.(fsmv2.Observation[snapshot.PushStatus])
			Expect(ok).To(BeTrue())
			Expect(typedObs.Status.HasValidToken).To(BeFalse())
		})

		It("HasValidToken is false when the desired AuthSession token is empty", func() {
			desired := &fsmv2.WrappedDesiredState[snapshot.PushDesiredState]{
				Config: snapshot.PushDesiredState{AuthSession: types.AuthSession{Token: "", Expiry: time.Now().Add(time.Hour)}},
			}
			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx, desired)

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

		It("surfaces last status_code and error_detail from the child deps", func() {
			worker.GetDependencies().RecordTypedError(types.ErrorTypeServerError, 0, 502, "HTTP 502 (server_error): error code: 502")

			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx, nil)

			Expect(err).ToNot(HaveOccurred())
			typedObs, ok := observed.(fsmv2.Observation[snapshot.PushStatus])
			Expect(ok).To(BeTrue())
			Expect(typedObs.Status.LastErrorType).To(Equal(types.ErrorTypeServerError))
			Expect(typedObs.Status.LastStatusCode).To(Equal(502))
			Expect(typedObs.Status.LastErrorDetail).To(Equal("HTTP 502 (server_error): error code: 502"))
		})
	})

	Describe("DeriveDesiredState", func() {
		BeforeEach(func() {
			var err error
			worker, err = push.NewPushWorker(identity, logger, nil, pushDeps)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return running state for nil spec", func() {
			desired, err := worker.DeriveDesiredState(nil)

			Expect(err).ToNot(HaveOccurred())
			Expect(desired).NotTo(BeNil())
			typed, ok := desired.(*fsmv2.WrappedDesiredState[snapshot.PushDesiredState])
			Expect(ok).To(BeTrue())
			Expect(typed).NotTo(BeNil())
		})

		It("parses a valid spec into a PushDesiredState wrapper without error", func() {
			spec := fsmv2config.UserSpec{
				Config:    `state: stopped`,
				Variables: fsmv2config.VariableBundle{},
			}

			desired, err := worker.DeriveDesiredState(spec)

			Expect(err).ToNot(HaveOccurred())
			_, ok := desired.(*fsmv2.WrappedDesiredState[snapshot.PushDesiredState])
			Expect(ok).To(BeTrue())
		})

		It("parses empty config into a PushDesiredState wrapper without error", func() {
			spec := fsmv2config.UserSpec{
				Config:    "",
				Variables: fsmv2config.VariableBundle{},
			}

			desired, err := worker.DeriveDesiredState(spec)

			Expect(err).ToNot(HaveOccurred())
			_, ok := desired.(*fsmv2.WrappedDesiredState[snapshot.PushDesiredState])
			Expect(ok).To(BeTrue())
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
			pd1, pdOk1 := desired1.(*fsmv2.WrappedDesiredState[snapshot.PushDesiredState])
			Expect(pdOk1).To(BeTrue())
			pd2, pdOk2 := desired2.(*fsmv2.WrappedDesiredState[snapshot.PushDesiredState])
			Expect(pdOk2).To(BeTrue())
			Expect(pd1).To(Equal(pd2))
		})

		It("should return error for invalid spec type", func() {
			_, err := worker.DeriveDesiredState("invalid-string-spec")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid spec type"))
		})

		It("should bind AuthSession from parsed ChildAuthUserSpec into PushDesiredState", func() {
			carrier := types.ChildAuthUserSpec{
				AuthSession: types.AuthSession{
					Token:        "test-token-push",
					InstanceUUID: "inst-uuid-push",
				},
			}
			raw, err := yaml.Marshal(carrier)
			Expect(err).ToNot(HaveOccurred())

			spec := fsmv2config.UserSpec{
				Config:    string(raw),
				Variables: fsmv2config.VariableBundle{},
			}

			desired, err := worker.DeriveDesiredState(spec)

			Expect(err).ToNot(HaveOccurred())
			typed, ok := desired.(*fsmv2.WrappedDesiredState[snapshot.PushDesiredState])
			Expect(ok).To(BeTrue())
			Expect(typed.Config.AuthSession.Token).To(Equal("test-token-push"))
			Expect(typed.Config.AuthSession.InstanceUUID).To(Equal("inst-uuid-push"))
		})
	})

	Describe("GetInitialState", func() {
		BeforeEach(func() {
			var err error
			worker, err = push.NewPushWorker(identity, logger, nil, pushDeps)
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
