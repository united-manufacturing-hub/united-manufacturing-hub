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
	"errors"
	"sync"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

// SaveProtocolConverter tests verify the behavior of the SaveProtocolConverterAction.
// Unlike deploy, save persists the configuration without waiting for the bridge to
// reach its desired state and never rolls the config back, so the configuration
// survives a failed deployment. The save is idempotent: re-saving an existing
// converter edits it in place instead of failing with a duplicate-name error.
var _ = Describe("SaveProtocolConverter", func() {
	var (
		action          *actions.SaveProtocolConverterAction
		userEmail       string
		actionUUID      uuid.UUID
		instanceUUID    uuid.UUID
		outboundChannel chan *models.UMHMessage
		mockConfig      *config.MockConfigManager
		pcName          string
		pcIP            string
		pcPort          uint32
		pcLocation      map[int]string
		messages        []*models.UMHMessage
		mu              sync.Mutex
	)

	// validPayload returns a minimal valid save payload for the shared fixtures.
	validPayload := func() map[string]interface{} {
		return map[string]interface{}{
			"name": pcName,
			"connection": map[string]interface{}{
				"ip":   pcIP,
				"port": pcPort,
			},
			"location": pcLocation,
		}
	}

	BeforeEach(func() {
		userEmail = "test@example.com"
		actionUUID = uuid.New()
		instanceUUID = uuid.New()
		outboundChannel = make(chan *models.UMHMessage, 10)
		pcName = "test-protocol-converter"
		pcIP = "192.168.1.100"
		pcPort = uint32(502)
		pcLocation = map[int]string{
			0: "TestEnterprise",
			1: "TestSite",
		}

		initialConfig := config.FullConfig{
			Agent: config.AgentConfig{
				MetricsPort: 8080,
				CommunicatorConfig: config.CommunicatorConfig{
					APIURL:    "https://example.com",
					AuthToken: "test-token",
				},
				ReleaseChannel: config.ReleaseChannelStable,
			},
			ProtocolConverter: []config.ProtocolConverterConfig{},
			Templates:         config.TemplatesConfig{},
		}

		mockConfig = config.NewMockConfigManager().WithConfig(initialConfig)
		action = actions.NewSaveProtocolConverterAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig)

		go actions.ConsumeOutboundMessages(outboundChannel, &messages, &mu, true)
	})

	AfterEach(func() {
		close(outboundChannel)
	})

	Describe("Parse", func() {
		It("should parse valid protocol converter payload", func() {
			err := action.Parse(validPayload())
			Expect(err).NotTo(HaveOccurred())

			parsedPayload := action.GetParsedPayload()
			Expect(parsedPayload.Name).To(Equal(pcName))
			Expect(parsedPayload.Connection.IP).To(Equal(pcIP))
			Expect(parsedPayload.Connection.Port).To(Equal(pcPort))
			Expect(parsedPayload.Location).To(Equal(pcLocation))
		})

		It("should return error for invalid payload format", func() {
			err := action.Parse("invalid-payload")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse payload"))
		})
	})

	Describe("Validate", func() {
		It("should pass validation with valid protocol converter configuration", func() {
			Expect(action.Parse(validPayload())).To(Succeed())
			Expect(action.Validate()).To(Succeed())
		})

		It("should fail validation with missing name", func() {
			payload := validPayload()
			payload["name"] = ""

			Expect(action.Parse(payload)).To(Succeed())
			err := action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing required field Name"))
		})

		It("should fail validation with missing connection IP", func() {
			payload := validPayload()
			payload["connection"] = map[string]interface{}{"port": pcPort}

			Expect(action.Parse(payload)).To(Succeed())
			err := action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing required field Connection.IP"))
		})
	})

	Describe("Execute", func() {
		It("should add a new protocol converter when none exists", func() {
			Expect(action.Parse(validPayload())).To(Succeed())
			mockConfig.ResetCalls()

			result, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata).To(BeNil())

			// New name -> add path, not edit.
			Expect(mockConfig.AtomicAddProtocolConverterCalled).To(BeTrue())
			Expect(mockConfig.AtomicEditProtocolConverterCalled).To(BeFalse())

			Expect(mockConfig.Config.ProtocolConverter).To(HaveLen(1))
			Expect(mockConfig.Config.ProtocolConverter[0].Name).To(Equal(pcName))

			responsePC, ok := result.(models.ProtocolConverter)
			Expect(ok).To(BeTrue(), "Result should be a ProtocolConverter")
			expectedUUID := dataflowcomponentserviceconfig.GenerateUUIDFromName(pcName)
			Expect(responsePC.UUID).NotTo(BeNil())
			Expect(*responsePC.UUID).To(Equal(expectedUUID))
			Expect(responsePC.Name).To(Equal(pcName))
			Expect(responsePC.Connection.IP).To(Equal(pcIP))
			Expect(responsePC.Connection.Port).To(Equal(pcPort))
			Expect(responsePC.Location).To(Equal(pcLocation))
		})

		It("should edit in place when a converter with the same name already exists", func() {
			// Idempotency: same name is the same logical bridge -> edit, not duplicate.
			mockConfig.Config.ProtocolConverter = []config.ProtocolConverterConfig{
				{
					FSMInstanceConfig: config.FSMInstanceConfig{Name: pcName},
				},
			}

			Expect(action.Parse(validPayload())).To(Succeed())
			mockConfig.ResetCalls()

			result, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata).To(BeNil())

			Expect(mockConfig.AtomicEditProtocolConverterCalled).To(BeTrue())
			Expect(mockConfig.AtomicAddProtocolConverterCalled).To(BeFalse())

			responsePC, ok := result.(models.ProtocolConverter)
			Expect(ok).To(BeTrue())
			Expect(responsePC.Name).To(Equal(pcName))
		})

		It("should fail when adding the protocol converter fails", func() {
			mockConfig.WithAtomicAddProtocolConverterError(errors.New("mock add protocol converter failure"))

			Expect(action.Parse(validPayload())).To(Succeed())

			result, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to save protocol converter"))
			Expect(err.Error()).To(ContainSubstring("mock add protocol converter failure"))
			Expect(result).To(BeNil())
			Expect(metadata).To(BeNil())
		})

		It("should fail when editing the existing protocol converter fails", func() {
			mockConfig.Config.ProtocolConverter = []config.ProtocolConverterConfig{
				{
					FSMInstanceConfig: config.FSMInstanceConfig{Name: pcName},
				},
			}
			mockConfig.WithAtomicEditProtocolConverterError(errors.New("mock edit protocol converter failure"))

			Expect(action.Parse(validPayload())).To(Succeed())

			result, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to save protocol converter"))
			Expect(err.Error()).To(ContainSubstring("mock edit protocol converter failure"))
			Expect(result).To(BeNil())
			Expect(metadata).To(BeNil())
		})
	})
})
