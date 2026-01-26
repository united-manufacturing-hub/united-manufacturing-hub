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

// Package fsmv2_adapter provides adapters for bridging FSMv2 communicator
// worker with existing UMH subsystems.
//
// # Purpose
//
// This adapter exists for the TRANSITIONAL PERIOD while FSMv2 Communicator
// is being integrated with the existing UMH codebase. It bridges the gap
// between FSMv2's transport.UMHMessage and the legacy models.UMHMessage
// used by the Router and other subsystems.
//
// # When to Use
//
//   - When integrating FSMv2 Communicator with existing UMH subsystems
//   - During migration from legacy Communicator to FSMv2 Communicator
//
// # When NOT to Use
//
//   - For new greenfield FSMv2 workers (use transport.UMHMessage directly)
//   - After full migration is complete (this package becomes obsolete)
//
// # Message Flow
//
//	┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
//	│  FSMv2 Worker   │────▶│  Legacy Bridge  │────▶│     Router      │
//	│  (transport.*)  │◀────│   (converts)    │◀────│  (models.*)     │
//	└─────────────────┘     └─────────────────┘     └─────────────────┘
//
// Flow:
//   - FSMv2 worker writes received messages to inbound channel
//   - Bridge converts transport.UMHMessage -> models.UMHMessage
//   - Writes to CommunicationState.InboundChannel for Router processing
//   - Router writes responses to CommunicationState.OutboundChannel
//   - Bridge converts models.UMHMessage -> transport.UMHMessage
//   - FSMv2 worker reads from outbound channel to push to HTTP
//
// # Deprecation Plan
//
// This adapter is intended to be removed once the full FSMv2 migration is
// complete and all subsystems use transport.UMHMessage directly.
package fsmv2_adapter

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

var (
	droppedMessagesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "umh",
			Subsystem: "fsmv2_adapter",
			Name:      "dropped_messages_total",
			Help:      "Total number of messages dropped due to full channels",
		},
		[]string{"direction"},
	)
)

// LegacyChannelBridge adapts CommunicationState channels for FSMv2 communicator.
// It converts between models.UMHMessage (legacy) and transport.UMHMessage (FSMv2).
type LegacyChannelBridge struct {
	// Intermediate channels with transport.UMHMessage type
	fsmInbound  chan *transport.UMHMessage // FSMv2 writes here
	fsmOutbound chan *transport.UMHMessage // FSMv2 reads from here

	// Legacy channels from CommunicationState
	legacyInbound  chan *models.UMHMessage // Router reads from here
	legacyOutbound chan *models.UMHMessage // Router writes here

	logger *zap.SugaredLogger

	// wg tracks the goroutines started by Start() for graceful shutdown
	wg sync.WaitGroup
}

// DefaultBufferSize is the default capacity for FSMv2 inbound/outbound channels.
const DefaultBufferSize = 100

// NewLegacyChannelBridge creates a bridge that adapts FSMv2 and legacy channels.
// The bridge uses buffered channels with configurable capacity for both
// inbound and outbound FSMv2 channels. If bufferSize is 0, DefaultBufferSize (100) is used.
func NewLegacyChannelBridge(
	legacyInbound chan *models.UMHMessage,
	legacyOutbound chan *models.UMHMessage,
	logger *zap.SugaredLogger,
	bufferSize int,
) *LegacyChannelBridge {
	if bufferSize <= 0 {
		bufferSize = DefaultBufferSize
	}

	return &LegacyChannelBridge{
		fsmInbound:     make(chan *transport.UMHMessage, bufferSize),
		fsmOutbound:    make(chan *transport.UMHMessage, bufferSize),
		legacyInbound:  legacyInbound,
		legacyOutbound: legacyOutbound,
		logger:         logger,
	}
}

// Start begins the conversion goroutines. Call this before starting FSMv2 supervisor.
// Use Wait() to block until all goroutines exit after context cancellation.
func (b *LegacyChannelBridge) Start(ctx context.Context) {
	// Goroutine: FSMv2 inbound -> Legacy inbound (for Router)
	b.wg.Add(1)

	go func() {
		defer b.wg.Done()

		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-b.fsmInbound:
				if !ok {
					b.logger.Infow("fsm_inbound_channel_closed")

					return
				}

				if msg == nil {
					continue
				}

				// Convert transport.UMHMessage -> models.UMHMessage
				var instanceUUID uuid.UUID
				if msg.InstanceUUID != "" {
					parsed, err := uuid.Parse(msg.InstanceUUID)
					if err != nil {
						b.logger.Warnw("instance_uuid_parse_failed",
							"instanceUUID", msg.InstanceUUID, "error", err)

						instanceUUID = uuid.Nil
					} else {
						instanceUUID = parsed
					}
				}

				// Parse TraceID if present
				var metadata *models.MessageMetadata

				if msg.TraceID != "" {
					traceID, err := uuid.Parse(msg.TraceID)
					if err != nil {
						b.logger.Warnw("trace_id_parse_failed",
							"traceID", msg.TraceID, "error", err)
					} else {
						metadata = &models.MessageMetadata{
							TraceID: traceID,
						}
					}
				}

				legacyMsg := &models.UMHMessage{
					Content:      msg.Content,
					InstanceUUID: instanceUUID,
					Email:        msg.Email,
					Metadata:     metadata,
				}

				// Non-blocking send to prevent deadlock
				select {
				case b.legacyInbound <- legacyMsg:
				case <-ctx.Done():
					return
				default:
					b.logger.Warnw("legacy_inbound_channel_full")
					droppedMessagesTotal.WithLabelValues("inbound").Inc()
				}
			}
		}
	}()

	// Goroutine: Legacy outbound -> FSMv2 outbound (for push)
	b.wg.Add(1)

	go func() {
		defer b.wg.Done()

		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-b.legacyOutbound:
				if !ok {
					b.logger.Infow("legacy_outbound_channel_closed")

					return
				}

				if msg == nil {
					continue
				}

				// Convert models.UMHMessage -> transport.UMHMessage
				var traceID string
				if msg.Metadata != nil && msg.Metadata.TraceID != uuid.Nil {
					traceID = msg.Metadata.TraceID.String()
				}

				fsmMsg := &transport.UMHMessage{
					Content:      msg.Content,
					InstanceUUID: msg.InstanceUUID.String(),
					Email:        msg.Email,
					TraceID:      traceID,
				}

				// Non-blocking send to prevent deadlock
				select {
				case b.fsmOutbound <- fsmMsg:
				case <-ctx.Done():
					return
				default:
					b.logger.Warnw("fsmv2_outbound_channel_full")
					droppedMessagesTotal.WithLabelValues("outbound").Inc()
				}
			}
		}
	}()
}

// Wait blocks until all goroutines started by Start() have exited.
// Call this after canceling the context passed to Start() for graceful shutdown.
func (b *LegacyChannelBridge) Wait() {
	b.wg.Wait()
}

// GetChannels returns channels for the FSMv2 communicator worker.
// Implements communicator.ChannelProvider interface.
func (b *LegacyChannelBridge) GetChannels(_ string) (
	inbound chan<- *transport.UMHMessage,
	outbound <-chan *transport.UMHMessage,
) {
	return b.fsmInbound, b.fsmOutbound
}

// GetOutboundWriteChannel returns the outbound channel for direct writing.
// This allows SubscriberHandler to bypass the legacy->FSMv2 conversion goroutine
// and write transport.UMHMessage directly to the FSMv2 outbound channel.
// Used for Priority 0: Remove Pusher from FSMv2 flow.
func (b *LegacyChannelBridge) GetOutboundWriteChannel() chan<- *transport.UMHMessage {
	return b.fsmOutbound
}
