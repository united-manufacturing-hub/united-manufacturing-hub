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

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthosfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	dfcfsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/protocolconverter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	benthossvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
	dfcsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
	protocolconvertersvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/protocolconverter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

var _ = Describe("DeployProtocolConverter with a benthos config error", func() {
	const (
		bridgeName   = "historian_pump"
		awaitTimeout = 3 * time.Second
	)

	var (
		mockCfg  *config.MockConfigManager
		snapMgr  *fsm.SnapshotManager
		outbound chan *models.UMHMessage
		msgs     []*models.UMHMessage
		mu       sync.Mutex
	)

	writeFlowPayload := map[string]any{
		"name":       bridgeName,
		"connection": map[string]any{"ip": "pgbouncer", "port": uint32(5432)},
		"location":   map[int]string{0: "enterprise"},
		"writeDFC": map[string]any{
			"destination": map[string]any{"protocol": "historian"},
			"source":      map[string]any{"topics": "umh.v1.enterprise.*"},
		},
	}

	publishWriteFlowLogs := func(logLines ...string) {
		logs := make([]s6.LogEntry, 0, len(logLines))
		for _, line := range logLines {
			logs = append(logs, s6.LogEntry{Content: line})
		}

		snapMgr.UpdateSnapshot(&fsm.SystemSnapshot{
			Managers: map[string]fsm.ManagerSnapshot{
				constants.ProtocolConverterManagerName: &actions.MockManagerSnapshot{
					Instances: map[string]*fsm.FSMInstanceSnapshot{
						bridgeName: {
							ID:           bridgeName,
							CurrentState: protocolconverter.OperationalStateStartingDFC,
							DesiredState: protocolconverter.OperationalStateActive,
							LastObservedState: &protocolconverter.ProtocolConverterObservedStateSnapshot{
								ServiceInfo: protocolconvertersvc.ServiceInfo{
									DataflowComponentWriteFSMState: dfcfsm.OperationalStateStarting,
									DataflowComponentWriteObservedState: dfcfsm.DataflowComponentObservedState{
										ServiceInfo: dfcsvc.ServiceInfo{
											BenthosObservedState: benthosfsm.BenthosObservedState{
												ServiceInfo: benthossvc.ServiceInfo{
													BenthosStatus: benthossvc.BenthosStatus{BenthosLogs: logs},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		})
	}

	deploy := func() (time.Duration, error) {
		action := actions.NewDeployProtocolConverterAction("probe@example.com", uuid.New(), uuid.New(), outbound, mockCfg, snapMgr)
		action.SetTickInterval(100 * time.Millisecond)
		action.SetAwaitTimeout(awaitTimeout)
		Expect(action.Parse(writeFlowPayload)).To(Succeed())
		Expect(action.Validate()).To(Succeed())

		start := time.Now()
		_, _, err := action.Execute()

		return time.Since(start), err
	}

	replies := func() []string {
		previousCount := -1

		Eventually(func() bool {
			mu.Lock()
			count := len(msgs)
			mu.Unlock()

			settled := count == previousCount
			previousCount = count

			return settled
		}, "3s", "50ms").Should(BeTrue())

		mu.Lock()
		defer mu.Unlock()

		decoded := make([]string, 0, len(msgs))
		for _, message := range msgs {
			decodedMessage, decodeErr := encoding.DecodeMessageFromUMHInstanceToUser(message.Content)
			if decodeErr != nil {
				continue
			}

			decoded = append(decoded, fmt.Sprintf("%v", decodedMessage.Payload))
		}

		return decoded
	}

	writeFlowDesiredState := func() string {
		Expect(mockCfg.Config.ProtocolConverter).To(HaveLen(1))

		return mockCfg.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.WriteDFCDesiredState
	}

	BeforeEach(func() {
		outbound = make(chan *models.UMHMessage, 200)

		mu.Lock()
		msgs = nil
		mu.Unlock()

		mockCfg = config.NewMockConfigManager().WithConfig(config.FullConfig{
			Agent: config.AgentConfig{MetricsPort: 8080},
		})
		snapMgr = fsm.NewSnapshotManager()

		go actions.ConsumeOutboundMessages(outbound, &msgs, &mu, false)
	})

	AfterEach(func() { close(outbound) })

	It("fails before the timeout with the lint message", func() {
		publishWriteFlowLogs(startupLine, lintErrorLine)

		elapsed, err := deploy()

		Expect(err).To(MatchError(ContainSubstring(lintErrorText)))
		Expect(elapsed).To(BeNumerically("<", awaitTimeout))
		Expect(replies()).To(ContainElement(ContainSubstring(models.ErrConfigFileInvalid)))
	})

	It("shows the benthos log lines in the action output", func() {
		publishWriteFlowLogs(startupLine, lintErrorLine)

		_, _ = deploy()

		Expect(replies()).To(ContainElement(ContainSubstring("received log line: " + startupLine)))
	})

	It("keeps the bridge and stops the failing write flow", func() {
		publishWriteFlowLogs(lintErrorLine)

		_, _ = deploy()

		Expect(writeFlowDesiredState()).To(Equal(dfcfsm.OperationalStateStopped))
	})

	It("waits for the timeout when the logs hold no config error", func() {
		publishWriteFlowLogs(startupLine, constructorErrorLine)

		elapsed, err := deploy()

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).NotTo(ContainSubstring(lintErrorText))
		Expect(elapsed).To(BeNumerically(">=", awaitTimeout))
	})
})
