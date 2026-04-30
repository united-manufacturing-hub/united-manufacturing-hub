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
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

// Test suite is registered in transport_suite_test.go to avoid duplicate RunSpecs

var _ = Describe("Channel Provider", func() {
	AfterEach(func() {
		transport.ClearChannelProvider()
	})

	Describe("Global channel provider", func() {
		Context("when no provider is set", func() {
			It("should return nil", func() {
				Expect(transport.GetChannelProvider()).To(BeNil())
			})
		})

		Context("when a provider is set", func() {
			var mockProvider *MockChannelProvider

			BeforeEach(func() {
				mockProvider = NewMockChannelProvider()
				transport.SetChannelProvider(mockProvider)
			})

			It("should return the set provider", func() {
				Expect(transport.GetChannelProvider()).To(Equal(mockProvider))
			})

			It("should be able to get channels from the provider", func() {
				provider := transport.GetChannelProvider()
				Expect(provider).NotTo(BeNil())

				inbound, outbound := provider.GetChannels("test-worker")
				Expect(inbound).NotTo(BeNil())
				Expect(outbound).NotTo(BeNil())
			})

			It("should be able to get inbound stats from the provider", func() {
				provider := transport.GetChannelProvider()
				Expect(provider).NotTo(BeNil())

				capacity, length := provider.GetInboundStats("test-worker")
				Expect(capacity).To(Equal(100))
				Expect(length).To(Equal(0))
			})
		})

		Context("when provider is cleared", func() {
			BeforeEach(func() {
				mockProvider := NewMockChannelProvider()
				transport.SetChannelProvider(mockProvider)
			})

			It("should return nil after clearing", func() {
				transport.ClearChannelProvider()
				Expect(transport.GetChannelProvider()).To(BeNil())
			})
		})

		Context("when provider is replaced", func() {
			It("should return the new provider", func() {
				provider1 := NewMockChannelProvider()
				provider2 := NewMockChannelProvider()

				transport.SetChannelProvider(provider1)
				Expect(transport.GetChannelProvider()).To(Equal(provider1))

				transport.SetChannelProvider(provider2)
				Expect(transport.GetChannelProvider()).To(Equal(provider2))
			})
		})
	})

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
