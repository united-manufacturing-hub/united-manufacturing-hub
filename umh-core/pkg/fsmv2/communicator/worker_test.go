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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/communicator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/communicator/transport"
)

func TestCommunicator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Communicator Suite")
}

var _ = Describe("CommunicatorWorker", func() {
	var (
		worker       *communicator.CommunicatorWorker
		ctx          context.Context
		mockTransport *MockTransport
		inboundChan  chan *transport.UMHMessage
		outboundChan chan *transport.UMHMessage
		logger       *zap.SugaredLogger
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = zap.NewNop().Sugar()
		mockTransport = NewMockTransport()
		inboundChan = make(chan *transport.UMHMessage, 100)
		outboundChan = make(chan *transport.UMHMessage, 100)
		worker = communicator.NewCommunicatorWorker(
			"test-id",
			"https://relay.example.com",
			inboundChan,
			outboundChan,
			mockTransport,
			"instance-uuid",
			"auth-token",
			logger,
		)
	})

	AfterEach(func() {
		close(inboundChan)
		close(outboundChan)
	})

	Describe("Worker interface implementation", func() {
		It("should create a new CommunicatorWorker with channels", func() {
			Expect(worker).NotTo(BeNil())
		})
	})

	Describe("GetInitialState", func() {
		It("should return StoppedState", func() {
			initialState := worker.GetInitialState()
			Expect(initialState).To(BeAssignableToTypeOf(&communicator.StoppedState{}))
		})
	})

	Describe("DeriveDesiredState", func() {
		Context("with nil spec", func() {
			It("should return default desired state", func() {
				desired, err := worker.DeriveDesiredState(nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(desired).NotTo(BeNil())

				communicatorDesired := desired.(*communicator.CommunicatorDesiredState)
				Expect(communicatorDesired.ShutdownRequested()).To(BeFalse())
			})
		})
	})

	Describe("CollectObservedState", func() {
		It("should return observed state with channel queue sizes", func() {
			// Add some messages to channels
			outboundChan <- &transport.UMHMessage{InstanceUUID: "test", Content: "msg1"}
			outboundChan <- &transport.UMHMessage{InstanceUUID: "test", Content: "msg2"}

			observed, err := worker.CollectObservedState(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(observed).NotTo(BeNil())

			communicatorObserved := observed.(*communicator.CommunicatorObservedState)
			Expect(communicatorObserved.CollectedAt).NotTo(BeZero())
			Expect(communicatorObserved.GetOutboundQueueSize()).To(Equal(2))
			Expect(communicatorObserved.GetInboundQueueSize()).To(Equal(0))
		})
	})
})
