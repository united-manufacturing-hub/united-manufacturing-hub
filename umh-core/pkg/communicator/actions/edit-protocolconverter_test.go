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
	"context"
	"errors"
	"sync"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/actions"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/variables"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
)

var _ = Describe("EditProtocolConverter", func() {
	// Variables used across tests
	var (
		action          *actions.EditProtocolConverterAction
		userEmail       string
		actionUUID      uuid.UUID
		instanceUUID    uuid.UUID
		outboundChannel chan *models.UMHMessage
		mockConfig      *config.MockConfigManager
		stateMocker     *actions.StateMocker
		messages        []*models.UMHMessage
		pcName          string
		pcUUID          uuid.UUID
		mu              sync.Mutex
	)

	// Setup before each test
	BeforeEach(func() {
		// Initialize test variables
		userEmail = "test@example.com"
		actionUUID = uuid.New()
		instanceUUID = uuid.New()
		outboundChannel = make(chan *models.UMHMessage, 10) // Buffer to prevent blocking
		pcName = "wetter"
		pcUUID = dataflowcomponentserviceconfig.GenerateUUIDFromName(pcName)

		// Create initial config with a basic protocol converter (like the wetter in config.yaml)
		initialConfig := config.FullConfig{
			Agent: config.AgentConfig{
				MetricsPort: 8080,
				CommunicatorConfig: config.CommunicatorConfig{
					APIURL:    "https://example.com",
					AuthToken: "test-token",
				},
				ReleaseChannel: config.ReleaseChannelStable,
				Location: map[int]string{
					0: "test-enterprise",
					1: "test-site",
					2: "test-area",
					3: "test-line",
				},
			},
			ProtocolConverter: []config.ProtocolConverterConfig{
				{
					FSMInstanceConfig: config.FSMInstanceConfig{
						Name:            pcName,
						DesiredFSMState: "active",
					},
					ProtocolConverterServiceConfig: protocolconverterserviceconfig.ProtocolConverterServiceConfigSpec{
						Config: protocolconverterserviceconfig.ProtocolConverterServiceConfigTemplate{
							ConnectionServiceConfig: connectionserviceconfig.ConnectionServiceConfigTemplate{
								NmapTemplate: &connectionserviceconfig.NmapConfigTemplate{
									Target: "{{ .IP }}",
									Port:   "{{ .PORT }}",
								},
							},
						},
						Variables: variables.VariableBundle{
							User: map[string]interface{}{
								"IP":   "wttr.in",
								"PORT": "80",
							},
						},
						Location: map[string]string{
							"0": "test-enterprise",
							"1": "test-site",
							"2": "test-area",
							"3": "test-line",
						},
					},
				},
			},
		}

		mockConfig = config.NewMockConfigManager().WithConfig(initialConfig)

		// Setup the state mocker and get the mock snapshot
		stateMocker = actions.NewStateMocker(mockConfig)
		stateMocker.Tick()

		action = actions.NewEditProtocolConverterAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, nil)

		go actions.ConsumeOutboundMessages(outboundChannel, &messages, &mu, true)
	})

	// Cleanup after each test
	AfterEach(func() {
		// Drain the outbound channel to prevent goroutine leaks
		for len(outboundChannel) > 0 {
			<-outboundChannel
		}
		close(outboundChannel)
	})

	Describe("Parse", func() {
		It("should parse valid read DFC payload", func() {
			payload := map[string]interface{}{
				"name": pcName,
				"connection": map[string]interface{}{
					"ip":   "wttr.in",
					"port": 80,
				},
				"location": map[string]interface{}{
					"0": "test-enterprise",
					"1": "test-site",
					"2": "test-area",
					"3": "test-line",
				},
				"uuid": pcUUID.String(),
				"readDFC": map[string]interface{}{
					"inputs": map[string]interface{}{
						"data": "input:\n  http_client:\n    url: null\n    verb: GET\n    headers: {}\n    timeout: 5s\n    retry_period: 1s\n    rate_limit: http_limiter",
						"type": "http_client",
					},
					"pipeline": map[string]interface{}{
						"processors": map[string]interface{}{
							"0": map[string]interface{}{
								"type": "bloblang",
								"data": "bloblang: |-\n  # these variables are autogenerated from your instance location\n  let enterprise = \"test-enterprise\"\n  let site = \"test-site\"\n  let area = \"test-area\"\n  let line = \"test-line\"\n  let schema = \"_historian\"\n\n  let payloadTagName = \"EUR_BTC\"\n  let value = this.bpi.EUR.rate_float\n  let tagname = \"machineState\"\n  let tagtype = \"raw\"\n\n  # the rest from here on will be autogenerated, see also documentation",
							},
						},
					},
					"rawYAML": map[string]interface{}{
						"data": "rate_limit_resources:\n  - label: http_limiter\n    local:\n      count: 1\n      interval: 1s",
					},
					"ignoreErrors": false,
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Verify parsed values
			Expect(action.GetProtocolConverterUUID()).To(Equal(pcUUID))
			Expect(action.GetDFCType()).To(Equal("read"))

			parsedPayload := action.GetParsedPayload()
			Expect(parsedPayload.Inputs.Type).To(Equal("http_client"))
			Expect(parsedPayload.Pipeline).To(HaveKey("0"))
			Expect(parsedPayload.IgnoreErrors).To(BeFalse())
		})

		It("should parse valid write DFC payload", func() {
			payload := map[string]interface{}{
				"name": pcName,
				"uuid": pcUUID.String(),
				"writeDFC": map[string]interface{}{
					"inputs": map[string]interface{}{
						"data": "input:\n  kafka:\n    addresses: [localhost:9092]\n    topics: [test]",
						"type": "kafka",
					},
					"pipeline": map[string]interface{}{
						"processors": map[string]interface{}{
							"0": map[string]interface{}{
								"type": "bloblang",
								"data": "bloblang: |-\n  root = content()",
							},
						},
					},
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Verify parsed values
			Expect(action.GetProtocolConverterUUID()).To(Equal(pcUUID))
			Expect(action.GetDFCType()).To(Equal("write"))

			parsedPayload := action.GetParsedPayload()
			Expect(parsedPayload.Inputs.Type).To(Equal("kafka"))
		})

		It("should return error for missing UUID", func() {
			payload := map[string]interface{}{
				"name": pcName,
				"readDFC": map[string]interface{}{
					"inputs": map[string]interface{}{
						"data": "input: something",
						"type": "http_client",
					},
					"pipeline": map[string]interface{}{
						"processors": map[string]interface{}{},
					},
				},
			}

			err := action.Parse(payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing required field UUID"))
		})

		It("should return error for invalid payload format", func() {
			// Invalid payload that cannot be parsed as ProtocolConverter
			payload := "invalid string payload"

			err := action.Parse(payload)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse protocol converter payload"))
		})

		It("should default state to 'active' when not provided", func() {
			payload := map[string]interface{}{
				"name": pcName,
				"connection": map[string]interface{}{
					"ip":   "wttr.in",
					"port": 80,
				},
				"uuid": pcUUID.String(),
				// No state field provided
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Verify that state defaults to "active"
			Expect(action.GetState()).To(Equal("active"))
		})

		It("should preserve explicit 'stopped' state", func() {
			payload := map[string]interface{}{
				"name": pcName,
				"connection": map[string]interface{}{
					"ip":   "wttr.in",
					"port": 80,
				},
				"uuid":  pcUUID.String(),
				"state": "stopped",
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Verify that state is preserved as "stopped"
			Expect(action.GetState()).To(Equal("stopped"))
		})
	})

	Describe("Validate", func() {
		It("should validate valid read DFC configuration", func() {
			payload := map[string]interface{}{
				"name": pcName,
				"uuid": pcUUID.String(),
				"readDFC": map[string]interface{}{
					"inputs": map[string]interface{}{
						"data": "input:\n  http_client:\n    url: 'http://example.com'",
						"type": "http_client",
					},
					"pipeline": map[string]interface{}{
						"processors": map[string]interface{}{
							"0": map[string]interface{}{
								"type": "bloblang",
								"data": "bloblang: |-\n  root = content()",
							},
						},
					},
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error for invalid UUID", func() {
			// Use action with empty UUID
			action = actions.NewEditProtocolConverterAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, nil)

			err := action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing or invalid protocol converter UUID"))
		})

		It("should return error for invalid DFC configuration", func() {
			payload := map[string]interface{}{
				"name": pcName,
				"uuid": pcUUID.String(),
				"readDFC": map[string]interface{}{
					"inputs": map[string]interface{}{
						"data": "", // Empty data
						"type": "", // Empty type
					},
					"pipeline": map[string]interface{}{
						"processors": map[string]interface{}{},
					},
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid dataflow component configuration"))
		})
	})

	Describe("Execute", func() {
		It("should successfully add read DFC to empty protocol converter", func() {
			// Parse the payload
			payload := map[string]interface{}{
				"name": pcName,
				"connection": map[string]interface{}{
					"ip":   "wttr.in",
					"port": 80,
				},
				"location": map[string]interface{}{
					"0": "test-enterprise",
					"1": "test-site",
					"2": "test-area",
					"3": "test-line",
				},
				"uuid": pcUUID.String(),
				"readDFC": map[string]interface{}{
					"inputs": map[string]interface{}{
						"data": "input:\n  http_client:\n    url: null\n    verb: GET\n    headers: {}\n    timeout: 5s\n    retry_period: 1s\n    rate_limit: http_limiter",
						"type": "http_client",
					},
					"pipeline": map[string]interface{}{
						"processors": map[string]interface{}{
							"0": map[string]interface{}{
								"type": "bloblang",
								"data": "bloblang: |-\n  # these variables are autogenerated from your instance location\n  let enterprise = \"test-enterprise\"\n  let site = \"test-site\"\n  let area = \"test-area\"\n  let line = \"test-line\"\n  let schema = \"_historian\"\n\n  let payloadTagName = \"EUR_BTC\"\n  let value = this.bpi.EUR.rate_float\n  let tagname = \"machineState\"\n  let tagtype = \"raw\"\n\n  # the rest from here on will be autogenerated, see also documentation",
							},
						},
					},
					"rawYAML": map[string]interface{}{
						"data": "rate_limit_resources:\n  - label: http_limiter\n    local:\n      count: 1\n      interval: 1s",
					},
					"ignoreErrors": false,
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).NotTo(HaveOccurred())

			// Start the state mocker
			err = stateMocker.Start()
			Expect(err).NotTo(HaveOccurred())

			// Execute the action
			_, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata).To(BeNil())

			// Stop the state mocker
			stateMocker.Stop()

			// Verify that the config was actually updated
			ctx := context.Background()
			updatedConfig, err := mockConfig.GetConfig(ctx, 0)
			Expect(err).NotTo(HaveOccurred())

			// Find the updated protocol converter
			var updatedPC *config.ProtocolConverterConfig
			for i, pc := range updatedConfig.ProtocolConverter {
				if pc.Name == pcName {
					updatedPC = &updatedConfig.ProtocolConverter[i]

					break
				}
			}
			Expect(updatedPC).NotTo(BeNil(), "Protocol converter should exist in updated config")

			// Verify the read DFC was added to the protocol converter configuration
			readDFCConfig := updatedPC.ProtocolConverterServiceConfig.Config.DataflowComponentReadServiceConfig
			Expect(readDFCConfig.BenthosConfig.Input).NotTo(BeEmpty())
			Expect(readDFCConfig.BenthosConfig.Input["input"]).To(HaveKey("http_client"))
			Expect(readDFCConfig.BenthosConfig.Pipeline).NotTo(BeEmpty())

			// Verify write DFC is still empty
			writeDFCConfig := updatedPC.ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig
			Expect(writeDFCConfig.BenthosConfig.Input).To(BeEmpty())

			// verify that the templateRef is set
			Expect(updatedPC.ProtocolConverterServiceConfig.TemplateRef).To(Equal(pcName))
		})

		It("should handle protocol converter not found error", func() {
			// Use a non-existent UUID
			nonExistentUUID := uuid.New()
			payload := map[string]interface{}{
				"name": "non-existent",
				"uuid": nonExistentUUID.String(),
				"readDFC": map[string]interface{}{
					"inputs": map[string]interface{}{
						"data": "input:\n  http_client:\n    url: http://example.com",
						"type": "http_client",
					},
					"pipeline": map[string]interface{}{
						"processors": map[string]interface{}{
							"0": map[string]interface{}{
								"type": "bloblang",
								"data": "bloblang: |-\n  root = content()",
							},
						},
					},
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Execute the action - should fail with protocol converter not found
			_, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("protocol converter with UUID"))
			Expect(err.Error()).To(ContainSubstring("not found"))
			Expect(metadata).To(BeNil())
		})

		It("should handle AtomicEditProtocolConverter failure", func() {
			// Set up mock to fail on AtomicEditProtocolConverter
			mockConfig.WithAtomicEditProtocolConverterError(errors.New("mock edit protocol converter failure"))

			payload := map[string]interface{}{
				"name": pcName,
				"uuid": pcUUID.String(),
				"readDFC": map[string]interface{}{
					"inputs": map[string]interface{}{
						"data": "input:\n  http_client:\n    url: http://example.com",
						"type": "http_client",
					},
					"pipeline": map[string]interface{}{
						"processors": map[string]interface{}{
							"0": map[string]interface{}{
								"type": "bloblang",
								"data": "bloblang: |-\n  root = content()",
							},
						},
					},
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Execute the action - should fail
			_, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to update protocol converter: mock edit protocol converter failure"))
			Expect(metadata).To(BeNil())
		})

		It("should handle config manager get config failure", func() {
			// Set up mock to fail on GetConfig
			mockConfig.WithConfigError(errors.New("mock get config failure"))

			payload := map[string]interface{}{
				"name": pcName,
				"uuid": pcUUID.String(),
				"readDFC": map[string]interface{}{
					"inputs": map[string]interface{}{
						"data": "input:\n  http_client:\n    url: http://example.com",
						"type": "http_client",
					},
					"pipeline": map[string]interface{}{
						"processors": map[string]interface{}{
							"0": map[string]interface{}{
								"type": "bloblang",
								"data": "bloblang: |-\n  root = content()",
							},
						},
					},
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Execute the action - should fail
			_, metadata, err := action.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get current configuration: mock get config failure"))
			Expect(metadata).To(BeNil())
		})
	})
})
