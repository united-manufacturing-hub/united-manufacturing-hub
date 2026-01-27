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

package communicator

import (
	"sync"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
)

// ChannelProvider supplies inbound/outbound channels for communicator workers.
type ChannelProvider interface {
	GetChannels(workerID string) (inbound chan<- *transport.UMHMessage, outbound <-chan *transport.UMHMessage)
	// GetInboundStats returns the capacity and current length of the inbound channel.
	// Used by SyncAction to detect backpressure before pulling messages.
	GetInboundStats(workerID string) (capacity int, length int)
}

var (
	globalChannelProvider ChannelProvider
	channelProviderMu     sync.RWMutex
)

// SetChannelProvider sets the global channel provider. Must be called before starting ApplicationSupervisor.
func SetChannelProvider(p ChannelProvider) {
	channelProviderMu.Lock()
	defer channelProviderMu.Unlock()

	globalChannelProvider = p
}

// GetChannelProvider returns the current channel provider, or nil if not set.
func GetChannelProvider() ChannelProvider {
	channelProviderMu.RLock()
	defer channelProviderMu.RUnlock()

	return globalChannelProvider
}

// ClearChannelProvider removes the channel provider (for test cleanup).
func ClearChannelProvider() {
	channelProviderMu.Lock()
	defer channelProviderMu.Unlock()

	globalChannelProvider = nil
}
