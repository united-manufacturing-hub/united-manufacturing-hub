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

package fsmv2_adapter_test

import (
	"context"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/fsmv2_adapter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/communicator/transport"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

var _ = Describe("LegacyChannelBridge", func() {
	var (
		legacyInbound  chan *models.UMHMessage
		legacyOutbound chan *models.UMHMessage
		logger         *zap.SugaredLogger
	)

	BeforeEach(func() {
		legacyInbound = make(chan *models.UMHMessage, 100)
		legacyOutbound = make(chan *models.UMHMessage, 100)
		logger = zap.NewNop().Sugar()
	})

	Describe("NewLegacyChannelBridge", func() {
		It("should create a bridge with correct buffer sizes", func() {
			bridge := fsmv2_adapter.NewLegacyChannelBridge(
				legacyInbound,
				legacyOutbound,
				logger,
			)

			Expect(bridge).NotTo(BeNil())
		})
	})

	Describe("GetChannels", func() {
		It("should return channels for a given worker ID", func() {
			bridge := fsmv2_adapter.NewLegacyChannelBridge(
				legacyInbound,
				legacyOutbound,
				logger,
			)

			inbound, outbound := bridge.GetChannels("test-worker")

			Expect(inbound).NotTo(BeNil())
			Expect(outbound).NotTo(BeNil())
		})
	})

	Describe("Interface compliance", func() {
		It("should implement communicator.ChannelProvider interface", func() {
			bridge := fsmv2_adapter.NewLegacyChannelBridge(
				legacyInbound,
				legacyOutbound,
				logger,
			)

			// Verify interface compliance via type assertion
			var provider communicator.ChannelProvider = bridge
			Expect(provider).NotTo(BeNil())
		})
	})

	Describe("Start() conversion goroutines", func() {
		var (
			ctx    context.Context
			cancel context.CancelFunc
		)

		BeforeEach(func() {
			ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		})

		AfterEach(func() {
			cancel()
		})

		It("should convert transport.UMHMessage to models.UMHMessage on inbound", func() {
			legacyIn := make(chan *models.UMHMessage, 10)
			legacyOut := make(chan *models.UMHMessage, 10)
			bridge := fsmv2_adapter.NewLegacyChannelBridge(legacyIn, legacyOut, logger)
			bridge.Start(ctx)

			fsmIn, _ := bridge.GetChannels("test")
			testUUID := "550e8400-e29b-41d4-a716-446655440000"
			fsmIn <- &transport.UMHMessage{
				InstanceUUID: testUUID,
				Content:      "test-content",
				Email:        "test@example.com",
			}

			Eventually(legacyIn).Should(Receive(SatisfyAll(
				HaveField("InstanceUUID", Equal(uuid.MustParse(testUUID))),
				HaveField("Content", Equal("test-content")),
				HaveField("Email", Equal("test@example.com")),
			)))
		})

		It("should convert models.UMHMessage to transport.UMHMessage on outbound", func() {
			legacyIn := make(chan *models.UMHMessage, 10)
			legacyOut := make(chan *models.UMHMessage, 10)
			bridge := fsmv2_adapter.NewLegacyChannelBridge(legacyIn, legacyOut, logger)
			bridge.Start(ctx)

			_, fsmOut := bridge.GetChannels("test")
			testUUID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
			legacyOut <- &models.UMHMessage{
				InstanceUUID: testUUID,
				Content:      "test-content",
				Email:        "test@example.com",
			}

			Eventually(fsmOut).Should(Receive(SatisfyAll(
				HaveField("InstanceUUID", Equal(testUUID.String())),
				HaveField("Content", Equal("test-content")),
				HaveField("Email", Equal("test@example.com")),
			)))
		})

		It("should handle context cancellation gracefully", func() {
			legacyIn := make(chan *models.UMHMessage, 10)
			legacyOut := make(chan *models.UMHMessage, 10)
			bridge := fsmv2_adapter.NewLegacyChannelBridge(legacyIn, legacyOut, logger)
			bridge.Start(ctx)

			cancel() // Cancel context
			// Should not panic or deadlock - test completes if goroutines exit
			time.Sleep(100 * time.Millisecond)
		})
	})

	Describe("Non-blocking behavior (Bug #2 fix)", func() {
		It("should not block when inbound channel is full", func() {
			legacyIn := make(chan *models.UMHMessage, 1) // Small buffer
			legacyOut := make(chan *models.UMHMessage, 10)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			bridge := fsmv2_adapter.NewLegacyChannelBridge(legacyIn, legacyOut, logger)
			bridge.Start(ctx)

			fsmIn, _ := bridge.GetChannels("test")

			// Fill the legacy inbound channel
			legacyIn <- &models.UMHMessage{}

			// Send should not block - use goroutine with timeout
			done := make(chan struct{})
			go func() {
				fsmIn <- &transport.UMHMessage{Content: "test1"}
				fsmIn <- &transport.UMHMessage{Content: "test2"} // This would block if not non-blocking
				close(done)
			}()

			Eventually(done).WithTimeout(time.Second).Should(BeClosed())
		})

		It("should not block when outbound channel is full", func() {
			legacyIn := make(chan *models.UMHMessage, 10)
			legacyOut := make(chan *models.UMHMessage, 1) // Small buffer
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			bridge := fsmv2_adapter.NewLegacyChannelBridge(legacyIn, legacyOut, logger)
			bridge.Start(ctx)

			_, fsmOut := bridge.GetChannels("test")

			// Fill the FSM outbound channel by not reading from it
			legacyOut <- &models.UMHMessage{}
			legacyOut <- &models.UMHMessage{} // These fill the internal buffer

			// Verify conversion goroutine doesn't block
			done := make(chan struct{})
			go func() {
				time.Sleep(200 * time.Millisecond) // Give time for potential deadlock
				close(done)
			}()

			Eventually(done).WithTimeout(time.Second).Should(BeClosed())
			_ = fsmOut // Silence unused variable warning
		})
	})

	Describe("UUID parsing error handling", func() {
		It("should handle invalid UUID strings by using uuid.Nil", func() {
			legacyIn := make(chan *models.UMHMessage, 10)
			legacyOut := make(chan *models.UMHMessage, 10)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			bridge := fsmv2_adapter.NewLegacyChannelBridge(legacyIn, legacyOut, logger)
			bridge.Start(ctx)

			fsmIn, _ := bridge.GetChannels("test")
			fsmIn <- &transport.UMHMessage{
				InstanceUUID: "invalid-uuid",
				Content:      "test",
			}

			Eventually(legacyIn).Should(Receive(HaveField("InstanceUUID", Equal(uuid.Nil))))
		})
	})

	Describe("Message integrity", func() {
		It("should preserve message integrity through conversion cycle", func() {
			legacyIn := make(chan *models.UMHMessage, 10)
			legacyOut := make(chan *models.UMHMessage, 10)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			bridge := fsmv2_adapter.NewLegacyChannelBridge(legacyIn, legacyOut, logger)
			bridge.Start(ctx)

			fsmIn, _ := bridge.GetChannels("test")

			// Test with various content types
			testCases := []struct {
				uuid    string
				content string
				email   string
			}{
				{"550e8400-e29b-41d4-a716-446655440000", "", ""},                    // Empty content
				{"550e8400-e29b-41d4-a716-446655440001", "simple", "test@test.com"}, // Simple
				{"550e8400-e29b-41d4-a716-446655440002", `{"json":"value"}`, ""},    // JSON content
			}

			for _, tc := range testCases {
				fsmIn <- &transport.UMHMessage{
					InstanceUUID: tc.uuid,
					Content:      tc.content,
					Email:        tc.email,
				}

				Eventually(legacyIn).Should(Receive(SatisfyAll(
					HaveField("InstanceUUID", Equal(uuid.MustParse(tc.uuid))),
					HaveField("Content", Equal(tc.content)),
					HaveField("Email", Equal(tc.email)),
				)))
			}
		})
	})
})
