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

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions/providers"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	dfc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	"go.uber.org/mock/gomock"
)

var _ = Describe("GetMetricsAction", func() {
	var (
		ctrl         *gomock.Controller
		mockProvider *providers.MockMetricsProvider

		action          *actions.GetMetricsAction
		userEmail       string
		actionUUID      uuid.UUID
		instanceUUID    uuid.UUID
		outboundChannel chan *models.UMHMessage
		dfcName         string
		dfcUUID         uuid.UUID
		snapshotManager *fsm.SnapshotManager
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockProvider = providers.NewMockMetricsProvider(ctrl)

		userEmail = "test@example.com"
		actionUUID = uuid.New()
		instanceUUID = uuid.New()
		outboundChannel = make(chan *models.UMHMessage, 10)
		dfcName = "test-dfc"
		dfcUUID = dataflowcomponentserviceconfig.GenerateUUIDFromName(dfcName)
		snapshotManager = fsm.NewSnapshotManager()

		// Add a mock DFC instance to test the error handling when the DFC is not found
		// NOTE: We could also create snapshot manager mocks with gomock, but it's not worth the effort now
		snapshotManager.UpdateSnapshot(&fsm.SystemSnapshot{
			Managers: map[string]fsm.ManagerSnapshot{
				constants.DataflowcomponentManagerName: &actions.MockManagerSnapshot{
					Instances: map[string]*fsm.FSMInstanceSnapshot{
						dfcName: {
							ID:                dfcName,
							CurrentState:      "active",
							LastObservedState: &dfc.DataflowComponentObservedStateSnapshot{},
						},
					},
				},
			},
		})

		// Set up the action with our mock provider
		action = actions.NewGetMetricsActionWithProvider(userEmail, actionUUID, instanceUUID, outboundChannel, snapshotManager, nil, mockProvider)
	})

	AfterEach(func() {
		// Drain the outbound channel to prevent goroutine leaks
		for len(outboundChannel) > 0 {
			<-outboundChannel
		}
		close(outboundChannel)
	})

	Describe("Parse", func() {
		It("should parse valid payload with UUID for DFC metrics", func() {
			payload := map[string]interface{}{
				"type": "dfc",
				"uuid": dfcUUID.String(),
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(action.GetParsedPayload().Type).To(Equal(models.DFCMetricResourceType))
			Expect(action.GetParsedPayload().UUID).To(Equal(dfcUUID.String()))
		})

		It("should parse valid payload for Redpanda metrics", func() {
			payload := map[string]interface{}{
				"type": "redpanda",
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(action.GetParsedPayload().Type).To(Equal(models.RedpandaMetricResourceType))
		})
	})

	Describe("Validate", func() {
		It("should validate a valid payload", func() {
			payload := map[string]interface{}{"type": "redpanda"}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return an error if the metrics type is invalid", func() {
			payload := map[string]interface{}{"type": "invalid"}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("metric type must be set and must be one of the following: dfc, redpanda"))
		})

		It("should return an error if the uuid is missing on DFC metrics type", func() {
			payload := map[string]interface{}{"type": "dfc"}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("uuid must be set to retrieve metrics for a DFC"))
		})

		It("should return an error if the uuid is invalid", func() {
			payload := map[string]interface{}{"type": "dfc", "uuid": "invalid"}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid UUID format"))
		})
	})

	Describe("Execute", func() {
		DescribeTable("should return metrics when metric type is", func(metricType models.MetricResourceType, setupMock func(), expectedMetricsCount int) {
			// Setup the mock expectations
			setupMock()

			// Prepare payload
			payload := map[string]interface{}{
				"type": metricType,
			}
			if metricType == models.DFCMetricResourceType {
				payload["uuid"] = dfcUUID.String()
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).NotTo(HaveOccurred())

			result, _, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())

			res, ok := result.(models.GetMetricsResponse)
			Expect(ok).To(BeTrue())
			Expect(len(res.Metrics)).To(Equal(expectedMetricsCount))

			// Validate that all metrics have the expected structure
			for _, metric := range res.Metrics {
				Expect(metric.Name).NotTo(BeEmpty())
				Expect(metric.Path).NotTo(BeEmpty())
				Expect(metric.ComponentType).NotTo(BeEmpty())
				Expect(metric.ValueType).NotTo(BeEmpty())
			}
		},
			Entry("dfc", models.DFCMetricResourceType, func() {
				// Mock DFC metrics response
				expectedRes := models.GetMetricsResponse{
					Metrics: []models.Metric{
						{
							Name:          "input_received",
							Path:          "dfc.input.received",
							ComponentType: "input",
							ValueType:     models.MetricValueTypeNumber,
							Value:         float64(100),
						},
						{
							Name:          "output_sent",
							Path:          "dfc.output.sent",
							ComponentType: "output",
							ValueType:     models.MetricValueTypeNumber,
							Value:         float64(95),
						},
					},
				}
				mockProvider.EXPECT().
					GetMetrics(gomock.Any(), snapshotManager).
					Return(expectedRes, nil).
					Times(1)
			}, 2),
			Entry("redpanda", models.RedpandaMetricResourceType, func() {
				// Mock Redpanda metrics response
				expectedRes := models.GetMetricsResponse{
					Metrics: []models.Metric{
						{
							Name:          "storage_disk_free_bytes",
							Path:          "redpanda.storage.disk_free_bytes",
							ComponentType: "storage",
							ValueType:     models.MetricValueTypeNumber,
							Value:         int64(1000000000),
						},
						{
							Name:          "storage_disk_free_space_alert",
							Path:          "redpanda.storage.disk_free_space_alert",
							ComponentType: "storage",
							ValueType:     models.MetricValueTypeBoolean,
							Value:         false,
						},
						{
							Name:          "request_bytes_total",
							Path:          "redpanda.kafka.request_bytes_total",
							ComponentType: "kafka",
							ValueType:     models.MetricValueTypeNumber,
							Value:         int64(500000),
						},
					},
				}
				mockProvider.EXPECT().
					GetMetrics(gomock.Any(), snapshotManager).
					Return(expectedRes, nil).
					Times(1)
			}, 3))

		It("should handle metrics provider errors gracefully", func() {
			// Setup mock provider to return an error
			mockProvider.EXPECT().
				GetMetrics(gomock.Any(), snapshotManager).
				Return(models.GetMetricsResponse{}, fmt.Errorf("failed to get metrics")).
				Times(1)

			payload := map[string]interface{}{
				"type": models.RedpandaMetricResourceType,
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).NotTo(HaveOccurred())

			result, _, err := action.Execute()
			Expect(result).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get metrics"))
		})

		DescribeTable("should handle missing FSM instance gracefully:", func(metricType models.MetricResourceType) {
			// Use REAL action with internal provider to test the error handling
			emptySnapshotManager := fsm.NewSnapshotManager()
			action = actions.NewGetMetricsAction(userEmail, actionUUID, instanceUUID, outboundChannel, emptySnapshotManager, nil)

			payload := map[string]interface{}{
				"type": metricType,
			}
			if metricType == models.DFCMetricResourceType {
				payload["uuid"] = dfcUUID.String()
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).NotTo(HaveOccurred())

			result, _, err := action.Execute()
			Expect(result).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		},
			Entry("dfc", models.DFCMetricResourceType),
			Entry("redpanda", models.RedpandaMetricResourceType),
		)

		It("should return an error when a non-existent DFC UUID is provided", func() {
			// Use REAL action with internal provider to test the error handling
			action = actions.NewGetMetricsAction(userEmail, actionUUID, instanceUUID, outboundChannel, snapshotManager, nil)

			payload := map[string]interface{}{
				"type": models.DFCMetricResourceType,
				"uuid": uuid.New().String(),
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).NotTo(HaveOccurred())

			result, _, err := action.Execute()
			Expect(result).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("the requested DFC with UUID %s was not found", payload["uuid"]))
		})
	})
})
