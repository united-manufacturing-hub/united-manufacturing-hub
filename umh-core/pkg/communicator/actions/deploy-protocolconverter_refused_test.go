// Copyright 2026 UMH Systems GmbH
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

package actions_test

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	internalfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/internal/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/bridgeadmission"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/protocolconverter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	protocolconvertersvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/protocolconverter"
)

// A deploy whose bridge is refused for a degraded resource fails at once with
// an error code naming that resource. Any other refusal waits for the timeout.
var _ = Describe("DeployProtocolConverter refused for resources", func() {
	const (
		bridgeName   = "refused_bridge"
		awaitTimeout = time.Second
		refusal      = "Memory degraded: memory is full"
	)

	var (
		mockCfg      *config.MockConfigManager
		sentryEvents *recordingFSMLogger
		snapMgr      *fsm.SnapshotManager
		outbound     chan *models.UMHMessage
		msgs         []*models.UMHMessage
		mu           sync.Mutex
	)

	publishRefusal := func(cause bridgeadmission.Cause) {
		snapMgr.UpdateSnapshot(&fsm.SystemSnapshot{
			Managers: map[string]fsm.ManagerSnapshot{
				constants.ProtocolConverterManagerName: &actions.MockManagerSnapshot{
					Instances: map[string]*fsm.FSMInstanceSnapshot{
						bridgeName: {
							ID:           bridgeName,
							CurrentState: internalfsm.LifecycleStateToBeCreated,
							DesiredState: protocolconverter.OperationalStateActive,
							LastObservedState: &protocolconverter.ProtocolConverterObservedStateSnapshot{
								ServiceInfo: protocolconvertersvc.ServiceInfo{
									Admission:    bridgeadmission.Decision{Cause: cause, Reason: refusal},
									StatusReason: refusal,
								},
							},
						},
					},
				},
			},
		})
	}

	// deploy deploys the bridge and returns how long it took and the error
	// code of the failure reply.
	deploy := func() (time.Duration, string) {
		action := actions.NewDeployProtocolConverterAction("probe@example.com", uuid.New(), uuid.New(), outbound, mockCfg, snapMgr)
		action.SetFSMLogger(sentryEvents)
		action.SetTickInterval(50 * time.Millisecond)
		action.SetAwaitTimeout(awaitTimeout)
		Expect(action.Parse(map[string]any{
			"name":       bridgeName,
			"connection": map[string]any{"ip": "localhost", "port": uint32(502)},
			"location":   map[int]string{0: "enterprise"},
		})).To(Succeed())
		Expect(action.Validate()).To(Succeed())

		start := time.Now()
		_, _, err := action.Execute()
		elapsed := time.Since(start)

		Expect(err).To(MatchError(ContainSubstring(refusal)))

		var errorCode string

		Eventually(func() string {
			mu.Lock()
			defer mu.Unlock()

			for _, message := range msgs {
				decoded, decodeErr := encoding.DecodeMessageFromUMHInstanceToUser(message.Content)
				if decodeErr != nil {
					continue
				}

				reply, ok := decoded.Payload.(map[string]interface{})
				if !ok {
					continue
				}

				payloadV2, ok := reply["actionReplyPayloadV2"].(map[string]interface{})
				if !ok || reply["actionReplyState"] != string(models.ActionFinishedWithFailure) {
					continue
				}

				errorCode = fmt.Sprintf("%v", payloadV2["errorCode"])
			}

			return errorCode
		}, "3s", "50ms").ShouldNot(BeEmpty())

		return elapsed, errorCode
	}

	BeforeEach(func() {
		outbound = make(chan *models.UMHMessage, 200)

		mu.Lock()
		msgs = nil
		mu.Unlock()

		mockCfg = config.NewMockConfigManager().WithConfig(config.FullConfig{
			Agent: config.AgentConfig{MetricsPort: 8080},
		})
		sentryEvents = &recordingFSMLogger{}
		snapMgr = fsm.NewSnapshotManager()

		go actions.ConsumeOutboundMessages(outbound, &msgs, &mu, false)
	})

	AfterEach(func() { close(outbound) })

	DescribeTable("fails before the timeout with the code naming the resource",
		func(cause bridgeadmission.Cause, errorCode string) {
			publishRefusal(cause)

			elapsed, got := deploy()

			Expect(got).To(Equal(errorCode))
			Expect(elapsed).To(BeNumerically("<", awaitTimeout))
			Expect(sentryEvents.eventNames()).To(BeEmpty(), "a degraded resource is not a bug")
		},
		Entry("CPU", bridgeadmission.CPU, models.ErrBridgeRefusedCPU),
		Entry("memory", bridgeadmission.Memory, models.ErrBridgeRefusedMemory),
		Entry("disk", bridgeadmission.Disk, models.ErrBridgeRefusedDisk),
	)

	DescribeTable("waits for the timeout when the refusal names no resource",
		func(cause bridgeadmission.Cause) {
			publishRefusal(cause)

			elapsed, got := deploy()

			Expect(got).To(Equal(models.ErrDeployTimeout))
			Expect(elapsed).To(BeNumerically(">=", awaitTimeout))
		},
		Entry("the bridge limit", bridgeadmission.BridgeLimit),
		Entry("resource health not proven yet", bridgeadmission.NotProven),
	)
})
