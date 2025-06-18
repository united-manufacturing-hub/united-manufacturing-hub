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

package actions_test

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	agent_monitor_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/agent_monitor"
	benthosfsmmanager "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	dfc_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	protocolconverter_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/protocolconverter"
	redpanda_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/redpanda"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/agent_monitor"
	benthossvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/protocolconverter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/redpanda"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

var _ = Describe("GetLogsAction", func() {
	var (
		action                *actions.GetLogsAction
		userEmail             string
		actionUUID            uuid.UUID
		instanceUUID          uuid.UUID
		outboundChannel       chan *models.UMHMessage
		dfcName               string
		dfcUUID               uuid.UUID
		protocolConverterName string
		protocolConverterUUID uuid.UUID
		snapshotManager       *fsm.SnapshotManager
		mockedLogs            []s6.LogEntry
	)

	BeforeEach(func() {
		userEmail = "test@example.com"
		actionUUID = uuid.New()
		instanceUUID = uuid.New()
		outboundChannel = make(chan *models.UMHMessage, 10)
		dfcName = "test-dfc"
		dfcUUID = dataflowcomponentserviceconfig.GenerateUUIDFromName(dfcName)
		protocolConverterName = "test-protocol-converter"
		protocolConverterUUID = dataflowcomponentserviceconfig.GenerateUUIDFromName(protocolConverterName)
		snapshotManager = fsm.NewSnapshotManager()

		// Mocked logs contain logs from 6h ago and 2h ago
		mockedLogs = []s6.LogEntry{
			{
				Timestamp: time.Now().Add(-6 * time.Hour).UTC(),
				Content:   "test log",
			},
			{
				Timestamp: time.Now().Add(-2 * time.Hour).UTC(),
				Content:   "test log 2",
			},
		}

		// DFC logs mock
		dfcServiceInfo := dataflowcomponent.ServiceInfo{
			BenthosObservedState: benthosfsmmanager.BenthosObservedState{
				ServiceInfo: benthossvc.ServiceInfo{
					BenthosStatus: benthossvc.BenthosStatus{
						BenthosLogs: mockedLogs,
					},
				},
			},
		}
		dfcMockedObservedState := &dfc_fsm.DataflowComponentObservedStateSnapshot{ServiceInfo: dfcServiceInfo}
		mockDfcInstances := map[string]*fsm.FSMInstanceSnapshot{
			dfcUUID.String(): {
				ID:                dfcName,
				CurrentState:      "active",
				LastObservedState: dfcMockedObservedState,
			},
		}
		dfcManagerSnapshot := &actions.MockManagerSnapshot{Instances: mockDfcInstances}

		// Protocol Converter logs mock
		protocolConverterServiceInfo := protocolconverter.ServiceInfo{
			DataflowComponentReadObservedState: dfc_fsm.DataflowComponentObservedState{
				ServiceInfo: dataflowcomponent.ServiceInfo{
					BenthosObservedState: benthosfsmmanager.BenthosObservedState{
						ServiceInfo: benthossvc.ServiceInfo{
							BenthosStatus: benthossvc.BenthosStatus{
								BenthosLogs: mockedLogs,
							},
						},
					},
				},
			},
			DataflowComponentWriteObservedState: dfc_fsm.DataflowComponentObservedState{
				ServiceInfo: dataflowcomponent.ServiceInfo{
					BenthosObservedState: benthosfsmmanager.BenthosObservedState{
						ServiceInfo: benthossvc.ServiceInfo{
							BenthosStatus: benthossvc.BenthosStatus{
								BenthosLogs: mockedLogs,
							},
						},
					},
				},
			},
		}
		protocolConverterMockedObservedState := &protocolconverter_fsm.ProtocolConverterObservedStateSnapshot{ServiceInfo: protocolConverterServiceInfo}
		mockProtocolConverterInstances := map[string]*fsm.FSMInstanceSnapshot{
			protocolConverterUUID.String(): {
				ID:                protocolConverterName,
				CurrentState:      "active",
				LastObservedState: protocolConverterMockedObservedState,
			},
		}
		protocolConverterManagerSnapshot := &actions.MockManagerSnapshot{Instances: mockProtocolConverterInstances}

		// Agent logs mock
		agentServiceInfo := agent_monitor.ServiceInfo{AgentLogs: mockedLogs}
		agentMockedObservedState := &agent_monitor_fsm.AgentObservedStateSnapshot{
			ServiceInfoSnapshot: agentServiceInfo,
		}
		mockAgentInstances := map[string]*fsm.FSMInstanceSnapshot{
			constants.AgentInstanceName: {
				ID:                constants.AgentInstanceName,
				CurrentState:      "active",
				LastObservedState: agentMockedObservedState,
			},
		}
		agentManagerSnapshot := &actions.MockManagerSnapshot{Instances: mockAgentInstances}

		// Redpanda logs mock
		redpandaServiceInfo := redpanda.ServiceInfo{
			RedpandaStatus: redpanda.RedpandaStatus{
				Logs: mockedLogs,
			},
		}
		redpandaMockedObservedState := &redpanda_fsm.RedpandaObservedStateSnapshot{ServiceInfoSnapshot: redpandaServiceInfo}
		mockRedpandaInstances := map[string]*fsm.FSMInstanceSnapshot{
			constants.RedpandaInstanceName: {
				ID:                constants.RedpandaInstanceName,
				CurrentState:      "active",
				LastObservedState: redpandaMockedObservedState,
			},
		}
		redpandaManagerSnapshot := &actions.MockManagerSnapshot{Instances: mockRedpandaInstances}

		// Update snapshot manager with all mocked instances
		snapshotManager.UpdateSnapshot(&fsm.SystemSnapshot{
			Managers: map[string]fsm.ManagerSnapshot{
				constants.DataflowcomponentManagerName: dfcManagerSnapshot,
				constants.ProtocolConverterManagerName: protocolConverterManagerSnapshot,
				constants.AgentManagerName:             agentManagerSnapshot,
				constants.RedpandaManagerName:          redpandaManagerSnapshot,
			},
		})

		mockConfig := config.NewMockConfigManager().WithConfig(config.FullConfig{})
		action = actions.NewGetLogsAction(userEmail, actionUUID, instanceUUID, outboundChannel, snapshotManager, mockConfig)
	})

	AfterEach(func() {
		close(outboundChannel)
	})

	Describe("Parse", func() {
		It("should parse valid get logs payload", func() {
			payload := map[string]interface{}{
				"uuid":      dfcUUID.String(),
				"type":      models.DFCLogType,
				"startTime": time.Now().Add(-24 * time.Hour).UnixMilli(),
			}

			err := action.Parse(payload)
			Expect(err).To(BeNil())
			Expect(action.GetPayload().UUID).To(Equal(dfcUUID.String()))
			Expect(action.GetPayload().Type).To(Equal(models.DFCLogType))
		})

		It("should return an error if the payload is invalid", func() {
			payload := map[string]interface{}{
				"uuid":      dfcUUID.String(),
				"type":      models.DFCLogType,
				"startTime": "not-a-number",
			}

			err := action.Parse(payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("error unmarshaling into target type"))
		})
	})

	Describe("Validate", func() {
		It("should validate a valid payload", func() {
			payload := map[string]interface{}{
				"uuid":      dfcUUID.String(),
				"type":      models.DFCLogType,
				"startTime": time.Now().Add(-24 * time.Hour).UnixMilli(),
			}

			err := action.Parse(payload)
			Expect(err).To(BeNil())

			err = action.Validate()
			Expect(err).To(BeNil())
		})

		It("should return an error if the payload is missing a required field", func() {
			// Missing log type
			payload := map[string]interface{}{
				"uuid":      dfcUUID.String(),
				"startTime": time.Now().Add(-24 * time.Hour).UnixMilli(),
			}
			err := action.Parse(payload)
			Expect(err).To(BeNil())

			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("log type must be set and must be one of the following: agent, dfc, protocol-converter-read, protocol-converter-write, redpanda, tag-browser"))

			// Missing start time
			payload = map[string]interface{}{
				"uuid": dfcUUID.String(),
				"type": models.DFCLogType,
			}
			err = action.Parse(payload)
			Expect(err).To(BeNil())

			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("start time must be greater than 0"))
		})

		It("should return an error if the log type is invalid", func() {
			payload := map[string]interface{}{
				"uuid":      dfcUUID.String(),
				"type":      "invalid",
				"startTime": time.Now().Add(-24 * time.Hour).UnixMilli(),
			}
			err := action.Parse(payload)
			Expect(err).To(BeNil())

			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("log type must be set and must be one of the following: agent, dfc, protocol-converter-read, protocol-converter-write, redpanda, tag-browser"))
		})

		It("should return an error if the uuid is missing on DFC log type", func() {
			payload := map[string]interface{}{
				"type":      models.DFCLogType,
				"startTime": time.Now().Add(-24 * time.Hour).UnixMilli(),
			}
			err := action.Parse(payload)
			Expect(err).To(BeNil())

			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("uuid must be set to retrieve logs for a DFC or Protocol Converter"))
		})

		It("should return an error if the uuid is missing on Protocol Converter log types", func() {
			payload := map[string]interface{}{
				"type":      models.ProtocolConverterReadLogType,
				"startTime": time.Now().Add(-24 * time.Hour).UnixMilli(),
			}
			err := action.Parse(payload)
			Expect(err).To(BeNil())

			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("uuid must be set to retrieve logs for a DFC or Protocol Converter"))

			payload = map[string]interface{}{
				"type":      models.ProtocolConverterWriteLogType,
				"startTime": time.Now().Add(-24 * time.Hour).UnixMilli(),
			}
			err = action.Parse(payload)
			Expect(err).To(BeNil())

			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("uuid must be set to retrieve logs for a DFC or Protocol Converter"))
		})

		It("should return an error if the uuid is invalid", func() {
			payload := map[string]interface{}{
				"uuid":      "invalid",
				"type":      models.DFCLogType,
				"startTime": time.Now().Add(-24 * time.Hour).UnixMilli(),
			}
			err := action.Parse(payload)
			Expect(err).To(BeNil())

			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid UUID format"))
		})
	})

	Describe("Execute", func() {
		DescribeTable("should return logs when log type is", func(logType models.LogType) {
			payload := map[string]interface{}{
				"type":      logType,
				"startTime": time.Now().Add(-24 * time.Hour).UnixMilli(),
			}

			if logType == models.DFCLogType {
				payload["uuid"] = dfcUUID.String()
			}

			if logType == models.ProtocolConverterReadLogType || logType == models.ProtocolConverterWriteLogType {
				payload["uuid"] = protocolConverterUUID.String()
			}

			err := action.Parse(payload)
			Expect(err).To(BeNil())

			err = action.Validate()
			Expect(err).To(BeNil())

			// add the time to the loglines
			expectedLogs := []string{
				fmt.Sprintf("[%s] test log", mockedLogs[0].Timestamp.Format(time.RFC3339)),
				fmt.Sprintf("[%s] test log 2", mockedLogs[1].Timestamp.Format(time.RFC3339)),
			}

			result, _, err := action.Execute()
			Expect(err).To(BeNil())
			Expect(result).To(Equal(models.GetLogsResponse{Logs: expectedLogs}))
		},
			Entry("dfc", models.DFCLogType),
			Entry("agent", models.AgentLogType),
			Entry("redpanda", models.RedpandaLogType),
			Entry("protocol-converter-read", models.ProtocolConverterReadLogType),
			Entry("protocol-converter-write", models.ProtocolConverterWriteLogType))

		It("should return logs for the given start time", func() {
			// Start time of 3h ago should only yield the mocked log from 2h ago
			payload := map[string]interface{}{
				"type":      models.AgentLogType,
				"startTime": time.Now().Add(-3 * time.Hour).UnixMilli(),
			}

			err := action.Parse(payload)
			Expect(err).To(BeNil())

			err = action.Validate()
			Expect(err).To(BeNil())

			result, _, err := action.Execute()
			Expect(err).To(BeNil())
			Expect(result).To(Equal(models.GetLogsResponse{Logs: []string{fmt.Sprintf("[%s] test log 2", mockedLogs[1].Timestamp.Format(time.RFC3339))}}))
		})

		It("should handle missing manager errors gracefully", func() {
			// Mock a log retrieval error by clearing all managers from the snapshot
			snapshotManager.UpdateSnapshot(&fsm.SystemSnapshot{})

			payload := map[string]interface{}{
				"type":      models.RedpandaLogType,
				"startTime": time.Now().Add(-24 * time.Hour).UnixMilli(),
			}

			err := action.Parse(payload)
			Expect(err).To(BeNil())

			err = action.Validate()
			Expect(err).To(BeNil())

			result, _, err := action.Execute()
			Expect(result).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to retrieve logs for redpanda: redpanda instance not found"))
		})

		It("should handle missing instance errors gracefully", func() {
			// Mock a missing instance by clearing the agent instance from the corresponding manager snapshot
			manager := snapshotManager.GetSnapshot().Managers[constants.AgentManagerName]
			manager.GetInstances()[constants.AgentInstanceName] = nil

			payload := map[string]interface{}{
				"type":      models.AgentLogType,
				"startTime": time.Now().Add(-24 * time.Hour).UnixMilli(),
			}

			err := action.Parse(payload)
			Expect(err).To(BeNil())

			err = action.Validate()
			Expect(err).To(BeNil())

			result, _, err := action.Execute()
			Expect(result).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to retrieve logs for agent: agent instance not found"))
		})
	})
})
