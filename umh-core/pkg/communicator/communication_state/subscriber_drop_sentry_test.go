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

package communication_state_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	sentrygo "github.com/getsentry/sentry-go"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v2 "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/api/v2/push"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/communication_state"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

// sentryEventStore provides thread-safe storage for captured Sentry events.
type sentryEventStore struct {
	events []*sentrygo.Event
	mutex  sync.Mutex
}

func (s *sentryEventStore) Add(event *sentrygo.Event) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.events = append(s.events, event)
}

func (s *sentryEventStore) GetAll() []*sentrygo.Event {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	result := make([]*sentrygo.Event, len(s.events))
	copy(result, s.events)

	return result
}

// sentryMockTransport captures Sentry events instead of sending them.
type sentryMockTransport struct {
	store *sentryEventStore
}

func (t *sentryMockTransport) Configure(_ sentrygo.ClientOptions) {}

func (t *sentryMockTransport) Flush(_ time.Duration) bool { return true }

func (t *sentryMockTransport) FlushWithContext(_ context.Context) bool { return true }

func (t *sentryMockTransport) Close() {}

func (t *sentryMockTransport) SendEvent(event *sentrygo.Event) {
	t.store.Add(event)
}

// hasFSMv2OutboundDropEvent reports whether any captured event carries the
// message and feature tag the FSMv2 subscriber drop site must send.
func hasFSMv2OutboundDropEvent(events []*sentrygo.Event) bool {
	for _, event := range events {
		if event.Message == "fsmv2_outbound_channel_full" && event.Tags["feature"] == "fsmv1_communicator" {
			return true
		}
	}

	return false
}

// eventContains returns whether the stringified message, tags or contexts of
// the event contain the given substring. The hook copies log fields into
// Contexts; sentry-go v0.49 has no separate Extra field on Event.
func eventContains(event *sentrygo.Event, substr string) bool {
	if strings.Contains(event.Message, substr) {
		return true
	}

	for _, value := range event.Tags {
		if strings.Contains(value, substr) {
			return true
		}
	}

	for _, contextValues := range event.Contexts {
		for _, value := range contextValues {
			if strings.Contains(fmt.Sprintf("%v", value), substr) {
				return true
			}
		}
	}

	return false
}

var _ = Describe("Subscriber drop reaches Sentry through the production wiring", func() {
	var store *sentryEventStore

	BeforeEach(func() {
		store = &sentryEventStore{}

		err := sentrygo.Init(sentrygo.ClientOptions{
			Dsn:       "https://test@sentry.io/123",
			Transport: &sentryMockTransport{store: store},
		})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		sentrygo.Flush(time.Second)
		time.Sleep(50 * time.Millisecond)
	})

	It("captures an fsmv2_outbound_channel_full Sentry event when the production subscriber handler drops a message on a full FSMv2 channel", func() {
		logger := zap.NewNop().Sugar()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		dog := watchdog.NewWatchdog(ctx, time.NewTicker(time.Minute), false, logger)

		snapshotManager := fsm.NewSnapshotManager()
		snapshotManager.UpdateSnapshot(&fsm.SystemSnapshot{
			SnapshotTime: time.Now(),
			Managers: map[string]fsm.ManagerSnapshot{
				"SubscriberDropSentryManager": &fsm.BaseManagerSnapshot{
					Name:         "SubscriberDropSentryManager",
					SnapshotTime: time.Now(),
				},
			},
		})

		configManager := config.NewMockConfigManager()

		state := communication_state.NewCommunicationState(
			dog,
			make(chan *models.UMHMessage, 1),
			make(chan *models.UMHMessage, 1),
			config.ReleaseChannelStable,
			snapshotManager,
			configManager,
			"https://example.invalid",
			logger,
			false,
			nil,
			nil,
		)
		state.LoginResponse = &v2.LoginResponse{
			UUID: uuid.New(),
			JWT:  "test-jwt",
			Name: "subscriber-drop-sentry-test",
		}
		state.Pusher = push.NewPusher(
			state.LoginResponse.UUID,
			state.LoginResponse.JWT,
			dog,
			state.OutboundChannel,
			push.DefaultDeadLetterChanBuffer(),
			push.DefaultBackoffPolicy(),
			false,
			"https://example.invalid",
			logger,
		)

		// A capacity-1 channel that is already full, so every notify tick drops.
		fsmOutboundChannel := make(chan *types.UMHMessage, 1)
		fsmOutboundChannel <- &types.UMHMessage{InstanceUUID: "subscriber-drop-sentry-prefill"}

		state.InitialiseAndStartSubscriberHandler(
			time.Minute,
			time.Minute,
			&config.FullConfig{},
			snapshotManager,
			configManager,
			fsmOutboundChannel,
			nil,
		)
		Expect(state.SubscriberHandler).NotTo(BeNil(), "the production constructor must build the subscriber handler")

		const email = "drop-test@example.com"
		state.SubscriberHandler.AddOrRefreshSubscriber(email, true)

		Eventually(func() bool {
			return hasFSMv2OutboundDropEvent(store.GetAll())
		}, 10*time.Second, 200*time.Millisecond).Should(BeTrue(),
			"a drop on the full FSMv2 outbound channel must reach Sentry as an fsmv2_outbound_channel_full event tagged feature=fsmv1_communicator")

		for _, event := range store.GetAll() {
			Expect(eventContains(event, email)).To(BeFalse(),
				"no captured Sentry event may contain the subscriber email")
		}
	})
})
