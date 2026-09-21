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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
	"go.uber.org/zap"
)

var _ = Describe("Outbound channel usage", func() {
	It("samples the transport channel, not the gatekeeper channel it writes to", func() {
		gatekeeperChannel := make(chan *types.MessageWithSender, 10)
		for range 10 {
			gatekeeperChannel <- &types.MessageWithSender{}
		}

		transportChannel := make(chan *types.UMHMessage, 10)
		for range 5 {
			transportChannel <- &types.UMHMessage{}
		}

		handler := subscriber.NewHandler(
			&mockWatchdog{},
			nil, // pusher
			uuid.New(),
			time.Minute,
			time.Minute,
			config.ReleaseChannelStable,
			false,
			nil, // systemSnapshotManager
			nil, // configManager
			zap.NewNop().Sugar(),
			nil, // topicBrowserCommunicator
			nil, // fsmOutboundChannel
			gatekeeperChannel,
			transportChannel,
			nil, // featureUsage
		)

		handler.StartNotifier()

		Eventually(func(g Gomega) float64 {
			verdict, ok := handler.OutboundUsage().Verdict()
			g.Expect(ok).To(BeTrue())

			return verdict.P95FillPercent
		}, 5*time.Second, 100*time.Millisecond).Should(BeNumerically("~", 50, 0.001))
	})
})
