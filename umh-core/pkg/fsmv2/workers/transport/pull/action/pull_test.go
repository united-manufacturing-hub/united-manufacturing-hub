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

package action_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/pull/action"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/pull/snapshot"
)

var _ snapshot.PullDependencies = (*mockPullDeps)(nil)

type mockTransport struct {
	pullErr       error
	pullCallCount int
	pullMessages  []*transport.UMHMessage
	pullFunc      func(ctx context.Context, jwtToken string) ([]*transport.UMHMessage, error)
}

func (m *mockTransport) Authenticate(_ context.Context, _ transport.AuthRequest) (transport.AuthResponse, error) {
	return transport.AuthResponse{}, nil
}

func (m *mockTransport) Pull(ctx context.Context, jwtToken string) ([]*transport.UMHMessage, error) {
	m.pullCallCount++

	if m.pullFunc != nil {
		return m.pullFunc(ctx, jwtToken)
	}

	return m.pullMessages, m.pullErr
}

func (m *mockTransport) Push(_ context.Context, _ string, _ []*transport.UMHMessage) error {
	return nil
}

func (m *mockTransport) Close() {}

func (m *mockTransport) Reset() {}

type mockPullDeps struct {
	transport       transport.Transport
	jwtToken        string
	metricsRecorder *deps.MetricsRecorder
	logger          *zap.SugaredLogger

	inboundBi    chan *transport.UMHMessage
	chanCapacity int
	chanLength   int

	recordTypedErrorCalls []typedErrorCall
	recordSuccessCalls    int
	recordErrorCalls      int
	consecutiveErrors     int
	lastErrorType         httpTransport.ErrorType

	pendingMessages   []*transport.UMHMessage
	tokenValid        bool
	resetGeneration   uint64
	resetCleared      bool
	lastRetryAfter    time.Duration
	degradedEnteredAt time.Time
	lastErrorAt       time.Time

	backpressured bool
}

type typedErrorCall struct {
	errType    httpTransport.ErrorType
	retryAfter time.Duration
}

func newMockPullDeps() *mockPullDeps {
	return &mockPullDeps{
		metricsRecorder: deps.NewMetricsRecorder(),
		logger:          zap.NewNop().Sugar(),
		tokenValid:      true,
		chanCapacity:    1000,
		chanLength:      0,
	}
}

func (m *mockPullDeps) GetLogger() *zap.SugaredLogger {
	return m.logger
}

func (m *mockPullDeps) ActionLogger(_ string) *zap.SugaredLogger {
	return m.logger
}

func (m *mockPullDeps) GetStateReader() deps.StateReader {
	return nil
}

func (m *mockPullDeps) GetInboundChan() chan<- *transport.UMHMessage {
	if m.inboundBi == nil {
		return nil
	}

	return m.inboundBi
}

func (m *mockPullDeps) GetInboundChanStats() (capacity int, length int) {
	return m.chanCapacity, m.chanLength
}

func (m *mockPullDeps) IsBackpressured() bool {
	return m.backpressured
}

func (m *mockPullDeps) SetBackpressured(v bool) {
	m.backpressured = v
}

func (m *mockPullDeps) GetTransport() transport.Transport {
	return m.transport
}

func (m *mockPullDeps) GetJWTToken() string {
	return m.jwtToken
}

func (m *mockPullDeps) RecordTypedError(errType httpTransport.ErrorType, retryAfter time.Duration) {
	m.recordTypedErrorCalls = append(m.recordTypedErrorCalls, typedErrorCall{
		errType:    errType,
		retryAfter: retryAfter,
	})
}

func (m *mockPullDeps) RecordSuccess() {
	m.recordSuccessCalls++
}

func (m *mockPullDeps) RecordError() {
	m.recordErrorCalls++
}

func (m *mockPullDeps) GetConsecutiveErrors() int {
	return m.consecutiveErrors
}

func (m *mockPullDeps) GetLastErrorType() httpTransport.ErrorType {
	return m.lastErrorType
}

func (m *mockPullDeps) MetricsRecorder() *deps.MetricsRecorder {
	return m.metricsRecorder
}

func (m *mockPullDeps) StorePendingMessages(msgs []*transport.UMHMessage) {
	m.pendingMessages = append(m.pendingMessages, msgs...)
}

func (m *mockPullDeps) DrainPendingMessages() []*transport.UMHMessage {
	msgs := m.pendingMessages
	m.pendingMessages = nil

	return msgs
}

func (m *mockPullDeps) PendingMessageCount() int {
	return len(m.pendingMessages)
}

func (m *mockPullDeps) IsTokenValid() bool {
	return m.tokenValid
}

func (m *mockPullDeps) GetResetGeneration() uint64 {
	return m.resetGeneration
}

func (m *mockPullDeps) CheckAndClearOnReset() bool {
	if m.resetCleared {
		m.pendingMessages = nil
		m.resetCleared = false

		return true
	}

	return false
}

func (m *mockPullDeps) GetLastRetryAfter() time.Duration {
	return m.lastRetryAfter
}

func (m *mockPullDeps) GetDegradedEnteredAt() time.Time {
	return m.degradedEnteredAt
}

func (m *mockPullDeps) GetLastErrorAt() time.Time {
	return m.lastErrorAt
}

var _ = Describe("PullAction", func() {
	var (
		act       *action.PullAction
		mockDeps  *mockPullDeps
		mockTrans *mockTransport
		inboundBi chan *transport.UMHMessage
	)

	BeforeEach(func() {
		act = &action.PullAction{}
		mockTrans = &mockTransport{}
		inboundBi = make(chan *transport.UMHMessage, 100)
		mockDeps = newMockPullDeps()
		mockDeps.inboundBi = inboundBi
		mockDeps.transport = mockTrans
		mockDeps.jwtToken = "test-jwt"
	})

	Describe("Successful pull", func() {
		It("should pull messages, deliver to inbound channel, and record metrics", func() {
			mockTrans.pullMessages = []*transport.UMHMessage{
				{InstanceUUID: "uuid-1", Content: "msg1", Email: "a@b.com"},
				{InstanceUUID: "uuid-2", Content: "msg2", Email: "c@d.com"},
			}

			err := act.Execute(context.Background(), mockDeps)
			Expect(err).NotTo(HaveOccurred())

			Expect(mockTrans.pullCallCount).To(Equal(1))
			Expect(mockDeps.recordSuccessCalls).To(Equal(1))

			Expect(inboundBi).To(HaveLen(2))

			drained := mockDeps.metricsRecorder.Drain()
			Expect(drained.Counters[string(deps.CounterPullOps)]).To(Equal(int64(1)))
			Expect(drained.Counters[string(deps.CounterPullSuccess)]).To(Equal(int64(1)))
			Expect(drained.Counters[string(deps.CounterMessagesPulled)]).To(Equal(int64(2)))
			Expect(drained.Counters[string(deps.CounterBytesPulled)]).To(BeNumerically(">", 0))
			Expect(drained.Gauges[string(deps.GaugeLastPullLatencyMs)]).To(BeNumerically(">=", 0))
		})
	})

	Describe("Failed pull with TransportError", func() {
		It("should record typed error and increment failure counter", func() {
			mockTrans.pullErr = &httpTransport.TransportError{
				Type:       httpTransport.ErrorTypeServerError,
				Message:    "HTTP 500: server_error",
				RetryAfter: 30 * time.Second,
			}

			err := act.Execute(context.Background(), mockDeps)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("pull failed"))

			Expect(mockDeps.recordTypedErrorCalls).To(HaveLen(1))
			Expect(mockDeps.recordTypedErrorCalls[0].errType).To(Equal(httpTransport.ErrorTypeServerError))
			Expect(mockDeps.recordTypedErrorCalls[0].retryAfter).To(Equal(30 * time.Second))

			drained := mockDeps.metricsRecorder.Drain()
			Expect(drained.Counters[string(deps.CounterPullOps)]).To(Equal(int64(1)))
			Expect(drained.Counters[string(deps.CounterPullFailures)]).To(Equal(int64(1)))
			Expect(drained.Counters[string(deps.CounterServerErrorsTotal)]).To(Equal(int64(1)))
		})
	})

	Describe("Failed pull with non-TransportError", func() {
		It("should default to ErrorTypeNetwork", func() {
			mockTrans.pullErr = errors.New("connection refused")

			err := act.Execute(context.Background(), mockDeps)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("pull failed"))

			Expect(mockDeps.recordTypedErrorCalls).To(HaveLen(1))
			Expect(mockDeps.recordTypedErrorCalls[0].errType).To(Equal(httpTransport.ErrorTypeNetwork))
			Expect(mockDeps.recordTypedErrorCalls[0].retryAfter).To(Equal(time.Duration(0)))

			drained := mockDeps.metricsRecorder.Drain()
			Expect(drained.Counters[string(deps.CounterPullFailures)]).To(Equal(int64(1)))
			Expect(drained.Counters[string(deps.CounterNetworkErrorsTotal)]).To(Equal(int64(1)))
		})
	})

	Describe("Empty pull (no messages)", func() {
		It("should call RecordSuccess with no delivery", func() {
			mockTrans.pullMessages = nil

			err := act.Execute(context.Background(), mockDeps)
			Expect(err).NotTo(HaveOccurred())

			Expect(mockTrans.pullCallCount).To(Equal(1))
			Expect(mockDeps.recordSuccessCalls).To(Equal(1))

			Expect(inboundBi).To(BeEmpty())

			drained := mockDeps.metricsRecorder.Drain()
			Expect(drained.Counters[string(deps.CounterPullOps)]).To(Equal(int64(1)))
			Expect(drained.Counters[string(deps.CounterPullSuccess)]).To(Equal(int64(1)))
			Expect(drained.Counters[string(deps.CounterMessagesPulled)]).To(Equal(int64(0)))
		})
	})

	Describe("Context cancellation", func() {
		It("should return ctx.Err()", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			err := act.Execute(ctx, mockDeps)
			Expect(err).To(Equal(context.Canceled))
			Expect(mockTrans.pullCallCount).To(Equal(0))
		})
	})

	Describe("Nil transport", func() {
		It("should return error without pulling", func() {
			mockDeps.transport = nil

			err := act.Execute(context.Background(), mockDeps)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("transport is nil"))

			Expect(mockTrans.pullCallCount).To(Equal(0))
		})
	})

	Describe("Nil inbound channel", func() {
		It("should return nil (no-op)", func() {
			mockDeps.inboundBi = nil

			err := act.Execute(context.Background(), mockDeps)
			Expect(err).NotTo(HaveOccurred())
			Expect(mockTrans.pullCallCount).To(Equal(0))
		})
	})

	Describe("Invalid dependencies type", func() {
		It("should return error for wrong deps type", func() {
			err := act.Execute(context.Background(), "not-deps")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid dependencies type"))
		})
	})

	Describe("Token pre-check", func() {
		It("should skip pull when token is invalid (without pulling)", func() {
			mockDeps.tokenValid = false

			err := act.Execute(context.Background(), mockDeps)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("token not valid"))

			Expect(mockTrans.pullCallCount).To(Equal(0))
		})
	})

	Describe("Backpressure entry", func() {
		It("should skip pull when channel near full, set backpressured=true, and record metrics", func() {
			mockDeps.chanCapacity = 200
			mockDeps.chanLength = 160
			mockDeps.backpressured = false

			err := act.Execute(context.Background(), mockDeps)
			Expect(err).NotTo(HaveOccurred())

			Expect(mockDeps.backpressured).To(BeTrue())
			Expect(mockTrans.pullCallCount).To(Equal(0))

			drained := mockDeps.metricsRecorder.Drain()
			Expect(drained.Gauges[string(deps.GaugeBackpressureActive)]).To(Equal(float64(1)))
			Expect(drained.Counters[string(deps.CounterBackpressureEntryTotal)]).To(Equal(int64(1)))
		})
	})

	Describe("Backpressure exit", func() {
		It("should resume when capacity recovered (available >= 100), set backpressured=false", func() {
			mockDeps.chanCapacity = 1000
			mockDeps.chanLength = 800
			mockDeps.backpressured = true

			mockTrans.pullMessages = []*transport.UMHMessage{
				{Content: "msg1"},
			}

			err := act.Execute(context.Background(), mockDeps)
			Expect(err).NotTo(HaveOccurred())

			Expect(mockDeps.backpressured).To(BeFalse())
			Expect(mockTrans.pullCallCount).To(Equal(1))

			drained := mockDeps.metricsRecorder.Drain()
			Expect(drained.Gauges[string(deps.GaugeBackpressureActive)]).To(Equal(float64(0)))
		})
	})

	Describe("Backpressure hysteresis", func() {
		It("should not exit backpressure when available < LowWaterMark (100)", func() {
			mockDeps.chanCapacity = 200
			mockDeps.chanLength = 130
			mockDeps.backpressured = true

			err := act.Execute(context.Background(), mockDeps)
			Expect(err).NotTo(HaveOccurred())

			Expect(mockDeps.backpressured).To(BeTrue())
			Expect(mockTrans.pullCallCount).To(Equal(0))
		})
	})

	Describe("Backpressure is not an error", func() {
		It("should not call RecordTypedError or RecordSuccess during backpressure skip", func() {
			mockDeps.chanCapacity = 200
			mockDeps.chanLength = 160
			mockDeps.backpressured = false

			err := act.Execute(context.Background(), mockDeps)
			Expect(err).NotTo(HaveOccurred())

			Expect(mockDeps.recordTypedErrorCalls).To(BeEmpty())
			Expect(mockDeps.recordSuccessCalls).To(Equal(0))
		})
	})

	Describe("Pending delivery", func() {
		It("should deliver pending messages before doing new pull", func() {
			mockDeps.pendingMessages = []*transport.UMHMessage{
				{Content: "pending1"},
				{Content: "pending2"},
			}

			mockTrans.pullMessages = []*transport.UMHMessage{
				{Content: "new-msg"},
			}

			err := act.Execute(context.Background(), mockDeps)
			Expect(err).NotTo(HaveOccurred())

			Expect(mockTrans.pullCallCount).To(Equal(0))

			Expect(inboundBi).To(HaveLen(2))
			msg1 := <-inboundBi
			Expect(msg1.Content).To(Equal("pending1"))
			msg2 := <-inboundBi
			Expect(msg2.Content).To(Equal("pending2"))

			Expect(mockDeps.recordSuccessCalls).To(Equal(1))

			drained := mockDeps.metricsRecorder.Drain()
			Expect(drained.Counters[string(deps.CounterPullOps)]).To(Equal(int64(1)))
			Expect(drained.Counters[string(deps.CounterPullSuccess)]).To(Equal(int64(1)))
			Expect(drained.Counters[string(deps.CounterPendingDelivered)]).To(Equal(int64(2)))
		})
	})

	Describe("Pending partial delivery", func() {
		It("should store remaining messages when channel full during pending delivery", func() {
			smallChan := make(chan *transport.UMHMessage, 1)
			mockDeps.inboundBi = smallChan

			mockDeps.pendingMessages = []*transport.UMHMessage{
				{Content: "pending1"},
				{Content: "pending2"},
				{Content: "pending3"},
			}

			err := act.Execute(context.Background(), mockDeps)
			Expect(err).NotTo(HaveOccurred())

			Expect(smallChan).To(HaveLen(1))
			Expect(mockDeps.PendingMessageCount()).To(Equal(2))

			Expect(mockDeps.recordSuccessCalls).To(Equal(0))
		})
	})

	Describe("Reset generation", func() {
		It("should clear pending messages on parent reset", func() {
			mockDeps.pendingMessages = []*transport.UMHMessage{
				{Content: "stale1"},
				{Content: "stale2"},
			}
			mockDeps.resetCleared = true

			err := act.Execute(context.Background(), mockDeps)
			Expect(err).NotTo(HaveOccurred())

			Expect(mockDeps.PendingMessageCount()).To(Equal(0))
		})

		It("should clear backpressure on parent reset when capacity recovers", func() {
			mockDeps.pendingMessages = []*transport.UMHMessage{
				{Content: "stale1"},
			}
			mockDeps.resetCleared = true
			mockDeps.backpressured = true
			mockDeps.chanCapacity = 1000
			mockDeps.chanLength = 0

			err := act.Execute(context.Background(), mockDeps)
			Expect(err).NotTo(HaveOccurred())

			Expect(mockDeps.backpressured).To(BeFalse())
			Expect(mockDeps.PendingMessageCount()).To(Equal(0))
		})
	})

	Describe("Context cancellation during Phase 4 delivery", func() {
		It("should return ctx.Err() when context cancelled with remaining messages", func() {
			ctx, cancel := context.WithCancel(context.Background())

			smallChan := make(chan *transport.UMHMessage, 1)
			mockDeps.inboundBi = smallChan
			mockDeps.chanCapacity = 1000
			mockDeps.chanLength = 0

			mockTrans.pullFunc = func(_ context.Context, _ string) ([]*transport.UMHMessage, error) {
				cancel()

				return []*transport.UMHMessage{
					{Content: "msg1"},
					{Content: "msg2"},
					{Content: "msg3"},
				}, nil
			}

			err := act.Execute(ctx, mockDeps)
			Expect(err).To(Equal(context.Canceled))
		})
	})

	Describe("Mid-batch channel full", func() {
		It("should store remaining as pending during phase 4 delivery and return nil", func() {
			smallChan := make(chan *transport.UMHMessage, 2)
			mockDeps.inboundBi = smallChan
			mockDeps.chanCapacity = 1000
			mockDeps.chanLength = 0

			mockTrans.pullMessages = []*transport.UMHMessage{
				{Content: "msg1"},
				{Content: "msg2"},
				{Content: "msg3"},
				{Content: "msg4"},
			}

			err := act.Execute(context.Background(), mockDeps)
			Expect(err).NotTo(HaveOccurred())

			Expect(smallChan).To(HaveLen(2))
			Expect(mockDeps.PendingMessageCount()).To(BeNumerically(">=", 1))

			Expect(mockDeps.recordSuccessCalls).To(Equal(0))
		})
	})
})
