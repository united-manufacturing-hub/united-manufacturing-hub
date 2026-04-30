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

package transport_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	fsmv2types "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/factory"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/state"
)

// Note: mockChannelProvider is defined in dependencies_test.go (same package)
// and is reused here via newTestChannelProvider()

var _ = Describe("TransportWorker", func() {
	var (
		worker   *transport.TransportWorker
		logger   deps.FSMLogger
		identity deps.Identity
	)

	BeforeEach(func() {
		logger = deps.NewNopFSMLogger()
		identity = deps.Identity{ID: "test-transport", Name: "Test Transport"}

		// Set up mock channel provider using helper from dependencies_test.go
		transport.SetChannelProvider(newTestChannelProvider())
	})

	AfterEach(func() {
		transport.ClearChannelProvider()
	})

	Describe("Compile-time interface check", func() {
		It("should implement fsmv2.Worker interface", func() {
			// This is checked at compile time by the var _ fsmv2.Worker = (*TransportWorker)(nil)
			// declaration in worker.go, but we verify it here too
			var _ fsmv2.Worker = (*transport.TransportWorker)(nil)
		})
	})

	Describe("NewTransportWorker", func() {
		Context("dependency validation", func() {
			It("should create a worker with valid dependencies", func() {
				var err error
				worker, err = transport.NewTransportWorker(identity, logger, nil)
				Expect(err).ToNot(HaveOccurred())
				Expect(worker).NotTo(BeNil())
			})

			It("should reject nil logger", func() {
				_, err := transport.NewTransportWorker(identity, nil, nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("logger"))
			})
		})
	})

	Describe("CollectObservedState", func() {
		BeforeEach(func() {
			var err error
			worker, err = transport.NewTransportWorker(identity, logger, nil)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return observed state with timestamp", func() {
			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx, nil)

			Expect(err).ToNot(HaveOccurred())
			Expect(observed).NotTo(BeNil())
			Expect(observed.GetTimestamp()).NotTo(BeZero())
		})

		It("should handle context cancellation at entry", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel immediately

			_, err := worker.CollectObservedState(ctx, nil)

			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(context.Canceled))
		})

		It("should call GetFrameworkState from dependencies", func() {
			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx, nil)

			Expect(err).ToNot(HaveOccurred())
			// The observed state should have metrics container
			typedObs, ok := observed.(snapshot.TransportObservedState)
			Expect(ok).To(BeTrue())
			Expect(typedObs.Metrics).NotTo(BeNil())
		})

		It("should populate FailedAuthConfig from dependencies", func() {
			// Set failed auth config on the worker's dependencies
			workerDeps := worker.GetDependencies()
			workerDeps.SetFailedAuthConfig("failed-token", "https://failed-relay.example.com", "failed-uuid")

			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx, nil)

			Expect(err).ToNot(HaveOccurred())
			typedObs, ok := observed.(snapshot.TransportObservedState)
			Expect(ok).To(BeTrue())
			Expect(typedObs.FailedAuthConfig.AuthToken).To(Equal("failed-token"))
			Expect(typedObs.FailedAuthConfig.RelayURL).To(Equal("https://failed-relay.example.com"))
			Expect(typedObs.FailedAuthConfig.InstanceUUID).To(Equal("failed-uuid"))
		})

		It("should return empty FailedAuthConfig when none is set", func() {
			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx, nil)

			Expect(err).ToNot(HaveOccurred())
			typedObs, ok := observed.(snapshot.TransportObservedState)
			Expect(ok).To(BeTrue())
			Expect(typedObs.FailedAuthConfig.IsEmpty()).To(BeTrue())
		})

		It("should call GetActionHistory from dependencies", func() {
			ctx := context.Background()
			observed, err := worker.CollectObservedState(ctx, nil)

			Expect(err).ToNot(HaveOccurred())
			// The observed state should have last action results (even if empty)
			typedObs, ok := observed.(snapshot.TransportObservedState)
			Expect(ok).To(BeTrue())
			// LastActionResults should be initialized (possibly empty slice)
			Expect(typedObs.LastActionResults).To(BeNil()) // Empty when no actions recorded
		})
	})

	Describe("DeriveDesiredState", func() {
		BeforeEach(func() {
			var err error
			worker, err = transport.NewTransportWorker(identity, logger, nil)
			Expect(err).ToNot(HaveOccurred())
		})

		Context("nil spec handling", func() {
			It("should handle nil spec gracefully", func() {
				desired, err := worker.DeriveDesiredState(nil)

				Expect(err).ToNot(HaveOccurred())
				Expect(desired).NotTo(BeNil())
				// Default to running when spec is nil
				Expect(desired.GetState()).To(Equal("running"))
			})

			It("should include PushWorker ChildrenSpecs even with nil spec", func() {
				desired, err := worker.DeriveDesiredState(nil)
				Expect(err).ToNot(HaveOccurred())

				transportDesired, ok := desired.(*snapshot.TransportDesiredState)
				Expect(ok).To(BeTrue())
				Expect(transportDesired.ChildrenSpecs).To(HaveLen(2))
				Expect(transportDesired.ChildrenSpecs[0].Name).To(Equal("push"))
				Expect(transportDesired.ChildrenSpecs[0].WorkerType).To(Equal("push"))
				Expect(transportDesired.ChildrenSpecs[1].Name).To(Equal("pull"))
				Expect(transportDesired.ChildrenSpecs[1].WorkerType).To(Equal("pull"))
			})
		})

		Context("valid spec handling", func() {
			It("should parse UserSpec config correctly", func() {
				spec := fsmv2types.UserSpec{
					Config: `relayURL: "https://relay.example.com"
instanceUUID: "test-uuid"
authToken: "test-token"`,
					Variables: fsmv2types.VariableBundle{},
				}

				desired, err := worker.DeriveDesiredState(spec)

				Expect(err).ToNot(HaveOccurred())
				Expect(desired).NotTo(BeNil())
				Expect(desired.GetState()).To(Equal("running"))

				// Type assert to access transport-specific fields
				transportDesired, ok := desired.(*snapshot.TransportDesiredState)
				Expect(ok).To(BeTrue())
				Expect(transportDesired.RelayURL).To(Equal("https://relay.example.com"))
				Expect(transportDesired.InstanceUUID).To(Equal("test-uuid"))
				Expect(transportDesired.AuthToken).To(Equal("test-token"))
			})

			It("should include PushWorker ChildrenSpecs", func() {
				spec := fsmv2types.UserSpec{
					Config: `relayURL: "https://relay.example.com"
instanceUUID: "test-uuid"
authToken: "test-token"`,
					Variables: fsmv2types.VariableBundle{},
				}

				desired, err := worker.DeriveDesiredState(spec)
				Expect(err).ToNot(HaveOccurred())

				transportDesired, ok := desired.(*snapshot.TransportDesiredState)
				Expect(ok).To(BeTrue())
				Expect(transportDesired.ChildrenSpecs).To(HaveLen(2))

				pushSpec := transportDesired.ChildrenSpecs[0]
				Expect(pushSpec.Name).To(Equal("push"))
				Expect(pushSpec.WorkerType).To(Equal("push"))
				Expect(pushSpec.ChildStartStates).To(ConsistOf("Running", "Degraded"))

				pullSpec := transportDesired.ChildrenSpecs[1]
				Expect(pullSpec.Name).To(Equal("pull"))
				Expect(pullSpec.WorkerType).To(Equal("pull"))
				Expect(pullSpec.ChildStartStates).To(ConsistOf("Running", "Degraded"))
			})

			It("should return stopped state when configured", func() {
				spec := fsmv2types.UserSpec{
					Config: `state: stopped
relayURL: "https://relay.example.com"`,
					Variables: fsmv2types.VariableBundle{},
				}

				desired, err := worker.DeriveDesiredState(spec)

				Expect(err).ToNot(HaveOccurred())
				Expect(desired.GetState()).To(Equal("stopped"))
			})

			It("should return running state when configured", func() {
				spec := fsmv2types.UserSpec{
					Config: `state: running
relayURL: "https://relay.example.com"
instanceUUID: "test-uuid"
authToken: "test-token"`,
					Variables: fsmv2types.VariableBundle{},
				}

				desired, err := worker.DeriveDesiredState(spec)

				Expect(err).ToNot(HaveOccurred())
				Expect(desired.GetState()).To(Equal("running"))
			})
		})

		Context("pure function requirement", func() {
			It("should be deterministic (same input produces same output)", func() {
				spec := fsmv2types.UserSpec{
					Config: `relayURL: "https://test.com"
instanceUUID: "test-uuid"
authToken: "test-token"`,
					Variables: fsmv2types.VariableBundle{},
				}

				desired1, err1 := worker.DeriveDesiredState(spec)
				desired2, err2 := worker.DeriveDesiredState(spec)

				Expect(err1).ToNot(HaveOccurred())
				Expect(err2).ToNot(HaveOccurred())
				Expect(desired1.GetState()).To(Equal(desired2.GetState()))
			})
		})

		Context("field validation when running", func() {
			It("should return error when relayURL is empty", func() {
				spec := fsmv2types.UserSpec{
					Config: `state: running
instanceUUID: "test-uuid"
authToken: "test-token"`,
					Variables: fsmv2types.VariableBundle{},
				}

				_, err := worker.DeriveDesiredState(spec)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("relayURL is required"))
			})

			It("should return error when instanceUUID is empty", func() {
				spec := fsmv2types.UserSpec{
					Config: `state: running
relayURL: "https://relay.example.com"
authToken: "test-token"`,
					Variables: fsmv2types.VariableBundle{},
				}

				_, err := worker.DeriveDesiredState(spec)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("instanceUUID is required"))
			})

			It("should return error when authToken is empty", func() {
				spec := fsmv2types.UserSpec{
					Config: `state: running
relayURL: "https://relay.example.com"
instanceUUID: "test-uuid"`,
					Variables: fsmv2types.VariableBundle{},
				}

				_, err := worker.DeriveDesiredState(spec)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("authToken is required"))
			})

			It("should not validate fields when state is stopped", func() {
				spec := fsmv2types.UserSpec{
					Config:    `state: stopped`,
					Variables: fsmv2types.VariableBundle{},
				}

				desired, err := worker.DeriveDesiredState(spec)

				Expect(err).ToNot(HaveOccurred())
				Expect(desired.GetState()).To(Equal("stopped"))
			})

			It("should default timeout when zero", func() {
				spec := fsmv2types.UserSpec{
					Config: `state: running
relayURL: "https://relay.example.com"
instanceUUID: "test-uuid"
authToken: "test-token"`,
					Variables: fsmv2types.VariableBundle{},
				}

				desired, err := worker.DeriveDesiredState(spec)

				Expect(err).ToNot(HaveOccurred())
				transportDesired := desired.(*snapshot.TransportDesiredState)
				Expect(transportDesired.Timeout).To(Equal(10 * time.Second))
			})
		})

		Context("invalid spec handling", func() {
			It("should return error for invalid YAML", func() {
				spec := fsmv2types.UserSpec{
					Config:    `invalid: [yaml: missing closing bracket`,
					Variables: fsmv2types.VariableBundle{},
				}

				_, err := worker.DeriveDesiredState(spec)

				Expect(err).To(HaveOccurred())
			})

			It("should return error for invalid spec type", func() {
				_, err := worker.DeriveDesiredState("invalid-string-spec")

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid spec type"))
			})
		})
	})

	Describe("GetInitialState", func() {
		BeforeEach(func() {
			var err error
			worker, err = transport.NewTransportWorker(identity, logger, nil)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return StoppedState", func() {
			initialState := worker.GetInitialState()

			Expect(initialState).To(BeAssignableToTypeOf(&state.StoppedState{}))
		})
	})

	Describe("Factory registration", func() {
		It("should be registered in the factory", func() {
			// The factory registration happens in init(), so we just need to verify
			// that we can retrieve a worker of type "transport"
			registeredTypes := factory.ListRegisteredTypes()
			Expect(registeredTypes).To(ContainElement("transport"))
		})

		It("should have matching supervisor factory", func() {
			// Check that supervisor factory is also registered
			workerOnly, supervisorOnly := factory.ValidateRegistryConsistency()
			Expect(workerOnly).NotTo(ContainElement("transport"))
			Expect(supervisorOnly).NotTo(ContainElement("transport"))
		})
	})

	Describe("Pointer receivers", func() {
		It("should use pointer receiver for all Worker methods", func() {
			// This is enforced at compile time by the interface implementation,
			// but we verify by testing that methods work on a pointer
			var err error
			worker, err = transport.NewTransportWorker(identity, logger, nil)
			Expect(err).ToNot(HaveOccurred())

			// All these should work with pointer receiver
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			_, err = worker.CollectObservedState(ctx, nil)
			Expect(err).ToNot(HaveOccurred())

			_, err = worker.DeriveDesiredState(nil)
			Expect(err).ToNot(HaveOccurred())

			initialState := worker.GetInitialState()
			Expect(initialState).NotTo(BeNil())
		})
	})
})
