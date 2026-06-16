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
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/communicator/pkg/encoding"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/connectionserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/dataflowcomponentserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/protocolconverterserviceconfig"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/config/variables"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/constants"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/fsm/protocolconverter"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/models"
	protocolconvertersvc "github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/service/protocolconverter"
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
		mu.Lock()
		messages = nil
		mu.Unlock()
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
			Expect(parsedPayload).NotTo(BeNil())
			Expect(parsedPayload.BenthosConfig.Input).To(HaveKey("input"))
			Expect(parsedPayload.BenthosConfig.Pipeline).To(HaveKey("processors"))
			Expect(action.GetIgnoreHealthCheck()).To(BeFalse())
		})

		It("should parse valid write DFC payload", func() {
			payload := map[string]interface{}{
				"name": pcName,
				"uuid": pcUUID.String(),
				"writeDFC": map[string]interface{}{
					"destination": map[string]interface{}{
						"protocol": "kafka",
						"code":     "addresses:\n- localhost:9092\n",
					},
					"source": map[string]interface{}{
						"topics": "umh.v1.factory.*",
					},
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			// Verify parsed values
			Expect(action.GetProtocolConverterUUID()).To(Equal(pcUUID))
			Expect(action.GetDFCType()).To(Equal("write"))

			writeCfg := action.GetDesiredWriteDFCConfig()
			Expect(writeCfg).NotTo(BeNil())
			Expect(writeCfg.Destination.Protocol).To(Equal("kafka"))
			Expect(writeCfg.Source.Topics).To(Equal("umh.v1.factory.*"))
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
						"processors": map[string]interface{}{
							"0": map[string]interface{}{
								"type": "bloblang",
								"data": "bloblang: root = this",
							},
						},
					},
				},
				"writeDFC": map[string]interface{}{
					"destination": map[string]interface{}{
						"protocol": "stdout",
					},
					"source": map[string]interface{}{
						"topics": "umh.v1.factory.*",
					},
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
					"destination": map[string]interface{}{
						"protocol": "stdout",
					},
					"source": map[string]interface{}{
						"topics": "umh.v1.factory.*\numh.v1.plant.*",
					},
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(action.GetDFCType()).To(Equal("write"))

			writeCfg := action.GetDesiredWriteDFCConfig()
			Expect(writeCfg).NotTo(BeNil())
			Expect(writeCfg.Source.Topics).To(Equal("umh.v1.factory.*\numh.v1.plant.*"))
		})

		It("should parse write DFC with golang template vars in input_topics", func() {
			// Regression test: template vars like {{ .IP }}, {{ .location_path }} must not
			// cause a parse error — rendering happens later in BuildRuntimeConfig with full scope.
			payload := map[string]interface{}{
				"name": pcName,
				"uuid": pcUUID.String(),
				"connection": map[string]interface{}{
					"ip":   "192.168.1.1",
					"port": 502,
				},
				"writeDFC": map[string]interface{}{
					"destination": map[string]interface{}{
						"protocol": "stdout",
					},
					"source": map[string]interface{}{
						"topics": "umh.v1.{{ .location_path }}.{{ .IP }}.*",
					},
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			writeCfg := action.GetDesiredWriteDFCConfig()
			Expect(writeCfg).NotTo(BeNil())
			Expect(writeCfg.Source.Topics).To(Equal("umh.v1.{{ .location_path }}.{{ .IP }}.*"))
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
						"type": "http_client",
					},
					"pipeline": map[string]interface{}{
						"processors": map[string]interface{}{
							"0": map[string]interface{}{
								"type": "bloblang",
								"data": "bloblang: root = this",
							},
						},
					},
					"state": "stopped",
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
					"destination": map[string]interface{}{
						"protocol": "stdout",
					},
					"source": map[string]interface{}{
						"topics": "umh.v1.factory.*",
					},
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
					"destination": map[string]interface{}{
						"protocol": "stdout",
					},
					// no source/topics
				},
			}

			err := action.Parse(payload)
			Expect(err).NotTo(HaveOccurred())

			err = action.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("input topic"))
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

			// Validation happens during Parse (build step validates the DFC)
			err := action.Parse(payload)
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
			Expect(writeDFCConfig.HasOutput()).To(BeFalse())

			// verify that the templateRef is set
			Expect(updatedPC.ProtocolConverterServiceConfig.TemplateRef).To(Equal(pcName))
		})

		It("should store Source.Topics in typed write config when adding write DFC", func() {
			payload := map[string]interface{}{
				"name": pcName,
				"uuid": pcUUID.String(),
				"writeDFC": map[string]interface{}{
					"destination": map[string]interface{}{
						"protocol": "stdout",
					},
					"source": map[string]interface{}{
						"topics": "umh.v1.factory.line-1.*\numh.v1.factory.line-2.*",
					},
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

			// Verify InputTopics stored in typed write config
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
			Expect(updatedPC.ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig.Source.Topics).To(Equal("umh.v1.factory.line-1.*\numh.v1.factory.line-2.*"))
		})

		Context("when protocol converter already has both DFCs configured", func() {
			// overwrite the BeforeEach config with one that has both DFCs populated
			BeforeEach(func() {
				existingReadInput := map[string]interface{}{
					"http_client": map[string]interface{}{
						"url": "http://original.example.com",
					},
				}

				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.Config.DataflowComponentReadServiceConfig = dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
					BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
						Input: existingReadInput,
					},
				}
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig = dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
					Destination: dataflowcomponentserviceconfig.WriteConfigDestination{Protocol: "stdout"},
					Source:      dataflowcomponentserviceconfig.WriteConfigSource{Topics: "umh.v1.factory.*"},
				}

				stateMocker = actions.NewStateMocker(mockConfig)
				stateMocker.Tick()
				action = actions.NewEditProtocolConverterAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, nil)
			})

			It("should not change read DFC when only write DFC is edited", Label("write-dfc", "isolation"), func() {
				payload := map[string]interface{}{
					"name": pcName,
					"uuid": pcUUID.String(),
					"writeDFC": map[string]interface{}{
						"destination": map[string]interface{}{
							"protocol": "kafka",
							"code":     "addresses:\n- localhost:9092\n",
						},
						"source": map[string]interface{}{
							"topics": "umh.v1.plant.*",
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

				// Write DFC output must have changed — new format stores directly without wrapper
				writeDestType := updatedPC.ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig.Destination.Protocol
				Expect(writeDestType).To(Equal("kafka"))

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
				writeDestType := updatedPC.ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig.Destination.Protocol
				Expect(writeDestType).To(Equal("stdout"))

				// Source.Topics must be untouched
				Expect(updatedPC.ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig.Source.Topics).To(Equal("umh.v1.factory.*"))
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

			// Minimal write DFC payload: no inputs, pipeline, or input_topics needed for
			// a state-only change. InputTopics is only required when output config is present.
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
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig = dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
					Destination: dataflowcomponentserviceconfig.WriteConfigDestination{Protocol: "stdout"},
					Source:      dataflowcomponentserviceconfig.WriteConfigSource{Topics: "umh.v1.factory.*"},
				}
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.ReadDFCDesiredState = "active"
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.WriteDFCDesiredState = "active"

				stateMocker = actions.NewStateMocker(mockConfig)
				stateMocker.Tick()
				action = actions.NewEditProtocolConverterAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, nil)
			})

			It("should stop read DFC only", Label("disable-bridges"), func() {
				pc := runEdit(map[string]interface{}{
					"name":    pcName,
					"uuid":    pcUUID.String(),
					"readDFC": minimalReadDFCPayload("stopped"),
				})

				Expect(pc.ProtocolConverterServiceConfig.ReadDFCDesiredState).To(Equal("stopped"))
				Expect(pc.ProtocolConverterServiceConfig.WriteDFCDesiredState).To(Equal("active"))
			})

			It("should start read DFC from stopped", Label("disable-bridges"), func() {
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.ReadDFCDesiredState = "stopped"

				pc := runEdit(map[string]interface{}{
					"name":    pcName,
					"uuid":    pcUUID.String(),
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

		Context("deriveDFCType smart diff", func() {
			// deployReadDFC executes a first edit that stores the given read DFC payload
			// into the mock config so the next Parse can detect "no change".
			deployReadDFC := func(readDFC map[string]interface{}) {
				firstPayload := map[string]interface{}{
					"name": pcName,
					"uuid": pcUUID.String(),
					"connection": map[string]interface{}{
						"ip":   "wttr.in",
						"port": 80,
					},
					"readDFC": readDFC,
				}
				firstAction := actions.NewEditProtocolConverterAction(userEmail, uuid.New(), instanceUUID, outboundChannel, mockConfig, nil)
				err := firstAction.Parse(firstPayload)
				ExpectWithOffset(1, err).NotTo(HaveOccurred())
				err = firstAction.Validate()
				ExpectWithOffset(1, err).NotTo(HaveOccurred())
				_, _, err = firstAction.Execute()
				ExpectWithOffset(1, err).NotTo(HaveOccurred())
			}

			// baseReadDFC is a minimal valid read DFC configuration.
			baseReadDFC := map[string]interface{}{
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
				"state": "active",
			}

			It("should return DFCTypeEmpty when read DFC config is identical to deployed", func() {
				deployReadDFC(baseReadDFC)

				action2 := actions.NewEditProtocolConverterAction(userEmail, uuid.New(), instanceUUID, outboundChannel, mockConfig, nil)
				payload := map[string]interface{}{
					"name": pcName,
					"uuid": pcUUID.String(),
					"connection": map[string]interface{}{
						"ip":   "wttr.in",
						"port": 80,
					},
					"readDFC": baseReadDFC,
				}
				err := action2.Parse(payload)
				Expect(err).NotTo(HaveOccurred())
				Expect(action2.GetDFCType()).To(Equal("empty"))
			})

			It("should return DFCTypeRead when per-DFC state changes from active to stopped", func() {
				deployReadDFC(baseReadDFC)

				stoppedDFC := make(map[string]interface{})
				for k, v := range baseReadDFC {
					stoppedDFC[k] = v
				}
				stoppedDFC["state"] = "stopped"

				action2 := actions.NewEditProtocolConverterAction(userEmail, uuid.New(), instanceUUID, outboundChannel, mockConfig, nil)
				payload := map[string]interface{}{
					"name": pcName,
					"uuid": pcUUID.String(),
					"connection": map[string]interface{}{
						"ip":   "wttr.in",
						"port": 80,
					},
					"readDFC": stoppedDFC,
				}
				err := action2.Parse(payload)
				Expect(err).NotTo(HaveOccurred())
				Expect(action2.GetDFCType()).To(Equal("read"))
			})

			It("should return DFCTypeWrite when InputTopics differ from deployed", func() {
				// First, deploy a write DFC with a known topic set.
				firstWritePayload := map[string]interface{}{
					"name": pcName,
					"uuid": pcUUID.String(),
					"connection": map[string]interface{}{
						"ip":   "wttr.in",
						"port": 80,
					},
					"writeDFC": map[string]interface{}{
						"destination": map[string]interface{}{
							"protocol": "stdout",
						},
						"source": map[string]interface{}{
							"topics": "umh.v1.factory.*",
						},
					},
				}
				firstAction := actions.NewEditProtocolConverterAction(userEmail, uuid.New(), instanceUUID, outboundChannel, mockConfig, nil)
				err := firstAction.Parse(firstWritePayload)
				Expect(err).NotTo(HaveOccurred())
				err = firstAction.Validate()
				Expect(err).NotTo(HaveOccurred())
				_, _, err = firstAction.Execute()
				Expect(err).NotTo(HaveOccurred())

				// Now parse the same config but with different topics.
				action2 := actions.NewEditProtocolConverterAction(userEmail, uuid.New(), instanceUUID, outboundChannel, mockConfig, nil)
				payload := map[string]interface{}{
					"name": pcName,
					"uuid": pcUUID.String(),
					"connection": map[string]interface{}{
						"ip":   "wttr.in",
						"port": 80,
					},
					"writeDFC": map[string]interface{}{
						"destination": map[string]interface{}{
							"protocol": "stdout",
						},
						"source": map[string]interface{}{
							"topics": "umh.v1.plant.*", // different topic
						},
					},
				}
				err = action2.Parse(payload)
				Expect(err).NotTo(HaveOccurred())
				Expect(action2.GetDFCType()).To(Equal("write"))
			})

			It("should return DFCTypeBoth when a custom template variable changes", Label("write-dfc"), func() {
				// First edit: deploy read + write with a custom template variable.
				firstPayload := map[string]interface{}{
					"name": pcName,
					"uuid": pcUUID.String(),
					"connection": map[string]interface{}{
						"ip":   "10.0.0.1",
						"port": 502,
					},
					"templateInfo": map[string]interface{}{
						"variables": []interface{}{
							map[string]interface{}{"label": "baudRate", "value": "9600"},
						},
					},
					"readDFC": map[string]interface{}{
						"inputs": map[string]interface{}{
							"data": "input:\n  http_client:\n    url: 'http://{{ .baudRate }}'",
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
						"state": "active",
					},
					"writeDFC": map[string]interface{}{
						"destination": map[string]interface{}{"protocol": "stdout"},
						"source":      map[string]interface{}{"topics": "umh.v1.factory.*"},
					},
				}
				firstAction := actions.NewEditProtocolConverterAction(userEmail, uuid.New(), instanceUUID, outboundChannel, mockConfig, nil)
				err := firstAction.Parse(firstPayload)
				Expect(err).NotTo(HaveOccurred())
				err = firstAction.Validate()
				Expect(err).NotTo(HaveOccurred())
				_, _, err = firstAction.Execute()
				Expect(err).NotTo(HaveOccurred())

				// Second edit: change only baudRate — DFC config text is identical.
				action2 := actions.NewEditProtocolConverterAction(userEmail, uuid.New(), instanceUUID, outboundChannel, mockConfig, nil)
				payload := map[string]interface{}{
					"name": pcName,
					"uuid": pcUUID.String(),
					"connection": map[string]interface{}{
						"ip":   "10.0.0.1",
						"port": 502,
					},
					"templateInfo": map[string]interface{}{
						"variables": []interface{}{
							map[string]interface{}{"label": "baudRate", "value": "19200"}, // changed
						},
					},
					"readDFC": map[string]interface{}{
						"inputs": map[string]interface{}{
							"data": "input:\n  http_client:\n    url: 'http://{{ .baudRate }}'",
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
						"state": "active",
					},
					"writeDFC": map[string]interface{}{
						"destination": map[string]interface{}{"protocol": "stdout"},
						"source":      map[string]interface{}{"topics": "umh.v1.factory.*"},
					},
				}
				err = action2.Parse(payload)
				Expect(err).NotTo(HaveOccurred())
				// baudRate changed → both DFCs need redeploy even though DFC config text is identical.
				Expect(action2.GetDFCType()).To(Equal("both"))
			})
		})

		Context("DFCTypeEmpty edit preserves per-DFC stopped states", func() {
			BeforeEach(func() {
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.ReadDFCDesiredState = "stopped"
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.WriteDFCDesiredState = "stopped"

				stateMocker = actions.NewStateMocker(mockConfig)
				stateMocker.Tick()
				action = actions.NewEditProtocolConverterAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, nil)
			})

			It("should preserve ReadDFCDesiredState and WriteDFCDesiredState when no DFC payload is sent", Label("disable-bridges"), func() {
				// Connection-only edit: no readDFC or writeDFC in payload.
				payload := map[string]interface{}{
					"name": pcName,
					"uuid": pcUUID.String(),
					"connection": map[string]interface{}{
						"ip":   "wttr.in",
						"port": 80,
					},
				}

				err := action.Parse(payload)
				Expect(err).NotTo(HaveOccurred())
				Expect(action.GetDFCType()).To(Equal("empty"))

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
				Expect(updatedPC.ProtocolConverterServiceConfig.ReadDFCDesiredState).To(Equal("stopped"))
				Expect(updatedPC.ProtocolConverterServiceConfig.WriteDFCDesiredState).To(Equal("stopped"))
			})
		})

		Context("DFCTypeBoth execute path", func() {
			BeforeEach(func() {
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.Config.DataflowComponentReadServiceConfig = dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
					BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
						Input: map[string]interface{}{"http_client": map[string]interface{}{"url": "http://old.example.com"}},
					},
				}
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig = dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
					Destination: dataflowcomponentserviceconfig.WriteConfigDestination{Protocol: "stdout"},
					Source:      dataflowcomponentserviceconfig.WriteConfigSource{Topics: "umh.v1.factory.*"},
				}
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.ReadDFCDesiredState = "active"
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.WriteDFCDesiredState = "active"

				stateMocker = actions.NewStateMocker(mockConfig)
				stateMocker.Tick()
				action = actions.NewEditProtocolConverterAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, nil)
			})

			It("should write both DFC configs when editing both simultaneously", Label("write-dfc"), func() {
				payload := map[string]interface{}{
					"name": pcName,
					"uuid": pcUUID.String(),
					"readDFC": map[string]interface{}{
						"inputs": map[string]interface{}{
							"data": "input:\n  http_client:\n    url: http://new.example.com",
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
					},
					"writeDFC": map[string]interface{}{
						"destination": map[string]interface{}{
							"protocol": "kafka",
							"code":     "addresses:\n- localhost:9092\n",
						},
						"source": map[string]interface{}{
							"topics": "umh.v1.plant.*",
						},
					},
				}

				err := action.Parse(payload)
				Expect(err).NotTo(HaveOccurred())
				Expect(action.GetDFCType()).To(Equal("both"))

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

				// Read DFC: parsed YAML wraps in outer "input" key
				readInput := updatedPC.ProtocolConverterServiceConfig.Config.DataflowComponentReadServiceConfig.BenthosConfig.Input
				Expect(readInput).To(HaveKey("input"))
				Expect(readInput["input"]).To(HaveKey("http_client"))
				Expect(readInput["input"].(map[string]interface{})["http_client"]).To(HaveKeyWithValue("url", "http://new.example.com"))

				// Write DFC: new format stores directly without wrapper
				writeDestType := updatedPC.ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig.Destination.Protocol
				Expect(writeDestType).To(Equal("kafka"))

				// Source.Topics updated
				Expect(updatedPC.ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig.Source.Topics).To(Equal("umh.v1.plant.*"))
			})
		})

		Context("compareSingleDFCConfig via awaitRollout", func() {
			// These tests exercise compareSingleDFCConfig by passing a real
			// systemSnapshotManager populated with a PC snapshot, so awaitRollout
			// actually runs and calls compareSingleDFCConfig on its first tick.

			It("should complete when read DFC has reached the stopped state", Label("disable-bridges"), func() {
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.ReadDFCDesiredState = "active"
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.Config.DataflowComponentReadServiceConfig = dataflowcomponentserviceconfig.DataflowComponentServiceConfig{
					BenthosConfig: dataflowcomponentserviceconfig.BenthosConfig{
						Input: map[string]interface{}{"http_client": map[string]interface{}{"url": "http://example.com"}},
					},
				}

				snapshotMgr := fsm.NewSnapshotManager()

				// Build a PC snapshot: overall state "active", read DFC FSM state "stopped".
				observedState := &protocolconverter.ProtocolConverterObservedStateSnapshot{
					ServiceInfo: protocolconvertersvc.ServiceInfo{
						DataflowComponentReadFSMState: protocolconverter.OperationalStateStopped,
					},
				}
				instance := &fsm.FSMInstanceSnapshot{
					ID:                pcName,
					CurrentState:      protocolconverter.OperationalStateActive,
					DesiredState:      protocolconverter.OperationalStateActive,
					LastObservedState: observedState,
				}
				snapshotMgr.UpdateSnapshot(&fsm.SystemSnapshot{
					Managers: map[string]fsm.ManagerSnapshot{
						constants.ProtocolConverterManagerName: &actions.MockManagerSnapshot{
							Instances: map[string]*fsm.FSMInstanceSnapshot{pcName: instance},
						},
					},
				})

				localAction := actions.NewEditProtocolConverterAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, snapshotMgr)

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
									"data": "bloblang: root = content()",
								},
							},
						},
						"state": "stopped",
					},
				}

				err := localAction.Parse(payload)
				Expect(err).NotTo(HaveOccurred())
				err = localAction.Validate()
				Expect(err).NotTo(HaveOccurred())

				type execResult struct {
					err error
				}
				ch := make(chan execResult, 1)
				go func() {
					_, _, err := localAction.Execute()
					ch <- execResult{err}
				}()

				Eventually(ch, "5s").Should(Receive(And(
					WithTransform(func(r execResult) error { return r.err }, BeNil()),
				)))
			})

			It("should complete when write DFC has reached the stopped state", Label("disable-bridges"), func() {
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.WriteDFCDesiredState = "active"
				mockConfig.Config.ProtocolConverter[0].ProtocolConverterServiceConfig.Config.DataflowComponentWriteServiceConfig = dataflowcomponentserviceconfig.DataflowComponentWriteConfigInput{
					Destination: dataflowcomponentserviceconfig.WriteConfigDestination{Protocol: "stdout"},
					Source:      dataflowcomponentserviceconfig.WriteConfigSource{Topics: "umh.v1.factory.*"},
				}

				snapshotMgr := fsm.NewSnapshotManager()

				observedState := &protocolconverter.ProtocolConverterObservedStateSnapshot{
					ServiceInfo: protocolconvertersvc.ServiceInfo{
						DataflowComponentWriteFSMState: protocolconverter.OperationalStateStopped,
					},
				}
				instance := &fsm.FSMInstanceSnapshot{
					ID:                pcName,
					CurrentState:      protocolconverter.OperationalStateActive,
					DesiredState:      protocolconverter.OperationalStateActive,
					LastObservedState: observedState,
				}
				snapshotMgr.UpdateSnapshot(&fsm.SystemSnapshot{
					Managers: map[string]fsm.ManagerSnapshot{
						constants.ProtocolConverterManagerName: &actions.MockManagerSnapshot{
							Instances: map[string]*fsm.FSMInstanceSnapshot{pcName: instance},
						},
					},
				})

				localAction := actions.NewEditProtocolConverterAction(userEmail, actionUUID, instanceUUID, outboundChannel, mockConfig, snapshotMgr)

				payload := map[string]interface{}{
					"name":     pcName,
					"uuid":     pcUUID.String(),
					"writeDFC": map[string]interface{}{"state": "stopped"},
				}

				err := localAction.Parse(payload)
				Expect(err).NotTo(HaveOccurred())
				err = localAction.Validate()
				Expect(err).NotTo(HaveOccurred())

				type execResult struct {
					err error
				}
				ch := make(chan execResult, 1)
				go func() {
					_, _, err := localAction.Execute()
					ch <- execResult{err}
				}()

				Eventually(ch, "5s").Should(Receive(And(
					WithTransform(func(r execResult) error { return r.err }, BeNil()),
				)))
			})
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

	// I1 — production switch path is safe end-to-end.
	//
	// Before the ENG-4959 wire-up fix, an EditProtocolConverter action built by
	// the production switch had a nil fsmLogger. Any code path in Execute()
	// that hits a.fsmLogger.SentryError (edit-protocolconverter.go:253, 267,
	// 291, 305, 319, 591, 860, 866) panicked at runtime. This test drives a
	// real Execute() failure through HandleActionMessage (the switch path,
	// NOT NewEditProtocolConverterAction, which already wired fsmLogger via
	// its constructor) by configuring the mock config manager to fail at
	// AtomicEditProtocolConverter. The persist failure hits a.fsmLogger.
	// SentryError at edit-protocolconverter.go:305. The pre-fix code crashed
	// the whole umh-core process here; this test asserts the action now
	// completes with ActionFinishedWithFailure instead.
	Describe("HandleActionMessage end-to-end (switch path)", func() {
		It("does not panic on AtomicEditProtocolConverter failure inside Execute", func() {
			// Force AtomicEditProtocolConverter to fail so Execute reaches
			// edit-protocolconverter.go:305, which calls a.fsmLogger.
			// SentryError. Without the wire-up fix, a.fsmLogger is nil here
			// and the SentryError call panics with a nil-pointer dereference,
			// killing the process.
			mockConfig.WithAtomicEditProtocolConverterError(errors.New("mock persist failure"))

			payload := map[string]interface{}{
				"name": pcName,
				"uuid": pcUUID.String(),
				"readDFC": map[string]interface{}{
					"state": "active",
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

			actionMsg := models.ActionMessagePayload{
				ActionType:    models.EditProtocolConverter,
				ActionUUID:    uuid.New(),
				ActionPayload: payload,
			}

			localOut := make(chan *models.UMHMessage, 16)
			done := make(chan struct{})

			go func() {
				defer GinkgoRecover()
				defer close(done)
				actions.HandleActionMessage(
					instanceUUID,
					actionMsg,
					userEmail,
					localOut,
					"",
					nil,
					uuid.Nil,
					nil,
					mockConfig,
				)
			}()

			Eventually(done, "3s").Should(BeClosed(), "HandleActionMessage should complete without panicking (ENG-4959)")

			// Drain the reply channel and assert at least one reply carries
			// the failure state. This pins the test to the failure branch:
			// if a future refactor accidentally turns Execute into a success
			// path here, the assertion fires.
			var sawFailure bool
			for len(localOut) > 0 {
				msg := <-localOut
				dec, err := encoding.DecodeMessageFromUMHInstanceToUser(msg.Content)
				if err != nil {
					continue
				}
				reply, ok := dec.Payload.(map[string]interface{})
				if !ok {
					continue
				}
				if state, _ := reply["actionReplyState"].(string); state == string(models.ActionFinishedWithFailure) {
					sawFailure = true
				}
			}
			Expect(sawFailure).To(BeTrue(), "expected at least one ActionFinishedWithFailure reply")

			// Pin the integration test to the actual crash path. Without
			// this assertion, a future Parse-leniency change could let the
			// test pass on a happy-path or early-validation failure without
			// ever reaching the a.fsmLogger.SentryError call inside
			// Execute() — the very call site that PR #2546's missing field
			// would have crashed on. AtomicEditProtocolConverter is the
			// gate we must cross to exercise the regression.
			Expect(mockConfig.AtomicEditProtocolConverterCalled).To(BeTrue(),
				"I1 must reach AtomicEditProtocolConverter to exercise the real fsmLogger crash path")
		})
	})
})
