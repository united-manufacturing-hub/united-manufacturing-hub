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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	transportpkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/pull"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

type mockTransport struct{}

func (m *mockTransport) Authenticate(_ context.Context, _ types.AuthRequest) (types.AuthResponse, error) {
	return types.AuthResponse{}, nil
}

func (m *mockTransport) Pull(_ context.Context, _ string) ([]*types.UMHMessage, error) {
	return nil, nil
}

func (m *mockTransport) Push(_ context.Context, _ string, _ []*types.UMHMessage) error {
	return nil
}

func (m *mockTransport) Close() {}

func (m *mockTransport) Reset() {}

type mockChannelProvider struct {
	inbound  chan<- *types.UMHMessage
	outbound <-chan *types.UMHMessage
}

func (m *mockChannelProvider) GetChannels(_ string) (
	inbound chan<- *types.UMHMessage,
	outbound <-chan *types.UMHMessage,
) {
	return m.inbound, m.outbound
}

func (m *mockChannelProvider) GetInboundStats(_ string) (capacity int, length int) {
	return 100, 0
}

func newTestChannelProvider() *mockChannelProvider {
	inboundBi := make(chan *types.UMHMessage, 100)
	outboundBi := make(chan *types.UMHMessage, 100)

	return &mockChannelProvider{
		inbound:  inboundBi,
		outbound: outboundBi,
	}
}

func createParentDeps(logger deps.FSMLogger) *transportpkg.TransportDependencies {
	mt := &mockTransport{}
	identity := deps.Identity{ID: "parent-id", WorkerType: "transport"}

	return transportpkg.NewTransportDependencies(mt, logger, nil, identity)
}

func makeMessages(n int) []*types.UMHMessage {
	msgs := make([]*types.UMHMessage, n)
	for i := range msgs {
		msgs[i] = &types.UMHMessage{}
	}

	return msgs
}

var _ = Describe("PullDependencies", func() {
	var (
		logger     deps.FSMLogger
		parentDeps *transportpkg.TransportDependencies
		identity   deps.Identity
	)

	BeforeEach(func() {
		logger = deps.NewNopFSMLogger()
		transportpkg.SetChannelProvider(newTestChannelProvider())
		parentDeps = createParentDeps(logger)
		identity = deps.Identity{ID: "pull-child-id", WorkerType: "pull"}
	})

	AfterEach(func() {
		transportpkg.ClearChannelProvider()
	})

	Describe("NewPullDependencies", func() {
		It("should return error with nil parentDeps", func() {
			d, err := pull.NewPullDependencies(nil, identity, logger, nil)
			Expect(err).To(HaveOccurred())
			Expect(d).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("parentDeps must not be nil"))
		})

		It("should return non-nil with valid parentDeps", func() {
			d, err := pull.NewPullDependencies(parentDeps, identity, logger, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(d).NotTo(BeNil())
		})
	})

	Describe("StorePendingMessages", func() {
		var d *pull.PullDependencies

		BeforeEach(func() {
			var err error
			d, err = pull.NewPullDependencies(parentDeps, identity, logger, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should store messages and report correct count", func() {
			msgs := makeMessages(5)
			d.StorePendingMessages(msgs)
			Expect(d.PendingMessageCount()).To(Equal(5))
		})

		It("should accumulate messages across multiple calls", func() {
			d.StorePendingMessages(makeMessages(3))
			d.StorePendingMessages(makeMessages(4))
			Expect(d.PendingMessageCount()).To(Equal(7))
		})

		It("should filter nil messages from a mixed slice", func() {
			msgs := []*types.UMHMessage{
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
			msgs := []*types.UMHMessage{nil, nil, nil}
			d.StorePendingMessages(msgs)
			Expect(d.PendingMessageCount()).To(Equal(0))
		})

		It("should cap at 1000 messages, keeping the newest", func() {
			first := makeMessages(800)
			d.StorePendingMessages(first)

			second := makeMessages(400)
			d.StorePendingMessages(second)

			Expect(d.PendingMessageCount()).To(Equal(1000))

			drained := d.DrainPendingMessages()
			Expect(drained).To(HaveLen(1000))

			Expect(drained[len(drained)-1]).To(BeIdenticalTo(second[len(second)-1]))
		})
	})

	Describe("DrainPendingMessages", func() {
		var d *pull.PullDependencies

		BeforeEach(func() {
			var err error
			d, err = pull.NewPullDependencies(parentDeps, identity, logger, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return all stored messages and clear the buffer", func() {
			msgs := makeMessages(10)
			d.StorePendingMessages(msgs)

			drained := d.DrainPendingMessages()
			Expect(drained).To(HaveLen(10))
			Expect(d.PendingMessageCount()).To(Equal(0))
		})

		It("should return only non-nil messages after mixed input", func() {
			msgs := []*types.UMHMessage{
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

		It("should return nil when buffer is empty", func() {
			drained := d.DrainPendingMessages()
			Expect(drained).To(BeNil())
		})
	})

	Describe("PendingMessageCount", func() {
		It("should return zero for a fresh instance", func() {
			d, err := pull.NewPullDependencies(parentDeps, identity, logger, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(d.PendingMessageCount()).To(Equal(0))
		})
	})

	Describe("IsBackpressured / SetBackpressured", func() {
		var d *pull.PullDependencies

		BeforeEach(func() {
			var err error
			d, err = pull.NewPullDependencies(parentDeps, identity, logger, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should default to false", func() {
			Expect(d.IsBackpressured()).To(BeFalse())
		})

		It("should reflect SetBackpressured(true)", func() {
			d.SetBackpressured(true)
			Expect(d.IsBackpressured()).To(BeTrue())
		})

		It("should reflect SetBackpressured(false) after being set to true", func() {
			d.SetBackpressured(true)
			d.SetBackpressured(false)
			Expect(d.IsBackpressured()).To(BeFalse())
		})
	})

	Describe("IsTokenValid", func() {
		var d *pull.PullDependencies

		BeforeEach(func() {
			var err error
			d, err = pull.NewPullDependencies(parentDeps, identity, logger, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return false when token is empty", func() {
			Expect(d.IsTokenValid()).To(BeFalse())
		})

		It("should return false when expiry is zero", func() {
			parentDeps.SetJWT("some-token", time.Time{})
			Expect(d.IsTokenValid()).To(BeFalse())
		})

		It("should return false when token is expired", func() {
			parentDeps.SetJWT("some-token", time.Now().Add(-10*time.Minute))
			Expect(d.IsTokenValid()).To(BeFalse())
		})

		It("should return false when token expires within the safety buffer", func() {
			parentDeps.SetJWT("some-token", time.Now().Add(30*time.Second))
			Expect(d.IsTokenValid()).To(BeFalse())
		})

		It("should return true when token is valid and not near expiry", func() {
			parentDeps.SetJWT("some-token", time.Now().Add(10*time.Minute))
			Expect(d.IsTokenValid()).To(BeTrue())
		})
	})

	Describe("CheckAndClearOnReset", func() {
		var d *pull.PullDependencies

		BeforeEach(func() {
			var err error
			d, err = pull.NewPullDependencies(parentDeps, identity, logger, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return true and clear pending messages when generation changes", func() {
			d.StorePendingMessages(makeMessages(5))
			d.SetBackpressured(true)

			parentDeps.IncrementResetGeneration()

			Expect(d.CheckAndClearOnReset()).To(BeTrue())
			Expect(d.PendingMessageCount()).To(Equal(0))
			Expect(d.IsBackpressured()).To(BeFalse())
		})

		It("should return false when generation has not changed", func() {
			parentDeps.IncrementResetGeneration()
			d.CheckAndClearOnReset()

			d.StorePendingMessages(makeMessages(3))
			d.SetBackpressured(true)

			Expect(d.CheckAndClearOnReset()).To(BeFalse())
			Expect(d.PendingMessageCount()).To(Equal(3))
			Expect(d.IsBackpressured()).To(BeTrue())
		})

		It("should only clear once per generation bump", func() {
			parentDeps.IncrementResetGeneration()

			Expect(d.CheckAndClearOnReset()).To(BeTrue())
			Expect(d.CheckAndClearOnReset()).To(BeFalse())
		})
	})

	Describe("Per-child error tracking", func() {
		var d *pull.PullDependencies

		BeforeEach(func() {
			var err error
			d, err = pull.NewPullDependencies(parentDeps, identity, logger, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		Describe("GetInboundChan", func() {
			It("should return the same channel as the parent", func() {
				Expect(d.GetInboundChan()).To(Equal(parentDeps.GetInboundChan()))
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
			It("should record on both child and parent", func() {
				d.RecordTypedError(types.ErrorTypeBackendRateLimit, 30*time.Second)
				Expect(d.GetConsecutiveErrors()).To(Equal(1))
				Expect(parentDeps.GetConsecutiveErrors()).To(Equal(1))
				Expect(parentDeps.GetLastErrorType()).To(Equal(types.ErrorTypeBackendRateLimit))
			})
		})

		Describe("RecordSuccess", func() {
			It("should zero child errors but not parent errors", func() {
				parentDeps.RecordError()
				parentDeps.RecordError()
				Expect(parentDeps.GetConsecutiveErrors()).To(Equal(2))

				d.RecordError()
				d.RecordError()
				Expect(d.GetConsecutiveErrors()).To(Equal(2))
				Expect(parentDeps.GetConsecutiveErrors()).To(Equal(4))

				d.RecordSuccess()
				Expect(d.GetConsecutiveErrors()).To(Equal(0))
				Expect(parentDeps.GetConsecutiveErrors()).To(Equal(4))
			})
		})

		Describe("RecordError", func() {
			It("should record on both child and parent", func() {
				d.RecordError()
				d.RecordError()
				Expect(d.GetConsecutiveErrors()).To(Equal(2))
				Expect(parentDeps.GetConsecutiveErrors()).To(Equal(2))
			})
		})

		Describe("GetConsecutiveErrors", func() {
			It("should read from child tracker, not parent", func() {
				Expect(d.GetConsecutiveErrors()).To(Equal(0))
				parentDeps.RecordError()
				parentDeps.RecordError()
				parentDeps.RecordError()
				Expect(d.GetConsecutiveErrors()).To(Equal(0))
			})
		})

		Describe("GetLastErrorType", func() {
			It("should read from child field", func() {
				d.RecordTypedError(types.ErrorTypeNetwork, 0)
				Expect(d.GetLastErrorType()).To(Equal(types.ErrorTypeNetwork))
				d.RecordSuccess()
				Expect(d.GetLastErrorType()).To(Equal(types.ErrorType(0)))
			})
		})

		Describe("GetDegradedEnteredAt", func() {
			It("should read from child tracker", func() {
				Expect(d.GetDegradedEnteredAt().IsZero()).To(BeTrue())
				d.RecordError()
				Expect(d.GetDegradedEnteredAt().IsZero()).To(BeFalse())
				d.RecordSuccess()
				Expect(d.GetDegradedEnteredAt().IsZero()).To(BeTrue())
			})
		})

		Describe("GetLastErrorAt", func() {
			It("should read from child tracker", func() {
				before := time.Now()
				d.RecordTypedError(types.ErrorTypeNetwork, 0)
				Expect(d.GetLastErrorAt()).To(BeTemporally(">=", before))
			})
		})

		Describe("GetLastRetryAfter", func() {
			It("should read from child tracker", func() {
				d.RecordTypedError(types.ErrorTypeBackendRateLimit, 30*time.Second)
				Expect(d.GetLastRetryAfter()).To(Equal(30 * time.Second))
			})
		})
	})

	Describe("BaseDependencies", func() {
		It("should have its own logger", func() {
			d, err := pull.NewPullDependencies(parentDeps, identity, logger, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(d.GetLogger()).NotTo(BeNil())
		})
	})
})
