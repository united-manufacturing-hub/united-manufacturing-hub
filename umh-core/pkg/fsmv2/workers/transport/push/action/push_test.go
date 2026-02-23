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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
	httpTransport "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport/http"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push/action"
)

type mockTransport struct {
	pushErr       error
	pushCallCount int
	pushedMsgs    []*transport.UMHMessage
	pushFunc      func(ctx context.Context, jwtToken string, messages []*transport.UMHMessage) error
}

func (m *mockTransport) Authenticate(_ context.Context, _ transport.AuthRequest) (transport.AuthResponse, error) {
	return transport.AuthResponse{}, nil
}

func (m *mockTransport) Pull(_ context.Context, _ string) ([]*transport.UMHMessage, error) {
	return nil, nil
}

func (m *mockTransport) Push(ctx context.Context, jwtToken string, messages []*transport.UMHMessage) error {
	m.pushCallCount++
	m.pushedMsgs = messages

	if m.pushFunc != nil {
		return m.pushFunc(ctx, jwtToken, messages)
	}

	return m.pushErr
}

func (m *mockTransport) Close() {}

func (m *mockTransport) Reset() {}

type mockPushDeps struct {
	outboundChan    <-chan *transport.UMHMessage
	transport       transport.Transport
	jwtToken        string
	metricsRecorder *deps.MetricsRecorder
	logger          deps.FSMLogger

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
}

type typedErrorCall struct {
	errType    httpTransport.ErrorType
	retryAfter time.Duration
}

func newMockPushDeps() *mockPushDeps {
	return &mockPushDeps{
		metricsRecorder: deps.NewMetricsRecorder(),
		logger:          deps.NewNopFSMLogger(),
		tokenValid:      true,
	}
}

func (m *mockPushDeps) GetLogger() deps.FSMLogger {
	return m.logger
}

func (m *mockPushDeps) ActionLogger(_ string) deps.FSMLogger {
	return m.logger
}

func (m *mockPushDeps) GetStateReader() deps.StateReader {
	return nil
}

func (m *mockPushDeps) GetHierarchyPath() string {
	return ""
}

func (m *mockPushDeps) GetOutboundChan() <-chan *transport.UMHMessage {
	return m.outboundChan
}

func (m *mockPushDeps) GetTransport() transport.Transport {
	return m.transport
}

func (m *mockPushDeps) GetJWTToken() string {
	return m.jwtToken
}

func (m *mockPushDeps) RecordTypedError(errType httpTransport.ErrorType, retryAfter time.Duration) {
	m.recordTypedErrorCalls = append(m.recordTypedErrorCalls, typedErrorCall{
		errType:    errType,
		retryAfter: retryAfter,
	})
}

func (m *mockPushDeps) RecordSuccess() {
	m.recordSuccessCalls++
}

func (m *mockPushDeps) RecordError() {
	m.recordErrorCalls++
}

func (m *mockPushDeps) GetConsecutiveErrors() int {
	return m.consecutiveErrors
}

func (m *mockPushDeps) GetLastErrorType() httpTransport.ErrorType {
	return m.lastErrorType
}

func (m *mockPushDeps) MetricsRecorder() *deps.MetricsRecorder {
	return m.metricsRecorder
}

func (m *mockPushDeps) StorePendingMessages(msgs []*transport.UMHMessage) {
	m.pendingMessages = append(m.pendingMessages, msgs...)
}

func (m *mockPushDeps) DrainPendingMessages() []*transport.UMHMessage {
	msgs := m.pendingMessages
	m.pendingMessages = nil

	return msgs
}

func (m *mockPushDeps) PendingMessageCount() int {
	return len(m.pendingMessages)
}

func (m *mockPushDeps) IsTokenValid() bool {
	return m.tokenValid
}

func (m *mockPushDeps) GetResetGeneration() uint64 {
	return m.resetGeneration
}

func (m *mockPushDeps) CheckAndClearOnReset() bool {
	if m.resetCleared {
		m.pendingMessages = nil
		m.resetCleared = false

		return true
	}

	return false
}

func (m *mockPushDeps) GetLastRetryAfter() time.Duration {
	return m.lastRetryAfter
}

func (m *mockPushDeps) GetDegradedEnteredAt() time.Time {
	return m.degradedEnteredAt
}

func (m *mockPushDeps) GetLastErrorAt() time.Time {
	return m.lastErrorAt
}

var _ = Describe("PushAction", func() {
	var (
		act           *action.PushAction
		mockDeps      *mockPushDeps
		mockTrans     *mockTransport
		outboundBi    chan *transport.UMHMessage
	)

	BeforeEach(func() {
		act = &action.PushAction{}
		mockTrans = &mockTransport{}
		outboundBi = make(chan *transport.UMHMessage, 100)
		mockDeps = newMockPushDeps()
		mockDeps.outboundChan = outboundBi
		mockDeps.transport = mockTrans
		mockDeps.jwtToken = "test-jwt"
	})

	Describe("Successful push", func() {
		It("should drain messages, call transport.Push, and record metrics", func() {
			outboundBi <- &transport.UMHMessage{InstanceUUID: "uuid-1", Content: "msg1", Email: "a@b.com"}
			outboundBi <- &transport.UMHMessage{InstanceUUID: "uuid-2", Content: "msg2", Email: "c@d.com"}

			err := act.Execute(context.Background(), mockDeps)
			Expect(err).NotTo(HaveOccurred())

			Expect(mockTrans.pushCallCount).To(Equal(1))
			Expect(mockTrans.pushedMsgs).To(HaveLen(2))

			Expect(mockDeps.recordSuccessCalls).To(Equal(1))

			drained := mockDeps.metricsRecorder.Drain()
			Expect(drained.Counters[string(deps.CounterPushOps)]).To(Equal(int64(1)))
			Expect(drained.Counters[string(deps.CounterPushSuccess)]).To(Equal(int64(1)))
			Expect(drained.Counters[string(deps.CounterMessagesPushed)]).To(Equal(int64(2)))
			Expect(drained.Counters[string(deps.CounterBytesPushed)]).To(BeNumerically(">", 0))
			Expect(drained.Gauges[string(deps.GaugeLastPushLatencyMs)]).To(BeNumerically(">=", 0))
		})
	})

	Describe("Failed push with TransportError", func() {
		It("should record typed error and increment failure counter", func() {
			outboundBi <- &transport.UMHMessage{Content: "msg1"}
			mockTrans.pushErr = &httpTransport.TransportError{
				Type:       httpTransport.ErrorTypeServerError,
				Message:    "HTTP 500: server_error",
				RetryAfter: 30 * time.Second,
			}

			err := act.Execute(context.Background(), mockDeps)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("push failed"))

			Expect(mockDeps.recordTypedErrorCalls).To(HaveLen(1))
			Expect(mockDeps.recordTypedErrorCalls[0].errType).To(Equal(httpTransport.ErrorTypeServerError))
			Expect(mockDeps.recordTypedErrorCalls[0].retryAfter).To(Equal(30 * time.Second))

			drained := mockDeps.metricsRecorder.Drain()
			Expect(drained.Counters[string(deps.CounterPushOps)]).To(Equal(int64(1)))
			Expect(drained.Counters[string(deps.CounterPushFailures)]).To(Equal(int64(1)))
			Expect(drained.Counters[string(deps.CounterServerErrorsTotal)]).To(Equal(int64(1)))
		})
	})

	Describe("Failed push with non-TransportError", func() {
		It("should default to ErrorTypeNetwork", func() {
			outboundBi <- &transport.UMHMessage{Content: "msg1"}
			mockTrans.pushErr = errors.New("connection refused")

			err := act.Execute(context.Background(), mockDeps)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("push failed"))

			Expect(mockDeps.recordTypedErrorCalls).To(HaveLen(1))
			Expect(mockDeps.recordTypedErrorCalls[0].errType).To(Equal(httpTransport.ErrorTypeNetwork))
			Expect(mockDeps.recordTypedErrorCalls[0].retryAfter).To(Equal(time.Duration(0)))

			drained := mockDeps.metricsRecorder.Drain()
			Expect(drained.Counters[string(deps.CounterPushFailures)]).To(Equal(int64(1)))
			Expect(drained.Counters[string(deps.CounterNetworkErrorsTotal)]).To(Equal(int64(1)))
		})
	})

	Describe("Empty outbound channel", func() {
		It("should be a no-op with no metrics and no transport call", func() {
			err := act.Execute(context.Background(), mockDeps)
			Expect(err).NotTo(HaveOccurred())

			Expect(mockTrans.pushCallCount).To(Equal(0))
			Expect(mockDeps.recordSuccessCalls).To(Equal(0))

			drained := mockDeps.metricsRecorder.Drain()
			Expect(drained.Counters).To(BeEmpty())
			Expect(drained.Gauges).To(BeEmpty())
		})
	})

	Describe("Context cancellation", func() {
		It("should return ctx.Err()", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			err := act.Execute(ctx, mockDeps)
			Expect(err).To(Equal(context.Canceled))
			Expect(mockTrans.pushCallCount).To(Equal(0))
		})
	})

	Describe("Nil transport", func() {
		It("should return error without draining channel", func() {
			outboundBi <- &transport.UMHMessage{Content: "msg1"}
			mockDeps.transport = nil

			err := act.Execute(context.Background(), mockDeps)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("transport is nil"))

			Expect(outboundBi).To(HaveLen(1))
		})
	})

	Describe("Nil outbound channel", func() {
		It("should return error when outbound channel is nil", func() {
			mockDeps.outboundChan = nil

			err := act.Execute(context.Background(), mockDeps)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("outbound channel is nil"))
			Expect(mockTrans.pushCallCount).To(Equal(0))
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
		It("should skip push when token is invalid (without draining)", func() {
			outboundBi <- &transport.UMHMessage{Content: "msg1"}
			mockDeps.tokenValid = false

			err := act.Execute(context.Background(), mockDeps)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("token not valid"))

			Expect(mockTrans.pushCallCount).To(Equal(0))
			Expect(outboundBi).To(HaveLen(1))
		})
	})

	Describe("Pending message retry", func() {
		It("should store messages in pending on push failure", func() {
			outboundBi <- &transport.UMHMessage{Content: "msg1"}
			outboundBi <- &transport.UMHMessage{Content: "msg2"}
			mockTrans.pushErr = errors.New("network error")

			err := act.Execute(context.Background(), mockDeps)
			Expect(err).To(HaveOccurred())

			Expect(mockDeps.PendingMessageCount()).To(Equal(2))
		})

		It("should not drain channel when pending messages exist", func() {
			mockDeps.pendingMessages = []*transport.UMHMessage{
				{Content: "pending1"},
			}
			outboundBi <- &transport.UMHMessage{Content: "new-msg"}

			err := act.Execute(context.Background(), mockDeps)
			Expect(err).NotTo(HaveOccurred())

			Expect(mockTrans.pushCallCount).To(Equal(1))
			Expect(mockTrans.pushedMsgs).To(HaveLen(1))
			Expect(mockTrans.pushedMsgs[0].Content).To(Equal("pending1"))

			Expect(outboundBi).To(HaveLen(1))
		})

		It("should retry pending one-by-one and drop on non-infrastructure error", func() {
			mockDeps.pendingMessages = []*transport.UMHMessage{
				{Content: "good-msg"},
				{Content: "poison-msg"},
				{Content: "after-poison"},
			}

			callCount := 0
			mockTrans.pushErr = nil
			mockTrans.pushFunc = func(_ context.Context, _ string, msgs []*transport.UMHMessage) error {
				callCount++
				if callCount == 2 {
					return &httpTransport.TransportError{
						Type:    httpTransport.ErrorTypeInstanceDeleted,
						Message: "instance deleted",
					}
				}

				return nil
			}

			err := act.Execute(context.Background(), mockDeps)
			Expect(err).NotTo(HaveOccurred())

			Expect(callCount).To(Equal(3))
			Expect(mockDeps.PendingMessageCount()).To(Equal(0))

			drained := mockDeps.metricsRecorder.Drain()
			Expect(drained.Counters[string(deps.CounterMessagesDropped)]).To(Equal(int64(1)))
		})

		It("should stop pending retry on infrastructure error and keep remaining", func() {
			mockDeps.pendingMessages = []*transport.UMHMessage{
				{Content: "msg1"},
				{Content: "msg2"},
				{Content: "msg3"},
			}

			callCount := 0
			mockTrans.pushFunc = func(_ context.Context, _ string, msgs []*transport.UMHMessage) error {
				callCount++
				if callCount == 2 {
					return &httpTransport.TransportError{
						Type:    httpTransport.ErrorTypeNetwork,
						Message: "connection refused",
					}
				}

				return nil
			}

			err := act.Execute(context.Background(), mockDeps)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("recoverable by parent"))

			Expect(mockDeps.PendingMessageCount()).To(Equal(2))
		})
	})

	Describe("Reset generation", func() {
		It("should clear pending messages when resetGeneration changes", func() {
			mockDeps.pendingMessages = []*transport.UMHMessage{
				{Content: "stale1"},
				{Content: "stale2"},
			}
			mockDeps.resetCleared = true

			err := act.Execute(context.Background(), mockDeps)
			Expect(err).NotTo(HaveOccurred())

			Expect(mockTrans.pushCallCount).To(Equal(0))
		})
	})
})
