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

package communicator_test

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/state"
	transportpkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
)

func TestCommunicator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Communicator Suite")
}

var _ = Describe("CommunicatorWorker", func() {
	var (
		worker        *communicator.CommunicatorWorker
		ctx           context.Context
		mockTransport *MockTransport
		logger        *zap.SugaredLogger
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = zap.NewNop().Sugar()
		mockTransport = NewMockTransport()
		var err error
		worker, err = communicator.NewCommunicatorWorker(
			"test-id",
			"Test Communicator",
			mockTransport,
			logger,
			nil,
		)
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("Worker interface implementation", func() {
		It("should create a new CommunicatorWorker with channels", func() {
			Expect(worker).NotTo(BeNil())
		})
	})

	Describe("GetInitialState", func() {
		It("should return StoppedState", func() {
			initialState := worker.GetInitialState()
			Expect(initialState).To(BeAssignableToTypeOf(&state.StoppedState{}))
		})
	})

	Describe("DeriveDesiredState", func() {
		Context("with nil spec", func() {
			It("should return default desired state", func() {
				desired, err := worker.DeriveDesiredState(nil)
				Expect(err).NotTo(HaveOccurred())

				Expect(desired.State).To(Equal("running"))
				Expect(desired.IsShutdownRequested()).To(BeFalse())
				Expect(desired.ChildrenSpecs).To(BeNil())
			})
		})
	})

	Describe("CollectObservedState", func() {
		It("should return observed state with CollectedAt timestamp", func() {
			observed, err := worker.CollectObservedState(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(observed).NotTo(BeNil())

			communicatorObserved := observed.(snapshot.CommunicatorObservedState)
			Expect(communicatorObserved.CollectedAt).NotTo(BeZero())
		})

		It("should return the JWT token stored in dependencies", func() {
			// Arrange: Set JWT token in dependencies
			deps := worker.GetDependencies()
			expectedToken := "test-jwt-token-12345"
			deps.SetJWT(expectedToken, time.Now().Add(1*time.Hour))

			// Act
			observed, err := worker.CollectObservedState(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Assert
			communicatorObserved := observed.(snapshot.CommunicatorObservedState)
			Expect(communicatorObserved.JWTToken).To(Equal(expectedToken))
		})

		It("should return the JWT expiry stored in dependencies", func() {
			// Arrange: Set JWT expiry in dependencies
			deps := worker.GetDependencies()
			expectedExpiry := time.Now().Add(2 * time.Hour).Truncate(time.Second)
			deps.SetJWT("some-token", expectedExpiry)

			// Act
			observed, err := worker.CollectObservedState(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Assert
			communicatorObserved := observed.(snapshot.CommunicatorObservedState)
			// Compare truncated times to avoid nanosecond precision issues
			Expect(communicatorObserved.JWTExpiry.Truncate(time.Second)).To(Equal(expectedExpiry))
		})

		It("should return the pulled messages stored in dependencies", func() {
			// Arrange: Set pulled messages in dependencies
			deps := worker.GetDependencies()
			expectedMessages := []*transportpkg.UMHMessage{
				{Content: "message-1"},
				{Content: "message-2"},
			}
			deps.SetPulledMessages(expectedMessages)

			// Act
			observed, err := worker.CollectObservedState(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Assert
			communicatorObserved := observed.(snapshot.CommunicatorObservedState)
			Expect(communicatorObserved.MessagesReceived).To(HaveLen(2))
			Expect(communicatorObserved.MessagesReceived[0].Content).To(Equal("message-1"))
			Expect(communicatorObserved.MessagesReceived[1].Content).To(Equal("message-2"))
		})

		It("should return the consecutive error count from dependencies", func() {
			// Arrange: Record some errors in dependencies
			deps := worker.GetDependencies()
			deps.RecordError()
			deps.RecordError()
			deps.RecordError()

			// Act
			observed, err := worker.CollectObservedState(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Assert
			communicatorObserved := observed.(snapshot.CommunicatorObservedState)
			Expect(communicatorObserved.ConsecutiveErrors).To(Equal(3))
		})

		It("should set Authenticated to true when JWT token is present and not expired", func() {
			// Arrange: Set valid JWT in dependencies
			deps := worker.GetDependencies()
			deps.SetJWT("valid-token", time.Now().Add(1*time.Hour))

			// Act
			observed, err := worker.CollectObservedState(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Assert
			communicatorObserved := observed.(snapshot.CommunicatorObservedState)
			Expect(communicatorObserved.Authenticated).To(BeTrue())
		})

		It("should set Authenticated to false when JWT token is empty", func() {
			// Arrange: No JWT token set (default state)
			// Dependencies start with empty JWT

			// Act
			observed, err := worker.CollectObservedState(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Assert
			communicatorObserved := observed.(snapshot.CommunicatorObservedState)
			Expect(communicatorObserved.Authenticated).To(BeFalse())
		})
	})
})
