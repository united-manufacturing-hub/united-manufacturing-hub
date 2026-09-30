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
	. "github.com/onsi/ginkgo/v2"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

// Test suite is registered in transport_suite_test.go to avoid duplicate RunSpecs

var _ = Describe("Channel Provider", func() {
	Describe("ChannelProvider interface", func() {
		It("should be implemented by mock provider", func() {
			var _ transport.ChannelProvider = (*MockChannelProvider)(nil)
		})
	})
})

// MockChannelProvider implements transport.ChannelProvider for testing.
type MockChannelProvider struct {
	inbound  chan<- *types.UMHMessage
	outbound <-chan *types.UMHMessage
}

// NewMockChannelProvider creates a mock channel provider with buffered channels.
func NewMockChannelProvider() *MockChannelProvider {
	inboundBi := make(chan *types.UMHMessage, 100)
	outboundBi := make(chan *types.UMHMessage, 100)

	return &MockChannelProvider{
		inbound:  inboundBi,
		outbound: outboundBi,
	}
}

func (m *MockChannelProvider) GetChannels(_ string) (
	inbound chan<- *types.UMHMessage,
	outbound <-chan *types.UMHMessage,
) {
	return m.inbound, m.outbound
}

func (m *MockChannelProvider) GetInboundStats(_ string) (capacity int, length int) {
	return 100, 0
}
