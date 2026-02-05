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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push/action"
)

type mockTransport struct {
	pushErr       error
	pushCallCount int
	pushedMsgs    []*transport.UMHMessage
}

func (m *mockTransport) Authenticate(_ context.Context, _ transport.AuthRequest) (transport.AuthResponse, error) {
	return transport.AuthResponse{}, nil
}

func (m *mockTransport) Pull(_ context.Context, _ string) ([]*transport.UMHMessage, error) {
	return nil, nil
}

func (m *mockTransport) Push(_ context.Context, _ string, messages []*transport.UMHMessage) error {
	m.pushCallCount++
	m.pushedMsgs = messages

	return m.pushErr
}

func (m *mockTransport) Close() {}

func (m *mockTransport) Reset() {}

type mockPushDeps struct {
	outboundChan    <-chan *transport.UMHMessage
	transport       transport.Transport
	jwtToken        string
	metricsRecorder *deps.MetricsRecorder
	logger          *zap.SugaredLogger

	recordTypedErrorCalls []typedErrorCall
	recordSuccessCalls    int
	recordErrorCalls      int
	consecutiveErrors     int
	lastErrorType         httpTransport.ErrorType
}

type typedErrorCall struct {
	errType    httpTransport.ErrorType
	retryAfter time.Duration
}

func newMockPushDeps() *mockPushDeps {
	return &mockPushDeps{
		metricsRecorder: deps.NewMetricsRecorder(),
		logger:          zap.NewNop().Sugar(),
	}
}

func (m *mockPushDeps) GetLogger() *zap.SugaredLogger {
	return m.logger
}

func (m *mockPushDeps) ActionLogger(_ string) *zap.SugaredLogger {
	return m.logger
}

func (m *mockPushDeps) GetStateReader() deps.StateReader {
	return nil
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
		It("should return error", func() {
			outboundBi <- &transport.UMHMessage{Content: "msg1"}
			mockDeps.transport = nil

			err := act.Execute(context.Background(), mockDeps)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("transport is nil"))
		})
	})

	Describe("Nil outbound channel", func() {
		It("should return nil (no-op)", func() {
			mockDeps.outboundChan = nil

			err := act.Execute(context.Background(), mockDeps)
			Expect(err).NotTo(HaveOccurred())
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
})
