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
	"sync"

	transportpkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
)

// MockTransport is a mock implementation of HTTPTransport for testing.
type MockTransport struct {
	mu sync.Mutex

	authenticateCalls int
	pullCalls         int
	pushCalls         int
	resetCalls        int

	authenticateErr error
	pullErr         error
	pushErr         error

	pullMessages   []*transportpkg.UMHMessage
	pushedMessages []*transportpkg.UMHMessage
	token          string
}

func NewMockTransport() *MockTransport {
	return &MockTransport{
		pullMessages:   []*transportpkg.UMHMessage{},
		pushedMessages: []*transportpkg.UMHMessage{},
	}
}

func (m *MockTransport) Authenticate(ctx context.Context, req transportpkg.AuthRequest) (transportpkg.AuthResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.authenticateCalls++

	if m.authenticateErr != nil {
		return transportpkg.AuthResponse{}, m.authenticateErr
	}

	m.token = "mock-jwt-token"

	return transportpkg.AuthResponse{Token: m.token}, nil
}

func (m *MockTransport) Pull(ctx context.Context, jwtToken string) ([]*transportpkg.UMHMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.pullCalls++

	if m.pullErr != nil {
		return nil, m.pullErr
	}

	return m.pullMessages, nil
}

func (m *MockTransport) Push(ctx context.Context, jwtToken string, messages []*transportpkg.UMHMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.pushCalls++

	if m.pushErr != nil {
		return m.pushErr
	}

	m.pushedMessages = append(m.pushedMessages, messages...)

	return nil
}

func (m *MockTransport) Close() {
	// No-op for mock
}

func (m *MockTransport) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.resetCalls++
}

func (m *MockTransport) SetAuthenticateError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.authenticateErr = err
}

func (m *MockTransport) SetPullError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.pullErr = err
}

func (m *MockTransport) SetPushError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.pushErr = err
}

func (m *MockTransport) SetPullMessages(messages []*transportpkg.UMHMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.pullMessages = messages
}

func (m *MockTransport) GetPushedMessages() []*transportpkg.UMHMessage {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.pushedMessages
}

func (m *MockTransport) AuthenticateCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.authenticateCalls
}

func (m *MockTransport) PullCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.pullCalls
}

func (m *MockTransport) PushCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.pushCalls
}

func (m *MockTransport) ResetCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.resetCalls
}

// MockChannelProvider implements communicator.ChannelProvider for testing.
type MockChannelProvider struct {
	inbound  chan<- *transportpkg.UMHMessage
	outbound <-chan *transportpkg.UMHMessage
}

// NewMockChannelProvider creates a mock channel provider with buffered channels.
func NewMockChannelProvider() *MockChannelProvider {
	// Create bidirectional channels with buffer size 100
	inboundBi := make(chan *transportpkg.UMHMessage, 100)
	outboundBi := make(chan *transportpkg.UMHMessage, 100)

	return &MockChannelProvider{
		inbound:  inboundBi,
		outbound: outboundBi,
	}
}

func (m *MockChannelProvider) GetChannels(_ string) (
	inbound chan<- *transportpkg.UMHMessage,
	outbound <-chan *transportpkg.UMHMessage,
) {
	return m.inbound, m.outbound
}

func (m *MockChannelProvider) GetInboundStats(_ string) (capacity int, length int) {
	// Return reasonable defaults for worker tests (not testing backpressure here)
	return 100, 0
}
