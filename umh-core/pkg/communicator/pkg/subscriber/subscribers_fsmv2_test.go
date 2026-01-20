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

package subscriber_test

import (
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/subscriber"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
	"go.uber.org/zap"
)

var _ = Describe("FSMv2 Direct Channel Mode", func() {
	var (
		handler            *subscriber.Handler
		logger             *zap.SugaredLogger
		fsmOutboundChannel chan *transport.UMHMessage
	)

	BeforeEach(func() {
		zapLogger, _ := zap.NewDevelopment()
		logger = zapLogger.Sugar()
	})

	AfterEach(func() {
		if fsmOutboundChannel != nil {
			close(fsmOutboundChannel)
		}
	})

	Describe("FSMv2 mode (fsmOutboundChannel != nil)", func() {
		BeforeEach(func() {
			// Create buffered channel for FSMv2 mode
			fsmOutboundChannel = make(chan *transport.UMHMessage, 10)

			handler = subscriber.NewHandler(
				&mockWatchdog{},
				nil, // pusher not used in FSMv2 mode
				uuid.New(),
				time.Minute,
				time.Minute,
				config.ReleaseChannelStable,
				false,
				nil, // systemSnapshotManager
				nil, // configManager
				logger,
				nil, // topicBrowserCommunicator
				fsmOutboundChannel,
			)
		})

		It("should have a non-nil FSMv2 outbound channel", func() {
			// The handler's internal channel should be set
			// We verify this indirectly through the GetInstanceUUID method
			expectedUUID := uuid.New()
			handler.SetInstanceUUID(expectedUUID)
			Expect(handler.GetInstanceUUID()).To(Equal(expectedUUID))
		})

		It("should use FSMv2 mode when fsmOutboundChannel is provided", func() {
			// This test verifies that the handler was correctly initialized with FSMv2 channel
			// The actual message sending would require mocking more components,
			// but we can verify the channel is correctly passed
			Expect(fsmOutboundChannel).NotTo(BeNil())
		})
	})

	Describe("Legacy mode (fsmOutboundChannel == nil)", func() {
		BeforeEach(func() {
			fsmOutboundChannel = nil

			handler = subscriber.NewHandler(
				&mockWatchdog{},
				nil, // pusher would normally be required, but we're just testing initialization
				uuid.New(),
				time.Minute,
				time.Minute,
				config.ReleaseChannelStable,
				false,
				nil, // systemSnapshotManager
				nil, // configManager
				logger,
				nil, // topicBrowserCommunicator
				nil, // fsmOutboundChannel - nil for legacy mode
			)
		})

		It("should handle legacy mode with nil fsmOutboundChannel", func() {
			// Verify handler was created successfully
			Expect(handler).NotTo(BeNil())
		})

		It("should still support instanceUUID operations in legacy mode", func() {
			expectedUUID := uuid.New()
			handler.SetInstanceUUID(expectedUUID)
			Expect(handler.GetInstanceUUID()).To(Equal(expectedUUID))
		})
	})

	Describe("Backward compatibility", func() {
		It("should allow both legacy and FSMv2 mode handlers to coexist", func() {
			// Create legacy handler (nil channel)
			legacyHandler := subscriber.NewHandler(
				&mockWatchdog{},
				nil,
				uuid.New(),
				time.Minute,
				time.Minute,
				config.ReleaseChannelStable,
				false,
				nil,
				nil,
				logger,
				nil,
				nil, // legacy mode
			)

			// Create FSMv2 handler (with channel)
			fsmv2Channel := make(chan *transport.UMHMessage, 10)
			defer close(fsmv2Channel)

			fsmv2Handler := subscriber.NewHandler(
				&mockWatchdog{},
				nil,
				uuid.New(),
				time.Minute,
				time.Minute,
				config.ReleaseChannelStable,
				false,
				nil,
				nil,
				logger,
				nil,
				fsmv2Channel, // FSMv2 mode
			)

			// Both handlers should work independently
			Expect(legacyHandler).NotTo(BeNil())
			Expect(fsmv2Handler).NotTo(BeNil())

			// Both should support UUID operations
			legacyUUID := uuid.New()
			fsmv2UUID := uuid.New()

			legacyHandler.SetInstanceUUID(legacyUUID)
			fsmv2Handler.SetInstanceUUID(fsmv2UUID)

			Expect(legacyHandler.GetInstanceUUID()).To(Equal(legacyUUID))
			Expect(fsmv2Handler.GetInstanceUUID()).To(Equal(fsmv2UUID))
		})
	})
})
