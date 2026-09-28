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
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
)

// CommunicatorDependencies provides transport and channel access for communicator worker actions.
type CommunicatorDependencies struct {
	transport types.Transport

	*deps.BaseDependencies
	channelProvider ChannelProvider
	inboundChan     chan<- *types.UMHMessage
	outboundChan    <-chan *types.UMHMessage
}

// NewCommunicatorDependencies creates dependencies for the communicator worker,
// acquiring channels from the global channel provider.
// The worker must have a provider before it is built: set the global with
// SetChannelProvider, or store one under ChannelProviderKey in the worker's
// dependency map; otherwise this (or the map path) panics.
// bd is the shared BaseDependencies returned by WorkerBase.InitBase.
func NewCommunicatorDependencies(t types.Transport, bd *deps.BaseDependencies) *CommunicatorDependencies {
	return newCommunicatorDependenciesWithProvider(t, bd, GetChannelProvider())
}

// newCommunicatorDependenciesWithProvider creates dependencies that acquire
// channels from the given provider instead of the global one. The worker
// factory passes the provider stored under ChannelProviderKey here.
func newCommunicatorDependenciesWithProvider(t types.Transport, bd *deps.BaseDependencies, provider ChannelProvider) *CommunicatorDependencies {
	if provider == nil {
		panic("ChannelProvider must be set before creating communicator dependencies. " +
			"Call SetChannelProvider() in main.go before starting the FSMv2 supervisor.")
	}

	inbound, outbound := provider.GetChannels(bd.GetWorkerID())

	return &CommunicatorDependencies{
		BaseDependencies: bd,
		transport:        t,
		channelProvider:  provider,
		inboundChan:      inbound,
		outboundChan:     outbound,
	}
}

// SetTransport sets the transport instance.
func (d *CommunicatorDependencies) SetTransport(t types.Transport) {
	d.transport = t
}

// GetTransport returns the transport instance, or nil if not yet set.
func (d *CommunicatorDependencies) GetTransport() types.Transport {
	return d.transport
}

// RecordError increments consecutive errors and records when degraded mode started.
func (d *CommunicatorDependencies) RecordError() {
	d.RetryTracker().RecordError()
}

// RecordSuccess resets all error tracking state.
func (d *CommunicatorDependencies) RecordSuccess() {
	d.RetryTracker().RecordSuccess()
}

// GetConsecutiveErrors returns the current consecutive error count.
// Delegates to RetryTracker for single source of truth.
func (d *CommunicatorDependencies) GetConsecutiveErrors() int {
	return d.RetryTracker().ConsecutiveErrors()
}

// GetDegradedEnteredAt returns when degraded mode started, or zero if not degraded.
func (d *CommunicatorDependencies) GetDegradedEnteredAt() time.Time {
	degradedSince, _ := d.RetryTracker().DegradedSince()

	return degradedSince
}

// GetInboundChan returns channel to write received messages, or nil if no provider set.
func (d *CommunicatorDependencies) GetInboundChan() chan<- *types.UMHMessage {
	return d.inboundChan
}

// GetOutboundChan returns channel to read messages for pushing, or nil if no provider set.
func (d *CommunicatorDependencies) GetOutboundChan() <-chan *types.UMHMessage {
	return d.outboundChan
}

// GetInboundChanStats returns the capacity and current length of the inbound channel,
// reported by the provider the dependencies were created with.
func (d *CommunicatorDependencies) GetInboundChanStats() (capacity int, length int) {
	return d.channelProvider.GetInboundStats(d.GetWorkerID())
}
