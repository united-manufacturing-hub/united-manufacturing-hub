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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/generator"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/deps"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/channelusage"
	pullsnapshot "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/pull/snapshot"
	pushsnapshot "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/push/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/snapshot"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsmv2/workers/transport/types"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

var _ = Describe("CommunicatorFromObservations", func() {
	measured := snapshot.TransportStatus{OutboundQueue: channelusage.Verdict{Measured: true, FillPercent: 40}}
	fullQueue := snapshot.TransportStatus{OutboundQueue: channelusage.Verdict{Measured: true, Degraded: true, FillPercent: 100}}

	fromTransport := func(state string, status snapshot.TransportStatus) *models.Communicator {
		return generator.CommunicatorFromObservations(
			fsmv2.Observation[snapshot.TransportStatus]{State: state, Status: status},
			fsmv2.Observation[pushsnapshot.PushStatus]{},
			fsmv2.Observation[pullsnapshot.PullStatus]{},
		)
	}

	DescribeTable("derives health from the transport state and the outbound queue",
		func(state string, status snapshot.TransportStatus, expected models.HealthCategory) {
			Expect(fromTransport(state, status).Health.Category).To(Equal(expected))
		},
		Entry("running, queue measured", "Running", measured, models.Active),
		Entry("running, queue not measured yet", "Running", snapshot.TransportStatus{}, models.Neutral),
		Entry("degraded, queue full", "Degraded", fullQueue, models.Degraded),
		Entry("degraded, children unhealthy", "Degraded", measured, models.Degraded),
		Entry("authentication failed", "AuthFailed", snapshot.TransportStatus{}, models.Degraded),
		Entry("starting", "Starting", snapshot.TransportStatus{}, models.Neutral),
	)

	It("maps an observed push child and leaves an unobserved pull child nil", func() {
		push := fsmv2.Observation[pushsnapshot.PushStatus]{
			CollectedAt: time.Now(),
			State:       "Degraded",
			Status: pushsnapshot.PushStatus{
				ConsecutiveErrors: 4, LastErrorType: types.ErrorTypeServerError, LastStatusCode: 503, PendingMessageCount: 120,
			},
		}
		push.Metrics.Worker = deps.Metrics{
			Counters: map[string]int64{string(deps.CounterMessagesPushed): 900, string(deps.CounterMessagesDropped): 7},
			Gauges:   map[string]float64{string(deps.GaugeLastPushLatencyMs): 250},
		}

		result := generator.CommunicatorFromObservations(
			fsmv2.Observation[snapshot.TransportStatus]{State: "Degraded"},
			push,
			fsmv2.Observation[pullsnapshot.PullStatus]{},
		)

		Expect(result.Pull).To(BeNil())
		Expect(result.Push).To(Equal(&models.CommunicatorChannel{
			State: "Degraded", LastErrorType: types.ErrorTypeServerError.String(), Messages: 900, MessagesDropped: 7,
			PendingMessages: 120, ConsecutiveErrors: 4, LastStatusCode: 503, LastLatencyMs: 250,
		}))
	})
})
