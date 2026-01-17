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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/action"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
)

// Note: TestSyncAction is not needed here - action_suite_test.go has the test runner

// MockTransport implements transport.Transport for testing.
type MockTransport struct {
	pullResults []pullResult
	pullIndex   int
	pushErr     error
}

type pullResult struct {
	messages []*transport.UMHMessage
	err      error
}

func NewMockTransport() *MockTransport {
	return &MockTransport{}
}

// Authenticate satisfies transport.Transport interface.
func (m *MockTransport) Authenticate(ctx context.Context, req transport.AuthRequest) (transport.AuthResponse, error) {
	return transport.AuthResponse{Token: "mock-token", ExpiresAt: time.Now().Add(time.Hour).Unix()}, nil
}

func (m *MockTransport) Pull(ctx context.Context, jwtToken string) ([]*transport.UMHMessage, error) {
	if m.pullIndex < len(m.pullResults) {
		result := m.pullResults[m.pullIndex]
		m.pullIndex++

		return result.messages, result.err
	}

	return nil, nil
}

func (m *MockTransport) Push(ctx context.Context, jwtToken string, messages []*transport.UMHMessage) error {
	return m.pushErr
}

// Close satisfies transport.Transport interface (no return value).
func (m *MockTransport) Close() {
}

func (m *MockTransport) PullReturns(messages []*transport.UMHMessage, err error) {
	m.pullResults = []pullResult{{messages: messages, err: err}}
	m.pullIndex = 0
}

func (m *MockTransport) PullReturnsOnCall(callIndex int, messages []*transport.UMHMessage, err error) {
	for len(m.pullResults) <= callIndex {
		m.pullResults = append(m.pullResults, pullResult{})
	}

	m.pullResults[callIndex] = pullResult{messages: messages, err: err}
}

var testLogger = zap.NewNop().Sugar()

var _ = Describe("SyncAction Error Handling", func() {
	var (
		ctx           context.Context
		cancel        context.CancelFunc
		mockTransport *MockTransport
		deps          *communicator.CommunicatorDependencies
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		mockTransport = NewMockTransport()
		identity := fsmv2.Identity{ID: "test-worker", Name: "Test Worker"}
		deps = communicator.NewCommunicatorDependencies(mockTransport, testLogger, nil, identity)
	})

	AfterEach(func() {
		cancel()
	})

	Describe("Bug #1: Watchdog Oscillation Fix", func() {
		It("calls RecordError on Pull failure", func() {
			// Arrange: Transport fails
			mockTransport.PullReturns(nil, errors.New("network error"))
			syncAction := action.NewSyncAction("test-jwt-token")

			// Initial state
			Expect(deps.GetConsecutiveErrors()).To(Equal(0))

			// Act: Execute fails
			err := syncAction.Execute(ctx, deps)
			Expect(err).To(HaveOccurred())

			// Assert: Error counter incremented via RecordError()
			Expect(deps.GetConsecutiveErrors()).To(Equal(1))
		})

		It("accumulates ConsecutiveErrors on repeated failures", func() {
			// Arrange: Transport always fails
			mockTransport.PullReturnsOnCall(0, nil, errors.New("error 1"))
			mockTransport.PullReturnsOnCall(1, nil, errors.New("error 2"))
			mockTransport.PullReturnsOnCall(2, nil, errors.New("error 3"))
			syncAction := action.NewSyncAction("test-jwt-token")

			// Act: Execute 3 times
			for range 3 {
				_ = syncAction.Execute(ctx, deps)
			}

			// Assert: Counter accumulated, NOT oscillating (0→1→0→1)
			Expect(deps.GetConsecutiveErrors()).To(Equal(3))
		})

		It("calls RecordSuccess only on success (resets counter)", func() {
			// Arrange: Fail twice, then succeed
			mockTransport.PullReturnsOnCall(0, nil, errors.New("error 1"))
			mockTransport.PullReturnsOnCall(1, nil, errors.New("error 2"))
			mockTransport.PullReturnsOnCall(2, []*transport.UMHMessage{}, nil)
			syncAction := action.NewSyncAction("test-jwt-token")

			// Act: Execute with failures then success
			_ = syncAction.Execute(ctx, deps)
			Expect(deps.GetConsecutiveErrors()).To(Equal(1))

			_ = syncAction.Execute(ctx, deps)
			Expect(deps.GetConsecutiveErrors()).To(Equal(2))

			err := syncAction.Execute(ctx, deps)
			Expect(err).NotTo(HaveOccurred())

			// Assert: Counter reset to 0 only after confirmed success via RecordSuccess()
			Expect(deps.GetConsecutiveErrors()).To(Equal(0))
		})
	})
})
