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

	transportpkg "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

func TestAction(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Transport Action Suite")
}

// mockActionChannelProvider implements transport.ChannelProvider for action tests.
type mockActionChannelProvider struct {
	inbound  chan<- *types.UMHMessage
	outbound <-chan *types.UMHMessage
}

func (m *mockActionChannelProvider) GetChannels(_ string) (
	chan<- *types.UMHMessage,
	<-chan *types.UMHMessage,
) {
	return m.inbound, m.outbound
}

func (m *mockActionChannelProvider) GetInboundStats(_ string) (capacity int, length int) {
	// Return reasonable defaults for action tests (not testing backpressure here)
	return 100, 0
}

// newActionChannelProvider builds a channel provider for action tests.
// Pass it to NewTransportDependencies.
func newActionChannelProvider() transportpkg.ChannelProvider {
	inboundBi := make(chan *types.UMHMessage, 100)
	outboundBi := make(chan *types.UMHMessage, 100)

	return &mockActionChannelProvider{
		inbound:  inboundBi,
		outbound: outboundBi,
	}
}
