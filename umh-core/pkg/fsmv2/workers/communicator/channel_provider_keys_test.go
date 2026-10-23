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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

// dualChannelProvider is a minimal channel provider. Both workers'
// ChannelProvider types are deps.ChannelProvider[*types.UMHMessage], so it can
// be stored under either worker's key.
type dualChannelProvider struct{}

func (p *dualChannelProvider) GetChannels(_ string) (inbound chan<- *types.UMHMessage, outbound <-chan *types.UMHMessage) {
	in := make(chan *types.UMHMessage)
	out := make(chan *types.UMHMessage)
	return in, out
}

func (p *dualChannelProvider) GetInboundStats(_ string) (capacity int, length int) {
	return 1, 0
}

var _ = Describe("channel provider keys", func() {
	It("checks that each worker's channel provider key stores under its own name", func() {
		Expect(transport.ChannelProviderKey.Name()).To(Equal("transport.channel_provider"))
		Expect(communicator.ChannelProviderKey.Name()).To(Equal("communicator.channel_provider"))
	})

	It("checks that a provider stored under one worker's key is absent under the other worker's key", func() {
		provider := &dualChannelProvider{}

		transportMap := map[string]any{}
		config.SetDependency(transportMap, transport.ChannelProviderKey, transport.ChannelProvider(provider))
		_, ok := config.LookupDependency(transportMap, communicator.ChannelProviderKey)
		Expect(ok).To(BeFalse())

		communicatorMap := map[string]any{}
		config.SetDependency(communicatorMap, communicator.ChannelProviderKey, communicator.ChannelProvider(provider))
		_, ok = config.LookupDependency(communicatorMap, transport.ChannelProviderKey)
		Expect(ok).To(BeFalse())
	})
})
