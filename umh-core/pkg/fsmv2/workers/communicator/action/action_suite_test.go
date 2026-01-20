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
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
)

func TestAction(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Action Suite")
}

// mockActionChannelProvider implements communicator.ChannelProvider for action tests.
// Phase 1 Architecture: ChannelProvider singleton MUST be set before creating dependencies.
type mockActionChannelProvider struct {
	inbound  chan<- *transport.UMHMessage
	outbound <-chan *transport.UMHMessage
}

func (m *mockActionChannelProvider) GetChannels(_ string) (
	chan<- *transport.UMHMessage,
	<-chan *transport.UMHMessage,
) {
	return m.inbound, m.outbound
}

// setupChannelProviderSingleton sets up the global singleton for tests.
// Must be called in BeforeEach before creating CommunicatorDependencies.
func setupChannelProviderSingleton() {
	inboundBi := make(chan *transport.UMHMessage, 100)
	outboundBi := make(chan *transport.UMHMessage, 100)
	provider := &mockActionChannelProvider{
		inbound:  inboundBi,
		outbound: outboundBi,
	}
	communicator.SetChannelProvider(provider)
}

// clearChannelProviderSingleton clears the global singleton.
// Must be called in AfterEach.
func clearChannelProviderSingleton() {
	communicator.ClearChannelProvider()
}

// Suite-level setup: ensure singleton is cleared before and after each test
var _ = BeforeEach(func() {
	// Phase 1: Set up ChannelProvider singleton for action tests
	setupChannelProviderSingleton()
})

var _ = AfterEach(func() {
	// Phase 1: Clear ChannelProvider singleton after each test
	clearChannelProviderSingleton()
})
