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
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	benthosfsmmanager "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/benthos"
	dfc_fsm "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	benthossvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/benthos"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/dataflowcomponent"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/s6"
)

var _ = Describe("GetLogsAction", func() {
	var (
		action          *actions.GetLogsAction
		userEmail       string
		actionUUID      uuid.UUID
		instanceUUID    uuid.UUID
		outboundChannel chan *models.UMHMessage
		dfcName         string
		dfcUUID         uuid.UUID
		messages        []*models.UMHMessage
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

		mockedLogs := []s6.LogEntry{
			{
				Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Content:   "test log",
			},
			{
				Timestamp: time.Date(2024, 1, 1, 0, 0, 1, 0, time.UTC),
				Content:   "test log 2",
			},
		}

		dfcServiceInfo := dataflowcomponent.ServiceInfo{
			BenthosObservedState: benthosfsmmanager.BenthosObservedState{
				ServiceInfo: benthossvc.ServiceInfo{
					BenthosStatus: benthossvc.BenthosStatus{
						BenthosLogs: mockedLogs,
					},
				},
			},
		}

		dfcMockedObservedState := &dfc_fsm.DataflowComponentObservedStateSnapshot{
			ServiceInfo: dfcServiceInfo,
		}

		mockDfcInstances := make(map[string]*fsm.FSMInstanceSnapshot)
		mockDfcInstances[dfcUUID.String()] = &fsm.FSMInstanceSnapshot{
			ID:                dfcName,
			CurrentState:      "active",
			LastObservedState: dfcMockedObservedState,
		}

		mockSnapshot := &actions.MockManagerSnapshot{
			Instances: mockDfcInstances,
		}

		snapshotManager.UpdateSnapshot(&fsm.SystemSnapshot{
			Managers: map[string]fsm.ManagerSnapshot{
				constants.DataflowcomponentManagerName: mockSnapshot,
			},
		})

		action = actions.NewGetLogsAction(userEmail, actionUUID, instanceUUID, outboundChannel, snapshotManager)
		go actions.ConsumeOutboundMessages(outboundChannel, &messages, true)

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
		It("should return an error if the payload is missing a required field", func() {
			// Missing log type
			payload := map[string]interface{}{
				"uuid":      dfcUUID.String(),
				"startTime": time.Now().Add(-24 * time.Hour).UnixMilli(),
			}
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("log type must be set and must be one of the following: agent, dfc, redpanda, tag-browser"))

			// Missing start time
			payload = map[string]interface{}{
				"uuid": dfcUUID.String(),
				"type": models.DFCLogType,
			}
			err = action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

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
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("log type must be set and must be one of the following: agent, dfc, redpanda, tag-browser"))
		})

		It("should return an error if the uuid is missing on DFC log type", func() {
			payload := map[string]interface{}{
				"type":      models.DFCLogType,
				"startTime": time.Now().Add(-24 * time.Hour).UnixMilli(),
			}
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("uuid must be set to retrieve logs for a DFC"))
		})

		It("should return an error if the uuid is invalid", func() {
			payload := map[string]interface{}{
				"uuid":      "invalid",
				"type":      models.DFCLogType,
				"startTime": time.Now().Add(-24 * time.Hour).UnixMilli(),
			}
			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid UUID format"))
		})
	})

	// Describe("Execute", func() {})
})
