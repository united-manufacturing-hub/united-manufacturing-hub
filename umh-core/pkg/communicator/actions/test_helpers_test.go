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

package actions_test

import (
	"sync"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

// ConsumeOutboundMessagesThreadSafe processes messages from the outbound channel in a thread-safe manner
// This method is used for testing purposes to consume messages that would normally be sent to the user
func ConsumeOutboundMessagesThreadSafe(outboundChannel chan *models.UMHMessage, messages *ThreadSafeMessages, logMessages bool) {
	for msg := range outboundChannel {
		messages.Append(msg)
		decodedMessage, err := encoding.DecodeMessageFromUMHInstanceToUser(msg.Content)
		if err != nil {
			zap.S().Error("error decoding message", zap.Error(err))
			continue
		}
		if logMessages {
			zap.S().Info("received message", decodedMessage.Payload)
		}

	}
}

// ThreadSafeMessages provides thread-safe access to a slice of UMH messages
type ThreadSafeMessages struct {
	mu       sync.Mutex
	messages []*models.UMHMessage
}

// NewThreadSafeMessages creates a new thread-safe message container
func NewThreadSafeMessages() *ThreadSafeMessages {
	return &ThreadSafeMessages{
		messages: make([]*models.UMHMessage, 0),
	}
}

// Append adds a message to the slice in a thread-safe manner
func (tsm *ThreadSafeMessages) Append(msg *models.UMHMessage) {
	tsm.mu.Lock()
	defer tsm.mu.Unlock()
	tsm.messages = append(tsm.messages, msg)
}

// Get returns a copy of the message at the given index in a thread-safe manner
func (tsm *ThreadSafeMessages) Get(index int) *models.UMHMessage {
	tsm.mu.Lock()
	defer tsm.mu.Unlock()
	if index < 0 || index >= len(tsm.messages) {
		return nil
	}
	return tsm.messages[index]
}

// Len returns the current number of messages in a thread-safe manner
func (tsm *ThreadSafeMessages) Len() int {
	tsm.mu.Lock()
	defer tsm.mu.Unlock()
	return len(tsm.messages)
}

// GetAll returns a copy of all messages in a thread-safe manner
func (tsm *ThreadSafeMessages) GetAll() []*models.UMHMessage {
	tsm.mu.Lock()
	defer tsm.mu.Unlock()
	result := make([]*models.UMHMessage, len(tsm.messages))
	copy(result, tsm.messages)
	return result
}

// ThreadSafeChannelCollector provides thread-safe channel message collection for testing
type ThreadSafeChannelCollector struct {
	mu       sync.Mutex
	messages []*models.UMHMessage
	ch       chan *models.UMHMessage
	done     chan struct{}
	started  bool
}

// NewThreadSafeChannelCollector creates a new thread-safe channel collector
func NewThreadSafeChannelCollector(channelSize int) *ThreadSafeChannelCollector {
	return &ThreadSafeChannelCollector{
		ch:       make(chan *models.UMHMessage, channelSize),
		done:     make(chan struct{}),
		messages: make([]*models.UMHMessage, 0),
	}
}

// GetChannel returns the channel for sending messages
func (tsc *ThreadSafeChannelCollector) GetChannel() chan *models.UMHMessage {
	return tsc.ch
}

// StartCollecting begins collecting messages from the channel in a separate goroutine
func (tsc *ThreadSafeChannelCollector) StartCollecting() {
	tsc.mu.Lock()
	defer tsc.mu.Unlock()

	if tsc.started {
		return
	}
	tsc.started = true

	go func() {
		for {
			select {
			case msg, ok := <-tsc.ch:
				if !ok {
					// Channel closed, exit
					return
				}
				tsc.mu.Lock()
				tsc.messages = append(tsc.messages, msg)
				tsc.mu.Unlock()
			case <-tsc.done:
				return
			}
		}
	}()
}

// GetMessages returns a copy of all collected messages
func (tsc *ThreadSafeChannelCollector) GetMessages() []*models.UMHMessage {
	tsc.mu.Lock()
	defer tsc.mu.Unlock()
	result := make([]*models.UMHMessage, len(tsc.messages))
	copy(result, tsc.messages)
	return result
}

// GetMessageCount returns the current number of collected messages
func (tsc *ThreadSafeChannelCollector) GetMessageCount() int {
	tsc.mu.Lock()
	defer tsc.mu.Unlock()
	return len(tsc.messages)
}

// Stop stops the collector and closes the channel safely
func (tsc *ThreadSafeChannelCollector) Stop() {
	tsc.mu.Lock()
	defer tsc.mu.Unlock()

	if !tsc.started {
		return
	}

	close(tsc.done)
	close(tsc.ch)
	tsc.started = false
}
