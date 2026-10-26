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

package generator_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/fsmv2client"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/managementconsole"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/generator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/tools/watchdog"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/topicbrowser"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/zap"
)

// newCollectorForConnection builds a status collector over the same snapshot
// the generator tests use: one container manager, one agent manager, a mock
// config manager, and a topic browser communicator. The fixed fields the
// specs assert on are the ones the snapshot and the config feed.
func newCollectorForConnection(status managementconsole.StatusFunc) *generator.StatusCollectorType {
	// Pin the client to nil for these specs, so the connection part comes
	// only from the Status input, whatever an earlier spec published.
	previous := fsmv2client.GetClient()
	fsmv2client.SetClient(nil)
	DeferCleanup(func() { fsmv2client.SetClient(previous) })

	logger := zap.NewNop().Sugar()

	snapshotManager := fsm.NewSnapshotManager()
	snapshotManager.UpdateSnapshot(&fsm.SystemSnapshot{
		SnapshotTime: time.Now(),
		Managers: map[string]fsm.ManagerSnapshot{
			constants.ContainerManagerName: &fsm.BaseManagerSnapshot{
				Name:      constants.ContainerManagerName,
				Instances: map[string]*fsm.FSMInstanceSnapshot{},
			},
			constants.AgentManagerName: &fsm.BaseManagerSnapshot{
				Name:      constants.AgentManagerName,
				Instances: map[string]*fsm.FSMInstanceSnapshot{},
			},
		},
	})

	return generator.NewStatusCollector(
		&watchdog.Watchdog{},
		snapshotManager,
		config.NewMockConfigManager(),
		logger,
		topicbrowser.NewTopicBrowserCommunicator(logger),
		nil, // featureUsage
		func() int { return 1 },
		status,
	)
}

var _ = Describe("Status connection part", func() {
	It("checks that the status message's connection part is empty when no provider is registered", func() {
		collector := newCollectorForConnection(nil)

		status := collector.GenerateStatusMessage(context.Background(), true)
		Expect(status).NotTo(BeNil())

		Expect(status.Core.Communicator).To(BeNil(),
			"a build from this repository alone provides no connection part")

		// A fixed set of the other fields equals the values the test sets up.
		Expect(status.Core.Release.Health).NotTo(BeNil())
		Expect(status.Core.Release.Health.ObservedState).To(Equal("running"))
		Expect(status.Core.Release.SupportedFeatures).To(ContainElement("core-health"))
		Expect(status.Core.Agent.Location).To(BeEmpty())
		Expect(status.Core.Container).To(Equal(models.Container{}))
		Expect(status.Core.Dfcs).To(BeEmpty())
	})

	It("checks that the provider's part appears in the status message as given", func() {
		// Every field holds a distinct non-zero value, so a field the
		// conversion drops or swaps makes the comparison fail.
		part := &managementconsole.Status{
			Health:              managementconsole.Degraded,
			HealthMessage:       "planted connection part",
			State:               "Degraded",
			ConsecutiveErrors:   4,
			LastErrorType:       "server_error",
			OutboundFillPercent: 12.5,
			OutboundPeakPercent: 40,
			Push:                &managementconsole.ChannelStatus{State: "Running", LastErrorType: "push_error", Messages: 900, MessagesDropped: 7, PendingMessages: 2, ConsecutiveErrors: 1},
			Pull:                &managementconsole.ChannelStatus{State: "Degraded", LastErrorType: "pull_error", Messages: 800, MessagesDropped: 6, PendingMessages: 5, ConsecutiveErrors: 8},
		}
		provider := func(ctx context.Context) *managementconsole.Status {
			Expect(ctx).NotTo(BeNil())

			return part
		}

		collector := newCollectorForConnection(provider)

		status := collector.GenerateStatusMessage(context.Background(), true)
		Expect(status).NotTo(BeNil())

		Expect(status.Core.Communicator).To(Equal(&models.Communicator{
			Health:                     &models.Health{Category: models.Degraded, Message: "planted connection part"},
			State:                      "Degraded",
			SubscriberCount:            1,
			ConsecutiveErrors:          4,
			LastErrorType:              "server_error",
			OutboundChannelFillPercent: 12.5,
			OutboundChannelPeakPercent: 40,
			Push:                       &models.CommunicatorChannel{State: "Running", LastErrorType: "push_error", Messages: 900, MessagesDropped: 7, PendingMessages: 2, ConsecutiveErrors: 1},
			Pull:                       &models.CommunicatorChannel{State: "Degraded", LastErrorType: "pull_error", Messages: 800, MessagesDropped: 6, PendingMessages: 5, ConsecutiveErrors: 8},
		}), "the provider's part must appear in the status message as given")
	})
})
