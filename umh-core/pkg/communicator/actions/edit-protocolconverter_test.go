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

		It("should default DFC type to empty when no DFCs provided", func() {
			payload := map[string]interface{}{
				"name": pcName,
				"connection": map[string]interface{}{
					"ip":   "wttr.in",
					"port": 80,
				},
				"uuid": pcUUID.String(),
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			Expect(action.GetDFCType()).To(Equal("empty"))
		})

		It("should parse both read and write DFCs as DFCTypeBoth", func() {
			payload := map[string]interface{}{
				"name": pcName,
				"uuid": pcUUID.String(),
				"readDFC": map[string]interface{}{
					"inputs": map[string]interface{}{
						"data": "input:\n  http_client:\n    url: 'http://example.com'",
						"type": "http_client",
					},
					"pipeline": map[string]interface{}{
						"processors": map[string]interface{}{},
					},
				},
				"writeDFC": map[string]interface{}{
					"outputs": map[string]interface{}{
						"data": "output:\n  stdout: {}",
						"type": "stdout",
					},
					"pipeline": map[string]interface{}{},
					"umh_topics": []interface{}{"umh.v1.factory.*"},
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(action.GetDFCType()).To(Equal("both"))
		})

		It("should parse write DFC with umh_topics", func() {
			payload := map[string]interface{}{
				"name": pcName,
				"uuid": pcUUID.String(),
				"writeDFC": map[string]interface{}{
					"outputs": map[string]interface{}{
						"data": "output:\n  stdout: {}",
						"type": "stdout",
					},
					"pipeline": map[string]interface{}{},
					"umh_topics": []interface{}{"umh.v1.factory.*", "umh.v1.plant.*"},
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(action.GetDFCType()).To(Equal("write"))

			parsedPayload := action.GetParsedPayload()
			Expect(parsedPayload.UMHTopics).To(ConsistOf("umh.v1.factory.*", "umh.v1.plant.*"))
		})

		It("should preserve explicit 'stopped' state on read DFC", func() {
			payload := map[string]interface{}{
				"name": pcName,
				"connection": map[string]interface{}{
					"ip":   "wttr.in",
					"port": 80,
				},
				"uuid": pcUUID.String(),
				"readDFC": map[string]interface{}{
					"inputs": map[string]interface{}{
						"data": "input:\n  http_client:\n    url: 'http://example.com'",
						"type": "yaml",
					},
					"pipeline": map[string]interface{}{},
					"state":    "stopped",
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			Expect(action.GetDFCType()).To(Equal("read"))
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

		It("should validate write DFC without explicit inputs (auto-generated)", func() {
			payload := map[string]interface{}{
				"name": pcName,
				"uuid": pcUUID.String(),
				"writeDFC": map[string]interface{}{
					"outputs": map[string]interface{}{
						"data": "output:\n  stdout: {}",
						"type": "stdout",
					},
					"pipeline": map[string]interface{}{},
					"umh_topics": []interface{}{"umh.v1.factory.*"},
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should pass validation for write DFC state-only change without umh_topics", func() {
			payload := map[string]interface{}{
				"name": pcName,
				"uuid": pcUUID.String(),
				"writeDFC": map[string]interface{}{
					"state": "stopped",
					// no outputs, no umh_topics — state-only change
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error when write DFC has no umh_topics", func() {
			payload := map[string]interface{}{
				"name": pcName,
				"uuid": pcUUID.String(),
				"writeDFC": map[string]interface{}{
					"outputs": map[string]interface{}{
						"data": "output:\n  stdout: {}",
						"type": "stdout",
					},
					"pipeline": map[string]interface{}{},
					// no umh_topics
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("umh_topics"))
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
			Expect(err.Error()).To(ContainSubstring("invalid read DFC configuration"))
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

		It("should store UMH_TOPICS in user variables when adding write DFC", func() {
			topics := []interface{}{"umh.v1.factory.line-1.*", "umh.v1.factory.line-2.*"}
			payload := map[string]interface{}{
				"name": pcName,
				"uuid": pcUUID.String(),
				"writeDFC": map[string]interface{}{
					"outputs": map[string]interface{}{
						"data": "output:\n  stdout: {}",
						"type": "stdout",
					},
					"pipeline": map[string]interface{}{},
					"umh_topics": topics,
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).NotTo(HaveOccurred())

			err = stateMocker.Start()
			Expect(err).NotTo(HaveOccurred())

			_, metadata, err := action.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata).To(BeNil())

			stateMocker.Stop()

			// Verify UMH_TOPICS stored in user variables
			ctx := context.Background()
			updatedConfig, err := mockConfig.GetConfig(ctx, 0)
			Expect(err).NotTo(HaveOccurred())

			var updatedPC *config.ProtocolConverterConfig
			for i, pc := range updatedConfig.ProtocolConverter {
				if pc.Name == pcName {
					updatedPC = &updatedConfig.ProtocolConverter[i]
					break
				}
			}
			Expect(updatedPC).NotTo(BeNil())
			Expect(updatedPC.ProtocolConverterServiceConfig.Variables.User).To(HaveKey("UMH_TOPICS"))
			Expect(updatedPC.ProtocolConverterServiceConfig.Variables.User["UMH_TOPICS"]).To(ConsistOf(
				"umh.v1.factory.line-1.*",
				"umh.v1.factory.line-2.*",
			))
		})

		Context("when protocol converter already has both DFCs configured", func() {
			// overwrite the BeforeEach config with one that has both DFCs populated
			BeforeEach(func() {
				existingReadInput := map[string]interface{}{
					"http_client": map[string]interface{}{
						"url": "http://original.example.com",
					},
				}
				existingWriteOutput := map[string]interface{}{
					"stdout": map[string]interface{}{},
				}

				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.Config.DataflowComponentReadServiceConfig = dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
					BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
						Input: existingReadInput,
					},
				}
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig = dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
					BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
						Output: existingWriteOutput,
					},
				}
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.Variables.User["UMH_TOPICS"] = []string{"umh.v1.factory.*"}

				stateMocker = actions.NewStateMocker(mockConfig)
				stateMocker.Tick()
				action = actions.NewEditProtocolConverterAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, nil)
			})

			It("should not change read DFC when only write DFC is edited", Label("write-dfc", "isolation"), func() {
				newTopics := []interface{}{"umh.v1.plant.*"}
				payload := map[string]interface{}{
					"name": pcName,
					"uuid": pcUUID.String(),
					"writeDFC": map[string]interface{}{
						"outputs": map[string]interface{}{
							"data": "output:\n  kafka:\n    addresses: [localhost:9092]",
							"type": "kafka",
						},
						"pipeline":    map[string]interface{}{},
						"umh_topics": newTopics,
					},
				}

				err := action.Parse(payload)
				Expect(err).NotTo(HaveOccurred())
				err = action.Validate()
				Expect(err).NotTo(HaveOccurred())

				err = stateMocker.Start()
				Expect(err).NotTo(HaveOccurred())
				_, _, err = action.Execute()
				Expect(err).NotTo(HaveOccurred())
				stateMocker.Stop()

				ctx := context.Background()
				updatedConfig, err := mockConfig.GetConfig(ctx, 0)
				Expect(err).NotTo(HaveOccurred())

				var updatedPC *config.ProtocolConverterConfig
				for i, pc := range updatedConfig.ProtocolConverter {
					if pc.Name == pcName {
						updatedPC = &updatedConfig.ProtocolConverter[i]
						break
					}
				}
				Expect(updatedPC).NotTo(BeNil())

				// Write DFC output must have changed — parsed YAML wraps in outer "output" key
				writeOutput := updatedPC.ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig.BenthosConfig.Output
				Expect(writeOutput).To(HaveKey("output"))
				Expect(writeOutput["output"]).To(HaveKey("kafka"))

				// Read DFC input must be untouched — initial config set directly without wrapper
				readInput := updatedPC.ProtocolConverterServiceConfig.Config.DataflowComponentReadServiceConfig.BenthosConfig.Input
				Expect(readInput).To(HaveKey("http_client"))
				Expect(readInput["http_client"]).To(HaveKeyWithValue("url", "http://original.example.com"))
			})

			It("should not change write DFC when only read DFC is edited", Label("write-dfc", "isolation"), func() {
				payload := map[string]interface{}{
					"name": pcName,
					"uuid": pcUUID.String(),
					"readDFC": map[string]interface{}{
						"inputs": map[string]interface{}{
							"data": "input:\n  http_client:\n    url: http://updated.example.com",
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

				err = stateMocker.Start()
				Expect(err).NotTo(HaveOccurred())
				_, _, err = action.Execute()
				Expect(err).NotTo(HaveOccurred())
				stateMocker.Stop()

				ctx := context.Background()
				updatedConfig, err := mockConfig.GetConfig(ctx, 0)
				Expect(err).NotTo(HaveOccurred())

				var updatedPC *config.ProtocolConverterConfig
				for i, pc := range updatedConfig.ProtocolConverter {
					if pc.Name == pcName {
						updatedPC = &updatedConfig.ProtocolConverter[i]
						break
					}
				}
				Expect(updatedPC).NotTo(BeNil())

				// Read DFC input must have changed — parsed YAML wraps in outer "input" key
				readInput := updatedPC.ProtocolConverterServiceConfig.Config.DataflowComponentReadServiceConfig.BenthosConfig.Input
				Expect(readInput).To(HaveKey("input"))
				Expect(readInput["input"]).To(HaveKey("http_client"))
				Expect(readInput["input"].(map[string]interface{})["http_client"]).To(HaveKeyWithValue("url", "http://updated.example.com"))

				// Write DFC output must be untouched — initial config set directly without wrapper
				writeOutput := updatedPC.ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig.BenthosConfig.Output
				Expect(writeOutput).To(HaveKey("stdout"))

				// UMH_TOPICS must be untouched
				Expect(updatedPC.ProtocolConverterServiceConfig.Variables.User["UMH_TOPICS"]).To(ConsistOf("umh.v1.factory.*"))
			})
		})

		Context("when starting and stopping individual DFCs", func() {
			// Minimal valid read DFC payload reused across several tests below.
			// The pipeline has one bloblang processor so read-DFC validation passes.
			minimalReadDFCPayload := func(state string) map[string]interface{} {
				return map[string]interface{}{
					"inputs": map[string]interface{}{
						"data": "input:\n  http_client:\n    url: 'http://example.com'",
						"type": "http_client",
					},
					"pipeline": map[string]interface{}{
						"processors": map[string]interface{}{
							"0": map[string]interface{}{
								"type": "bloblang",
								"data": "bloblang: root = content()",
							},
						},
					},
					"state": state,
				}
			}

			// Minimal write DFC payload: no inputs, pipeline, or umh_topics needed for
			// a state-only change. UMH_TOPICS is only required when output config is present.
			minimalWriteDFCPayload := func(state string) map[string]interface{} {
				return map[string]interface{}{
					"state": state,
				}
			}

			// getUpdatedPC is a helper that runs Parse+Validate+Execute and returns the
			// saved protocol converter config, asserting no error along the way.
			runEdit := func(payloadMap map[string]interface{}) *config.ProtocolConverterConfig {
				err := action.Parse(payloadMap)
				ExpectWithOffset(1, err).NotTo(HaveOccurred())
				err = action.Validate()
				ExpectWithOffset(1, err).NotTo(HaveOccurred())

				err = stateMocker.Start()
				ExpectWithOffset(1, err).NotTo(HaveOccurred())
				_, _, err = action.Execute()
				ExpectWithOffset(1, err).NotTo(HaveOccurred())
				stateMocker.Stop()

				ctx := context.Background()
				updatedConfig, err := mockConfig.GetConfig(ctx, 0)
				ExpectWithOffset(1, err).NotTo(HaveOccurred())

				for i, pc := range updatedConfig.ProtocolConverter {
					if pc.Name == pcName {
						return &updatedConfig.ProtocolConverter[i]
					}
				}
				Fail("protocol converter not found after execute")
				return nil
			}

			BeforeEach(func() {
				// Both DFCs are present and explicitly set to "active" so that
				// dfcPayloadDiffers can use the state-diff short-circuit path.
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.Config.DataflowComponentReadServiceConfig = dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
					BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
						Input: map[string]interface{}{"http_client": map[string]interface{}{"url": "http://example.com"}},
					},
				}
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig = dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
					BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
						Output: map[string]interface{}{"stdout": map[string]interface{}{}},
					},
				}
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.Variables.User["UMH_TOPICS"] = []string{"umh.v1.factory.*"}
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.ReadDFCDesiredState = "active"
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.WriteDFCDesiredState = "active"

				stateMocker = actions.NewStateMocker(mockConfig)
				stateMocker.Tick()
				action = actions.NewEditProtocolConverterAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, nil)
			})

			It("should stop read DFC only", Label("disable-bridges"), func() {
				pc := runEdit(map[string]interface{}{
					"name": pcName,
					"uuid": pcUUID.String(),
					"readDFC": minimalReadDFCPayload("stopped"),
				})

				Expect(pc.ProtocolConverterServiceConfig.ReadDFCDesiredState).To(Equal("stopped"))
				Expect(pc.ProtocolConverterServiceConfig.WriteDFCDesiredState).To(Equal("active"))
			})

			It("should start read DFC from stopped", Label("disable-bridges"), func() {
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.ReadDFCDesiredState = "stopped"

				pc := runEdit(map[string]interface{}{
					"name": pcName,
					"uuid": pcUUID.String(),
					"readDFC": minimalReadDFCPayload("active"),
				})

				Expect(pc.ProtocolConverterServiceConfig.ReadDFCDesiredState).To(Equal("active"))
				Expect(pc.ProtocolConverterServiceConfig.WriteDFCDesiredState).To(Equal("active"))
			})

			It("should stop write DFC only", Label("disable-bridges"), func() {
				pc := runEdit(map[string]interface{}{
					"name":     pcName,
					"uuid":     pcUUID.String(),
					"writeDFC": minimalWriteDFCPayload("stopped"),
				})

				Expect(pc.ProtocolConverterServiceConfig.WriteDFCDesiredState).To(Equal("stopped"))
				Expect(pc.ProtocolConverterServiceConfig.ReadDFCDesiredState).To(Equal("active"))
			})

			It("should start write DFC from stopped", Label("disable-bridges"), func() {
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.WriteDFCDesiredState = "stopped"

				pc := runEdit(map[string]interface{}{
					"name":     pcName,
					"uuid":     pcUUID.String(),
					"writeDFC": minimalWriteDFCPayload("active"),
				})

				Expect(pc.ProtocolConverterServiceConfig.WriteDFCDesiredState).To(Equal("active"))
				Expect(pc.ProtocolConverterServiceConfig.ReadDFCDesiredState).To(Equal("active"))
			})

			It("should stop both DFCs simultaneously", Label("disable-bridges"), func() {
				pc := runEdit(map[string]interface{}{
					"name":     pcName,
					"uuid":     pcUUID.String(),
					"readDFC":  minimalReadDFCPayload("stopped"),
					"writeDFC": minimalWriteDFCPayload("stopped"),
				})

				Expect(pc.ProtocolConverterServiceConfig.ReadDFCDesiredState).To(Equal("stopped"))
				Expect(pc.ProtocolConverterServiceConfig.WriteDFCDesiredState).To(Equal("stopped"))
			})

			It("should start both DFCs simultaneously", Label("disable-bridges"), func() {
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.ReadDFCDesiredState = "stopped"
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.WriteDFCDesiredState = "stopped"

				pc := runEdit(map[string]interface{}{
					"name":     pcName,
					"uuid":     pcUUID.String(),
					"readDFC":  minimalReadDFCPayload("active"),
					"writeDFC": minimalWriteDFCPayload("active"),
				})

				Expect(pc.ProtocolConverterServiceConfig.ReadDFCDesiredState).To(Equal("active"))
				Expect(pc.ProtocolConverterServiceConfig.WriteDFCDesiredState).To(Equal("active"))
			})
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
