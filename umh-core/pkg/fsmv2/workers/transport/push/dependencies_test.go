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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	communicator_transport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push"
)

type mockTransport struct{}

func (m *mockTransport) Authenticate(_ context.Context, _ communicator_transport.AuthRequest) (communicator_transport.AuthResponse, error) {
	return communicator_transport.AuthResponse{}, nil
}

func (m *mockTransport) Pull(_ context.Context, _ string) ([]*communicator_transport.UMHMessage, error) {
	return nil, nil
}

func (m *mockTransport) Push(_ context.Context, _ string, _ []*communicator_transport.UMHMessage) error {
	return nil
}

func (m *mockTransport) Close() {}

func (m *mockTransport) Reset() {}

type mockChannelProvider struct {
	inbound  chan<- *communicator_transport.UMHMessage
	outbound <-chan *communicator_transport.UMHMessage
}

func (m *mockChannelProvider) GetChannels(_ string) (
	inbound chan<- *communicator_transport.UMHMessage,
	outbound <-chan *communicator_transport.UMHMessage,
) {
	return m.inbound, m.outbound
}

func (m *mockChannelProvider) GetInboundStats(_ string) (capacity int, length int) {
	return 100, 0
}

func newTestChannelProvider() *mockChannelProvider {
	inboundBi := make(chan *communicator_transport.UMHMessage, 100)
	outboundBi := make(chan *communicator_transport.UMHMessage, 100)

	return &mockChannelProvider{
		inbound:  inboundBi,
		outbound: outboundBi,
	}
}

func createParentDeps(logger deps.FSMLogger) *transport.TransportDependencies {
	mt := &mockTransport{}
	identity := deps.Identity{ID: "parent-id", WorkerType: "transport"}

	return transport.NewTransportDependencies(mt, logger, nil, identity)
}

var _ = Describe("PushDependencies", func() {
	var (
		logger     deps.FSMLogger
		parentDeps *transport.TransportDependencies
		identity   deps.Identity
	)

	BeforeEach(func() {
		logger = deps.NewNopFSMLogger()
		transport.SetChannelProvider(newTestChannelProvider())
		parentDeps = createParentDeps(logger)
		identity = deps.Identity{ID: "push-child-id", WorkerType: "push"}
	})

	AfterEach(func() {
		transport.ClearChannelProvider()
	})

	Describe("NewPushDependencies", func() {
		It("should return non-nil with valid parentDeps", func() {
			d, err := push.NewPushDependencies(parentDeps, identity, logger, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(d).NotTo(BeNil())
		})

		It("should return error with nil parentDeps", func() {
			d, err := push.NewPushDependencies(nil, identity, logger, nil)
			Expect(err).To(HaveOccurred())
			Expect(d).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("parentDeps must not be nil"))
		})
	})

	Describe("Delegation to parent", func() {
		var d *push.PushDependencies

		BeforeEach(func() {
			var err error
			d, err = push.NewPushDependencies(parentDeps, identity, logger, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		Describe("GetOutboundChan", func() {
			It("should return the same channel as the parent", func() {
				Expect(d.GetOutboundChan()).To(Equal(parentDeps.GetOutboundChan()))
			})
		})

		Describe("GetTransport", func() {
			It("should return the same transport as the parent", func() {
				Expect(d.GetTransport()).To(Equal(parentDeps.GetTransport()))
			})

			It("should reflect parent transport changes", func() {
				newMt := &mockTransport{}
				parentDeps.SetTransport(newMt)
				Expect(d.GetTransport()).To(Equal(newMt))
			})
		})

		Describe("GetJWTToken", func() {
			It("should return empty when parent has no token", func() {
				Expect(d.GetJWTToken()).To(BeEmpty())
			})

			It("should reflect parent JWT changes", func() {
				parentDeps.SetJWT("test-token", time.Now().Add(time.Hour))
				Expect(d.GetJWTToken()).To(Equal("test-token"))
			})
		})

		Describe("RecordTypedError", func() {
			It("should delegate to parent", func() {
				d.RecordTypedError(httpTransport.ErrorTypeBackendRateLimit, 30*time.Second)
				Expect(parentDeps.GetLastErrorType()).To(Equal(httpTransport.ErrorTypeBackendRateLimit))
				Expect(parentDeps.GetConsecutiveErrors()).To(Equal(1))
			})
		})

		Describe("RecordSuccess", func() {
			It("should delegate to parent", func() {
				parentDeps.RecordError()
				parentDeps.RecordError()
				Expect(parentDeps.GetConsecutiveErrors()).To(Equal(2))

				d.RecordSuccess()
				Expect(parentDeps.GetConsecutiveErrors()).To(Equal(0))
			})
		})

		Describe("RecordError", func() {
			It("should delegate to parent", func() {
				d.RecordError()
				d.RecordError()
				Expect(parentDeps.GetConsecutiveErrors()).To(Equal(2))
			})
		})

		Describe("GetConsecutiveErrors", func() {
			It("should return parent consecutive errors", func() {
				Expect(d.GetConsecutiveErrors()).To(Equal(0))
				parentDeps.RecordError()
				parentDeps.RecordError()
				parentDeps.RecordError()
				Expect(d.GetConsecutiveErrors()).To(Equal(3))
			})
		})

		Describe("GetLastErrorType", func() {
			It("should return parent last error type", func() {
				parentDeps.RecordTypedError(httpTransport.ErrorTypeNetwork, 0)
				Expect(d.GetLastErrorType()).To(Equal(httpTransport.ErrorTypeNetwork))
			})
		})
	})

	Describe("StorePendingMessages nil filtering", func() {
		var d *push.PushDependencies

		BeforeEach(func() {
			var err error
			d, err = push.NewPushDependencies(parentDeps, identity, logger, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should filter nil messages from a mixed slice", func() {
			msgs := []*communicator_transport.UMHMessage{
				{Content: "a"},
				nil,
				{Content: "b"},
				nil,
				{Content: "c"},
			}
			d.StorePendingMessages(msgs)
			Expect(d.PendingMessageCount()).To(Equal(3))
		})

		It("should store nothing when all messages are nil", func() {
			msgs := []*communicator_transport.UMHMessage{nil, nil, nil}
			d.StorePendingMessages(msgs)
			Expect(d.PendingMessageCount()).To(Equal(0))
		})

		It("should return only non-nil messages after mixed input via drain", func() {
			msgs := []*communicator_transport.UMHMessage{
				nil,
				{Content: "x"},
				nil,
				{Content: "y"},
			}
			d.StorePendingMessages(msgs)

			drained := d.DrainPendingMessages()
			Expect(drained).To(HaveLen(2))
			Expect(drained[0].Content).To(Equal("x"))
			Expect(drained[1].Content).To(Equal("y"))
			Expect(d.PendingMessageCount()).To(Equal(0))
		})
	})

	Describe("StorePendingMessages overflow", func() {
		var d *push.PushDependencies

		BeforeEach(func() {
			var err error
			d, err = push.NewPushDependencies(parentDeps, identity, logger, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should increment CounterMessagesDropped when buffer overflows", func() {
			msgs := make([]*communicator_transport.UMHMessage, 1005)
			for i := range msgs {
				msgs[i] = &communicator_transport.UMHMessage{Content: fmt.Sprintf("msg-%d", i)}
			}

			d.StorePendingMessages(msgs)

			drained := d.MetricsRecorder().Drain()
			Expect(drained.Counters[string(deps.CounterMessagesDropped)]).To(Equal(int64(5)))
		})
	})

	Describe("IsTokenValid", func() {
		var d *push.PushDependencies

		BeforeEach(func() {
			var err error
			d, err = push.NewPushDependencies(parentDeps, identity, logger, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return false when token is empty", func() {
			Expect(d.IsTokenValid()).To(BeFalse())
		})

		It("should return false when token is expired", func() {
			parentDeps.SetJWT("expired-token", time.Now().Add(-1*time.Hour))
			Expect(d.IsTokenValid()).To(BeFalse())
		})

		It("should return true when token is valid and not near expiry", func() {
			parentDeps.SetJWT("good-token", time.Now().Add(1*time.Hour))
			Expect(d.IsTokenValid()).To(BeTrue())
		})
	})

	Describe("BaseDependencies", func() {
		It("should have its own logger", func() {
			d, err := push.NewPushDependencies(parentDeps, identity, logger, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(d.GetLogger()).NotTo(BeNil())
		})
	})
})
