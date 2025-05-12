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
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthosfsmmanager "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	dfc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	benthos "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
	benthos_monitor "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos_monitor"
	dfcsvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

// These structs mock the necessary types from benthos_monitor package
// They don't need to be complete, just enough to satisfy our test requirements

type MockLatencyMetrics struct {
	P50   float64
	P90   float64
	P99   float64
	Sum   float64
	Count int64
}

type MockInputMetrics struct {
	ConnectionFailed int64
	ConnectionLost   int64
	ConnectionUp     int64
	Received         int64
	LatencyNS        MockLatencyMetrics
}

type MockOutputMetrics struct {
	BatchSent        int64
	ConnectionFailed int64
	ConnectionLost   int64
	ConnectionUp     int64
	Error            int64
	Sent             int64
	LatencyNS        MockLatencyMetrics
}

type MockProcessorMetrics struct {
	Label         string
	Received      int64
	BatchReceived int64
	Sent          int64
	BatchSent     int64
	Error         int64
	LatencyNS     MockLatencyMetrics
}

type MockProcessMetrics struct {
	Processors map[string]MockProcessorMetrics
}

type MockMetrics struct {
	Input   MockInputMetrics
	Output  MockOutputMetrics
	Process MockProcessMetrics
}

type MockBenthosMetrics struct {
	Metrics MockMetrics
}

type MockBenthosStatus struct {
	BenthosMetrics MockBenthosMetrics
	BenthosLogs    []s6.LogEntry
}

type MockServiceInfo struct {
	BenthosStatus MockBenthosStatus
}

var _ = Describe("GetDataflowcomponentMetricsAction", func() {
	var (
		action          *actions.GetDataflowcomponentMetricsAction
		userEmail       string
		actionUUID      uuid.UUID
		instanceUUID    uuid.UUID
		outboundChannel chan *models.UMHMessage
		dfcName         string
		dfcUUID         uuid.UUID
		snapshotManager *fsm.SnapshotManager
	)

	BeforeEach(func() {
		userEmail = "test@example.com"
		actionUUID = uuid.New()
		instanceUUID = uuid.New()
		outboundChannel = make(chan *models.UMHMessage, 10)
		dfcName = "test-dfc"
		dfcUUID = dataflowcomponentserviceconfig.GenerateUUIDFromName(dfcName)
		snapshotManager = fsm.NewSnapshotManager()

		// Create mock metrics
		latencyNSInput := MockLatencyMetrics{
			P50:   50000000,
			P90:   90000000,
			P99:   99000000,
			Sum:   240000000,
			Count: 3,
		}

		inputMetrics := MockInputMetrics{
			ConnectionFailed: 1,
			ConnectionLost:   2,
			ConnectionUp:     3,
			Received:         100,
			LatencyNS:        latencyNSInput,
		}

		latencyNSOutput := MockLatencyMetrics{
			P50:   55000000,
			P90:   95000000,
			P99:   105000000,
			Sum:   255000000,
			Count: 5,
		}

		outputMetrics := MockOutputMetrics{
			BatchSent:        5,
			ConnectionFailed: 1,
			ConnectionLost:   2,
			ConnectionUp:     3,
			Error:            1,
			Sent:             95,
			LatencyNS:        latencyNSOutput,
		}

		latencyNSProc1 := MockLatencyMetrics{
			P50:   60000000,
			P90:   110000000,
			P99:   150000000,
			Sum:   320000000,
			Count: 10,
		}

		latencyNSProc2 := MockLatencyMetrics{
			P50:   45000000,
			P90:   75000000,
			P99:   95000000,
			Sum:   215000000,
			Count: 8,
		}

		proc1Metrics := MockProcessorMetrics{
			Label:         "Processor 1",
			Received:      100,
			BatchReceived: 10,
			Sent:          98,
			BatchSent:     10,
			Error:         2,
			LatencyNS:     latencyNSProc1,
		}

		proc2Metrics := MockProcessorMetrics{
			Label:         "Processor 2",
			Received:      98,
			BatchReceived: 10,
			Sent:          97,
			BatchSent:     10,
			Error:         1,
			LatencyNS:     latencyNSProc2,
		}

		processors := map[string]MockProcessorMetrics{
			"processor_1": proc1Metrics,
			"processor_2": proc2Metrics,
		}

		processMetrics := MockProcessMetrics{
			Processors: processors,
		}

		metrics := MockMetrics{
			Input:   inputMetrics,
			Output:  outputMetrics,
			Process: processMetrics,
		}

		benthosMetrics := MockBenthosMetrics{
			Metrics: metrics,
		}

		// Create mock DFC observed state with metrics
		benthosStatus := MockBenthosStatus{
			BenthosMetrics: benthosMetrics,
		}

		// Create a proper benthos.ServiceInfo using our mock data
		benthosObservedState := benthosfsmmanager.BenthosObservedState{
			ServiceInfo: benthos.ServiceInfo{
				BenthosStatus: benthos.BenthosStatus{
					BenthosMetrics: benthos_monitor.BenthosMetrics{
						Metrics: benthos_monitor.Metrics{
							Input: benthos_monitor.InputMetrics{
								ConnectionFailed: inputMetrics.ConnectionFailed,
								ConnectionLost:   inputMetrics.ConnectionLost,
								ConnectionUp:     inputMetrics.ConnectionUp,
								Received:         inputMetrics.Received,
								LatencyNS: benthos_monitor.Latency{
									P50:   latencyNSInput.P50,
									P90:   latencyNSInput.P90,
									P99:   latencyNSInput.P99,
									Sum:   latencyNSInput.Sum,
									Count: latencyNSInput.Count,
								},
							},
							Output: benthos_monitor.OutputMetrics{
								BatchSent:        outputMetrics.BatchSent,
								ConnectionFailed: outputMetrics.ConnectionFailed,
								ConnectionLost:   outputMetrics.ConnectionLost,
								ConnectionUp:     outputMetrics.ConnectionUp,
								Error:            outputMetrics.Error,
								Sent:             outputMetrics.Sent,
								LatencyNS: benthos_monitor.Latency{
									P50:   latencyNSOutput.P50,
									P90:   latencyNSOutput.P90,
									P99:   latencyNSOutput.P99,
									Sum:   latencyNSOutput.Sum,
									Count: latencyNSOutput.Count,
								},
							},
							Process: benthos_monitor.ProcessMetrics{
								Processors: map[string]benthos_monitor.ProcessorMetrics{
									"processor_1": {
										Label:         proc1Metrics.Label,
										Received:      proc1Metrics.Received,
										BatchReceived: proc1Metrics.BatchReceived,
										Sent:          proc1Metrics.Sent,
										BatchSent:     proc1Metrics.BatchSent,
										Error:         proc1Metrics.Error,
										LatencyNS: benthos_monitor.Latency{
											P50:   latencyNSProc1.P50,
											P90:   latencyNSProc1.P90,
											P99:   latencyNSProc1.P99,
											Sum:   latencyNSProc1.Sum,
											Count: latencyNSProc1.Count,
										},
									},
									"processor_2": {
										Label:         proc2Metrics.Label,
										Received:      proc2Metrics.Received,
										BatchReceived: proc2Metrics.BatchReceived,
										Sent:          proc2Metrics.Sent,
										BatchSent:     proc2Metrics.BatchSent,
										Error:         proc2Metrics.Error,
										LatencyNS: benthos_monitor.Latency{
											P50:   latencyNSProc2.P50,
											P90:   latencyNSProc2.P90,
											P99:   latencyNSProc2.P99,
											Sum:   latencyNSProc2.Sum,
											Count: latencyNSProc2.Count,
										},
									},
								},
							},
						},
					},
					BenthosLogs: benthosStatus.BenthosLogs,
				},
			},
		}

		dfcServiceInfo := dfcsvc.ServiceInfo{
			BenthosObservedState: benthosObservedState,
		}

		dfcObservedState := &dfc.DataflowComponentObservedStateSnapshot{
			ServiceInfo: dfcServiceInfo,
		}

		// Mock DFC instance
		mockDfcInstances := map[string]*fsm.FSMInstanceSnapshot{
			dfcName: {
				ID:                dfcName,
				CurrentState:      "active",
				LastObservedState: dfcObservedState,
			},
		}

		dfcManagerSnapshot := &actions.MockManagerSnapshot{Instances: mockDfcInstances}

		// Update snapshot manager with mocked DFC instance
		snapshotManager.UpdateSnapshot(&fsm.SystemSnapshot{
			Managers: map[string]fsm.ManagerSnapshot{
				constants.DataflowcomponentManagerName: dfcManagerSnapshot,
			},
		})

		// Create the action
		action = actions.NewGetDataflowcomponentMetricsAction(userEmail, actionUUID, instanceUUID, outboundChannel, snapshotManager)
	})

	AfterEach(func() {
		// Drain the outbound channel to prevent goroutine leaks
		for len(outboundChannel) > 0 {
			<-outboundChannel
		}
		close(outboundChannel)
	})

	Describe("Parse", func() {
		It("should parse valid payload with UUID", func() {
			payload := map[string]interface{}{
				"uuid": dfcUUID.String(),
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(action.GetParsedPayload().UUID).To(Equal(dfcUUID.String()))
		})

		It("should return an error for invalid payload", func() {
			payload := map[string]interface{}{
				"uuid": 123, // Invalid type, should be string
			}

			err := action.Parse(payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("error unmarshaling into target type"))
		})
	})

	Describe("Validate", func() {
		It("should validate a valid payload", func() {
			payload := map[string]interface{}{
				"uuid": dfcUUID.String(),
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return an error if the UUID is missing", func() {
			payload := map[string]interface{}{}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("uuid must be set"))
		})

		It("should return an error if the UUID is invalid", func() {
			payload := map[string]interface{}{
				"uuid": "invalid-uuid",
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid UUID format"))
		})
	})

	Describe("Execute", func() {
		It("should return metrics for a valid DFC UUID", func() {
			payload := map[string]interface{}{
				"uuid": dfcUUID.String(),
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).NotTo(HaveOccurred())

			result, _, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())

			// Verify the structure and contents of the result
			metricsResult, ok := result.(actions.DfcMetrics)
			Expect(ok).To(BeTrue())

			// Check number of metrics (9 for input, 11 for output, 11*2=22 for processors)
			Expect(metricsResult.Metrics).To(HaveLen(9 + 11 + 22))

			// Check some specific metrics values
			var inputReceivedMetric *actions.DfcMetric
			var outputSentMetric *actions.DfcMetric
			var processor1LabelMetric *actions.DfcMetric

			for _, metric := range metricsResult.Metrics {
				if metric.Location == actions.DfcMetricLocationInput &&
					metric.Path == "messages" &&
					metric.Name == "received" {
					inputReceivedMetric = &metric
				}
				if metric.Location == actions.DfcMetricLocationOutput &&
					metric.Path == "messages" &&
					metric.Name == "sent" {
					outputSentMetric = &metric
				}
				if metric.Location == actions.DfcMetricLocationProcessing &&
					metric.Path == "processor_1" &&
					metric.Name == "label" {
					processor1LabelMetric = &metric
				}
			}

			Expect(inputReceivedMetric).NotTo(BeNil())
			Expect(inputReceivedMetric.Value).To(Equal(int64(100)))
			Expect(inputReceivedMetric.ValueType).To(Equal(actions.DfcMetricTypeNumber))

			Expect(outputSentMetric).NotTo(BeNil())
			Expect(outputSentMetric.Value).To(Equal(int64(95)))
			Expect(outputSentMetric.ValueType).To(Equal(actions.DfcMetricTypeNumber))

			Expect(processor1LabelMetric).NotTo(BeNil())
			Expect(processor1LabelMetric.Value).To(Equal("Processor 1"))
			Expect(processor1LabelMetric.ValueType).To(Equal(actions.DfcMetricTypeString))
		})

		It("should return an error when the DFC instance is not found", func() {
			// Use a non-existent UUID
			nonExistentUUID := uuid.New().String()
			payload := map[string]interface{}{
				"uuid": nonExistentUUID,
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).NotTo(HaveOccurred())

			result, _, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("was not found"))
			Expect(result).To(BeNil())
		})

		It("should handle the case when no dataflowcomponent manager exists", func() {
			// Create a new system snapshot without a dataflowcomponent manager
			emptySnapshot := &fsm.SystemSnapshot{
				Managers: map[string]fsm.ManagerSnapshot{},
			}
			emptySnapshotManager := fsm.NewSnapshotManager()
			emptySnapshotManager.UpdateSnapshot(emptySnapshot)

			// Create action with empty snapshot
			emptyAction := actions.NewGetDataflowcomponentMetricsAction(
				userEmail,
				actionUUID,
				instanceUUID,
				outboundChannel,
				emptySnapshotManager,
			)

			payload := map[string]interface{}{
				"uuid": dfcUUID.String(),
			}

			err := emptyAction.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = emptyAction.Validate()
			Expect(err).NotTo(HaveOccurred())

			result, _, err := emptyAction.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("dfc manager not found"))
			Expect(result).To(BeNil())
		})
	})
})
