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

package examples_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/examples"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

var _ = Describe("TransportTestChannelProvider", func() {
	It("provides channels for test scenarios", func() {
		provider := examples.NewTransportTestChannelProvider(10)
		inbound, outbound := provider.GetChannels("test-worker")

		Expect(inbound).NotTo(BeNil())
		Expect(outbound).NotTo(BeNil())
	})

	It("queues and drains messages correctly", func() {
		provider := examples.NewTransportTestChannelProvider(10)

		// Queue a message via outbound
		testMsg := &types.UMHMessage{Content: "test-message"}
		provider.QueueOutbound(testMsg)

		// Get channels and read from outbound
		_, outbound := provider.GetChannels("test-worker")
		receivedMsg := <-outbound
		Expect(receivedMsg.Content).To(Equal("test-message"))
	})

	It("drains inbound messages", func() {
		provider := examples.NewTransportTestChannelProvider(10)
		inbound, _ := provider.GetChannels("test-worker")

		// Send messages to inbound
		inbound <- &types.UMHMessage{Content: "msg1"}
		inbound <- &types.UMHMessage{Content: "msg2"}

		// Drain
		messages := provider.DrainInbound()
		Expect(messages).To(HaveLen(2))
		Expect(messages[0].Content).To(Equal("msg1"))
		Expect(messages[1].Content).To(Equal("msg2"))
	})

	It("returns empty slice when no messages available", func() {
		provider := examples.NewTransportTestChannelProvider(10)
		messages := provider.DrainInbound()
		Expect(messages).To(BeEmpty())
	})

	It("handles channel closure gracefully", func() {
		provider := examples.NewTransportTestChannelProvider(10)
		inbound, _ := provider.GetChannels("test-worker")

		inbound <- &types.UMHMessage{Content: "msg1"}
		close(inbound)

		messages := provider.DrainInbound()
		Expect(messages).To(HaveLen(1))
		Expect(messages[0].Content).To(Equal("msg1"))
	})
})
