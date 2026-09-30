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

package transport

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

// ChannelProvider supplies inbound/outbound channels for transport child workers (PushWorker, PullWorker).
type ChannelProvider interface {
	GetChannels(workerID string) (inbound chan<- *types.UMHMessage, outbound <-chan *types.UMHMessage)
	// GetInboundStats returns the capacity and current length of the inbound channel.
	// Used by PullWorker to detect backpressure before pulling messages.
	GetInboundStats(workerID string) (capacity int, length int)
}

// channelProviderKeyName is ChannelProviderKey's name. It is a constant so
// NewTransportWorker's error can print it, because DependencyKey does not
// expose its name.
const channelProviderKeyName = "transport.channel_provider"

// ChannelProviderKey names the dependency-map entry that holds the transport
// worker's ChannelProvider.
var ChannelProviderKey = config.NewDependencyKey[ChannelProvider](channelProviderKeyName)
