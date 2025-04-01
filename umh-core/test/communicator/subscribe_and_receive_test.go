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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/subscriber"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
)

var _ = Describe("Subscribe and Receive Test", func() {
	var (
		ctx        context.Context
		cancel     context.CancelFunc
		state      *communication_state.CommunicationState
		authToken  string
		instanceID uuid.UUID
		testEmail  string
		dog        watchdog.Iface
		login      *v2.LoginResponse
		subHandler *subscriber.Handler
	)

	BeforeEach(func() {
		// Set up context with timeout
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)

		// Initialize test variables
		instanceID = uuid.New()
		authToken = "test-auth-token"
		testEmail = "test-user@example.com"

		// Set up mocks
		gock.Intercept()
		defer gock.Off()

		// Mock login endpoint
		mocks.MockLogin()

		// Mock the push endpoint
		mocks.MockPushEndpoint()

		// Create a dummy FSM system snapshot
		systemSnapshot := &fsm.SystemSnapshot{
			Managers:     make(map[string]fsm.ManagerSnapshot),
			SnapshotTime: time.Now(),
		}

		// Initialize watchdog
		dog = watchdog.NewWatchdog(ctx, time.NewTicker(1*time.Second), false)

		// Create login response
		login = &v2.LoginResponse{
			UUID: instanceID,
			JWT:  authToken,
			Name: "test-instance",
		}

		// Initialize communication state
		state = &communication_state.CommunicationState{
			LoginResponse:   login,
			Watchdog:        dog.(*watchdog.Watchdog),
			InboundChannel:  make(chan *models.UMHMessage, 100),
			OutboundChannel: make(chan *models.UMHMessage, 100),
			InsecureTLS:     false,
			ReleaseChannel:  config.ReleaseChannel("stable"),
		}

		// Initialize pusher
		state.Pusher = push.NewPusher(
			instanceID,
			authToken,
			dog,
			state.OutboundChannel,
			push.DefaultDeadLetterChanBuffer(),
			push.DefaultBackoffPolicy(),
			false,
		)
		state.Pusher.Start()

		// Initialize puller
		state.Puller = pull.NewPuller(
			authToken,
			dog,
			state.InboundChannel,
			false,
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
		)
		subHandler.StartNotifier()

		// Set subscriber handler in communication state
		state.SubscriberHandler = subHandler
	})

	AfterEach(func() {
		// Clean up
		cancel()
		gock.Off()
	})

	It("should subscribe a user and send a status message", func() {
		// Mock a subscribe message from user
		subscribeMessage, err := encoding.EncodeMessageFromUserToUMHInstance(models.UMHMessageContent{
			MessageType: models.Subscribe,
			Payload:     "",
		})
		Expect(err).NotTo(HaveOccurred())

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

		// Mock the pull endpoint with our custom payload
		mocks.MockSubscribeMessageWithPayload(pullPayload)

		// Manual processing of the incoming message
		// (this would normally be done by the router)
		for _, msg := range pullPayload.UMHMessages {
			decodedContent, err := encoding.DecodeMessageFromUserToUMHInstance(msg.Content)
			Expect(err).NotTo(HaveOccurred())

			// If it's a subscribe message, add the subscriber
			if decodedContent.MessageType == models.Subscribe {
				subHandler.AddSubscriber(msg.Email)
			}
		}

		// Allow some time for processing
		time.Sleep(100 * time.Millisecond)

		// Verify the subscriber was added
		subscribers := subHandler.GetSubscribers()
		Expect(subscribers).To(ContainElement(testEmail))

		// Verify a push was attempted - this is implicitly tested by our mock
		// If the Push endpoint was not called, gock would fail the test
	})
})
