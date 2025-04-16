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
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/h2non/gock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/mocks"
	v2 "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/pull"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/push"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/backend_api_structs"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/communication_state"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/subscriber"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/container"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/logger"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/container_monitor"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/filesystem"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/snapshot"
	"go.uber.org/zap"
)

var _ = Describe("Subscribe and Receive Test", func() {
	var (
		ctx           context.Context
		cancel        context.CancelFunc
		state         *communication_state.CommunicationState
		authToken     string
		instanceID    uuid.UUID
		testEmail     string
		dog           watchdog.Iface
		login         *v2.LoginResponse
		subHandler    *subscriber.Handler
		outboundChan  chan *models.UMHMessage
		capturedMsgs  []*models.UMHMessage
		capturedMutex sync.Mutex

		mockFS *filesystem.MockFileSystem
		apiUrl string
		log    *zap.SugaredLogger
	)

	BeforeEach(func() {
		// Set up context with timeout
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)

		mockFS = filesystem.NewMockFileSystem()

		// Initialize test variables
		instanceID = uuid.New()
		authToken = "test-auth-token"
		testEmail = "test-user@example.com"

		// Set up a channel to capture outbound messages
		outboundChan = make(chan *models.UMHMessage, 100)
		capturedMsgs = []*models.UMHMessage{}
		capturedMutex = sync.Mutex{}

		apiUrl = "https://management.umh.app/api"
		log = logger.For("subscribe_and_receive_test")

		// Set up a goroutine to consume and capture the outbound messages
		go func() {
			for msg := range outboundChan {
				capturedMutex.Lock()
				capturedMsgs = append(capturedMsgs, msg)

				// More detailed debug info
				decodedContent, err := encoding.DecodeMessageFromUMHInstanceToUser(msg.Content)
				if err != nil {
					GinkgoWriter.Printf("Captured message for user %s but couldn't decode: %v\n", msg.Email, err)
				} else {
					GinkgoWriter.Printf("Captured %s message for user: %s\n", decodedContent.MessageType, msg.Email)
				}

				capturedMutex.Unlock()
			}
		}()

		// Set up mocks
		gock.Intercept()
		defer gock.Off()

		// Mock login endpoint
		mocks.MockLogin()

		// Setup the system snapshot with a ContainerManager
		systemSnapshot := &snapshot.SystemSnapshot{
			Managers:     make(map[string]snapshot.ManagerSnapshot),
			SnapshotTime: time.Now(),
		}

		// Create and add a ContainerManager
		mockSvc := container_monitor.NewMockService()
		mockSvc.SetupMockForHealthyState()
		containerManager := container.NewContainerManagerWithMockedService("Core", *mockSvc)

		// Initialize the manager with a reconcile call to create instances
		dummyConfig := config.FullConfig{}
		containerManager.Reconcile(ctx, snapshot.SystemSnapshot{CurrentConfig: dummyConfig, Tick: 1}, mockFS)

		// Create the snapshot after reconciliation
		containerManagerSnapshot := containerManager.CreateSnapshot()
		systemSnapshot.Managers["ContainerManager_Core"] = containerManagerSnapshot

		// Initialize watchdog
		dog = watchdog.NewWatchdog(ctx, time.NewTicker(1*time.Second), false, logger.For(logger.ComponentCommunicator))

		// Create login response
		login = &v2.LoginResponse{
			UUID: instanceID,
			JWT:  authToken,
			Name: "test-instance",
		}

		// Initialize communication state with our outbound channel
		state = &communication_state.CommunicationState{
			LoginResponse:   login,
			Watchdog:        dog.(*watchdog.Watchdog),
			InboundChannel:  make(chan *models.UMHMessage, 100),
			OutboundChannel: outboundChan, // Use our custom outbound channel
			InsecureTLS:     false,
			ReleaseChannel:  config.ReleaseChannel("stable"),
		}

		// Initialize pusher
		state.Pusher = push.NewPusher(
			instanceID,
			authToken,
			dog,
			outboundChan, // Use our custom outbound channel
			push.DefaultDeadLetterChanBuffer(),
			push.DefaultBackoffPolicy(),
			false,
			apiUrl,
			log,
		)
		state.Pusher.Start()

		// Initialize puller
		state.Puller = pull.NewPuller(
			authToken,
			dog,
			state.InboundChannel,
			false,
			apiUrl,
			log,
		)

		// Initialize subscriber handler
		ttl := 5 * time.Minute
		cull := 1 * time.Minute
		systemMu := &sync.Mutex{}
		subHandler = subscriber.NewHandler(
			dog,
			state.Pusher,
			instanceID,
			ttl,
			cull,
			config.ReleaseChannel("stable"),
			false,
			systemSnapshot,
			systemMu,
			config.NewMockConfigManager(),
			logger.For(logger.ComponentCommunicator),
		)
		subHandler.StartNotifier()

		// Set subscriber handler in communication state
		state.SubscriberHandler = subHandler
	})

	AfterEach(func() {
		// Clean up
		cancel()
		gock.Off()
		close(outboundChan)
	})

	It("should subscribe a user and send a status message", func() {
		By("Creating a subscribe message from user")
		// Mock a subscribe message from user
		subscribeMessage, err := encoding.EncodeMessageFromUserToUMHInstance(models.UMHMessageContent{
			MessageType: models.Subscribe,
			Payload:     "",
		})
		Expect(err).NotTo(HaveOccurred())

		By("Creating a pull payload with the subscribe message")
		// Create a pull payload containing the subscribe message
		pullPayload := backend_api_structs.PullPayload{
			UMHMessages: []models.UMHMessage{
				{
					InstanceUUID: instanceID,
					Email:        testEmail,
					Content:      subscribeMessage,
				},
			},
		}

		By("Mocking the pull endpoint")
		// Mock the pull endpoint with our custom payload
		mocks.MockSubscribeMessageWithPayload(pullPayload)

		By("Processing the incoming message and adding subscriber")
		// Manual processing of the incoming message
		// (this would normally be done by the router)
		for _, msg := range pullPayload.UMHMessages {
			decodedContent, err := encoding.DecodeMessageFromUserToUMHInstance(msg.Content)
			Expect(err).NotTo(HaveOccurred())

			// If it's a subscribe message, add the subscriber
			if decodedContent.MessageType == models.Subscribe {
				subHandler.AddSubscriber(msg.Email)
				GinkgoWriter.Printf("Added subscriber: %s\n", msg.Email)
			}
		}

		// Allow more time for the notifier to run and send status messages
		eventuallyTimeout := 5 * time.Second
		pollingInterval := 100 * time.Millisecond

		By("Verifying the subscriber was added")
		// Verify the subscriber was added
		subscribers := subHandler.GetSubscribers()
		Expect(subscribers).To(ContainElement(testEmail))
		GinkgoWriter.Printf("Confirmed subscriber was added\n")

		By("Waiting for status messages to be sent to the subscriber")
		// Wait for and verify that status messages are sent to the subscriber
		var capturedStatusMessages []string
		Eventually(func() bool {
			// Check the captured messages directly
			capturedMutex.Lock()
			defer capturedMutex.Unlock()

			GinkgoWriter.Println("===== Checking captured messages =====")
			GinkgoWriter.Printf("Found %d captured messages\n", len(capturedMsgs))

			capturedStatusMessages = nil // Reset for each attempt

			if len(capturedMsgs) == 0 {
				return false
			}

			// Check each message
			foundStatus := false
			for i, msg := range capturedMsgs {
				msgInfo := fmt.Sprintf("Message %d for %s", i, msg.Email)
				GinkgoWriter.Println(msgInfo)

				if msg.Email != testEmail {
					continue
				}

				// Try to decode
				decodedContent, err := encoding.DecodeMessageFromUMHInstanceToUser(msg.Content)
				if err != nil {
					GinkgoWriter.Printf("  - Failed to decode: %v\n", err)
					continue
				}

				msgTypeInfo := fmt.Sprintf("  - Type: %s", decodedContent.MessageType)
				GinkgoWriter.Println(msgTypeInfo)

				// Check if it's a status message
				if decodedContent.MessageType == models.Status {
					foundMsg := fmt.Sprintf("  - FOUND STATUS MESSAGE for %s", msg.Email)
					GinkgoWriter.Println(foundMsg)
					capturedStatusMessages = append(capturedStatusMessages, foundMsg)
					foundStatus = true
				}
			}

			return foundStatus
		}, eventuallyTimeout, pollingInterval).Should(BeTrue(), "Should send status messages to the subscriber")

		// Double-check that we actually found status messages
		Expect(capturedStatusMessages).NotTo(BeEmpty(), "Should have captured status messages")
	})
})
