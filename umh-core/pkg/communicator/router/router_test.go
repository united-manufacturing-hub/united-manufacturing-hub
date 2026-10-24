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

package router_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	deps "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/handoff"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/subscriber"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/router"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

func TestRouter(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Router Suite")
}

// mockWatchdog implements watchdog.Iface for testing. The subscriber package
// has its own mock with the same name that cannot be imported.
type mockWatchdog struct{}

func (m *mockWatchdog) Start()                                                    {}
func (m *mockWatchdog) RegisterHeartbeat(string, uint64, uint64, bool) uuid.UUID  { return uuid.New() }
func (m *mockWatchdog) UnregisterHeartbeat(uuid.UUID)                             {}
func (m *mockWatchdog) ReportHeartbeatStatus(uuid.UUID, watchdog.HeartbeatStatus) {}
func (m *mockWatchdog) SetHasSubscribers(bool)                                    {}

// encodeInbound encodes a message the way the producers do before handing it
// to the router.
func encodeInbound(messageType models.MessageType, payload any, sender, traceID string) handoff.Message {
	encoded, err := encoding.EncodeMessageFromUserToUMHInstance(models.UMHMessageContent{
		MessageType: messageType,
		Payload:     payload,
	})
	Expect(err).NotTo(HaveOccurred())

	return handoff.Message{Content: []byte(encoded), SenderEmail: sender, TraceID: traceID}
}

// newTestHandler builds a real subscriber.Handler the way the subscriber
// package's own tests do. The handler pushes status on its own outbound
// channel, of the type the subscriber package sends; action replies take
// the router's outbound channel instead, so the specs read that one.
func newTestHandler(fsmOutboundChannel chan *types.UMHMessage) *subscriber.Handler {
	return subscriber.NewHandler(
		&mockWatchdog{},
		uuid.New(),
		time.Minute,
		time.Minute,
		config.ReleaseChannelStable,
		false,
		nil, // systemSnapshotManager
		nil, // configManager
		zap.NewNop().Sugar(),
		nil, // topicBrowserCommunicator
		fsmOutboundChannel,
		nil,                    // gatekeeperOutboundChannel
		nil,                    // featureUsage
		deps.NewNopFSMLogger(), // fsmLogger
	)
}

var _ = Describe("Router", func() {
	var (
		inbound       chan handoff.Message
		outbound      chan *models.UMHMessage
		handlerPushes chan *types.UMHMessage
		handler       *subscriber.Handler
		r             *router.Router
		observed      *observer.ObservedLogs
	)

	newRouter := func(logger *zap.SugaredLogger) *router.Router {
		return router.NewRouterForFSMv2(
			&mockWatchdog{},
			inbound,
			uuid.New(),
			outbound,
			config.ReleaseChannelStable,
			handler,
			nil, // systemSnapshotManager
			nil, // configManager
			logger,
		)
	}

	BeforeEach(func() {
		inbound = make(chan handoff.Message, 10)
		outbound = make(chan *models.UMHMessage, 100)
		handlerPushes = make(chan *types.UMHMessage, 100)
		handler = newTestHandler(handlerPushes)
	})

	AfterEach(func() {
		if r != nil {
			// Nothing stops the router; the test process ends with it.
			r = nil
		}
	})

	It("adds a subscriber from a subscribe message that names its sender", func() {
		r = newRouter(zap.NewNop().Sugar())
		r.Start()

		inbound <- encodeInbound(models.Subscribe, models.SubscribeMessagePayload{}, "a@example.com", "")

		Eventually(func() []string {
			return handler.GetSubscribers()
		}).Should(ContainElement("a@example.com"))
	})

	It("refuses messages that name no sender and keeps serving later ones", func() {
		// Each refused message's payload carries a unique marker, so the
		// search can tell one refusal from another.
		first := encodeInbound(models.Subscribe, map[string]any{"marker": "first-refused-payload"}, "", "")
		second := encodeInbound(models.Action, map[string]any{"marker": "second-refused-payload", "actionType": "no-such-action", "actionUUID": "00000000-0000-0000-0000-000000000001"}, "", "")
		third := encodeInbound(models.Subscribe, models.SubscribeMessagePayload{}, "a@example.com", "")

		core, logs := observer.New(zapcore.DebugLevel)
		observed = logs
		r = newRouter(zap.New(core).Sugar())
		r.Start()

		inbound <- first
		inbound <- second

		// Two refusals, one warning each, before the valid message runs.
		Eventually(func() int {
			return observed.FilterMessage("Refusing message because it names no sender").Len()
		}).Should(Equal(2))

		// The valid action is the barrier: a reply for it proves both refused
		// messages were fully processed, because the router handles actions in
		// goroutines after the loop moved on.
		emails := make([]string, 0, 4)
		inbound <- encodeInbound(models.Action, map[string]any{
			"actionType": "no-such-action",
			"actionUUID": "00000000-0000-0000-0000-000000000002",
		}, "a@example.com", "")

		Eventually(func() bool {
			for {
				select {
				case msg := <-outbound:
					emails = append(emails, msg.Email)
					if msg.Email == "a@example.com" {
						return true
					}
				default:
					return false
				}
			}
		}).Should(BeTrue())

		// No reply went to an empty address.
		Expect(emails).NotTo(ContainElement(""))

		// No subscriber with an empty address was ever added.
		Expect(handler.GetSubscribers()).NotTo(ContainElement(""))

		// The warnings carry the reason only: none of the message's bytes or
		// decoded payload appears in the message or in the fields.
		entries := observed.FilterMessage("Refusing message because it names no sender").All()
		Expect(entries).NotTo(BeEmpty())
		for _, entry := range entries {
			Expect(entry.Message).NotTo(ContainSubstring("first-refused-payload"))
			Expect(entry.Message).NotTo(ContainSubstring("second-refused-payload"))
			for _, f := range entry.Context {
				rendered := anyToString(f.Interface)
				Expect(rendered).NotTo(ContainSubstring("first-refused-payload"))
				Expect(rendered).NotTo(ContainSubstring("second-refused-payload"))
			}
		}

		// After the refusals the router keeps running and serves a valid
		// subscribe.
		inbound <- third
		Eventually(func() []string {
			return handler.GetSubscribers()
		}).Should(ContainElement("a@example.com"))
	})

	It("sends the action reply to the message's sender", func() {
		r = newRouter(zap.NewNop().Sugar())
		r.Start()

		inbound <- encodeInbound(models.Action, map[string]any{
			"actionType": "no-such-action",
			"actionUUID": "00000000-0000-0000-0000-000000000003",
		}, "a@example.com", "")

		Eventually(func() string {
			select {
			case msg := <-outbound:
				return msg.Email
			default:
				return ""
			}
		}).Should(Equal("a@example.com"))
	})

	It("drops bytes that do not decode and keeps serving later messages", func() {
		core, logs := observer.New(zapcore.DebugLevel)
		observed = logs
		r = newRouter(zap.New(core).Sugar())
		r.Start()

		inbound <- handoff.Message{Content: []byte("not-valid-base64!!!"), SenderEmail: "a@example.com", TraceID: ""}

		Eventually(func() int {
			return observed.FilterMessage("Failed to decode message").Len()
		}).Should(Equal(1))

		inbound <- encodeInbound(models.Subscribe, models.SubscribeMessagePayload{}, "a@example.com", "")
		Eventually(func() []string {
			return handler.GetSubscribers()
		}).Should(ContainElement("a@example.com"))
	})
})

// anyToString renders a log field's value the way the logging and the
// rendering searches do: its fmt form.
func anyToString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%+v", v)
}
