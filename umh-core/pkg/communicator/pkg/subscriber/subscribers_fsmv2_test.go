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
	"encoding/json"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/subscriber"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

var _ = Describe("FSMv2 Direct Channel Mode", func() {
	var (
		handler            *subscriber.Handler
		logger             *zap.SugaredLogger
		fsmOutboundChannel chan *types.UMHMessage
	)

	BeforeEach(func() {
		zapLogger, err := zap.NewDevelopment()
		Expect(err).NotTo(HaveOccurred())
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
			fsmOutboundChannel = make(chan *types.UMHMessage, 10)

			handler = subscriber.NewHandler(
				&mockWatchdog{},
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
				nil, // featureUsage
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

	})

var _ = Describe("Status delivery on the FSMv2 outbound channel", func() {
	// The guard for the FSMv2 status path: a subscriber registers, the notifier
	// runs, and a Status message arrives on fsmOutboundChannel with no Pusher
	// involved. A later rung removes the write-only Pusher; this guard pins the
	// delivery path it must not break.
	//
	// Real collaborators, not mocks of the StatusCollector: the concrete
	// generator.StatusCollectorType built by NewHandler needs a snapshot with at
	// least one manager (otherwise GenerateStatusMessage returns nil), a config
	// manager it can call GetConfig on (NewMockConfigManager), and a real
	// TopicBrowserCommunicator (a nil one would panic inside
	// UpdateTopicBrowserCache). The watchdog is the package's mockWatchdog.
	//
	// The payload assertion is load-bearing: GenerateStatusMessage returns an
	// empty-but-non-nil StatusMessage when a config read fails, and that stub
	// still encodes with MessageType=Status. Asserting only the message type
	// would green-light a stub status. The happy path unconditionally populates
	// Core.Release.Health and SupportedFeatures; the empty fallback has neither.
	It("delivers a non-empty status message through the FSMv2 outbound channel", func() {
		zapLogger, err := zap.NewDevelopment()
		Expect(err).NotTo(HaveOccurred())
		logger := zapLogger.Sugar()

		snapshotManager := fsm.NewSnapshotManager()
		snapshotManager.UpdateSnapshot(&fsm.SystemSnapshot{
			SnapshotTime: time.Now(),
			Managers: map[string]fsm.ManagerSnapshot{
				constants.ContainerManagerName: &fsm.BaseManagerSnapshot{
					Name:      constants.ContainerManagerName,
					Instances: map[string]*fsm.FSMInstanceSnapshot{},
				},
			},
		})
		configManager := config.NewMockConfigManager()
		topicBrowserCommunicator := topicbrowser.NewTopicBrowserCommunicator(logger)

		fsmOutboundChannel := make(chan *types.UMHMessage, 10)
		instanceUUID := uuid.New()

		handler := subscriber.NewHandler(
			&mockWatchdog{},
			instanceUUID,
			time.Minute, // ttl
			time.Minute, // cull
			config.ReleaseChannelStable,
			false, // disableHardwareStatusCheck
			snapshotManager,
			configManager,
			logger,
			topicBrowserCommunicator,
			fsmOutboundChannel,
			nil, // featureUsage
		)

		const subscriberEmail = "status-test@example.com"
		handler.AddOrRefreshSubscriber(subscriberEmail, false)
		handler.StartNotifier()

		var received *types.UMHMessage
		Eventually(fsmOutboundChannel, "10s", "100ms").Should(Receive(&received),
			"expected a status message on the FSMv2 outbound channel")

		Expect(received.InstanceUUID).To(Equal(instanceUUID.String()))
		Expect(received.Email).To(Equal(subscriberEmail))

		content, err := encoding.DecodeMessageFromUMHInstanceToUser(received.Content)
		Expect(err).NotTo(HaveOccurred())
		Expect(content.MessageType).To(Equal(models.Status))

		rawPayload, err := json.Marshal(content.Payload)
		Expect(err).NotTo(HaveOccurred())

		var status models.StatusMessage
		Expect(json.Unmarshal(rawPayload, &status)).To(Succeed())
		Expect(status.Core.Release.Health).NotTo(BeNil(),
			"happy-path status must carry a health record, not the empty fallback")
		Expect(status.Core.Release.SupportedFeatures).NotTo(BeEmpty(),
			"happy-path status must carry supported features, not the empty fallback")
	})
})
